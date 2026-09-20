//go:build darwin

package lock

import (
	"unsafe"

	"golang.org/x/sys/unix"
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

func hasExtendedACL(path string) (bool, error) {
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
	return buffer.Reference.Length != 0, nil
}
