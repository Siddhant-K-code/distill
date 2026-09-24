package studylocalv2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedPublicEvidence(t *testing.T) {
	directory := filepath.Join("..", "..", "research", "context-is-a-build-artifact")
	if err := ValidatePublicEvidence(directory); err != nil {
		t.Fatal(err)
	}
	record, err := os.ReadFile(filepath.Join(directory, PublicAggregateFilename))
	if err != nil {
		t.Fatal(err)
	}
	if got := DigestBytes(record); got != KnownPublicAggregateSHA256 {
		t.Fatalf("public aggregate SHA-256 = %s", got)
	}
}

func TestPublicAggregateRejectsPrivateFields(t *testing.T) {
	data := []byte("{\"schema_name\":\"local-context-control-public-aggregate\",\"username\":\"private\"}\n")
	if err := rejectPrivateJSONFields(data); err == nil {
		t.Fatal("private field was accepted")
	}
}

func TestPublicEvidenceRejectsRecordDrift(t *testing.T) {
	source := filepath.Join("..", "..", "research", "context-is-a-build-artifact")
	destination := t.TempDir()
	for _, name := range []string{PublicAggregateFilename, PublicReportFilename, PublicChecksumsFilename} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == PublicAggregateFilename {
			data[0] = '['
		}
		if err := os.WriteFile(filepath.Join(destination, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidatePublicEvidence(destination); err == nil {
		t.Fatal("drifted public record was accepted")
	}
}
