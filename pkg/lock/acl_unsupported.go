//go:build !darwin && !linux

package lock

func evaluateACL(string) (aclEvaluation, error) {
	return aclEvaluation{}, nil
}
