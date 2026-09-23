//go:build !darwin && !linux

package studylocalv2

import (
	"fmt"
	"io/fs"
)

func requireCurrentOwner(fs.FileInfo) error {
	return fmt.Errorf("owner verification is supported only on macOS and Linux")
}
