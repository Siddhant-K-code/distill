//go:build darwin

package artifact

import (
	"encoding/binary"
	"testing"
)

func TestDarwinACLEvaluationTreatsPermitAsExposure(t *testing.T) {
	data := make([]byte, darwinFileSecurityHeaderBytes+darwinACEBytes)
	binary.LittleEndian.PutUint32(data[36:40], 1)
	binary.LittleEndian.PutUint32(data[darwinFileSecurityHeaderBytes+16:], darwinACEPermit)
	binary.LittleEndian.PutUint32(data[darwinFileSecurityHeaderBytes+20:], 1)

	unsafeACL, exposedACL, err := darwinACLEvaluation(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if unsafeACL || !exposedACL {
		t.Fatalf("unsafe=%t exposed=%t, want false/true", unsafeACL, exposedACL)
	}
}
