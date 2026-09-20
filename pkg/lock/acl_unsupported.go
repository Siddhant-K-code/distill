//go:build !darwin && !linux

package lock

func hasUnsafeACL(string) (bool, error) {
	return false, nil
}
