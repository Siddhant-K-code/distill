//go:build linux

package artifact

import (
	"encoding/binary"
	"testing"
)

func TestLinuxACLEvaluationTreatsReadGrantAsExposure(t *testing.T) {
	data := encodeLinuxACL([]linuxACLEntry{
		{tag: linuxACLUserObj, permissions: 6},
		{tag: linuxACLUser, permissions: 4, id: 1001},
		{tag: linuxACLMask, permissions: 7},
		{tag: linuxACLOther, permissions: 0},
	})
	unsafeACL, exposedACL, err := linuxACLEvaluation(data, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if unsafeACL || !exposedACL {
		t.Fatalf("unsafe=%t exposed=%t, want false/true", unsafeACL, exposedACL)
	}
}

func encodeLinuxACL(entries []linuxACLEntry) []byte {
	data := make([]byte, 4+len(entries)*8)
	binary.LittleEndian.PutUint32(data[:4], linuxACLVersion)
	for index, entry := range entries {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(data[offset:offset+2], entry.tag)
		binary.LittleEndian.PutUint16(data[offset+2:offset+4], entry.permissions)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], entry.id)
	}
	return data
}
