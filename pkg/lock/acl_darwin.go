//go:build darwin

package lock

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinFileSecurityHeaderBytes = 44
	darwinACEBytes                = 24
	darwinACEPermit               = 1
	darwinACEKindMask             = 0xf
	darwinWriteRights             = (1 << 2) | (1 << 4) | (1 << 5) | (1 << 6) |
		(1 << 8) | (1 << 10) | (1 << 12) | (1 << 13) | (1 << 21) | (1 << 23) |
		(1 << 25)
)

type darwinAttributeList struct {
	BitmapCount uint16
	Reserved    uint16
	Common      uint32
	Volume      uint32
	Directory   uint32
	File        uint32
	Fork        uint32
}

type darwinAttributeReference struct {
	Offset int32
	Length uint32
}

type darwinACLBuffer struct {
	Length    uint32
	Reference darwinAttributeReference
	Data      [4096]byte
}

func hasUnsafeACL(path string) (bool, error) {
	pathPointer, err := unix.BytePtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes := darwinAttributeList{
		BitmapCount: 5,
		Common:      unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	var buffer darwinACLBuffer
	//nolint:staticcheck // No pure-Go libSystem ACL wrapper is available with CGO disabled.
	_, _, errno := unix.Syscall6(
		unix.SYS_GETATTRLIST,
		uintptr(unsafe.Pointer(pathPointer)),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Sizeof(buffer)),
		unix.FSOPT_NOFOLLOW,
		0,
	)
	if errno != 0 {
		return false, errno
	}
	if buffer.Reference.Length == 0 {
		return false, nil
	}
	return darwinACLGrantsMutation(buffer.Data[:], int(buffer.Reference.Length))
}

func darwinACLGrantsMutation(data []byte, length int) (bool, error) {
	if length < darwinFileSecurityHeaderBytes || length > len(data) {
		return false, fmt.Errorf("invalid extended security data length %d", length)
	}
	data = data[:length]
	entryCount := int(binary.LittleEndian.Uint32(data[36:40]))
	if expected := darwinFileSecurityHeaderBytes + entryCount*darwinACEBytes; expected != length {
		return false, fmt.Errorf("invalid ACL entry count %d for length %d", entryCount, length)
	}
	for index := 0; index < entryCount; index++ {
		offset := darwinFileSecurityHeaderBytes + index*darwinACEBytes
		flags := binary.LittleEndian.Uint32(data[offset+16 : offset+20])
		rights := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
		if flags&darwinACEKindMask == darwinACEPermit && rights&darwinWriteRights != 0 {
			return true, nil
		}
	}
	return false, nil
}
