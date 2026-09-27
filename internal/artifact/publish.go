package artifact

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type PublishedDurabilityError struct {
	Path string
	Err  error
}

func (err *PublishedDurabilityError) Error() string {
	return fmt.Sprintf("output published at %q, but parent-directory durability is unconfirmed: %v; verify it before any retry", err.Path, err.Err)
}

func (err *PublishedDurabilityError) Unwrap() error {
	return err.Err
}

func RenderChecksums(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buffer bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&buffer, "%s  %d  %s\n", DigestBytes(files[name]), len(files[name]), name)
	}
	return buffer.Bytes()
}

func PublishDirectory(outputDirectory string, files map[string][]byte, verify func(string) error) error {
	return publishDirectoryWith(outputDirectory, files, verify, syncDirectory, renameNoReplace)
}

func publishDirectoryWith(
	outputDirectory string,
	files map[string][]byte,
	verify func(string) error,
	syncDirectoryFn func(string) error,
	renameNoReplaceFn func(string, string) error,
) error {
	outputAbsolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	outputParent, err := ResolveDirectory(filepath.Dir(outputAbsolute))
	if err != nil {
		return fmt.Errorf("resolve output parent: %w", err)
	}
	outputAbsolute = filepath.Join(outputParent, filepath.Base(outputAbsolute))
	if filepath.Base(outputAbsolute) == "." || filepath.Base(outputAbsolute) == string(filepath.Separator) {
		return fmt.Errorf("output directory path is unsafe")
	}
	if _, err := os.Lstat(outputAbsolute); err == nil {
		return fmt.Errorf("output directory %q already exists; a fresh destination is required", outputAbsolute)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output directory: %w", err)
	}

	temporary, err := os.MkdirTemp(outputParent, "."+filepath.Base(outputAbsolute)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary output directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.Chmod(temporary, 0o755); err != nil {
		return fmt.Errorf("set temporary output permissions: %w", err)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		canonical, err := CanonicalPortablePath(name)
		if err != nil {
			return fmt.Errorf("output path %q: %w", name, err)
		}
		if canonical != name {
			return fmt.Errorf("output path %q is not canonical; use %q", name, canonical)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		destination := filepath.Join(temporary, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create output parent for %q: %w", name, err)
		}
		if err := writeSyncedFile(destination, files[name]); err != nil {
			return fmt.Errorf("write output %q: %w", name, err)
		}
	}
	if err := syncDirectoryTree(temporary, syncDirectoryFn); err != nil {
		return fmt.Errorf("sync temporary output: %w", err)
	}
	if err := verify(temporary); err != nil {
		return fmt.Errorf("verify temporary output: %w", err)
	}
	if err := syncDirectoryFn(outputParent); err != nil {
		return fmt.Errorf("sync output parent before publication: %w", err)
	}
	if err := renameNoReplaceFn(temporary, outputAbsolute); err != nil {
		return fmt.Errorf("publish output directory: %w", err)
	}
	published = true
	if err := syncDirectoryFn(outputParent); err != nil {
		return &PublishedDurabilityError{Path: outputAbsolute, Err: err}
	}
	return nil
}

func writeSyncedFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
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

func syncDirectoryTree(root string, syncDirectoryFn func(string) error) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		return len(directories[i]) > len(directories[j])
	})
	for _, directory := range directories {
		if err := syncDirectoryFn(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() {
		_ = handle.Close()
	}()
	return handle.Sync()
}
