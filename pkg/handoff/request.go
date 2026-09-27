package handoff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Siddhant-K-code/distill/internal/artifact"
)

type requestBundle struct {
	root         string
	request      Request
	requestBytes []byte
	conversation []byte
	documents    map[string][]byte
}

func loadRequestBundle(requestPath string) (*requestBundle, []byte, error) {
	if filepath.Base(requestPath) != RequestFileName {
		return nil, nil, fmt.Errorf("request file must be named %q", RequestFileName)
	}
	root, err := artifact.ResolveDirectory(filepath.Dir(requestPath))
	if err != nil {
		return nil, nil, fmt.Errorf("resolve request bundle: %w", err)
	}
	requestPath = filepath.Join(root, RequestFileName)
	requestBytes, err := readSafeFile(requestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read request: %w", err)
	}
	var request Request
	if err := artifact.DecodeCanonicalJSON(requestBytes, &request); err != nil {
		return nil, nil, fmt.Errorf("request: %w", err)
	}
	if err := validateRequest(request); err != nil {
		return nil, nil, fmt.Errorf("request: %w", err)
	}

	expected := map[string]InputIdentity{
		request.Conversation.BundlePath: request.Conversation,
	}
	for _, document := range request.Documents {
		expected[document.BundlePath] = document
	}
	expectedFiles := map[string]struct{}{
		RequestFileName: {}, InstructionsFileName: {}, ProposalSchemaName: {}, ChecksumsFileName: {},
	}
	for name := range expected {
		expectedFiles[name] = struct{}{}
	}
	expectedDirectories := allowedDirectories(expectedFiles)

	actualFiles := make(map[string][]byte, len(expectedFiles))
	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		portable := filepath.ToSlash(relative)
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("request bundle path %q is a symbolic link", portable)
		}
		if info.IsDir() {
			if _, ok := expectedDirectories[portable]; !ok {
				return fmt.Errorf("unexpected request bundle directory %q", portable)
			}
			return artifact.ValidateTrustedInfo(info, filePath, true)
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("request bundle path %q is not a regular file", portable)
		}
		if _, ok := expectedFiles[portable]; !ok {
			return fmt.Errorf("unexpected request bundle file %q", portable)
		}
		data, err := artifact.ReadRegularFile(filePath, info)
		if err != nil {
			return err
		}
		actualFiles[portable] = data
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(actualFiles) != len(expectedFiles) {
		return nil, nil, fmt.Errorf("request bundle file count mismatch")
	}
	if !bytes.Equal(actualFiles[RequestFileName], requestBytes) {
		return nil, nil, fmt.Errorf("request bytes changed while validating bundle")
	}
	if !bytes.Equal(actualFiles[InstructionsFileName], instructionsBytes) {
		return nil, nil, fmt.Errorf("agent instructions do not match this schema version")
	}
	if !bytes.Equal(actualFiles[ProposalSchemaName], proposalSchemaBytes) {
		return nil, nil, fmt.Errorf("proposal schema does not match this schema version")
	}

	checksumInputs := make(map[string][]byte, len(actualFiles)-1)
	for name, data := range actualFiles {
		if name != ChecksumsFileName {
			checksumInputs[name] = data
		}
	}
	expectedChecksums := artifact.RenderChecksums(checksumInputs)
	if !bytes.Equal(actualFiles[ChecksumsFileName], expectedChecksums) {
		return nil, nil, fmt.Errorf("request bundle SHA256SUMS mismatch")
	}
	if _, err := parseChecksums(actualFiles[ChecksumsFileName], len(checksumInputs)); err != nil {
		return nil, nil, fmt.Errorf("request bundle SHA256SUMS: %w", err)
	}

	documents := make(map[string][]byte, len(request.Documents))
	for bundlePath, identity := range expected {
		data := actualFiles[bundlePath]
		if int64(len(data)) != identity.NormalizedBytes ||
			artifact.DigestBytes(data) != identity.NormalizedSHA256 {
			return nil, nil, fmt.Errorf("normalized input identity mismatch for %q", identity.Path)
		}
		normalized, err := artifact.NormalizeText(data)
		if err != nil || !bytes.Equal(normalized, data) {
			return nil, nil, fmt.Errorf("input %q is not normalized", identity.Path)
		}
		if bundlePath != ConversationPath {
			documents[identity.Path] = data
		}
	}
	return &requestBundle{
		root: root, request: request, requestBytes: requestBytes,
		conversation: actualFiles[ConversationPath], documents: documents,
	}, requestBytes, nil
}

func allowedDirectories[T any](files map[string]T) map[string]struct{} {
	directories := make(map[string]struct{})
	for name := range files {
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(name))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			directories[directory] = struct{}{}
		}
	}
	return directories
}

func readSafeFile(filePath string) ([]byte, error) {
	return readSafeFileLimited(filePath, 0)
}

func readSafeFileLimited(filePath string, maximumBytes int64) ([]byte, error) {
	parent, err := artifact.ResolveDirectory(filepath.Dir(filePath))
	if err != nil {
		return nil, err
	}
	filePath = filepath.Join(parent, filepath.Base(filePath))
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("must be a regular file, not a symlink")
	}
	if maximumBytes > 0 && info.Size() > maximumBytes {
		return nil, fmt.Errorf("file exceeds the %d-byte limit", maximumBytes)
	}
	if err := artifact.ValidateTrustedInfo(info, filePath, false); err != nil {
		return nil, err
	}
	data, err := artifact.ReadRegularFile(filePath, info)
	if err != nil {
		return nil, err
	}
	if maximumBytes > 0 && int64(len(data)) > maximumBytes {
		return nil, fmt.Errorf("file exceeds the %d-byte limit", maximumBytes)
	}
	return data, nil
}

func parseChecksums(data []byte, expectedCount int) (map[string]string, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("must end with LF")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != expectedCount {
		return nil, fmt.Errorf("entry count = %d, want %d", len(lines), expectedCount)
	}
	result := make(map[string]string, len(lines))
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 3)
		if len(parts) != 3 || !artifact.ValidDigest(parts[0]) {
			return nil, fmt.Errorf("invalid checksum line %q", line)
		}
		if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
			return nil, fmt.Errorf("invalid byte length in %q", line)
		}
		canonical, err := artifact.CanonicalPortablePath(parts[2])
		if err != nil || canonical != parts[2] || parts[2] == ChecksumsFileName {
			return nil, fmt.Errorf("invalid checksum path %q", parts[2])
		}
		if _, duplicate := result[parts[2]]; duplicate {
			return nil, fmt.Errorf("duplicate checksum path %q", parts[2])
		}
		result[parts[2]] = parts[0]
		names = append(names, parts[2])
	}
	if !sort.StringsAreSorted(names) {
		return nil, fmt.Errorf("paths must be sorted")
	}
	return result, nil
}
