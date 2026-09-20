package lock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Build validates all locked inputs and atomically publishes a fresh output directory.
func Build(lockPath, outputDirectory string) (Summary, error) {
	if err := validateSupportedRuntime(); err != nil {
		return Summary{}, err
	}
	lockBytes, lockFile, err := readLockFile(lockPath)
	if err != nil {
		return Summary{}, err
	}

	lockDirectory, err := resolvedDirectory(filepath.Dir(lockPath))
	if err != nil {
		return Summary{}, fmt.Errorf("resolve lockfile directory: %w", err)
	}
	sourceRoot := filepath.Join(lockDirectory, filepath.FromSlash(lockFile.Configuration.SourceRoot))
	sourceRoot, err = resolvedDirectory(sourceRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve locked source root: %w", err)
	}
	inside, err := isWithin(lockDirectory, sourceRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("validate locked source root: %w", err)
	}
	if !inside || lockDirectory == sourceRoot {
		return Summary{}, fmt.Errorf("locked source root must be a strict descendant of lockfile directory")
	}

	inputs, err := scanSourceRoot(sourceRoot)
	if err != nil {
		return Summary{}, err
	}
	currentLock, chunkContents, err := buildLock(lockFile.Configuration, inputs)
	if err != nil {
		return Summary{}, err
	}
	equal, err := equalLock(lockFile, currentLock)
	if err != nil {
		return Summary{}, err
	}
	if !equal {
		return Summary{}, fmt.Errorf("locked inputs or identities drifted; run lock explicitly to review changes")
	}

	bundle, err := renderBundle(lockFile.Chunks, chunkContents)
	if err != nil {
		return Summary{}, err
	}
	manifest := Manifest{
		SchemaVersion:       ManifestSchemaVersion,
		Identities:          lockFile.Identities,
		ConfigurationSHA256: lockFile.ConfigurationSHA256,
		LockSHA256:          digestBytes(lockBytes),
		Sources:             lockFile.Sources,
		Chunks:              lockFile.Chunks,
		Stats:               lockFile.Stats,
		Bundle: Output{
			Path:   BundleFileName,
			SHA256: digestBytes(bundle),
			Bytes:  int64(len(bundle)),
		},
	}
	manifestBytes, err := canonicalJSON(manifest)
	if err != nil {
		return Summary{}, err
	}
	checksums := renderChecksums(map[string][]byte{
		BundleFileName:   bundle,
		LockFileName:     lockBytes,
		ManifestFileName: manifestBytes,
	})

	outputAbsolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := validateBuildDestination(outputAbsolute, sourceRoot); err != nil {
		return Summary{}, err
	}
	parent := filepath.Dir(outputAbsolute)
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(outputAbsolute)+".tmp-")
	if err != nil {
		return Summary{}, fmt.Errorf("create temporary output directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.Chmod(temporary, 0o755); err != nil {
		return Summary{}, fmt.Errorf("set temporary output permissions: %w", err)
	}

	files := map[string][]byte{
		BundleFileName:    bundle,
		LockFileName:      lockBytes,
		ManifestFileName:  manifestBytes,
		ChecksumsFileName: checksums,
	}
	for _, name := range outputFileNames {
		if err := writeSyncedFile(filepath.Join(temporary, name), files[name]); err != nil {
			return Summary{}, fmt.Errorf("write output %q: %w", name, err)
		}
	}
	if err := syncDirectory(temporary); err != nil {
		return Summary{}, fmt.Errorf("sync temporary output: %w", err)
	}
	if _, err := Verify(temporary); err != nil {
		return Summary{}, fmt.Errorf("verify temporary output: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return Summary{}, fmt.Errorf("sync output parent before publication: %w", err)
	}
	if err := renameNoReplace(temporary, outputAbsolute); err != nil {
		return Summary{}, fmt.Errorf("publish output directory: %w", err)
	}
	published = true

	return Summary{
		SourceCount:    lockFile.Stats.SourceCount,
		DuplicateCount: lockFile.Stats.DuplicateCount,
		SelectedCount:  lockFile.Stats.SelectedCount,
		SelectedTokens: lockFile.Stats.SelectedTokens,
		BundleSHA256:   manifest.Bundle.SHA256,
		LockSHA256:     manifest.LockSHA256,
		ManifestSHA256: digestBytes(manifestBytes),
	}, nil
}

func readLockFile(lockPath string) ([]byte, LockFile, error) {
	info, err := os.Lstat(lockPath)
	if err != nil {
		return nil, LockFile{}, fmt.Errorf("inspect lockfile: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, LockFile{}, fmt.Errorf("lockfile must be a regular file, not a symlink")
	}
	data, err := readRegularFile(lockPath, info)
	if err != nil {
		return nil, LockFile{}, fmt.Errorf("read lockfile: %w", err)
	}
	var lockFile LockFile
	if err := decodeCanonicalJSON(data, &lockFile); err != nil {
		return nil, LockFile{}, fmt.Errorf("lockfile: %w", err)
	}
	if err := validateLock(lockFile); err != nil {
		return nil, LockFile{}, fmt.Errorf("lockfile: %w", err)
	}
	return data, lockFile, nil
}

func renderBundle(chunks []Chunk, contents map[string][]byte) ([]byte, error) {
	selected := make([]Chunk, 0)
	for _, chunk := range chunks {
		if chunk.Selected {
			selected = append(selected, chunk)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].SelectionOrder < selected[j].SelectionOrder
	})

	var buffer bytes.Buffer
	buffer.WriteString("# Distill Context Bundle\n\n")
	buffer.WriteString("Schema: distill-lock/v0\n\n")
	for _, chunk := range selected {
		content, ok := contents[chunkKey(chunk)]
		if !ok {
			return nil, fmt.Errorf("missing normalized content for %s:%d-%d", chunk.Path, chunk.StartByte, chunk.EndByte)
		}
		pathJSON, err := json.Marshal(chunk.Path)
		if err != nil {
			return nil, fmt.Errorf("encode bundle path: %w", err)
		}
		fmt.Fprintf(
			&buffer,
			"<!-- distill-lock/v0 chunk=%d path=%s start=%d end=%d sha256=%s -->\n",
			chunk.SelectionOrder,
			pathJSON,
			chunk.StartByte,
			chunk.EndByte,
			chunk.SHA256,
		)
		buffer.Write(content)
		buffer.WriteString("\n<!-- /distill-lock/v0 -->\n\n")
	}
	return buffer.Bytes(), nil
}

func renderChecksums(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buffer bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&buffer, "%s  %d  %s\n", digestBytes(files[name]), len(files[name]), name)
	}
	return buffer.Bytes()
}

func validateBuildDestination(destination, sourceRoot string) error {
	if filepath.Base(destination) == "." || filepath.Base(destination) == string(filepath.Separator) {
		return fmt.Errorf("output directory path is unsafe")
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("output directory %q already exists; build requires a fresh destination", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output directory: %w", err)
	}

	parent, err := resolvedDirectory(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("resolve output parent: %w", err)
	}
	sourceResolved, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	outputResolved := filepath.Join(parent, filepath.Base(destination))
	inside, err := isWithin(sourceResolved, outputResolved)
	if err != nil {
		return fmt.Errorf("compare output and source paths: %w", err)
	}
	if inside {
		return fmt.Errorf("output directory must be outside the source root")
	}
	return nil
}

func writeSyncedFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
