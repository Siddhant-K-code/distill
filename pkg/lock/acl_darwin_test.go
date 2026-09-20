//go:build darwin

package lock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
