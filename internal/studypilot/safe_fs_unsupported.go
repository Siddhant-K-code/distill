//go:build !darwin && !linux

package studypilot

import (
	"fmt"
	"io/fs"
)

func ensureSafeDirectory(string) error {
	return fmt.Errorf("safe artifact publication is supported only on macOS and Linux")
}

func validateExistingSafeDirectory(string) error {
	return fmt.Errorf("safe artifact validation is supported only on macOS and Linux")
}

func validateSafeInfo(string, fs.FileInfo, bool) error {
	return fmt.Errorf("safe artifact validation is supported only on macOS and Linux")
}
