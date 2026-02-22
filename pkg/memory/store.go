// Package memory provides a persistent context memory store with
// write-time deduplication, tag-based recall, and hierarchical decay.
package memory

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by memory stores.
var (
	ErrNotFound     = errors.New("memory not found")
	ErrEmptyText    = errors.New("entry text is empty")
	ErrStoreClosed  = errors.New("memory store is closed")
	ErrInvalidQuery = errors.New("query text is empty")
)

// DecayLevel represents how compressed a memory is.
// Memories decay over time: full text -> summary -> keywords -> evicted.
type DecayLevel int

const (
	DecayFull     DecayLevel = 0 // Original text, no compression
	DecaySummary  DecayLevel = 1 // Paragraph-level summary (~20% of original)
	DecayKeywords DecayLevel = 2 // Keywords only (~5% of original)
)

// Entry is a single memory stored in the system.
type Entry struct {
	ID             string                 `json:"id"`
	Text           string                 `json:"text"`
	Embedding      []float32              `json:"embedding,omitempty"`
	Source         string                 `json:"source,omitempty"`
	Tags           []string               `json:"tags,omitempty"`
	SessionID      string                 `json:"session_id,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	DecayLevel     DecayLevel             `json:"decay_level"`
	CreatedAt      time.Time              `json:"created_at"`
	LastReferenced time.Time              `json:"last_referenced"`
	AccessCount    int                    `json:"access_count"`
}

// StoreRequest is the input for storing memories.
type StoreRequest struct {
	SessionID string       `json:"session_id,omitempty"`
	Entries   []StoreEntry `json:"entries"`
}

// StoreEntry is a single entry in a store request.
type StoreEntry struct {
	Text      string                 `json:"text"`
	Embedding []float32              `json:"embedding,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// StoreResult is the output of a store operation.
type StoreResult struct {
	Stored        int `json:"stored"`
	Merged        int `json:"merged"`
	Deduplicated  int `json:"deduplicated"`
	TotalMemories int `json:"total_memories"`
}

// RecallRequest is the input for recalling memories.
type RecallRequest struct {
	Query         string   `json:"query"`
	QueryEmbedding []float32 `json:"query_embedding,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	MaxTokens     int      `json:"max_tokens,omitempty"`
	MaxResults    int      `json:"max_results,omitempty"`
	RecencyWeight float64  `json:"recency_weight,omitempty"`
}

// RecallResult is the output of a recall operation.
type RecallResult struct {
	Memories []RecalledMemory `json:"memories"`
	Stats    RecallStats      `json:"stats"`
}

// RecalledMemory is a single memory returned from recall.
type RecalledMemory struct {
	ID             string     `json:"id"`
	Text           string     `json:"text"`
	Source         string     `json:"source,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	Relevance      float64    `json:"relevance"`
	DecayLevel     DecayLevel `json:"decay_level"`
	LastReferenced time.Time  `json:"last_referenced"`
}

// RecallStats contains recall operation metrics.
type RecallStats struct {
	Candidates   int `json:"candidates"`
	Deduplicated int `json:"deduplicated"`
	Returned     int `json:"returned"`
	TokenCount   int `json:"token_count"`
}

// ForgetRequest specifies which memories to remove.
type ForgetRequest struct {
	IDs       []string  `json:"ids,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	OlderThan time.Time `json:"older_than,omitempty"`
}

// ForgetResult is the output of a forget operation.
type ForgetResult struct {
	Removed       int `json:"removed"`
	TotalMemories int `json:"total_memories"`
}

// Stats contains memory store statistics.
type Stats struct {
	TotalMemories  int            `json:"total_memories"`
	ByDecayLevel   map[int]int    `json:"by_decay_level"`
	BySource       map[string]int `json:"by_source"`
	OldestMemory   time.Time      `json:"oldest_memory,omitempty"`
	NewestMemory   time.Time      `json:"newest_memory,omitempty"`
}

// Store is the interface for persistent memory backends.
type Store interface {
	// Store adds entries to memory with write-time deduplication.
	Store(ctx context.Context, req StoreRequest) (*StoreResult, error)

	// Recall retrieves memories matching a query, ranked by relevance and recency.
	Recall(ctx context.Context, req RecallRequest) (*RecallResult, error)

	// Forget removes memories matching the given criteria.
	Forget(ctx context.Context, req ForgetRequest) (*ForgetResult, error)

	// Stats returns memory store statistics.
	Stats(ctx context.Context) (*Stats, error)

	// Close releases resources held by the store.
	Close() error
}

// Config holds memory store configuration.
type Config struct {
	// DedupThreshold is the cosine distance below which entries are
	// considered duplicates. Default: 0.15.
	DedupThreshold float64

	// DecayEnabled enables the background decay worker.
	DecayEnabled bool

	// DecayInterval is how often the decay worker runs. Default: 1h.
	DecayInterval time.Duration

	// SummaryAge is the age after which memories are compressed to summaries.
	// Default: 24h.
	SummaryAge time.Duration

	// KeywordsAge is the age after which memories are compressed to keywords.
	// Default: 168h (7 days).
	KeywordsAge time.Duration

	// EvictAge is the age after which unreferenced memories are evicted.
	// Default: 720h (30 days).
	EvictAge time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DedupThreshold: 0.15,
		DecayEnabled:   true,
		DecayInterval:  1 * time.Hour,
		SummaryAge:     24 * time.Hour,
		KeywordsAge:    168 * time.Hour,
		EvictAge:       720 * time.Hour,
	}
}
