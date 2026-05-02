package memory

import "time"

// MemoryEventType identifies the kind of lifecycle transition a memory entry
// has undergone. Subscribers (e.g. a cache boundary manager) use these events
// to keep their state consistent with the memory store.
type MemoryEventType string

const (
	// EventStabilized fires when an entry has been present for enough turns
	// without modification to be considered stable.
	EventStabilized MemoryEventType = "stabilized"

	// EventCompressed fires when an entry is compressed to a shorter form
	// (full text → summary, or summary → keywords). The cached prefix that
	// contained the full text is now stale and must be invalidated.
	EventCompressed MemoryEventType = "compressed"

	// EventEvicted fires when an entry is removed from the store entirely.
	// Any cached prefix that included this entry must retreat.
	EventEvicted MemoryEventType = "evicted"
)

// MemoryEvent describes a single lifecycle transition for a memory entry.
type MemoryEvent struct {
	Type    MemoryEventType
	EntryID string

	// TokensBefore is the token count before the transition (0 for stabilized).
	TokensBefore int

	// TokensAfter is the token count after the transition (0 for evicted).
	TokensAfter int

	// CompressionLevel is the new decay level (only set for EventCompressed).
	CompressionLevel DecayLevel

	OccurredAt time.Time
}

// MemoryEventHandler is a callback invoked when a memory lifecycle event fires.
// Implementations must be non-blocking; long work should be dispatched to a
// goroutine.
type MemoryEventHandler func(event MemoryEvent)

// CacheBoundaryHint is included in RecallResult to give the cache boundary
// manager early signal about which entries are stable this turn, without
// waiting for the normal stability promotion cycle.
type CacheBoundaryHint struct {
	// StableEntryIDs lists entries that are likely stable this turn, ranked
	// by the recall scoring function (high recency + high similarity).
	StableEntryIDs []string

	// ConfidenceScore is the mean recall score of the stable entries (0–1).
	// Higher values mean the boundary manager can act with more confidence.
	ConfidenceScore float64
}
