package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishDirectoryFailureLeavesNoDestination(t *testing.T) {
	root := realTempDir(t)
	output := filepath.Join(root, "output")
	expected := errors.New("verification failed")
	err := PublishDirectory(output, map[string][]byte{"artifact.txt": []byte("content\n")}, func(string) error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("got %v, want verification failure", err)
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatal("failed publication left a destination")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed publication left staging entries: %v", entries)
	}
}

func TestPublishDirectoryDoesNotReplaceDestination(t *testing.T) {
	root := realTempDir(t)
	output := filepath.Join(root, "output")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PublishDirectory(output, map[string][]byte{"artifact.txt": []byte("new\n")}, func(string) error {
		return nil
	}); err == nil {
		t.Fatal("existing destination was replaced")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
		t.Fatalf("destination marker changed: %q, %v", data, err)
	}
}

func TestPostPublishSyncFailureIsClassified(t *testing.T) {
	root := realTempDir(t)
	output := filepath.Join(root, "output")
	syncFailure := errors.New("sync failed")
	parentSyncs := 0
	err := publishDirectoryWith(
		output,
		map[string][]byte{"artifact.txt": []byte("content\n")},
		func(string) error { return nil },
		func(directory string) error {
			if directory == root {
				parentSyncs++
				if parentSyncs == 2 {
					return syncFailure
				}
			}
			return nil
		},
		renameNoReplace,
	)
	var publishedError *PublishedDurabilityError
	if !errors.As(err, &publishedError) || !errors.Is(err, syncFailure) {
		t.Fatalf("got %v, want classified published durability error", err)
	}
	if publishedError.Path != output {
		t.Fatalf("published path = %q, want %q", publishedError.Path, output)
	}
	if data, readErr := os.ReadFile(filepath.Join(output, "artifact.txt")); readErr != nil || string(data) != "content\n" {
		t.Fatalf("published output missing after durability error: %q, %v", data, readErr)
	}
}

func realTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
