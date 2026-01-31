// Package compress provides semantic compression for LLM context.
// It reduces token count while preserving semantic meaning.
package compress

import (
	"context"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Mode specifies the compression strategy.
type Mode string

const (
	// ModeExtractive selects salient spans from content.
	ModeExtractive Mode = "extractive"
	// ModePlaceholder replaces verbose outputs with compact summaries.
	ModePlaceholder Mode = "placeholder"
	// ModeHybrid combines extractive and placeholder strategies.
	ModeHybrid Mode = "hybrid"
)

// Options configures compression behavior.
type Options struct {
	// TargetReduction is the desired reduction ratio (e.g., 0.3 = reduce to 30% of original).
	TargetReduction float64

	// PreserveStructure keeps JSON/code structure intact when possible.
	PreserveStructure bool

	// Mode selects the compression strategy.
	Mode Mode

	// MinChunkLength is the minimum length to consider for compression.
	MinChunkLength int

	// MaxOutputTokens caps the total output tokens (0 = no limit).
	MaxOutputTokens int
}

// DefaultOptions returns sensible defaults for compression.
func DefaultOptions() Options {
	return Options{
		TargetReduction:   0.5,
		PreserveStructure: true,
		Mode:              ModeHybrid,
		MinChunkLength:    50,
		MaxOutputTokens:   0,
	}
}

// Stats tracks compression metrics.
type Stats struct {
	// InputTokens is the estimated token count before compression.
	InputTokens int

	// OutputTokens is the estimated token count after compression.
	OutputTokens int

	// ReductionPercent is the percentage of tokens removed.
	ReductionPercent float64

	// ChunksProcessed is the number of chunks that were compressed.
	ChunksProcessed int

	// ChunksSkipped is the number of chunks below MinChunkLength.
	ChunksSkipped int

	// Latency is the compression processing time.
	Latency time.Duration
}

// Compressor defines the interface for semantic compression.
type Compressor interface {
	// Compress reduces the token count of chunks while preserving meaning.
	Compress(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error)
}

// Result holds compressed chunks and statistics.
type Result struct {
	// Chunks are the compressed chunks.
	Chunks []types.Chunk

	// Stats contains compression metrics.
	Stats Stats
}

// Pipeline chains multiple compression strategies.
type Pipeline struct {
	compressors []Compressor
}

// NewPipeline creates a compression pipeline with the given strategies.
func NewPipeline(compressors ...Compressor) *Pipeline {
	return &Pipeline{compressors: compressors}
}

// Compress applies all compressors in sequence.
func (p *Pipeline) Compress(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error) {
	start := time.Now()
	result := chunks
	var totalStats Stats

	for _, c := range p.compressors {
		compressed, stats, err := c.Compress(ctx, result, opts)
		if err != nil {
			return nil, Stats{}, err
		}
		result = compressed
		totalStats.InputTokens = stats.InputTokens
		totalStats.OutputTokens = stats.OutputTokens
		totalStats.ChunksProcessed += stats.ChunksProcessed
		totalStats.ChunksSkipped += stats.ChunksSkipped
	}

	totalStats.Latency = time.Since(start)
	if totalStats.InputTokens > 0 {
		totalStats.ReductionPercent = float64(totalStats.InputTokens-totalStats.OutputTokens) / float64(totalStats.InputTokens) * 100
	}

	return result, totalStats, nil
}
