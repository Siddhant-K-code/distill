//go:build !darwin && !linux

package artifact

func evaluateACL(string) (aclEvaluation, error) {
	return aclEvaluation{}, nil
}
