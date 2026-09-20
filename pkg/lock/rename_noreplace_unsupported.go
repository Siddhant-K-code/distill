//go:build !darwin && !linux

package lock

import (
	"fmt"
	"runtime"
)

func renameNoReplace(_, _ string) error {
	return fmt.Errorf("atomic no-replace publication is unsupported on %s", runtime.GOOS)
}
