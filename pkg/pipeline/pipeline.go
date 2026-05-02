// Package pipeline chains dedup → compress → summarize into a single pass.
// Each stage is independently configurable and can be disabled.
package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/compress"
	"github.com/Siddhant-K-code/distill/pkg/contextlab"
	"github.com/Siddhant-K-code/distill/pkg/summarize"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// StageStats records token counts and latency for one pipeline stage.
type StageStats struct {
	Enabled      bool
	InputTokens  int
	OutputTokens int
	Reduction    float64 // 0–1
	Latency      time.Duration
}

// Stats aggregates per-stage and overall pipeline statistics.
type Stats struct {
	OriginalTokens int
	FinalTokens    int
	TotalReduction float64
	Stages         map[string]StageStats
	TotalLatency   time.Duration
}

// Options configures which stages run and how.
type Options struct {
	// Dedup stage.
	DedupEnabled   bool
	DedupThreshold float64 // cosine distance threshold (default 0.15)
	DedupLambda    float64 // MMR diversity weight (default 0.7)
	DedupTargetK   int     // max chunks to keep (0 = no limit)

	// Compress stage.
	CompressEnabled         bool
	CompressTargetReduction float64 // e.g. 0.5 = reduce to 50% of tokens

	// Summarize stage.
	SummarizeEnabled   bool
	SummarizeMaxTokens int
	SummarizeRecent    int // turns to preserve at full fidelity
}

// DefaultOptions returns sensible defaults with all stages enabled.
func DefaultOptions() Options {
	return Options{
		DedupEnabled:            true,
		DedupThreshold:          0.15,
		DedupLambda:             0.7,
		CompressEnabled:         true,
		CompressTargetReduction: 0.5,
		SummarizeEnabled:        false, // opt-in; requires turn structure
		SummarizeMaxTokens:      4000,
		SummarizeRecent:         10,
	}
}

// Runner executes the pipeline stages in order.
type Runner struct{}

// New creates a new Runner.
func New() *Runner { return &Runner{} }

