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
		0o755,
		0o644,
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

func TestPublishPrivateDirectoryUsesPrivateModes(t *testing.T) {
	root := realTempDir(t)
	output := filepath.Join(root, "output")
	files := map[string][]byte{
		"request.json":      []byte("{}\n"),
		"nested/input.md":   []byte("private\n"),
		"nested/deeper/doc": []byte("private\n"),
	}
	verify := func(directory string) error {
		return verifyPrivateTree(directory)
	}
	if err := PublishPrivateDirectory(output, files, verify); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivateTree(output); err != nil {
		t.Fatal(err)
	}
}

func TestPublishDirectoryRetainsPortableModes(t *testing.T) {
	root := realTempDir(t)
	output := filepath.Join(root, "output")
	if err := PublishDirectory(
		output,
		map[string][]byte{"nested/artifact.txt": []byte("content\n")},
		func(string) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		output:                          0o755,
		filepath.Join(output, "nested"): 0o755,
		filepath.Join(output, "nested", "artifact.txt"): 0o644,
	} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%q permissions = %04o, want %04o", path, got, want)
		}
	}
}

func TestValidatePrivateInfoRejectsReadableMode(t *testing.T) {
	root := realTempDir(t)
	path := filepath.Join(root, "artifact")
	if err := os.WriteFile(path, []byte("private\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateInfo(info, path, false); err == nil {
		t.Fatal("group-readable private artifact was accepted")
	}
}

func verifyPrivateTree(root string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if err := ValidatePrivateInfo(rootInfo, root, true); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		return ValidatePrivateInfo(info, path, entry.IsDir())
	})
}

func realTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
