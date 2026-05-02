package pipeline

import (
	"context"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func makeChunk(id, text string) types.Chunk {
	return types.Chunk{ID: id, Text: text}
}

func TestRun_AllStagesDisabled(t *testing.T) {
	r := New()
	ctx := context.Background()
	chunks := []types.Chunk{makeChunk("a", "hello world"), makeChunk("b", "foo bar")}
	opts := Options{} // all disabled

	result, stats, err := r.Run(ctx, chunks, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("want 2 chunks, got %d", len(result))
	}
	if stats.TotalReduction != 0 {
		t.Errorf("want 0 reduction with all stages off, got %.2f", stats.TotalReduction)
	}
}

func TestRun_DedupOnly(t *testing.T) {
	r := New()
	ctx := context.Background()

	// Two near-identical chunks (no embeddings → cluster by text similarity won't fire,
	// but dedup stage should still run without error).
	chunks := []types.Chunk{
		makeChunk("a", "The quick brown fox jumps over the lazy dog"),
		makeChunk("b", "A completely different sentence about something else entirely"),
		makeChunk("c", "The quick brown fox jumps over the lazy dog"), // duplicate
	}
	opts := Options{DedupEnabled: true, DedupThreshold: 0.15}

	result, stats, err := r.Run(ctx, chunks, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected at least one chunk after dedup")
	}
	if !stats.Stages["dedup"].Enabled {
		t.Error("dedup stage should be marked enabled")
	}
}

func TestRun_CompressOnly(t *testing.T) {
	r := New()
	ctx := context.Background()
	long := "This is a long sentence. It has multiple parts. Each part adds tokens. More content here. Even more."
	chunks := []types.Chunk{makeChunk("a", long)}
	opts := Options{CompressEnabled: true, CompressTargetReduction: 0.5}

	result, stats, err := r.Run(ctx, chunks, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected at least one chunk after compress")
	}
	if !stats.Stages["compress"].Enabled {
		t.Error("compress stage should be marked enabled")
	}
}

func TestRun_SummarizeEnabled(t *testing.T) {
	r := New()
	ctx := context.Background()
	chunks := []types.Chunk{
		makeChunk("a", "This is a detailed message about the system architecture and design decisions."),
		makeChunk("b", "Another message about implementation details and code structure."),
	}
	opts := Options{
		SummarizeEnabled:   true,
		SummarizeMaxTokens: 10,
		SummarizeRecent:    0,
	}

	result, stats, err := r.Run(ctx, chunks, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !stats.Stages["summarize"].Enabled {
		t.Error("summarize stage should be marked enabled")
	}
	_ = result
}

func TestRun_DefaultOptions(t *testing.T) {
	r := New()
	ctx := context.Background()
	chunks := []types.Chunk{
		makeChunk("a", "First chunk with some content about the topic."),
		makeChunk("b", "Second chunk with different content about another topic."),
	}

	result, stats, err := r.Run(ctx, chunks, DefaultOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	if stats.TotalLatency == 0 {
		t.Error("expected non-zero latency")
	}
}

func TestRun_EmptyInput(t *testing.T) {
	r := New()
	ctx := context.Background()
	result, stats, err := r.Run(ctx, nil, DefaultOptions())
	if err != nil {
		t.Fatalf("Run with empty input: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d chunks", len(result))
	}
	if stats.OriginalTokens != 0 {
		t.Errorf("expected 0 original tokens, got %d", stats.OriginalTokens)
	}
}

func TestStats_StagesPresent(t *testing.T) {
	r := New()
	ctx := context.Background()
	chunks := []types.Chunk{makeChunk("a", "hello")}
	_, stats, err := r.Run(ctx, chunks, DefaultOptions())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, stage := range []string{"dedup", "compress", "summarize"} {
		if _, ok := stats.Stages[stage]; !ok {
			t.Errorf("missing stage %q in stats", stage)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	chunks := []types.Chunk{{Text: "hello world"}}
	n := estimateTokens(chunks)
	if n == 0 {
		t.Error("expected non-zero token estimate")
	}
}

func TestReduction(t *testing.T) {
	if r := reduction(100, 50); r != 0.5 {
		t.Errorf("want 0.5, got %.2f", r)
	}
	if r := reduction(0, 0); r != 0 {
		t.Errorf("want 0 for zero input, got %.2f", r)
	}
	if r := reduction(100, 150); r != 0 {
		t.Errorf("want 0 for negative reduction, got %.2f", r)
	}
}
