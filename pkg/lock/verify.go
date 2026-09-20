package lock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// Verify checks a built directory without reading source files or using a network.
func Verify(outputDirectory string) (Summary, error) {
	return VerifyWithExpectedLock(outputDirectory, "")
}

// VerifyWithExpectedLock checks a built directory and, when non-empty, anchors
// its contents to an out-of-band trusted lockfile SHA-256.
func VerifyWithExpectedLock(outputDirectory, expectedLockSHA256 string) (Summary, error) {
	if expectedLockSHA256 != "" && !validDigest(expectedLockSHA256) {
		return Summary{}, fmt.Errorf("expected lock SHA-256 is not a lowercase SHA-256 digest")
	}

	directoryInfo, err := os.Lstat(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("inspect output directory: %w", err)
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return Summary{}, fmt.Errorf("output must be a directory, not a symlink")
	}

	entries, err := os.ReadDir(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("read output directory: %w", err)
	}
	if len(entries) != len(outputFileNames) {
		return Summary{}, fmt.Errorf("output file set mismatch: got %d entries, want %d", len(entries), len(outputFileNames))
	}

	files := make(map[string][]byte, len(entries))
	for index, entry := range entries {
		expectedName := outputFileNames[index]
		if entry.Name() != expectedName {
			return Summary{}, fmt.Errorf("unexpected output entry %q; expected %q", entry.Name(), expectedName)
		}
		path := filepath.Join(outputDirectory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return Summary{}, fmt.Errorf("inspect output %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Summary{}, fmt.Errorf("output %q must be a regular file, not a symlink", entry.Name())
		}
		data, err := readRegularFile(path, info)
		if err != nil {
			return Summary{}, fmt.Errorf("read output %q: %w", entry.Name(), err)
		}
		files[entry.Name()] = data
	}
	actualLockSHA256 := digestBytes(files[LockFileName])
	if expectedLockSHA256 != "" && actualLockSHA256 != expectedLockSHA256 {
		return Summary{}, fmt.Errorf("trusted lock digest mismatch: got %s, want %s", actualLockSHA256, expectedLockSHA256)
	}

	var lockFile LockFile
	if err := decodeCanonicalJSON(files[LockFileName], &lockFile); err != nil {
		return Summary{}, fmt.Errorf("%s: %w", LockFileName, err)
	}
	if err := validateLock(lockFile); err != nil {
		return Summary{}, fmt.Errorf("%s: %w", LockFileName, err)
	}

	var manifest Manifest
	if err := decodeCanonicalJSON(files[ManifestFileName], &manifest); err != nil {
		return Summary{}, fmt.Errorf("%s: %w", ManifestFileName, err)
	}
	if err := validateManifest(manifest, lockFile, files[LockFileName], files[BundleFileName]); err != nil {
		return Summary{}, fmt.Errorf("%s: %w", ManifestFileName, err)
	}
	if err := verifyBundle(files[BundleFileName], manifest.Chunks); err != nil {
		return Summary{}, fmt.Errorf("%s: %w", BundleFileName, err)
	}

	expectedChecksums := renderChecksums(map[string][]byte{
		BundleFileName:   files[BundleFileName],
		LockFileName:     files[LockFileName],
		ManifestFileName: files[ManifestFileName],
	})
	if !bytes.Equal(files[ChecksumsFileName], expectedChecksums) {
		return Summary{}, fmt.Errorf("%s mismatch", ChecksumsFileName)
	}

	return Summary{
		SourceCount:    manifest.Stats.SourceCount,
		DuplicateCount: manifest.Stats.DuplicateCount,
		SelectedCount:  manifest.Stats.SelectedCount,
		SelectedTokens: manifest.Stats.SelectedTokens,
		BundleSHA256:   manifest.Bundle.SHA256,
		LockSHA256:     actualLockSHA256,
		ManifestSHA256: digestBytes(files[ManifestFileName]),
	}, nil
}

func validateManifest(manifest Manifest, lockFile LockFile, lockBytes, bundle []byte) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("schema_version: got %q, want %q", manifest.SchemaVersion, ManifestSchemaVersion)
	}
	if !reflect.DeepEqual(manifest.Identities, currentIdentities()) ||
		!reflect.DeepEqual(manifest.Identities, lockFile.Identities) {
		return fmt.Errorf("identity mismatch")
	}
	if manifest.ConfigurationSHA256 != lockFile.ConfigurationSHA256 {
		return fmt.Errorf("configuration digest mismatch")
	}
	if manifest.LockSHA256 != digestBytes(lockBytes) {
		return fmt.Errorf("lock digest mismatch")
	}
	if !reflect.DeepEqual(manifest.Sources, lockFile.Sources) {
		return fmt.Errorf("source inventory mismatch")
	}
	if !reflect.DeepEqual(manifest.Chunks, lockFile.Chunks) {
		return fmt.Errorf("chunk inventory mismatch")
	}
	if !reflect.DeepEqual(manifest.Stats, lockFile.Stats) {
		return fmt.Errorf("stats mismatch")
	}
	if manifest.Bundle.Path != BundleFileName ||
		manifest.Bundle.Bytes != int64(len(bundle)) ||
		manifest.Bundle.SHA256 != digestBytes(bundle) {
		return fmt.Errorf("bundle identity mismatch")
	}
	return nil
}

