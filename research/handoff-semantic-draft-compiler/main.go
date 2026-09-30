package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	requestPath := flag.String("request", "", "canonical Handoff request path")
	expectedSHA256 := flag.String("expected-request-sha256", "", "trusted request SHA-256")
	draftPath := flag.String("draft", "", "raw semantic draft path")
	outputPath := flag.String("out", "", "compiled Handoff proposal path")
	canonicalDraftPath := flag.String("canonical-draft-out", "", "optional canonical semantic draft path")
	flag.Parse()

	if *requestPath == "" || *expectedSHA256 == "" || *draftPath == "" || *outputPath == "" {
		fail("request, expected-request-sha256, draft, and out are required")
	}
	result, err := compileSemanticDraft(*requestPath, *expectedSHA256, *draftPath)
	if err != nil {
		fail(err.Error())
	}
	if err := writeExclusive(*outputPath, result.Proposal); err != nil {
		fail(fmt.Sprintf("write proposal: %v", err))
	}
	if *canonicalDraftPath != "" {
		if err := writeExclusive(*canonicalDraftPath, result.CanonicalDraft); err != nil {
			fail(fmt.Sprintf("write canonical draft: %v", err))
		}
	}
}

func writeExclusive(path string, data []byte) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("output parent is not a directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "semantic-draft compiler:", message)
	os.Exit(1)
}
