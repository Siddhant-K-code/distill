// Package summarize provides hierarchical multi-level summarization for
// long conversation sessions. Turns are compressed progressively as they age:
//
//	Level 0: full content (recent)
//	Level 1: paragraph summary (medium age)
//	Level 2: single-sentence summary (old)
//	Level 3: keywords only (very old)
package summarize

import (
	"context"
	"time"
)

// Level represents a compression level for a conversation turn.
type Level int

const (
	LevelFull      Level = 0 // original content
	LevelParagraph Level = 1 // paragraph summary
	LevelSentence  Level = 2 // single-sentence summary
	LevelKeywords  Level = 3 // keywords only
	LevelEvicted   Level = 4 // dropped entirely (zero tokens)
)

// Turn represents a single conversation turn.
type Turn struct {
	ID        string
	Role      string // "user", "assistant", "tool", "system"
	Content   string
	Original  string    // preserved original for reversibility
	Timestamp time.Time
	Level     Level
	Importance float64  // 0–1; high-importance turns resist compression
	TokenCount int
}

// SummarizeOptions configures a summarization pass.
type SummarizeOptions struct {
	// MaxTokens is the target total token budget. 0 = no limit.
	MaxTokens int

	// PreserveRecent keeps the N most recent turns at LevelFull regardless
	// of age or token pressure. Default: 10.
	PreserveRecent int

	// ImportanceThreshold: turns with Importance >= this value are never
	// compressed beyond LevelParagraph. Default: 0.7.
	ImportanceThreshold float64

	// AgeLevels maps turn age to the maximum compression level allowed.
	// Turns older than AgeLevels[i].After are compressed to AgeLevels[i].MaxLevel.
	AgeLevels []AgeLevel
}

// AgeLevel maps a minimum age to a maximum compression level.
type AgeLevel struct {
	After    time.Duration
	MaxLevel Level
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() SummarizeOptions {
	return SummarizeOptions{
		MaxTokens:           0,
		PreserveRecent:      10,
		ImportanceThreshold: 0.7,
		AgeLevels: []AgeLevel{
			{After: 30 * time.Minute, MaxLevel: LevelParagraph},
			{After: 2 * time.Hour, MaxLevel: LevelSentence},
			{After: 24 * time.Hour, MaxLevel: LevelKeywords},
		},
	}
}

// SummarizeStats reports what happened during a summarization pass.
type SummarizeStats struct {
	InputTurns      int
	OutputTurns     int
	InputTokens     int
	OutputTokens    int
	CompressedTurns int
	PreservedTurns  int
	ReductionPct    float64
	Latency         time.Duration
}

// Summarizer compresses conversation turns to fit within a token budget.
type Summarizer interface {
	Summarize(ctx context.Context, turns []Turn, opts SummarizeOptions) ([]Turn, SummarizeStats, error)
}
