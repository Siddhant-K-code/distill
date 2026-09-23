//go:build !darwin && !linux

package studylocal

import (
	"fmt"
	"io/fs"
)

func requireRootOrCurrentOwner(fs.FileInfo) error {
	return fmt.Errorf("root/current owner verification is supported only on macOS and Linux")
}
