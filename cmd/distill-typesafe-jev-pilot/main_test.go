package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRejectsMissingArgumentsAndUnknownCommands(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"authorize"},
		{"run"},
		{"validate"},
		{"unknown"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("arguments unexpectedly accepted: %v", args)
		}
	}
}

func TestRunDoesNotAcceptCredentialFlag(t *testing.T) {
	err := run([]string{"run", "--pilot", "pilot", "--run-dir", "run", "--api-key", "secret"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("credential flag unexpectedly accepted: %v", err)
	}
}
