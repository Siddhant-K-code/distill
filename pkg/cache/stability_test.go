package cache

import (
	"fmt"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func stableChunks(systemPrompt string) []types.Chunk {
	return []types.Chunk{
		{
			ID:   "sys",
			Text: systemPrompt,
			Metadata: map[string]interface{}{
				"cache_control": map[string]interface{}{"type": "ephemeral"},
			},
		},
		{ID: "user", Text: "What is 2+2?"},
	}
}

func TestStabilityValidator_StablePrefix(t *testing.T) {
	v := NewStabilityValidator(StabilityConfig{
		WarmupChecks:      2,
		UnstableThreshold: 0.8,
		MaxHashHistory:    10,
		DynamicPatterns:   DefaultStabilityConfig().DynamicPatterns,
	})

	chunks := stableChunks("You are a helpful assistant.")

	// Warmup period — no issues expected.
	for i := 0; i < 5; i++ {
		issues := v.Check("agent.go:42", chunks)
		if len(issues) > 0 {
			t.Errorf("turn %d: expected no issues for stable prefix, got %v", i, issues)
		}
	}

	stats := v.Stats("agent.go:42")
	if stats == nil {
		t.Fatal("expected stats, got nil")
	}
	if stats.StabilityRate() != 1.0 {
		t.Errorf("expected 1.0 stability rate, got %f", stats.StabilityRate())
	}
}

func TestStabilityValidator_UnstablePrefix(t *testing.T) {
	v := NewStabilityValidator(StabilityConfig{
		WarmupChecks:      2,
		UnstableThreshold: 0.8,
		MaxHashHistory:    10,
		DynamicPatterns:   DefaultStabilityConfig().DynamicPatterns,
	})

	// Each call uses a different request ID in the prefix — simulates dynamic content.
	for i := 0; i < 6; i++ {
		chunks := stableChunks(fmt.Sprintf("You are helpful. Request ID: req-%d", i))
		issues := v.Check("reviewer.go:201", chunks)
		if i >= 2 && len(issues) > 0 {
			// Found instability — test passes.
			return
		}
	}
	t.Error("expected instability to be detected after warmup, but no issues were reported")
}

func TestStabilityValidator_WarmupPeriod(t *testing.T) {
	v := NewStabilityValidator(StabilityConfig{
		WarmupChecks:      5,
		UnstableThreshold: 0.5,
		MaxHashHistory:    10,
		DynamicPatterns:   DefaultStabilityConfig().DynamicPatterns,
	})

	// Even with changing prefixes, no issues during warmup.
	for i := 0; i < 4; i++ {
		chunks := stableChunks(fmt.Sprintf("Prompt version %d", i))
		issues := v.Check("planner.go:84", chunks)
		if len(issues) > 0 {
			t.Errorf("turn %d: expected no issues during warmup, got %v", i, issues)
		}
	}
}

func TestStabilityValidator_NoMarkers(t *testing.T) {
	v := NewStabilityValidator(DefaultStabilityConfig())

	// Chunks without cache_control markers — nothing to validate.
	chunks := []types.Chunk{
		{ID: "1", Text: "no marker here"},
		{ID: "2", Text: "also no marker"},
	}
	issues := v.Check("anywhere.go:1", chunks)
	if len(issues) > 0 {
		t.Errorf("expected no issues for chunks without markers, got %v", issues)
	}
}

func TestStabilityValidator_ValidateText_DynamicPatterns(t *testing.T) {
	v := NewStabilityValidator(DefaultStabilityConfig())

	tests := []struct {
		text    string
		wantHit bool
	}{
		{"You are a helpful assistant.", false},
		{"Request ID: abc-123 is included here.", true},
		{"Current timestamp: 2026-05-02T10:00:00Z", true},
		{"User UUID: 550e8400-e29b-41d4-a716-446655440000", true},
		{"Static system prompt with no dynamic fields.", false},
	}

	for _, tt := range tests {
		found := v.ValidateText(tt.text)
		if tt.wantHit && len(found) == 0 {
			t.Errorf("expected dynamic pattern in %q, found none", tt.text)
		}
		if !tt.wantHit && len(found) > 0 {
			t.Errorf("unexpected dynamic pattern in %q: %v", tt.text, found)
		}
	}
}

func TestStabilityValidator_Reset(t *testing.T) {
	v := NewStabilityValidator(DefaultStabilityConfig())
	chunks := stableChunks("static prompt")

	for i := 0; i < 3; i++ {
		v.Check("file.go:1", chunks)
	}
	if v.Stats("file.go:1") == nil {
		t.Fatal("expected stats before reset")
	}

	v.Reset("file.go:1")
	if v.Stats("file.go:1") != nil {
		t.Error("expected nil stats after reset")
	}
}

func TestStabilityValidator_AllStats(t *testing.T) {
	v := NewStabilityValidator(DefaultStabilityConfig())

	v.Check("a.go:1", stableChunks("prompt a"))
	v.Check("b.go:2", stableChunks("prompt b"))

	all := v.AllStats()
	if len(all) != 2 {
		t.Errorf("expected 2 records, got %d", len(all))
	}
}
