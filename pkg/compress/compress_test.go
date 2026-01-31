package compress

import (
	"context"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func TestExtractiveCompressor(t *testing.T) {
	compressor := NewExtractiveCompressor()
	ctx := context.Background()

	tests := []struct {
		name            string
		input           string
		targetReduction float64
		wantShorter     bool
	}{
		{
			name: "multi-sentence text",
			input: "This is the first sentence. This is the second sentence. " +
				"This is the third sentence. This is the fourth sentence. " +
				"This is the fifth sentence with important information.",
			targetReduction: 0.5,
			wantShorter:     true,
		},
		{
			name:            "single sentence",
			input:           "This is a single sentence.",
			targetReduction: 0.5,
			wantShorter:     false,
		},
		{
			name:            "empty text",
			input:           "",
			targetReduction: 0.5,
			wantShorter:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := []types.Chunk{{ID: "1", Text: tt.input}}
			opts := Options{
				TargetReduction: tt.targetReduction,
				MinChunkLength:  10,
			}

			result, stats, err := compressor.Compress(ctx, chunks, opts)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			if len(result) != 1 {
				t.Fatalf("expected 1 chunk, got %d", len(result))
			}

			if tt.wantShorter && len(result[0].Text) >= len(tt.input) {
				t.Errorf("expected shorter output, got %d >= %d", len(result[0].Text), len(tt.input))
			}

			if stats.InputTokens == 0 && len(tt.input) > 0 {
				t.Error("expected non-zero input tokens")
			}
		})
	}
}

func TestPlaceholderCompressor(t *testing.T) {
	compressor := NewPlaceholderCompressor()
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		wantContain string
	}{
		{
			name:        "JSON object",
			input:       `{"id": 1, "name": "test", "data": {"nested": "value"}, "items": [1,2,3,4,5,6,7,8,9,10]}`,
			wantContain: "id",
		},
		{
			name:        "JSON array",
			input:       `[{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5}]`,
			wantContain: "more",
		},
		{
			name:        "plain text",
			input:       "This is just plain text without any structure.",
			wantContain: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := []types.Chunk{{ID: "1", Text: tt.input}}
			opts := Options{
				PreserveStructure: true,
				MinChunkLength:    10,
			}

			result, _, err := compressor.Compress(ctx, chunks, opts)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			if len(result) != 1 {
				t.Fatalf("expected 1 chunk, got %d", len(result))
			}

			if tt.wantContain != "" && !contains(result[0].Text, tt.wantContain) {
				t.Errorf("expected output to contain %q, got %q", tt.wantContain, result[0].Text)
			}
		})
	}
}

func TestPruner(t *testing.T) {
	pruner := NewPruner()
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		wantShorter bool
		notContain  string
	}{
		{
			name:        "filler phrases",
			input:       "As mentioned earlier, it is important to note that the system works. Basically, it just runs.",
			wantShorter: true,
			notContain:  "as mentioned earlier",
		},
		{
			name:        "intensifiers",
			input:       "This is a very important and really critical feature that is quite useful.",
			wantShorter: true,
			notContain:  "very",
		},
		{
			name:        "multiple whitespace",
			input:       "This   has    multiple     spaces.",
			wantShorter: true,
			notContain:  "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := []types.Chunk{{ID: "1", Text: tt.input}}
			opts := Options{MinChunkLength: 10}

			result, _, err := pruner.Compress(ctx, chunks, opts)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			if len(result) != 1 {
				t.Fatalf("expected 1 chunk, got %d", len(result))
			}

			if tt.wantShorter && len(result[0].Text) >= len(tt.input) {
				t.Errorf("expected shorter output, got %d >= %d", len(result[0].Text), len(tt.input))
			}

			if tt.notContain != "" && containsLower(result[0].Text, tt.notContain) {
				t.Errorf("output should not contain %q, got %q", tt.notContain, result[0].Text)
			}
		})
	}
}

func TestPipeline(t *testing.T) {
	pipeline := NewPipeline(
		NewPruner(),
		NewExtractiveCompressor(),
	)
	ctx := context.Background()

	input := "As mentioned earlier, this is the first important sentence. " +
		"Basically, this is the second sentence. " +
		"It is important to note that this is the third sentence. " +
		"Obviously, this is the fourth sentence. " +
		"This is the fifth sentence with key information."

	chunks := []types.Chunk{{ID: "1", Text: input}}
	opts := Options{
		TargetReduction: 0.5,
		MinChunkLength:  10,
	}

	result, stats, err := pipeline.Compress(ctx, chunks, opts)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}

	if len(result[0].Text) >= len(input) {
		t.Errorf("expected shorter output from pipeline, got %d >= %d", len(result[0].Text), len(input))
	}

	if stats.Latency == 0 {
		t.Error("expected non-zero latency")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"test", 1},
		{"hello world", 3},
		{"this is a longer sentence", 7},
	}

	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.TargetReduction != 0.5 {
		t.Errorf("expected TargetReduction 0.5, got %f", opts.TargetReduction)
	}

	if opts.Mode != ModeHybrid {
		t.Errorf("expected Mode Hybrid, got %s", opts.Mode)
	}

	if !opts.PreserveStructure {
		t.Error("expected PreserveStructure true")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsLower(s, substr string) bool {
	return contains(toLower(s), toLower(substr))
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}
