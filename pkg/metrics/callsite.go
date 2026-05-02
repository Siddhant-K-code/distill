package metrics

import (
	"fmt"
	"sync"
	"time"
)

// CallSiteRecord holds cumulative cache usage for a single call site.
type CallSiteRecord struct {
	CallSite string

	// Token counters from Anthropic API usage blocks.
	CacheCreationTokens int64
	CacheReadTokens     int64
	UncachedInputTokens int64
	OutputTokens        int64

	// Request counts.
	TotalRequests int64
	// CacheHitRequests is the number of requests where cache_read_input_tokens > 0.
	CacheHitRequests int64

	FirstSeen time.Time
	LastSeen  time.Time
}

// HitRate returns cache_read / (cache_read + cache_creation + uncached_input).
// Returns 0 when no tokens have been recorded.
func (r *CallSiteRecord) HitRate() float64 {
	total := r.CacheReadTokens + r.CacheCreationTokens + r.UncachedInputTokens
	if total == 0 {
		return 0
	}
	return float64(r.CacheReadTokens) / float64(total)
}

// WriteEfficiency returns cache_read / cache_creation.
// Returns 0 when no cache writes have been recorded.
func (r *CallSiteRecord) WriteEfficiency() float64 {
	if r.CacheCreationTokens == 0 {
		return 0
	}
	return float64(r.CacheReadTokens) / float64(r.CacheCreationTokens)
}

// RequestHitRate returns the fraction of requests that had at least one
// cache read token (i.e. the cache was warm for that request).
func (r *CallSiteRecord) RequestHitRate() float64 {
	if r.TotalRequests == 0 {
		return 0
	}
	return float64(r.CacheHitRequests) / float64(r.TotalRequests)
}

// CallSiteTracker records per-call-site Anthropic cache usage and exposes
// aggregated hit rate statistics. It is safe for concurrent use.
//
// Typical usage:
//
//	tracker := metrics.NewCallSiteTracker()
//	// after each Anthropic API call:
//	tracker.Record("agent/planner.go:84", metrics.UsageRecord{
//	    CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
//	    CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
//	    InputTokens:              resp.Usage.InputTokens,
//	    OutputTokens:             resp.Usage.OutputTokens,
//	})
//	stats := tracker.Stats("agent/planner.go:84")
//	fmt.Printf("hit rate: %.0f%%\n", stats.HitRate()*100)
type CallSiteTracker struct {
	mu      sync.RWMutex
	records map[string]*CallSiteRecord
}

// NewCallSiteTracker creates a new tracker.
func NewCallSiteTracker() *CallSiteTracker {
	return &CallSiteTracker{
		records: make(map[string]*CallSiteRecord),
	}
}

// Record adds a usage observation for callSite.
func (t *CallSiteTracker) Record(callSite string, u UsageRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()

	rec, ok := t.records[callSite]
	if !ok {
		rec = &CallSiteRecord{
			CallSite:  callSite,
			FirstSeen: time.Now(),
		}
		t.records[callSite] = rec
	}

	rec.CacheCreationTokens += int64(u.CacheCreationInputTokens)
	rec.CacheReadTokens += int64(u.CacheReadInputTokens)
	rec.UncachedInputTokens += int64(u.InputTokens)
	rec.OutputTokens += int64(u.OutputTokens)
	rec.TotalRequests++
	if u.CacheReadInputTokens > 0 {
		rec.CacheHitRequests++
	}
	rec.LastSeen = time.Now()
}

// Stats returns a snapshot of the record for callSite, or nil if not found.
func (t *CallSiteTracker) Stats(callSite string) *CallSiteRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r := t.records[callSite]
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

// AllStats returns snapshots of all recorded call sites, sorted by hit rate
// ascending (worst performers first).
func (t *CallSiteTracker) AllStats() []*CallSiteRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]*CallSiteRecord, 0, len(t.records))
	for _, r := range t.records {
		cp := *r
		out = append(out, &cp)
	}

	// Sort worst hit rate first so callers can surface actionable items.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].HitRate() < out[j-1].HitRate(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Reset clears all observations for callSite.
func (t *CallSiteTracker) Reset(callSite string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, callSite)
}

// ResetAll clears all observations.
func (t *CallSiteTracker) ResetAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make(map[string]*CallSiteRecord)
}

// Summary returns a human-readable summary of all call sites.
func (t *CallSiteTracker) Summary() string {
	stats := t.AllStats()
	if len(stats) == 0 {
		return "no call sites recorded"
	}
	out := fmt.Sprintf("%-40s %8s %8s %8s\n", "call site", "hit%", "eff", "reqs")
	for _, s := range stats {
		out += fmt.Sprintf("%-40s %7.0f%% %7.1fx %8d\n",
			s.CallSite,
			s.HitRate()*100,
			s.WriteEfficiency(),
			s.TotalRequests,
		)
	}
	return out
}
