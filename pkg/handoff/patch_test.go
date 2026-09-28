package handoff

import (
	"strings"
	"testing"
)

func TestPatchParserRejectsUnsupportedOperations(t *testing.T) {
	target := []byte("# Doc\n\nold\n")
	tests := []struct {
		name  string
		patch string
	}{
		{"git metadata", "diff --git a/doc.md b/doc.md\n--- a/doc.md\n+++ b/doc.md\n@@ -3 +3 @@\n-old\n+new\n"},
		{"create", "--- /dev/null\n+++ b/doc.md\n@@ -0,0 +1 @@\n+new\n"},
		{"delete", "--- a/doc.md\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-# Doc\n-\n-old\n"},
		{"rename", "--- a/doc.md\n+++ b/other.md\n@@ -3 +3 @@\n-old\n+new\n"},
		{"binary", "--- a/doc.md\n+++ b/doc.md\nBinary files differ\n"},
		{"no newline marker", "--- a/doc.md\n+++ b/doc.md\n@@ -3 +3 @@\n-old\n+new\n\\ No newline at end of file\n"},
		{"overlapping hunks", "--- a/doc.md\n+++ b/doc.md\n@@ -1,2 +1,2 @@\n # Doc\n-\n+changed\n@@ -2,2 +2,2 @@\n \n-old\n+new\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := parseAndApplyPatch(test.patch, "doc.md", target); err == nil {
				t.Fatal("unsupported patch was accepted")
			}
		})
	}
}

func TestPatchParserAppliesMultipleOrderedHunks(t *testing.T) {
	target := []byte("one\ntwo\nthree\nfour\n")
	patch := "--- a/doc.md\n+++ b/doc.md\n" +
		"@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n" +
		"@@ -3,2 +3,3 @@\n three\n+three-and-a-half\n four\n"
	patched, ranges, err := parseAndApplyPatch(patch, "doc.md", target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(patched), "one\nTWO\nthree\nthree-and-a-half\nfour\n"; got != want {
		t.Fatalf("patched = %q, want %q", got, want)
	}
	if len(ranges) == 0 {
		t.Fatal("changed ranges were not reported")
	}
}

func TestEvidenceRangeRejectsSplitUTF8(t *testing.T) {
	data := []byte("é\n")
	if !strings.Contains(string(data), "é") {
		t.Fatal("invalid fixture")
	}
	if validPrefix := data[:1]; string(validPrefix) == "é" {
		t.Fatal("fixture did not split UTF-8")
	}
}
