package lock

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
)

const (
	minimumGoMinor = 24
	maximumGoMinor = 26
)

var goRuntimePattern = regexp.MustCompile(`^go1\.([0-9]+)(?:\.[0-9]+)?$`)

func validateSupportedRuntime() error {
	return validateRuntimeVersion(runtime.Version())
}

func validateRuntimeVersion(version string) error {
	match := goRuntimePattern.FindStringSubmatch(version)
	if match == nil {
		return fmt.Errorf("unsupported Go runtime %q; Distill Lock v0 requires Go 1.%d through 1.%d", version, minimumGoMinor, maximumGoMinor)
	}
	minor, err := strconv.Atoi(match[1])
	if err != nil || minor < minimumGoMinor || minor > maximumGoMinor {
		return fmt.Errorf("unsupported Go runtime %q; Distill Lock v0 requires Go 1.%d through 1.%d", version, minimumGoMinor, maximumGoMinor)
	}
	return nil
}
