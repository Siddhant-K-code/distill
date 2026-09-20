//go:build darwin

package lock

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDarwinGenericWriteACLIsUnsafe(t *testing.T) {
	for _, rights := range []uint32{1 << 21, 1 << 23} {
		data := make([]byte, darwinFileSecurityHeaderBytes+darwinACEBytes)
		binary.LittleEndian.PutUint32(data[36:40], 1)
		binary.LittleEndian.PutUint32(data[darwinFileSecurityHeaderBytes+16:darwinFileSecurityHeaderBytes+20], darwinACEPermit)
		binary.LittleEndian.PutUint32(data[darwinFileSecurityHeaderBytes+20:darwinFileSecurityHeaderBytes+24], rights)
		unsafe, err := darwinACLGrantsMutation(data, len(data))
		if err != nil {
			t.Fatal(err)
		}
		if !unsafe {
			t.Fatalf("generic ACL right %#x was accepted", rights)
		}
	}
}

func TestExtendedACLIsRejectedOnDarwin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acl.txt")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("chmod", "-N", path).Run()
	})
	if output, err := exec.Command("chmod", "+a", "everyone allow write", path).CombinedOutput(); err != nil {
		t.Fatalf("create test ACL: %v: %s", err, output)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTrustedInfo(info, path, false); err == nil {
		t.Fatal("file with extended ACL was accepted")
	}
}