func verifyBundle(bundle []byte, chunks []Chunk) error {
	selected := selectedChunks(chunks)
	position := 0
	header := []byte("# Distill Context Bundle\n\nSchema: distill-lock/v0\n\n")
	if !bytes.HasPrefix(bundle, header) {
		return fmt.Errorf("invalid bundle header")
	}
	position += len(header)

	contents := make(map[string][]byte, len(selected))
	for _, chunk := range selected {
		pathJSON, err := json.Marshal(chunk.Path)
		if err != nil {
			return err
		}
		prefix := []byte(fmt.Sprintf(
			"<!-- distill-lock/v0 chunk=%d path=%s start=%d end=%d sha256=%s -->\n",
			chunk.SelectionOrder,
			pathJSON,
			chunk.StartByte,
			chunk.EndByte,
			chunk.SHA256,
		))
		if position+len(prefix) > len(bundle) || !bytes.Equal(bundle[position:position+len(prefix)], prefix) {
			return fmt.Errorf("chunk %d header mismatch", chunk.SelectionOrder)
		}
		position += len(prefix)
		contentLength := int(chunk.EndByte - chunk.StartByte)
		if contentLength < 0 || position+contentLength > len(bundle) {
			return fmt.Errorf("chunk %d content length is invalid", chunk.SelectionOrder)
		}
		content := bundle[position : position+contentLength]
		if digestBytes(content) != chunk.SHA256 {
			return fmt.Errorf("chunk %d content digest mismatch", chunk.SelectionOrder)
		}
		normalized, err := normalizeText(content)
		if err != nil || !bytes.Equal(content, normalized) {
			return fmt.Errorf("chunk %d content is not canonical UTF-8/NFC/LF text", chunk.SelectionOrder)
		}
		contents[chunkKey(chunk)] = append([]byte(nil), content...)
		position += contentLength
		suffix := []byte("\n<!-- /distill-lock/v0 -->\n\n")
		if position+len(suffix) > len(bundle) || !bytes.Equal(bundle[position:position+len(suffix)], suffix) {
			return fmt.Errorf("chunk %d footer mismatch", chunk.SelectionOrder)
		}
		position += len(suffix)
	}
	if position != len(bundle) {
		return fmt.Errorf("unexpected trailing bundle bytes")
	}

	regenerated, err := renderBundle(chunks, contents)
	if err != nil {
		return err
	}
	if !bytes.Equal(bundle, regenerated) {
		return fmt.Errorf("bundle regeneration mismatch")
	}
	return nil
}
