//go:build linux

package artifact

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const (
	linuxACLVersion  = 2
	linuxACLUserObj  = 0x01
	linuxACLUser     = 0x02
	linuxACLGroupObj = 0x04
	linuxACLGroup    = 0x08
	linuxACLMask     = 0x10
	linuxACLOther    = 0x20
	linuxACLWrite    = 0x02
)

type linuxACLEntry struct {
	tag         uint16
	permissions uint16
	id          uint32
}

func evaluateACL(path string) (aclEvaluation, error) {
	var result aclEvaluation
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		size, err := unix.Lgetxattr(path, name, nil)
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			continue
		}
		if err != nil {
			return aclEvaluation{}, err
		}
		if size == 0 {
			continue
		}
		if name == "system.posix_acl_access" {
			result.groupModeCovered = true
		}
		data := make([]byte, size)
		read, err := unix.Lgetxattr(path, name, data)
		if err != nil {
			return aclEvaluation{}, err
		}
		unsafeACL, exposedACL, err := linuxACLEvaluation(data[:read], uint32(os.Geteuid()))
		if err != nil {
			return aclEvaluation{}, fmt.Errorf("%s: %w", name, err)
		}
		result.unsafe = result.unsafe || unsafeACL
		result.exposed = result.exposed || exposedACL
	}
	return result, nil
}

func linuxACLEvaluation(data []byte, currentUID uint32) (bool, bool, error) {
	if len(data) < 4 || (len(data)-4)%8 != 0 {
		return false, false, fmt.Errorf("invalid POSIX ACL length %d", len(data))
	}
	if version := binary.LittleEndian.Uint32(data[:4]); version != linuxACLVersion {
		return false, false, fmt.Errorf("unsupported POSIX ACL version %d", version)
	}
	entries := make([]linuxACLEntry, 0, (len(data)-4)/8)
	mask := uint16(7)
	for offset := 4; offset < len(data); offset += 8 {
		entry := linuxACLEntry{
			tag:         binary.LittleEndian.Uint16(data[offset : offset+2]),
			permissions: binary.LittleEndian.Uint16(data[offset+2 : offset+4]),
			id:          binary.LittleEndian.Uint32(data[offset+4 : offset+8]),
		}
		if entry.tag == linuxACLMask {
			mask = entry.permissions
		}
		entries = append(entries, entry)
	}
	var unsafeACL bool
	var exposedACL bool
	for _, entry := range entries {
		var effective uint16
		switch entry.tag {
		case linuxACLUserObj, linuxACLMask:
			continue
		case linuxACLUser:
			if entry.id == 0 || entry.id == currentUID {
				continue
			}
			effective = entry.permissions & mask
		case linuxACLGroupObj, linuxACLGroup:
			effective = entry.permissions & mask
		case linuxACLOther:
			effective = entry.permissions
		default:
			return false, false, fmt.Errorf("unsupported POSIX ACL tag %#x", entry.tag)
		}
		if effective != 0 {
			exposedACL = true
		}
		if effective&linuxACLWrite != 0 {
			unsafeACL = true
		}
	}
	return unsafeACL, exposedACL, nil
}
