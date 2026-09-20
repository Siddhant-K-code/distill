//go:build linux

package lock

import (
	"encoding/binary"
	"testing"
)

func TestLinuxACLEffectiveMutationRights(t *testing.T) {
	tests := []struct {
		name       string
		entries    []linuxACLEntry
		currentUID uint32
		unsafe     bool
	}{
		{
			name: "named reader",
			entries: []linuxACLEntry{
				{tag: linuxACLUserObj, permissions: 7},
				{tag: linuxACLUser, permissions: 4, id: 1001},
				{tag: linuxACLMask, permissions: 7},
				{tag: linuxACLOther, permissions: 4},
			},
			currentUID: 1000,
		},
		{
			name: "masked named writer",
			entries: []linuxACLEntry{
				{tag: linuxACLUserObj, permissions: 7},
				{tag: linuxACLUser, permissions: 6, id: 1001},
				{tag: linuxACLMask, permissions: 4},
				{tag: linuxACLOther, permissions: 4},
			},
			currentUID: 1000,
		},
		{
			name: "current user writer",
			entries: []linuxACLEntry{
				{tag: linuxACLUserObj, permissions: 7},
				{tag: linuxACLUser, permissions: 6, id: 1000},
				{tag: linuxACLMask, permissions: 7},
				{tag: linuxACLOther, permissions: 4},
			},
			currentUID: 1000,
		},
		{
			name: "named writer",
			entries: []linuxACLEntry{
				{tag: linuxACLUserObj, permissions: 7},
				{tag: linuxACLUser, permissions: 6, id: 1001},
				{tag: linuxACLMask, permissions: 7},
				{tag: linuxACLOther, permissions: 4},
			},
			currentUID: 1000,
			unsafe:     true,
		},
		{
			name: "group writer",
			entries: []linuxACLEntry{
				{tag: linuxACLUserObj, permissions: 7},
				{tag: linuxACLGroupObj, permissions: 6},
				{tag: linuxACLMask, permissions: 7},
				{tag: linuxACLOther, permissions: 4},
			},
			currentUID: 1000,
			unsafe:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := encodeLinuxACL(test.entries)
			unsafe, err := linuxACLGrantsMutation(data, test.currentUID)
			if err != nil {
				t.Fatal(err)
			}
			if unsafe != test.unsafe {
				t.Fatalf("unsafe = %t, want %t", unsafe, test.unsafe)
			}
		})
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
