package main

import (
	"bytes"
	"testing"
)

func TestRejectsProviderAndCredentialFlags(t *testing.T) {
	for _, arguments := range [][]string{
		{"prepare", "--output", "build/pilot", "--model", "anything"},
		{"prepare", "--output", "build/pilot", "--provider", "anything"},
		{"prepare", "--output", "build/pilot", "--api-key", "anything"},
		{"prepare", "--output", "build/pilot", "--network"},
	} {
		if err := run(arguments, &bytes.Buffer{}); err == nil {
			t.Fatalf("unsupported arguments unexpectedly accepted: %v", arguments)
		}
	}
}
