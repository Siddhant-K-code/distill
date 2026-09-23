package studylocal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type treeManifestEntry struct {
	kind   string
	path   string
	mode   os.FileMode
	size   int64
	digest string
	target string
}

func verifyPrivateRuntimeInputs(options RuntimeManifestOptions) error {
	python, err := safePythonExecutable(options.RuntimePython)
	if err != nil {
		return err
	}
	for _, path := range []string{
		options.RuntimeTreeManifestPath,
		options.BaseTreeManifestPath,
		options.WheelVerificationPath,
	} {
		if _, err := safeRegularAbsolute(path); err != nil {
			return err
		}
		if err := requireTrustedPathAncestors(path); err != nil {
			return err
		}
	}
	venvRoot := filepath.Dir(filepath.Dir(python))
	resolvedPython, err := filepath.EvalSymlinks(python)
	if err != nil {
		return err
	}
	baseRoot := filepath.Dir(filepath.Dir(resolvedPython))
	if err := requireTrustedPathAncestors(venvRoot); err != nil {
		return err
	}
	if err := requireTrustedPathAncestors(baseRoot); err != nil {
		return err
	}
	runtimeManifest, err := os.ReadFile(options.RuntimeTreeManifestPath)
	if err != nil {
		return err
	}
	if DigestBytes(runtimeManifest) != RuntimeVenvManifestSHA256 {
		return fmt.Errorf("private venv tree manifest digest mismatch")
	}
	if err := verifyTreeManifest(venvRoot, runtimeManifest, 5532, 831, 3, true); err != nil {
		return fmt.Errorf("verify venv tree: %w", err)
	}
	baseManifest, err := os.ReadFile(options.BaseTreeManifestPath)
	if err != nil {
		return err
	}
	if DigestBytes(baseManifest) != BasePythonManifestSHA256 {
		return fmt.Errorf("private base Python tree manifest digest mismatch")
	}
	if err := verifyTreeManifest(baseRoot, baseManifest, 1942, 191, 8, false); err != nil {
		return fmt.Errorf("verify base Python tree: %w", err)
	}
	info, err := os.Stat(resolvedPython)
	if err != nil {
		return err
	}
	digest, err := digestRegularFile(resolvedPython, info)
	if err != nil || digest != BasePythonBinarySHA256 {
		return fmt.Errorf("base Python binary identity mismatch")
	}
	wheelVerification, err := os.ReadFile(options.WheelVerificationPath)
	if err != nil {
		return err
	}
	if DigestBytes(wheelVerification) != RuntimeWheelVerificationSHA256 {
		return fmt.Errorf("wheel verification record digest mismatch")
	}
	return nil
}

func parseTreeManifest(data []byte) (map[string]treeManifestEntry, error) {
	entries := map[string]treeManifestEntry{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 3 || (fields[0] != "D" && fields[0] != "F" && fields[0] != "L") ||
			!safeRelative(fields[1]) {
			return nil, fmt.Errorf("malformed tree manifest entry")
		}
		if _, exists := entries[fields[1]]; exists {
			return nil, fmt.Errorf("duplicate tree manifest path")
		}
		modeValue, err := strconv.ParseUint(fields[2], 8, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid tree manifest mode")
		}
		entry := treeManifestEntry{kind: fields[0], path: fields[1], mode: os.FileMode(modeValue)}
		switch entry.kind {
		case "D":
			if len(fields) != 3 {
				return nil, fmt.Errorf("invalid directory manifest entry")
			}
		case "F":
			if len(fields) != 5 || len(fields[4]) != 64 {
				return nil, fmt.Errorf("invalid file manifest entry")
			}
			entry.size, err = strconv.ParseInt(fields[3], 10, 64)
			if err != nil || entry.size < 0 {
				return nil, fmt.Errorf("invalid file manifest size")
			}
			entry.digest = fields[4]
		case "L":
			if len(fields) != 4 || fields[3] == "" {
				return nil, fmt.Errorf("invalid symlink manifest entry")
			}
			entry.target = fields[3]
		}
		entries[entry.path] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func verifyTreeManifest(root string, data []byte, expectedFiles, expectedDirectories, expectedSymlinks int, rejectPythonHooks bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	entries, err := parseTreeManifest(data)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	files, directories, symlinks := 0, 0, 0
	err = filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		entry, ok := entries[relative]
		if !ok || seen[relative] {
			return fmt.Errorf("unregistered tree path %q", relative)
		}
		seen[relative] = true
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("tree entry is group/world writable: %q", relative)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			if entry.kind != "L" {
				return fmt.Errorf("tree type mismatch for %q", relative)
			}
			target, err := os.Readlink(path)
			if err != nil || target != entry.target {
				return fmt.Errorf("tree symlink mismatch for %q", relative)
			}
			targetInfo, err := os.Stat(path)
			if err != nil || targetInfo.Mode().Perm() != entry.mode {
				return fmt.Errorf("tree symlink target mode mismatch for %q", relative)
			}
			symlinks++
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
		case info.IsDir():
			if entry.kind != "D" || info.Mode().Perm() != entry.mode {
				return fmt.Errorf("tree directory mismatch for %q", relative)
			}
			directories++
		case info.Mode().IsRegular():
			if entry.kind != "F" || info.Mode().Perm() != entry.mode || info.Size() != entry.size {
				return fmt.Errorf("tree file mismatch for %q", relative)
			}
			digest, err := digestRegularFile(path, info)
			if err != nil || digest != entry.digest {
				return fmt.Errorf("tree file digest mismatch for %q", relative)
			}
			files++
		default:
			return fmt.Errorf("unsupported tree entry %q", relative)
		}
		if rejectPythonHooks {
			name := strings.ToLower(filepath.Base(relative))
			if name == "sitecustomize.py" || name == "usercustomize.py" ||
				name == "__pycache__" || strings.HasSuffix(name, ".pth") ||
				strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, ".pyo") {
				return fmt.Errorf("forbidden Python startup or bytecode path %q", relative)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(entries) || files != expectedFiles || directories != expectedDirectories || symlinks != expectedSymlinks {
		return fmt.Errorf("tree inventory mismatch: files=%d directories=%d symlinks=%d entries=%d/%d",
			files, directories, symlinks, len(seen), len(entries))
	}
	return nil
}

func requireTrustedPathAncestors(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(absolute, string(filepath.Separator)), string(filepath.Separator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if index < len(parts)-1 && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("trusted path ancestor is a symlink: %s", current)
		}
		if info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("trusted path component is group/world writable: %s", current)
		}
		if err := requireRootOrCurrentOwner(info); err != nil {
			return fmt.Errorf("%s: %w", current, err)
		}
	}
	return nil
}
