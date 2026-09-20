//go:build !darwin && !linux

package lock

import "io/fs"

func ownedByCurrentUserOrRoot(fs.FileInfo) bool {
	return false
}
