//go:build linux

package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxInvalidUTF8FilenameFailsClosed(t *testing.T) {
	root := t.TempDir()
	invalidName := string([]byte{'b', 'a', 'd', 0xff, '.', 't', 'x', 't'})
	if err := os.WriteFile(filepath.Join(root, invalidName), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanSourceRoot(root); err == nil {
		t.Fatal("source enumeration accepted an invalid UTF-8 filename")
	}
}
