package artifact

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type aclEvaluation struct {
	unsafe           bool
	exposed          bool
	groupModeCovered bool
}

func NormalizeText(original []byte) ([]byte, error) {
	if bytes.IndexByte(original, 0) >= 0 {
		return nil, fmt.Errorf("NUL byte indicates binary input")
	}
	if !utf8.Valid(original) {
		return nil, fmt.Errorf("input is not valid UTF-8")
	}

	nfc := norm.NFC.Bytes(original)
	lf := bytes.ReplaceAll(nfc, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(lf, []byte("\r"), []byte("\n")), nil
}

func CanonicalPortablePath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("path is not valid UTF-8")
	}
	if filepath.IsAbs(value) || path.IsAbs(value) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if strings.Contains(value, `\`) {
		return "", fmt.Errorf("backslashes are not allowed; use POSIX separators")
	}
	if value != path.Clean(value) {
		return "", fmt.Errorf("path is not lexically normalized")
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("empty, dot, and dot-dot segments are not allowed")
		}
	}
	for _, r := range value {
		if r == 0 || unicode.IsControl(r) {
			return "", fmt.Errorf("control characters are not allowed")
		}
	}
	return norm.NFC.String(value), nil
}

func ReadRegularFile(filePath string, before fs.FileInfo) ([]byte, error) {
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

func ResolveDirectory(dir string) (string, error) {
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

func ValidateTrustedInfo(info fs.FileInfo, filePath string, directory bool) error {
	if !ownedByCurrentUserOrRoot(info) {
		return fmt.Errorf("%q is not owned by the current user or root", filePath)
	}
	acl, err := evaluateACL(filePath)
	if err != nil {
		return fmt.Errorf("inspect access controls for %q: %w", filePath, err)
	}
	if acl.unsafe {
		return fmt.Errorf("%q has an access-control list granting mutation rights", filePath)
	}
	permissions := info.Mode().Perm()
	if acl.groupModeCovered {
		permissions &^= 0o020
	}
	if permissions&0o022 == 0 {
		return nil
	}
	if directory && info.Mode()&os.ModeSticky != 0 {
		return nil
	}
	return fmt.Errorf("%q is writable by group or other users", filePath)
}

func ValidatePrivateInfo(info fs.FileInfo, filePath string, directory bool) error {
	if err := ValidateTrustedInfo(info, filePath, directory); err != nil {
		return err
	}
	want := fs.FileMode(0o600)
	if directory {
		want = 0o700
	}
	if info.Mode().Perm() != want {
		return fmt.Errorf("%q permissions are %04o, want %04o", filePath, info.Mode().Perm(), want)
	}
	acl, err := evaluateACL(filePath)
	if err != nil {
		return fmt.Errorf("inspect access controls for %q: %w", filePath, err)
	}
	if acl.exposed {
		return fmt.Errorf("%q has an access-control list granting access to another principal", filePath)
	}
	return nil
}

func ExistingPathWithin(parent, child string) (bool, error) {
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return false, err
	}
	current := filepath.Clean(child)
	for {
		info, err := os.Stat(current)
		if err != nil {
			return false, err
		}
		if os.SameFile(parentInfo, info) {
			return true, nil
		}
		next := filepath.Dir(current)
		if next == current {
			return false, nil
		}
		current = next
	}
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
	if err := ValidateTrustedInfo(rootInfo, root, true); err != nil {
		return err
	}

	relative := strings.TrimPrefix(absolute, root)
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
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
		if err := ValidateTrustedInfo(info, current, true); err != nil {
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
		if err := ValidateTrustedInfo(info, current, true); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}
