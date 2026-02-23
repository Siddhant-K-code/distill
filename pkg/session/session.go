// Package session provides stateful context window management for AI agent
// sessions. Entries are deduplicated on push, compressed as they age, and
// evicted when the token budget is exceeded.
package session

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExists   = errors.New("session already exists")
	ErrOverBudget      = errors.New("single entry exceeds token budget")
)

// CompressionLevel indicates how compressed an entry is.
type CompressionLevel int

const (
	LevelFull     CompressionLevel = 0 // Original content
	LevelSummary  CompressionLevel = 1 // Paragraph summary (~20%)
	LevelSentence CompressionLevel = 2 // Single sentence (~5%)
	LevelKeywords CompressionLevel = 3 // Keywords only (~1%)
)

// Entry is a single item in a session's context window.
type Entry struct {
	ID               string           `json:"id"`
	Role             string           `json:"role"`               // user, assistant, tool, system
	Content          string           `json:"content"`            // current (possibly compressed) text
	OriginalContent  string           `json:"-"`                  // full text, kept for re-compression
	Source           string           `json:"source,omitempty"`   // e.g. file_read, search, conversation
	Embedding        []float32        `json:"-"`                  // for dedup comparison
	Importance       float64          `json:"importance"`         // 0-1, higher = harder to evict
	Level            CompressionLevel `json:"level"`              // current compression level
	Tokens           int              `json:"tokens"`             // token count of current content
	CreatedAt        time.Time        `json:"created_at"`
	CompressedAt     time.Time        `json:"compressed_at,omitempty"`
}

// Session holds the state of a single context window.
type Session struct {
	ID            string    `json:"session_id"`
	MaxTokens     int       `json:"max_tokens"`
	CurrentTokens int       `json:"current_tokens"`
	EntryCount    int       `json:"entry_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateRequest is the input for creating a session.
type CreateRequest struct {
	SessionID      string  `json:"session_id,omitempty"` // auto-generated if empty
	MaxTokens      int     `json:"max_tokens"`
	DedupThreshold float64 `json:"dedup_threshold,omitempty"`
	PreserveRecent int     `json:"preserve_recent,omitempty"` // always keep last N at full fidelity
}

// PushRequest is the input for adding entries to a session.
type PushRequest struct {
	SessionID string       `json:"session_id"`
	Entries   []PushEntry  `json:"entries"`
}

// PushEntry is a single entry in a push request.
type PushEntry struct {
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	Source     string    `json:"source,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	Importance float64   `json:"importance,omitempty"` // default 0.5
}

// PushResult is the output of a push operation.
type PushResult struct {
	SessionID      string `json:"session_id"`
	Accepted       int    `json:"accepted"`
	Deduplicated   int    `json:"deduplicated"`
	Compressed     int    `json:"compressed"`
	Evicted        int    `json:"evicted"`
	CurrentTokens  int    `json:"current_tokens"`
	BudgetRemaining int   `json:"budget_remaining"`
}

// ContextRequest is the input for reading a session's context window.
type ContextRequest struct {
	SessionID string `json:"session_id"`
	MaxTokens int    `json:"max_tokens,omitempty"` // 0 = return full window
	Role      string `json:"role,omitempty"`       // filter by role
}

// ContextResult is the output of a context read.
type ContextResult struct {
	Entries []ContextEntry `json:"entries"`
	Stats   ContextStats   `json:"stats"`
}

// ContextEntry is a single entry returned from a context read.
type ContextEntry struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Source    string           `json:"source,omitempty"`
	Level     CompressionLevel `json:"level"`
	Tokens    int              `json:"tokens"`
	Age       string           `json:"age"`
}

// ContextStats contains context window metrics.
type ContextStats struct {
	TotalEntries      int            `json:"total_entries"`
	TotalTokens       int            `json:"total_tokens"`
	CompressionLevels map[int]int    `json:"compression_levels"`
	CompressionSavings int           `json:"compression_savings"` // tokens saved by compression
}

// DeleteResult is the output of deleting a session.
type DeleteResult struct {
	SessionID    string `json:"session_id"`
	EntriesRemoved int  `json:"entries_removed"`
}

// Store is the interface for session backends.
type Store interface {
	Create(ctx context.Context, req CreateRequest) (*Session, error)
	Push(ctx context.Context, req PushRequest) (*PushResult, error)
	Context(ctx context.Context, req ContextRequest) (*ContextResult, error)
	Get(ctx context.Context, sessionID string) (*Session, error)
	Delete(ctx context.Context, sessionID string) (*DeleteResult, error)
	Close() error
}

// Config holds session store configuration.
type Config struct {
	// DefaultMaxTokens is the default token budget for new sessions.
	DefaultMaxTokens int

	// DefaultDedupThreshold is the cosine distance below which entries
	// are considered duplicates. Default: 0.15.
	DefaultDedupThreshold float64

	// DefaultPreserveRecent is how many recent entries to keep uncompressed.
	// Default: 10.
	DefaultPreserveRecent int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultMaxTokens:      128000,
		DefaultDedupThreshold: 0.15,
		DefaultPreserveRecent: 10,
	}
}
