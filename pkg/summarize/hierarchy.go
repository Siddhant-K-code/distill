package summarize

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// HierarchicalSummarizer implements Summarizer using rule-based compression.
// It does not require an LLM — compression is performed locally using
// extractive techniques (sentence selection, keyword extraction).
//
// For LLM-backed summarization, wrap this with an LLMSummarizer that
// overrides the compress method.
type HierarchicalSummarizer struct{}

// NewHierarchicalSummarizer creates a new summarizer.
func NewHierarchicalSummarizer() *HierarchicalSummarizer {
	return &HierarchicalSummarizer{}
}

// Summarize compresses turns to fit within opts.MaxTokens.
// Turns are processed oldest-first; recent turns and high-importance turns
// are preserved at full fidelity.
func (s *HierarchicalSummarizer) Summarize(
	ctx context.Context,
	turns []Turn,
	opts SummarizeOptions,
) ([]Turn, SummarizeStats, error) {
	start := time.Now()

	if opts.PreserveRecent < 0 {
		opts.PreserveRecent = 10
	}
	if opts.ImportanceThreshold <= 0 {
		opts.ImportanceThreshold = 0.7
	}
	if len(opts.AgeLevels) == 0 {
		opts.AgeLevels = DefaultOptions().AgeLevels
	}

	// Score importance for turns that don't have it set.
	ScoreTurns(turns)

	// Count input tokens.
	inputTokens := 0
	for i := range turns {
		turns[i].TokenCount = estimateTokens(turns[i].Content)
		inputTokens += turns[i].TokenCount
	}

	stats := SummarizeStats{
		InputTurns:  len(turns),
		InputTokens: inputTokens,
	}

	// Determine which turns to compress.
	now := time.Now()
	result := make([]Turn, len(turns))
	copy(result, turns)

	recentCutoff := len(result) - opts.PreserveRecent
	if recentCutoff < 0 {
		recentCutoff = 0
	}

	for i := range result {
		t := &result[i]

		// Always preserve recent turns (only when PreserveRecent > 0).
		if opts.PreserveRecent > 0 && i >= recentCutoff {
			stats.PreservedTurns++
			continue
		}

		// Preserve high-importance turns at LevelFull or LevelParagraph.
		maxLevel := s.maxLevelForAge(now.Sub(t.Timestamp), opts.AgeLevels)
		if t.Importance >= opts.ImportanceThreshold && maxLevel > LevelParagraph {
			maxLevel = LevelParagraph
		}

		if maxLevel <= t.Level {
			// Already at or beyond target level.
			stats.PreservedTurns++
			continue
		}

		// Compress to target level.
		if err := s.compressTo(t, maxLevel); err != nil {
			return nil, stats, fmt.Errorf("compress turn %s: %w", t.ID, err)
		}
		t.TokenCount = estimateTokens(t.Content)
		stats.CompressedTurns++
	}

	// If MaxTokens is set and we're still over budget, do a second pass
	// compressing more aggressively from oldest to newest.
	if opts.MaxTokens > 0 {
		result = s.enforceTokenBudget(result, opts, recentCutoff)
	}

	// Compute output stats.
	outputTokens := 0
	for _, t := range result {
		outputTokens += t.TokenCount
	}
	stats.OutputTurns = len(result)
	stats.OutputTokens = outputTokens
	if stats.InputTokens > 0 {
		stats.ReductionPct = float64(stats.InputTokens-stats.OutputTokens) / float64(stats.InputTokens) * 100
	}
	stats.Latency = time.Since(start)

	return result, stats, nil
}

// enforceTokenBudget does a second compression pass when still over budget.
// It progressively compresses oldest turns through all levels, including
// eviction (dropping turns entirely) as a last resort.
func (s *HierarchicalSummarizer) enforceTokenBudget(
	turns []Turn,
	opts SummarizeOptions,
	recentCutoff int,
) []Turn {
	total := 0
	for _, t := range turns {
		total += t.TokenCount
	}
	if total <= opts.MaxTokens {
		return turns
	}

	// Compress oldest non-recent turns progressively through all levels.
	for level := LevelParagraph; level <= LevelEvicted && total > opts.MaxTokens; level++ {
		for i := range turns {
			if opts.PreserveRecent > 0 && i >= recentCutoff {
				break
			}
			t := &turns[i]
			if t.Level >= level {
				continue
			}
			if t.Importance >= opts.ImportanceThreshold && level > LevelParagraph {
				continue
			}
			before := t.TokenCount
			if level == LevelEvicted {
				t.Level = LevelEvicted
				t.Content = ""
				t.TokenCount = 0
			} else {
				_ = s.compressTo(t, level)
				t.TokenCount = estimateTokens(t.Content)
			}
			total -= before - t.TokenCount
			if total <= opts.MaxTokens {
				break
			}
		}
	}

	// Remove evicted turns from the slice.
	out := turns[:0]
	for _, t := range turns {
		if t.Level != LevelEvicted {
			out = append(out, t)
		}
	}
	return out
}

