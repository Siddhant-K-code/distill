package summarize

import (
	"strings"
	"unicode"
)

// ScoreImportance returns an importance score (0–1) for a turn based on
// heuristic signals. Higher scores mean the turn should resist compression.
//
// Signals:
//   - Contains a code block (```) → +0.4
//   - Contains an error keyword → +0.3
//   - Contains a decision keyword → +0.2
//   - System role → always 1.0
//   - Tool role → +0.2
//   - Short content (< 50 chars) → −0.1
func ScoreImportance(t Turn) float64 {
	if t.Role == "system" {
		return 1.0
	}

	score := 0.5 // baseline
	lower := strings.ToLower(t.Content)

	// Code blocks are high-value.
	if strings.Contains(t.Content, "```") || strings.Contains(t.Content, "\t") {
		score += 0.4
	}

	// Error signals.
	for _, kw := range errorKeywords {
		if strings.Contains(lower, kw) {
			score += 0.3
			break
		}
	}

	// Decision / conclusion signals.
	for _, kw := range decisionKeywords {
		if strings.Contains(lower, kw) {
			score += 0.2
			break
		}
	}

	// Tool calls carry structured data.
	if t.Role == "tool" {
		score += 0.2
	}

	// Very short turns are usually low-value.
	if len([]rune(t.Content)) < 50 {
		score -= 0.1
	}

	// Clamp to [0, 1].
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	return score
}

// ScoreTurns sets Importance on each turn in-place.
func ScoreTurns(turns []Turn) {
	for i := range turns {
		if turns[i].Importance == 0 {
			turns[i].Importance = ScoreImportance(turns[i])
		}
	}
}

// estimateTokens approximates token count using the 4-chars-per-token heuristic.
func estimateTokens(s string) int {
	// Count printable runes only.
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return (n + 3) / 4
}

var errorKeywords = []string{
	"error", "exception", "panic", "fatal", "failed", "failure",
	"crash", "bug", "traceback", "stack trace", "nil pointer",
	"segfault", "timeout", "deadlock",
}

var decisionKeywords = []string{
	"decided", "decision", "conclusion", "therefore", "we will",
	"we should", "let's use", "going with", "chosen", "agreed",
	"final answer", "solution is", "approach is",
}