// Run executes the configured stages against chunks and returns the result.
func (r *Runner) Run(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error) {
	start := time.Now()
	stats := Stats{
		Stages:         make(map[string]StageStats),
		OriginalTokens: estimateTokens(chunks),
	}

	current := chunks

	// ── Stage 1: Dedup ────────────────────────────────────────────────────────
	dedupStats := StageStats{Enabled: opts.DedupEnabled}
	if opts.DedupEnabled && len(current) > 1 {
		t0 := time.Now()
		dedupStats.InputTokens = estimateTokens(current)

		threshold := opts.DedupThreshold
		if threshold <= 0 {
			threshold = 0.15
		}
		lambda := opts.DedupLambda
		if lambda <= 0 {
			lambda = 0.7
		}

		clusterResult := contextlab.ClusterByThreshold(current, threshold)
		sel := contextlab.NewSelector(contextlab.DefaultSelectorConfig())
		selected := sel.Select(clusterResult)

		if opts.DedupTargetK > 0 && len(selected) > opts.DedupTargetK {
			mmrResult := contextlab.MMRRerank(selected, lambda, opts.DedupTargetK)
			current = mmrResult
		} else {
			current = selected
		}

		dedupStats.OutputTokens = estimateTokens(current)
		dedupStats.Reduction = reduction(dedupStats.InputTokens, dedupStats.OutputTokens)
		dedupStats.Latency = time.Since(t0)
	} else {
		dedupStats.InputTokens = estimateTokens(current)
		dedupStats.OutputTokens = dedupStats.InputTokens
	}
	stats.Stages["dedup"] = dedupStats

	// ── Stage 2: Compress ─────────────────────────────────────────────────────
	compressStats := StageStats{Enabled: opts.CompressEnabled}
	if opts.CompressEnabled && len(current) > 0 {
		t0 := time.Now()
		compressStats.InputTokens = estimateTokens(current)

		compOpts := compress.DefaultOptions()
		if opts.CompressTargetReduction > 0 {
			compOpts.TargetReduction = opts.CompressTargetReduction
		}

		c := compress.NewExtractiveCompressor()
		compressed, _, err := c.Compress(ctx, current, compOpts)
		if err != nil {
			return nil, stats, fmt.Errorf("compress stage: %w", err)
		}
		current = compressed

		compressStats.OutputTokens = estimateTokens(current)
		compressStats.Reduction = reduction(compressStats.InputTokens, compressStats.OutputTokens)
		compressStats.Latency = time.Since(t0)
	} else {
		compressStats.InputTokens = estimateTokens(current)
		compressStats.OutputTokens = compressStats.InputTokens
	}
	stats.Stages["compress"] = compressStats

	// ── Stage 3: Summarize ────────────────────────────────────────────────────
	summarizeStats := StageStats{Enabled: opts.SummarizeEnabled}
	if opts.SummarizeEnabled && len(current) > 0 {
		t0 := time.Now()
		summarizeStats.InputTokens = estimateTokens(current)

		turns := chunksToTurns(current)
		sumOpts := summarize.DefaultOptions()
		sumOpts.MaxTokens = opts.SummarizeMaxTokens
		sumOpts.PreserveRecent = opts.SummarizeRecent

		s := summarize.NewHierarchicalSummarizer()
		summarized, sumStats, err := s.Summarize(ctx, turns, sumOpts)
		if err != nil {
			return nil, stats, fmt.Errorf("summarize stage: %w", err)
		}
		current = turnsToChunks(summarized, current)
		_ = sumStats

		summarizeStats.OutputTokens = estimateTokens(current)
		summarizeStats.Reduction = reduction(summarizeStats.InputTokens, summarizeStats.OutputTokens)
		summarizeStats.Latency = time.Since(t0)
	} else {
		summarizeStats.InputTokens = estimateTokens(current)
		summarizeStats.OutputTokens = summarizeStats.InputTokens
	}
	stats.Stages["summarize"] = summarizeStats

	stats.FinalTokens = estimateTokens(current)
	stats.TotalReduction = reduction(stats.OriginalTokens, stats.FinalTokens)
	stats.TotalLatency = time.Since(start)

	return current, stats, nil
}

// estimateTokens approximates total token count across chunks (4 chars ≈ 1 token).
func estimateTokens(chunks []types.Chunk) int {
	total := 0
	for _, c := range chunks {
		n := 0
		for _, r := range c.Text {
			if r != ' ' && r != '\n' && r != '\t' {
				n++
			}
		}
		total += (n + 3) / 4
	}
	return total
}

// reduction returns the fractional reduction from in to out (0–1).
func reduction(in, out int) float64 {
	if in == 0 {
		return 0
	}
	r := float64(in-out) / float64(in)
	if r < 0 {
		return 0
	}
	return r
}

// chunksToTurns converts chunks to summarize.Turn for the summarize stage.
func chunksToTurns(chunks []types.Chunk) []summarize.Turn {
	turns := make([]summarize.Turn, len(chunks))
	for i, c := range chunks {
		turns[i] = summarize.Turn{
			ID:      c.ID,
			Role:    "user",
			Content: c.Text,
		}
	}
	return turns
}

// turnsToChunks maps summarized turns back to chunks, preserving metadata.
func turnsToChunks(turns []summarize.Turn, original []types.Chunk) []types.Chunk {
	byID := make(map[string]types.Chunk, len(original))
	for _, c := range original {
		byID[c.ID] = c
	}
	out := make([]types.Chunk, 0, len(turns))
	for _, t := range turns {
		c, ok := byID[t.ID]
		if !ok {
			c = types.Chunk{ID: t.ID}
		}
		c.Text = t.Content
		out = append(out, c)
	}
	return out
}
