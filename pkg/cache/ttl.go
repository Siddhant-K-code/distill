package cache

import (
	"sync"
	"time"
)

// AnthropicCacheTTL is the Anthropic prompt cache TTL. The cache entry is
// kept alive as long as requests arrive within this window; it expires if
// no request references it for this duration.
const AnthropicCacheTTL = 5 * time.Minute

// TTLEntry tracks the last-seen time and expiry state of a single cache
// prefix identified by its hash.
type TTLEntry struct {
	PrefixHash  string
	LastSeen    time.Time
	ExpiresAt   time.Time
	HitCount    int
	MissCount   int
	// Expired is true when the entry has not been refreshed within AnthropicCacheTTL.
	Expired bool
}

// IsAlive returns true when the entry is still within its TTL window.
func (e *TTLEntry) IsAlive() bool {
	return time.Now().Before(e.ExpiresAt)
}

// TTLTracker monitors cache prefix liveness across requests. It detects
// when a prefix has gone cold (no requests within AnthropicCacheTTL) and
// records the resulting cold-start penalty.
//
// For batch workloads, use Schedule to determine the latest safe time to
// send the next request without letting the cache expire.
type TTLTracker struct {
	mu      sync.Mutex
	entries map[string]*TTLEntry
	ttl     time.Duration
}

// NewTTLTracker creates a tracker with the given TTL.
// Pass 0 to use AnthropicCacheTTL (5 minutes).
func NewTTLTracker(ttl time.Duration) *TTLTracker {
	if ttl <= 0 {
		ttl = AnthropicCacheTTL
	}
	return &TTLTracker{
		entries: make(map[string]*TTLEntry),
		ttl:     ttl,
	}
}

// Touch records a request for the given prefix hash and returns whether the
// cache was alive (warm) or had expired (cold start).
//
// Call Touch after every request that carries a cache_control marker.
func (t *TTLTracker) Touch(prefixHash string) (wasAlive bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	entry, ok := t.entries[prefixHash]
	if !ok {
		// First time seeing this prefix.
		t.entries[prefixHash] = &TTLEntry{
			PrefixHash: prefixHash,
			LastSeen:   now,
			ExpiresAt:  now.Add(t.ttl),
			MissCount:  1,
		}
		return false
	}

	wasAlive = now.Before(entry.ExpiresAt)
	if wasAlive {
		entry.HitCount++
	} else {
		entry.MissCount++
		entry.Expired = true
	}
	entry.LastSeen = now
	entry.ExpiresAt = now.Add(t.ttl)
	return wasAlive
}

// NextDeadline returns the time by which the next request must arrive to
// keep the cache entry alive. Returns zero time if the prefix is unknown.
func (t *TTLTracker) NextDeadline(prefixHash string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[prefixHash]
	if e == nil {
		return time.Time{}
	}
	return e.ExpiresAt
}

// TimeUntilExpiry returns how long until the cache entry expires.
// Returns 0 if already expired or unknown.
func (t *TTLTracker) TimeUntilExpiry(prefixHash string) time.Duration {
	deadline := t.NextDeadline(prefixHash)
	if deadline.IsZero() {
		return 0
	}
	d := time.Until(deadline)
	if d < 0 {
		return 0
	}
	return d
}

// ScheduleDeadline returns the latest time a batch job can wait before
// sending its next request without letting the cache expire. It subtracts
// a safety margin from the TTL deadline.
//
// safetyMargin should account for network latency and scheduling jitter.
// A value of 30s is reasonable for most deployments.
func (t *TTLTracker) ScheduleDeadline(prefixHash string, safetyMargin time.Duration) time.Time {
	deadline := t.NextDeadline(prefixHash)
	if deadline.IsZero() {
		return time.Time{}
	}
	return deadline.Add(-safetyMargin)
}

// Entry returns a snapshot of the TTL entry for a prefix hash, or nil.
func (t *TTLTracker) Entry(prefixHash string) *TTLEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[prefixHash]
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

// ExpiredEntries returns all entries that have passed their TTL deadline.
func (t *TTLTracker) ExpiredEntries() []*TTLEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	var out []*TTLEntry
	for _, e := range t.entries {
		if now.After(e.ExpiresAt) {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out
}

// Evict removes the entry for a prefix hash (e.g. after a boundary retreat).
func (t *TTLTracker) Evict(prefixHash string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, prefixHash)
}

// Stats returns a summary of all tracked prefixes.
type TTLStats struct {
	TotalPrefixes   int
	AlivePrefixes   int
	ExpiredPrefixes int
	TotalHits       int64
	TotalMisses     int64
}

// Stats returns aggregate TTL statistics.
func (t *TTLTracker) Stats() TTLStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	var s TTLStats
	s.TotalPrefixes = len(t.entries)
	for _, e := range t.entries {
		if now.Before(e.ExpiresAt) {
			s.AlivePrefixes++
		} else {
			s.ExpiredPrefixes++
		}
		s.TotalHits += int64(e.HitCount)
		s.TotalMisses += int64(e.MissCount)
	}
	return s
}
