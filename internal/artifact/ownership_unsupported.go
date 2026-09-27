//go:build !darwin && !linux

package artifact

import "io/fs"

func ownedByCurrentUserOrRoot(fs.FileInfo) bool {
	return false
}
