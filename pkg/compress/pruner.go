package compress

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Pruner removes low-importance tokens and filler phrases.
type Pruner struct {
	// FillerPhrases are phrases to remove entirely.
	FillerPhrases []string

	// RedundantPatterns are regex patterns for redundant content.
	RedundantPatterns []*regexp.Regexp

	// CollapseWhitespace normalizes multiple spaces to single space.
	CollapseWhitespace bool
}

// NewPruner creates a pruner with default filler phrases.
func NewPruner() *Pruner {
	return &Pruner{
		FillerPhrases: []string{
			"as mentioned earlier",
			"as we discussed",
			"it is important to note that",
			"it should be noted that",
			"please note that",
			"in order to",
			"for the purpose of",
			"at this point in time",
			"at the present time",
			"in the event that",
			"due to the fact that",
			"in light of the fact that",
			"it goes without saying",
			"needless to say",
			"as a matter of fact",
			"in actual fact",
			"basically",
			"essentially",
			"fundamentally",
			"literally",
			"actually",
			"obviously",
			"clearly",
			"of course",
			"as you know",
			"as you can see",
			"it is worth mentioning",
			"i would like to point out",
			"let me explain",
			"allow me to",
		},
		RedundantPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\s+`),                          // Multiple whitespace
			regexp.MustCompile(`\.{2,}`),                       // Multiple periods
			regexp.MustCompile(`\n{3,}`),                       // Multiple newlines
			regexp.MustCompile(`(?i)\b(very|really|quite)\s+`), // Intensifiers
		},
		CollapseWhitespace: true,
	}
}

// Compress removes filler phrases and redundant patterns.
func (p *Pruner) Compress(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error) {
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

		pruned := p.prune(chunk.Text)
		stats.ChunksProcessed++
		stats.OutputTokens += estimateTokens(pruned)

		newChunk := chunk.Clone()
		newChunk.Text = pruned
		result = append(result, *newChunk)
	}

	stats.Latency = time.Since(start)
	if stats.InputTokens > 0 {
		stats.ReductionPercent = float64(stats.InputTokens-stats.OutputTokens) / float64(stats.InputTokens) * 100
	}

	return result, stats, nil
}

// prune removes filler phrases and applies pattern-based cleanup.
func (p *Pruner) prune(text string) string {
	result := text

	// Remove filler phrases (case-insensitive)
	for _, phrase := range p.FillerPhrases {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(phrase) + `\b[,]?\s*`)
		result = pattern.ReplaceAllString(result, "")
	}

	// Apply redundant patterns
	for _, pattern := range p.RedundantPatterns {
		if p.CollapseWhitespace && pattern.String() == `\s+` {
			result = pattern.ReplaceAllString(result, " ")
		} else if pattern.String() == `\n{3,}` {
			result = pattern.ReplaceAllString(result, "\n\n")
		} else if pattern.String() == `\.{2,}` {
			result = pattern.ReplaceAllString(result, ".")
		} else {
			result = pattern.ReplaceAllString(result, "")
		}
	}

	// Clean up any double spaces created by removals
	result = regexp.MustCompile(`\s{2,}`).ReplaceAllString(result, " ")

	// Fix punctuation spacing
	result = regexp.MustCompile(`\s+([.,;:!?])`).ReplaceAllString(result, "$1")

	// Trim
	result = strings.TrimSpace(result)

	return result
}

// AddFillerPhrase adds a custom filler phrase to remove.
func (p *Pruner) AddFillerPhrase(phrase string) {
	p.FillerPhrases = append(p.FillerPhrases, phrase)
}

// AddRedundantPattern adds a custom regex pattern for removal.
func (p *Pruner) AddRedundantPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	p.RedundantPatterns = append(p.RedundantPatterns, re)
	return nil
}
