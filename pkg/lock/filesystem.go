package lock

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type sourceInput struct {
	path       string
	original   []byte
	normalized []byte
}

var supportedExtensions = map[string]struct{}{
	".bash": {}, ".c": {}, ".cc": {}, ".cfg": {}, ".conf": {}, ".cpp": {},
	".cs": {}, ".css": {}, ".csv": {}, ".go": {}, ".graphql": {}, ".h": {},
	".hpp": {}, ".html": {}, ".ini": {}, ".java": {}, ".js": {}, ".jsx": {},
	".json": {}, ".md": {}, ".php": {}, ".proto": {}, ".py": {}, ".rb": {},
	".rs": {}, ".scss": {}, ".sh": {}, ".sql": {}, ".toml": {}, ".ts": {},
	".tsx": {}, ".txt": {}, ".xml": {}, ".yaml": {}, ".yml": {}, ".zsh": {},
}

func scanSourceRoot(root string) ([]sourceInput, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect source root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source root %q is a symbolic link", root)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("source root %q is not a directory", root)
	}
	if err := validateTrustedInfo(rootInfo, root, true); err != nil {
		return nil, err
	}

	var inputs []sourceInput
	canonicalPaths := make(map[string]string)
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", filePath, walkErr)
		}
		if filePath == root {
			return nil
		}

		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return fmt.Errorf("make source path relative: %w", err)
		}
		portable := filepath.ToSlash(relative)
		canonical, err := canonicalPortablePath(portable)
		if err != nil {
			return fmt.Errorf("source path %q: %w", portable, err)
		}

		info, err := os.Lstat(filePath)
		if err != nil {
			return fmt.Errorf("inspect source %q: %w", portable, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source %q is a symbolic link; v0 rejects all symlinks", portable)
		}
		if info.IsDir() {
			if err := validateTrustedInfo(info, filePath, true); err != nil {
				return err
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source %q is not a regular file", portable)
		}
		if err := validateTrustedInfo(info, filePath, false); err != nil {
			return err
		}
		if previous, ok := canonicalPaths[canonical]; ok {
			return fmt.Errorf("duplicate canonical source path %q from %q and %q", canonical, previous, portable)
		}
		canonicalPaths[canonical] = portable
		if err := validateSupportedFile(canonical); err != nil {
			return err
		}

		original, err := readRegularFile(filePath, info)
		if err != nil {
			return fmt.Errorf("read source %q: %w", portable, err)
		}
		normalized, err := normalizeText(original)
		if err != nil {
			return fmt.Errorf("normalize source %q: %w", portable, err)
		}
		inputs = append(inputs, sourceInput{
			path:       canonical,
			original:   original,
			normalized: normalized,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].path < inputs[j].path
	})
	return inputs, nil
}

func validateSupportedFile(portable string) error {
	base := path.Base(portable)
	extension := strings.ToLower(path.Ext(base))
	if extension == "" {
		return nil
	}
	if _, ok := supportedExtensions[extension]; !ok {
		return fmt.Errorf("source %q has unsupported file type %q", portable, extension)
	}
	return nil
}

func readRegularFile(filePath string, before fs.FileInfo) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("file identity changed while opening")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, after) || opened.Size() != after.Size() {
		return nil, fmt.Errorf("file identity changed while reading")
	}
	return data, nil
}

func normalizeText(original []byte) ([]byte, error) {
	if bytes.IndexByte(original, 0) >= 0 {
		return nil, fmt.Errorf("NUL byte indicates binary input")
	}
	if !utf8.Valid(original) {
		return nil, fmt.Errorf("input is not valid UTF-8")
	}

	nfc := norm.NFC.Bytes(original)
	lf := bytes.ReplaceAll(nfc, []byte("\r\n"), []byte("\n"))
	lf = bytes.ReplaceAll(lf, []byte("\r"), []byte("\n"))
	return lf, nil
}

func resolvedDirectory(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := validateLexicalDirectoryPath(absolute); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", dir)
	}
	if err := validateTrustedAncestors(resolved); err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func validateLexicalDirectoryPath(absolute string) error {
	volume := filepath.VolumeName(absolute)
	root := volume + string(filepath.Separator)
	if volume == "" {
		root = string(filepath.Separator)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect directory root %q: %w", root, err)
	}
	if err := validateTrustedInfo(rootInfo, root, true); err != nil {
		return err
	}

	relative := strings.TrimPrefix(absolute, root)
	parts := strings.Split(relative, string(filepath.Separator))
	current := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect directory component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("directory component %q is a symbolic link", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("directory component %q is not a directory", current)
		}
		if err := validateTrustedInfo(info, current, true); err != nil {
			return err
		}
	}
	return nil
}

func validateTrustedAncestors(directory string) error {
	current := filepath.Clean(directory)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect directory ancestor %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("directory ancestor %q is not a real directory", current)
		}
		if err := validateTrustedInfo(info, current, true); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func validateTrustedInfo(info fs.FileInfo, filePath string, directory bool) error {
	if !ownedByCurrentUserOrRoot(info) {
		return fmt.Errorf("%q is not owned by the current user or root", filePath)
	}
	hasACL, err := hasUnsafeACL(filePath)
	if err != nil {
		return fmt.Errorf("inspect access controls for %q: %w", filePath, err)
	}
	if hasACL {
		return fmt.Errorf("%q has an access-control list granting mutation rights", filePath)
	}
	if info.Mode().Perm()&0o022 == 0 {
		return nil
	}
	if directory && info.Mode()&os.ModeSticky != 0 {
		return nil
	}
	return fmt.Errorf("%q is writable by group or other users", filePath)
}

func isWithin(parent, child string) (bool, error) {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false, err
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}
