//go:build linux

package lock

import (
	"errors"

	"golang.org/x/sys/unix"
)

func hasExtendedACL(path string) (bool, error) {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		size, err := unix.Lgetxattr(path, name, nil)
		if err == nil {
			if size > 0 {
				return true, nil
			}
			continue
		}
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			continue
		}
		return false, err
	}
	return false, nil
}
