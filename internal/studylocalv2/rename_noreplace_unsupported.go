//go:build !darwin && !linux

package studylocalv2

import "fmt"

func renameNoReplace(_, _ string) error {
	return fmt.Errorf("atomic no-replace publication is supported only on macOS and Linux")
}
