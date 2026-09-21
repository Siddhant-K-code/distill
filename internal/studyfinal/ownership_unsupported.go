//go:build !darwin && !linux

package studyfinal

import (
	"fmt"
	"io/fs"
)

func requireCurrentOwner(fs.FileInfo) error {
	return fmt.Errorf("owner validation is supported only on macOS and Linux")
}
