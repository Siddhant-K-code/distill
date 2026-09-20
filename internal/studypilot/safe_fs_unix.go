//go:build darwin || linux

package studypilot

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func ensureSafeDirectory(directory string) error {
	return walkSafeDirectory(directory, true)
}

func validateExistingSafeDirectory(directory string) error {
	return walkSafeDirectory(directory, false)
}

func walkSafeDirectory(directory string, create bool) error {
	directory = filepath.Clean(directory)
	volume := filepath.VolumeName(directory)
	root := volume + string(filepath.Separator)
	if volume == "" {
		root = string(filepath.Separator)
	}
	relative := strings.TrimPrefix(directory, root)
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && create {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%q is not a real directory", current)
		}
		if err := validateSafeInfo(current, info, true); err != nil {
			return fmt.Errorf("%q: %w", current, err)
		}
	}
	return nil
}

func validateSafeInfo(path string, info fs.FileInfo, directory bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine ownership")
	}
	uid := uint32(os.Geteuid())
	if stat.Uid != uid && stat.Uid != 0 {
		return fmt.Errorf("owner is neither current user nor root")
	}
	acl, err := evaluateACL(path)
	if err != nil {
		return fmt.Errorf("inspect access-control list: %w", err)
	}
	if acl.unsafe {
		return fmt.Errorf("access-control list grants mutation rights")
	}
	permissions := info.Mode().Perm()
	if acl.groupModeCovered {
		permissions &^= 0o020
	}
	if permissions&0o022 != 0 && (!directory || info.Mode()&os.ModeSticky == 0) {
		return fmt.Errorf("group/world writable")
	}
	return nil
}
