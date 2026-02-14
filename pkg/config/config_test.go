package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Dedup.Threshold != 0.15 {
		t.Errorf("expected default threshold 0.15, got %f", cfg.Dedup.Threshold)
	}
	if cfg.Dedup.Linkage != "average" {
		t.Errorf("expected default linkage average, got %s", cfg.Dedup.Linkage)
	}
	if cfg.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("expected default model text-embedding-3-small, got %s", cfg.Embedding.Model)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := Validate(cfg); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 70000
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestValidate_InvalidThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dedup.Threshold = 1.5
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for threshold > 1")
	}

	cfg.Dedup.Threshold = -0.1
	err = Validate(cfg)
	if err == nil {
		t.Error("expected error for negative threshold")
	}
}

func TestValidate_InvalidLambda(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dedup.Lambda = 2.0
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for lambda > 1")
	}
}

func TestValidate_InvalidBackend(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Retriever.Backend = "elasticsearch"
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for unsupported backend")
	}
}

func TestValidate_InvalidLinkage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dedup.Linkage = "ward"
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for unsupported linkage")
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = -1
	cfg.Dedup.Threshold = 5.0
	cfg.Dedup.Lambda = -1.0
	err := Validate(cfg)
	if err == nil {
		t.Error("expected multiple validation errors")
	}
}

func TestInterpolateEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")

	tests := []struct {
		input    string
		expected string
	}{
		{"${TEST_VAR}", "hello"},
		{"prefix-${TEST_VAR}-suffix", "prefix-hello-suffix"},
		{"${NONEXISTENT_VAR:-fallback}", "fallback"},
		{"${NONEXISTENT_VAR}", "${NONEXISTENT_VAR}"},
		{"no-vars-here", "no-vars-here"},
		{"${TEST_VAR:-default}", "hello"}, // env var exists, ignore default
	}

	for _, tt := range tests {
		result := InterpolateEnv(tt.input)
		if result != tt.expected {
			t.Errorf("InterpolateEnv(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `
server:
  port: 9090
  host: 127.0.0.1

dedup:
  threshold: 0.20
  linkage: complete
  lambda: 0.7
  enable_mmr: false

retriever:
  backend: qdrant
  index: test-collection
  host: localhost:6334
  top_k: 100
  target_k: 10
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "distill.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Dedup.Threshold != 0.20 {
		t.Errorf("expected threshold 0.20, got %f", cfg.Dedup.Threshold)
	}
	if cfg.Dedup.Linkage != "complete" {
		t.Errorf("expected linkage complete, got %s", cfg.Dedup.Linkage)
	}
	if cfg.Dedup.Lambda != 0.7 {
		t.Errorf("expected lambda 0.7, got %f", cfg.Dedup.Lambda)
	}
	if cfg.Dedup.EnableMMR {
		t.Error("expected enable_mmr false")
	}
	if cfg.Retriever.Backend != "qdrant" {
		t.Errorf("expected backend qdrant, got %s", cfg.Retriever.Backend)
	}
	if cfg.Retriever.Index != "test-collection" {
		t.Errorf("expected index test-collection, got %s", cfg.Retriever.Index)
	}
	if cfg.Retriever.TopK != 100 {
		t.Errorf("expected top_k 100, got %d", cfg.Retriever.TopK)
	}
}

func TestLoadFromFile_WithEnvInterpolation(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-test-123")

	content := `
auth:
  api_keys:
    - ${TEST_API_KEY}
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "distill.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if len(cfg.Auth.APIKeys) != 1 {
		t.Fatalf("expected 1 API key, got %d", len(cfg.Auth.APIKeys))
	}
	if cfg.Auth.APIKeys[0] != "sk-test-123" {
		t.Errorf("expected interpolated API key, got %s", cfg.Auth.APIKeys[0])
	}
}

func TestLoadFromFile_InvalidFile(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/distill.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "distill.yaml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadFromFile(cfgPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadFromFile_InvalidValues(t *testing.T) {
	content := `
server:
  port: 99999
dedup:
  threshold: 5.0
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "distill.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadFromFile(cfgPath)
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestLoadFromFile_DefaultsPreserved(t *testing.T) {
	// Partial config should preserve defaults for unset fields
	content := `
server:
  port: 3000
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "distill.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	// Defaults should be preserved for unset fields
	if cfg.Dedup.Threshold != 0.15 {
		t.Errorf("expected default threshold 0.15, got %f", cfg.Dedup.Threshold)
	}
	if cfg.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("expected default model, got %s", cfg.Embedding.Model)
	}
}

func TestGenerateTemplate(t *testing.T) {
	tmpl := GenerateTemplate()

	// Verify key sections exist
	required := []string{
		"server:", "port:", "host:",
		"embedding:", "provider:", "model:",
		"dedup:", "threshold:", "linkage:", "lambda:",
		"retriever:", "backend:", "index:",
		"auth:", "api_keys:",
	}

	for _, s := range required {
		if !strings.Contains(tmpl, s) {
			t.Errorf("template missing %q", s)
		}
	}
}
