package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Siddhant-K-code/distill/internal/studyfinal"
)

func TestOfflineCommands(t *testing.T) {
	root, err := os.MkdirTemp(".", ".final-cli-local-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	destination := filepath.Join(root, "package")
	if err := run([]string{"prepare", "--output", destination}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "--input", destination}); err != nil {
		t.Fatal(err)
	}
	corpus, _, err := studyfinal.ValidatePackage(destination)
	if err != nil || !corpus.Executable {
		t.Fatalf("verified fixture validation failed: executable=%v err=%v", corpus.Executable, err)
	}
	if err := run([]string{"run"}); err == nil {
		t.Fatal("real transport scaffold did not fail closed")
	}
	noGoPath := filepath.Join(root, "no-go.json")
	noGo := studyfinal.GateAttestation{
		SchemaVersion: studyfinal.SchemaVersion + "/gate-attestation",
		Status:        "denied", DenyReason: "current external gate record is NO-GO",
	}
	noGoBytes, _ := json.Marshal(noGo)
	if err := os.WriteFile(noGoPath, noGoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"authorize", "--input", destination, "--output", filepath.Join(root, "authorization.json"),
		"--analysis-sha256", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--expires", "2099-01-01T00:00:00Z", "--attestation", noGoPath,
		"--ledger", filepath.Join(root, "missing-ledger.jsonl"),
	}); err == nil {
		t.Fatal("CLI authorized current NO-GO attestation")
	}
	if err := run([]string{"run", "--fake"}); err != nil {
		t.Fatal(err)
	}
}

func TestSummarize(t *testing.T) {
	root, err := os.MkdirTemp(".", ".final-summary-local-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	destination := filepath.Join(root, "package")
	if err := run([]string{"prepare", "--output", destination}); err != nil {
		t.Fatal(err)
	}
	_, schedule, err := studyfinal.ValidatePackage(destination)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	err = run([]string{"summarize", "--input", destination})
	_ = write.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	output, _ := io.ReadAll(read)
	_ = read.Close()
	if !bytes.Contains(output, []byte(`"scheduled_calls":420`)) || !bytes.Contains(output, []byte(`"conditions":150`)) {
		t.Fatalf("unexpected summary: %s", output)
	}
	for _, field := range []string{"corpus_sha256", "split_sha256", "schedule_sha256", "question_schema_sha256", "receipt_registry_sha256", "execution_status", "agenttrace_contamination_status"} {
		if !bytes.Contains(output, []byte(`"`+field+`"`)) {
			t.Fatalf("summary omitted %s: %s", field, output)
		}
	}
	if !bytes.Contains(output, []byte(`"agenttrace_contamination_status":"downgraded-confirmatory-secondary"`)) ||
		!bytes.Contains(output, []byte(`"execution_status":"no-go-without-signed-passed-attestation"`)) {
		t.Fatalf("summary reports inconsistent execution state: %s", output)
	}
	var summary map[string]any
	if err := json.Unmarshal(output, &summary); err != nil {
		t.Fatal(err)
	}
	scheduleSHA256, err := studyfinal.DigestDomain("run-schedule", schedule)
	if err != nil || summary["schedule_sha256"] != scheduleSHA256 {
		t.Fatalf("summary schedule digest is outside frozen run-schedule domain: got=%v want=%s err=%v", summary["schedule_sha256"], scheduleSHA256, err)
	}
}
