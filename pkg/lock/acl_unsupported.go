//go:build !darwin && !linux

package lock

func hasExtendedACL(string) (bool, error) {
	return false, nil
}
