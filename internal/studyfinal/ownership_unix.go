//go:build darwin || linux

package studyfinal

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func requireCurrentOwner(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("artifact owner is not the current user")
	}
	return nil
}
