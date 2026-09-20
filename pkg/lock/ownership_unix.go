//go:build darwin || linux

package lock

import (
	"io/fs"
	"os"
	"syscall"
)

func ownedByCurrentUserOrRoot(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	effectiveUser := uint32(os.Geteuid())
	return stat.Uid == effectiveUser || stat.Uid == 0
}
