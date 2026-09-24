package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineLifecycle(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "local-package")
	if err := run([]string{"prepare", "--output", directory}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "--input", directory}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"summarize", "--input", directory}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "schedule.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsMissingArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"prepare"}, {"validate"}, {"summarize"}, {"validate-public"}, {"unknown"}} {
		if err := run(args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestValidatePublishedPublicEvidence(t *testing.T) {
	if err := run([]string{
		"validate-public",
		"--input", filepath.Join("..", "..", "research", "context-is-a-build-artifact"),
	}); err != nil {
		t.Fatal(err)
	}
}
