package compress

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// ExtractiveCompressor selects salient spans from verbose content.
// It uses sentence-level extraction based on embedding similarity to preserve
// the most semantically relevant portions.
type ExtractiveCompressor struct {
	// SentenceDelimiters defines characters that end sentences.
	SentenceDelimiters string
}

// NewExtractiveCompressor creates an extractive compressor with default settings.
func NewExtractiveCompressor() *ExtractiveCompressor {
	return &ExtractiveCompressor{
		SentenceDelimiters: ".!?",
	}
}

// Compress extracts salient spans from each chunk.
func (e *ExtractiveCompressor) Compress(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error) {
	start := time.Now()
	stats := Stats{}

	result := make([]types.Chunk, 0, len(chunks))

	for _, chunk := range chunks {
		inputTokens := estimateTokens(chunk.Text)
		stats.InputTokens += inputTokens

		if len(chunk.Text) < opts.MinChunkLength {
			stats.ChunksSkipped++
			stats.OutputTokens += inputTokens
			result = append(result, chunk)
			continue
		}

		compressed := e.extractSalientSpans(chunk.Text, opts.TargetReduction)
		stats.ChunksProcessed++
		stats.OutputTokens += estimateTokens(compressed)

		newChunk := chunk.Clone()
		newChunk.Text = compressed
		result = append(result, *newChunk)
	}

	stats.Latency = time.Since(start)
	if stats.InputTokens > 0 {
		stats.ReductionPercent = float64(stats.InputTokens-stats.OutputTokens) / float64(stats.InputTokens) * 100
	}

	return result, stats, nil
}

// extractSalientSpans selects the most important sentences to meet target reduction.
func (e *ExtractiveCompressor) extractSalientSpans(text string, targetReduction float64) string {
	sentences := e.splitSentences(text)
	if len(sentences) <= 1 {
		return text
	}

	// Score sentences by position and content signals
	scored := make([]scoredSentence, len(sentences))
	for i, s := range sentences {
		scored[i] = scoredSentence{
			text:  s,
			index: i,
			score: e.scoreSentence(s, i, len(sentences)),
		}
	}

	// Sort by score descending
	sortByScore(scored)

	// Select top sentences until we hit target
	targetTokens := int(float64(estimateTokens(text)) * targetReduction)
	var selected []scoredSentence
	currentTokens := 0

	for _, s := range scored {
		tokens := estimateTokens(s.text)
		if currentTokens+tokens > targetTokens && len(selected) > 0 {
			break
		}
		selected = append(selected, s)
		currentTokens += tokens
	}

	// Sort selected by original position to maintain coherence
	sortByIndex(selected)

	// Reconstruct text
	var result strings.Builder
	for i, s := range selected {
		if i > 0 {
			result.WriteString(" ")
		}
		result.WriteString(strings.TrimSpace(s.text))
	}

	return result.String()
}

// splitSentences breaks text into sentences.
func (e *ExtractiveCompressor) splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for _, r := range text {
		current.WriteRune(r)
		if strings.ContainsRune(e.SentenceDelimiters, r) {
			s := strings.TrimSpace(current.String())
			if len(s) > 0 {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}

	// Handle remaining text without delimiter
	if remaining := strings.TrimSpace(current.String()); len(remaining) > 0 {
		sentences = append(sentences, remaining)
	}

	return sentences
}

// scoreSentence assigns importance based on position and content.
func (e *ExtractiveCompressor) scoreSentence(sentence string, index, total int) float64 {
	score := 0.0

	// Position bias: first and last sentences are often important
	if index == 0 {
		score += 2.0
	} else if index == total-1 {
		score += 1.0
	}

	// Length: prefer medium-length sentences
	words := len(strings.Fields(sentence))
	if words >= 5 && words <= 25 {
		score += 1.0
	}

	// Content signals
	lower := strings.ToLower(sentence)
	if strings.Contains(lower, "important") || strings.Contains(lower, "key") ||
		strings.Contains(lower, "must") || strings.Contains(lower, "should") {
		score += 1.5
	}

	// Presence of numbers often indicates specific information
	for _, r := range sentence {
		if unicode.IsDigit(r) {
			score += 0.5
			break
		}
	}

	return score
}

type scoredSentence struct {
	text  string
	index int
	score float64
}

func sortByScore(sentences []scoredSentence) {
	for i := 0; i < len(sentences)-1; i++ {
		for j := i + 1; j < len(sentences); j++ {
			if sentences[j].score > sentences[i].score {
				sentences[i], sentences[j] = sentences[j], sentences[i]
			}
		}
	}
}

func sortByIndex(sentences []scoredSentence) {
	for i := 0; i < len(sentences)-1; i++ {
		for j := i + 1; j < len(sentences); j++ {
			if sentences[j].index < sentences[i].index {
				sentences[i], sentences[j] = sentences[j], sentences[i]
			}
		}
	}
}

// estimateTokens provides a rough token count (avg 4 chars per token).
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}