// maxLevelForAge returns the maximum compression level for a given age.
func (s *HierarchicalSummarizer) maxLevelForAge(age time.Duration, levels []AgeLevel) Level {
	max := LevelFull
	for _, al := range levels {
		if age >= al.After && al.MaxLevel > max {
			max = al.MaxLevel
		}
	}
	return max
}

// compressTo compresses a turn to the target level in-place.
// The original content is preserved in Turn.Original on first compression.
func (s *HierarchicalSummarizer) compressTo(t *Turn, target Level) error {
	if t.Original == "" {
		t.Original = t.Content
	}

	switch target {
	case LevelParagraph:
		t.Content = extractParagraphSummary(t.Original)
	case LevelSentence:
		t.Content = extractSentenceSummary(t.Original)
	case LevelKeywords:
		t.Content = extractKeywordSummary(t.Original)
	}
	t.Level = target
	return nil
}

// extractParagraphSummary keeps the first paragraph and any code blocks.
func extractParagraphSummary(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	inCode := false
	paragraphDone := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			out = append(out, line)
			continue
		}
		if inCode {
			out = append(out, line)
			continue
		}
		if !paragraphDone {
			out = append(out, line)
			if line == "" && len(out) > 1 {
				paragraphDone = true
			}
		}
	}
	result := strings.TrimSpace(strings.Join(out, "\n"))
	if result == "" {
		return truncate(text, 300)
	}
	return result
}

// extractSentenceSummary returns the first 1–2 sentences.
func extractSentenceSummary(text string) string {
	// Strip code blocks first.
	text = stripCodeBlocks(text)
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return truncate(text, 150)
	}
	if len(sentences) == 1 {
		return sentences[0]
	}
	return sentences[0] + " " + sentences[1]
}

// extractKeywordSummary extracts the most significant words.
func extractKeywordSummary(text string) string {
	text = stripCodeBlocks(text)
	words := strings.Fields(text)
	var keywords []string
	seen := map[string]bool{}
	for _, w := range words {
		w = strings.Trim(w, `.,;:!?"'()[]{}`)
		lower := strings.ToLower(w)
		if len(w) < 4 || isStopWord(lower) || seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, w)
		if len(keywords) >= 12 {
			break
		}
	}
	return strings.Join(keywords, ", ")
}

func stripCodeBlocks(text string) string {
	var out strings.Builder
	inCode := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if !inCode {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func splitSentences(text string) []string {
	var sentences []string
	var cur strings.Builder
	for _, r := range text {
		cur.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			s := strings.TrimSpace(cur.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

func isStopWord(w string) bool {
	return stopWords[w]
}

var stopWords = func() map[string]bool {
	words := []string{
		"the", "and", "for", "that", "this", "with", "from", "have",
		"will", "been", "were", "they", "their", "there", "when",
		"what", "which", "would", "could", "should", "about", "into",
		"more", "also", "some", "than", "then", "just", "like",
	}
	m := map[string]bool{}
	for _, w := range words {
		m[w] = true
	}
	return m
}()

// DetectTurns segments a flat message list into Turn structs, assigning
// timestamps based on index when real timestamps are unavailable.
func DetectTurns(messages []struct {
	Role    string
	Content string
}) []Turn {
	now := time.Now()
	turns := make([]Turn, len(messages))
	for i, m := range messages {
		turns[i] = Turn{
			ID:        fmt.Sprintf("turn-%d", i),
			Role:      m.Role,
			Content:   m.Content,
			Original:  m.Content,
			Timestamp: now.Add(-time.Duration(len(messages)-i) * time.Minute),
			Level:     LevelFull,
			TokenCount: estimateTokens(m.Content),
		}
	}
	return turns
}

// TotalTokens returns the sum of token counts across all turns.
func TotalTokens(turns []Turn) int {
	total := 0
	for _, t := range turns {
		total += t.TokenCount
	}
	return total
}

// isLetter is used by keyword extraction.
func isLetter(r rune) bool {
	return unicode.IsLetter(r)
}

var _ = isLetter // suppress unused warning
