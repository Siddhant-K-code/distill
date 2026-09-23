//go:build darwin || linux

package studylocal

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func requireRootOrCurrentOwner(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) {
		return fmt.Errorf("path owner is neither root nor the current user")
	}
	return nil
}
