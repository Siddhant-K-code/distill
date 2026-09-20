//go:build linux

package studypilot

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

type aclEvaluation struct {
	unsafe           bool
	groupModeCovered bool
}

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
		unsafeACL, err := linuxACLGrantsMutation(data[:read], uint32(os.Geteuid()))
		if err != nil {
			return aclEvaluation{}, fmt.Errorf("%s: %w", name, err)
		}
		if unsafeACL {
			result.unsafe = true
			return result, nil
		}
	}
	return result, nil
}

func linuxACLGrantsMutation(data []byte, currentUID uint32) (bool, error) {
	if len(data) < 4 || (len(data)-4)%8 != 0 {
		return false, fmt.Errorf("invalid POSIX ACL length %d", len(data))
	}
	if version := binary.LittleEndian.Uint32(data[:4]); version != linuxACLVersion {
		return false, fmt.Errorf("unsupported POSIX ACL version %d", version)
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
	for _, entry := range entries {
		switch entry.tag {
		case linuxACLUserObj, linuxACLMask:
			continue
		case linuxACLUser:
			if entry.id == 0 || entry.id == currentUID {
				continue
			}
			if entry.permissions&mask&linuxACLWrite != 0 {
				return true, nil
			}
		case linuxACLGroupObj, linuxACLGroup:
			if entry.permissions&mask&linuxACLWrite != 0 {
				return true, nil
			}
		case linuxACLOther:
			if entry.permissions&linuxACLWrite != 0 {
				return true, nil
			}
		default:
			return false, fmt.Errorf("unsupported POSIX ACL tag %#x", entry.tag)
		}
	}
	return false, nil
}
