package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// StabilityRecord tracks prefix hash observations for a single call site.
type StabilityRecord struct {
	CallSite    string
	Hashes      []string
	FirstSeen   time.Time
	LastSeen    time.Time
	TotalChecks int
	Changes     int
}

// StabilityRate returns the fraction of checks where the prefix was unchanged
// (1.0 = perfectly stable, 0.0 = changes every request).
func (r *StabilityRecord) StabilityRate() float64 {
	if r.TotalChecks <= 1 {
		return 1.0
	}
	return 1.0 - float64(r.Changes)/float64(r.TotalChecks-1)
}

// StabilityIssue describes a detected prefix instability.
type StabilityIssue struct {
	// CallSite is the identifier provided by the caller (e.g. "file:line").
	CallSite string

	// StabilityRate is the fraction of requests where the prefix was unchanged.
	StabilityRate float64

	// TotalChecks is the number of requests observed.
	TotalChecks int

	// Changes is the number of times the prefix hash changed.
	Changes int

	// PreviousHash is the last stable prefix hash.
	PreviousHash string

	// CurrentHash is the new (changed) prefix hash.
	CurrentHash string

	// Diff is a short human-readable description of what changed.
	Diff string

	// LikelyCause is a heuristic explanation of the instability.
	LikelyCause string
}

func (i StabilityIssue) Error() string {
	return fmt.Sprintf("cache-prefix-unstable %s: stability=%.0f%% (%d/%d changes) — %s",
		i.CallSite, i.StabilityRate*100, i.Changes, i.TotalChecks, i.LikelyCause)
}

// StabilityValidator tracks prefix hashes across requests and detects
// dynamic content bleeding into cached prefixes.
//
// Usage:
//
//	validator := cache.NewStabilityValidator(cache.DefaultStabilityConfig())
//	issues := validator.Check("agent/planner.go:84", chunks)
//	if len(issues) > 0 {
//	    log.Warn(issues[0])
//	}
type StabilityValidator struct {
	cfg     StabilityConfig
	mu      sync.Mutex
	records map[string]*StabilityRecord
}

// StabilityConfig controls the validator's sensitivity.
type StabilityConfig struct {
	// WarmupChecks is the number of requests to observe before reporting
	// issues. Avoids false positives on the first few requests.
	// Default: 3.
	WarmupChecks int

	// UnstableThreshold is the stability rate below which an issue is
	// reported. Default: 0.8 (report if prefix changes > 20% of requests).
	UnstableThreshold float64

	// MaxHashHistory is the maximum number of hashes to retain per call site.
	// Default: 100.
	MaxHashHistory int

	// DynamicPatterns is a list of substrings that, when found in the prefix
	// text, are flagged as likely causes of instability.
	DynamicPatterns []string
}

// DefaultStabilityConfig returns sensible defaults.
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		WarmupChecks:      3,
		UnstableThreshold: 0.8,
		MaxHashHistory:    100,
		DynamicPatterns: []string{
			"request_id", "requestid", "request-id", "request id",
			"timestamp", "datetime", "time.now", "date.now",
			"uuid", "random", "rand.",
			"user_id", "userid", "user-id",
			"session_id", "sessionid",
			"nonce", "token:",
		},
	}
}

// NewStabilityValidator creates a new validator.
func NewStabilityValidator(cfg StabilityConfig) *StabilityValidator {
	if cfg.WarmupChecks <= 0 {
		cfg.WarmupChecks = 3
	}
	if cfg.UnstableThreshold <= 0 {
		cfg.UnstableThreshold = 0.8
	}
	if cfg.MaxHashHistory <= 0 {
		cfg.MaxHashHistory = 100
	}
	if len(cfg.DynamicPatterns) == 0 {
		cfg.DynamicPatterns = DefaultStabilityConfig().DynamicPatterns
	}
	return &StabilityValidator{
		cfg:     cfg,
		records: make(map[string]*StabilityRecord),
	}
}

// Check records the current prefix hash for callSite and returns any
// stability issues detected. Returns nil when the prefix is stable or
// the validator is still in the warmup period.
//
// chunks should be the full message array (including cache_control markers).
// The prefix is extracted automatically using PartitionForCacheAwareDedup.
func (v *StabilityValidator) Check(callSite string, chunks []types.Chunk) []StabilityIssue {
	partition := PartitionForCacheAwareDedup(chunks)

	// If no cache_control markers, nothing to validate.
	if partition.MarkerCount == 0 {
		return nil
	}

	currentHash := partition.PrefixHash
	prefixText := extractPrefixText(partition.Prefix)

	v.mu.Lock()
	defer v.mu.Unlock()

	rec, exists := v.records[callSite]
	if !exists {
		rec = &StabilityRecord{
			CallSite:  callSite,
			FirstSeen: time.Now(),
		}
		v.records[callSite] = rec
	}

	rec.LastSeen = time.Now()
	rec.TotalChecks++

	var prevHash string
	if len(rec.Hashes) > 0 {
		prevHash = rec.Hashes[len(rec.Hashes)-1]
	}

	changed := prevHash != "" && prevHash != currentHash
	if changed {
		rec.Changes++
	}

	// Maintain bounded history.
	rec.Hashes = append(rec.Hashes, currentHash)
	if len(rec.Hashes) > v.cfg.MaxHashHistory {
		rec.Hashes = rec.Hashes[len(rec.Hashes)-v.cfg.MaxHashHistory:]
	}

	// Still warming up.
	if rec.TotalChecks < v.cfg.WarmupChecks {
		return nil
	}

	rate := rec.StabilityRate()
	if rate >= v.cfg.UnstableThreshold {
		return nil
	}

	issue := StabilityIssue{
		CallSite:      callSite,
		StabilityRate: rate,
		TotalChecks:   rec.TotalChecks,
		Changes:       rec.Changes,
		PreviousHash:  prevHash,
		CurrentHash:   currentHash,
		LikelyCause:   v.diagnoseCause(prefixText),
	}

	if changed && prevHash != "" {
		issue.Diff = fmt.Sprintf("prefix hash changed: %s → %s", prevHash[:8], currentHash[:8])
	}

	return []StabilityIssue{issue}
}

// ValidateText performs a one-shot static analysis of prefix text for
// patterns that commonly cause cache instability. Returns a list of
// suspected dynamic patterns found in the text.
//
// This is useful as a pre-flight check before sending the first request.
func (v *StabilityValidator) ValidateText(prefixText string) []string {
	lower := strings.ToLower(prefixText)
	var found []string
	seen := map[string]bool{}
	for _, pattern := range v.cfg.DynamicPatterns {
		if !seen[pattern] && strings.Contains(lower, pattern) {
			found = append(found, pattern)
			seen[pattern] = true
		}
	}
	return found
}

// Stats returns the current stability record for a call site, or nil if
// no observations have been recorded.
func (v *StabilityValidator) Stats(callSite string) *StabilityRecord {
	v.mu.Lock()
	defer v.mu.Unlock()
	r := v.records[callSite]
	if r == nil {
		return nil
	}
	// Return a copy to avoid data races.
	cp := *r
	cp.Hashes = append([]string(nil), r.Hashes...)
	return &cp
}

// AllStats returns stability records for all observed call sites.
func (v *StabilityValidator) AllStats() []*StabilityRecord {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]*StabilityRecord, 0, len(v.records))
	for _, r := range v.records {
		cp := *r
		cp.Hashes = append([]string(nil), r.Hashes...)
		out = append(out, &cp)
	}
	return out
}

// Reset clears all recorded observations for a call site.
func (v *StabilityValidator) Reset(callSite string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.records, callSite)
}

// diagnoseCause scans prefix text for known dynamic patterns and returns a
// human-readable explanation.
func (v *StabilityValidator) diagnoseCause(text string) string {
	found := v.ValidateText(text)
	if len(found) == 0 {
		return "unknown — prefix content changes between requests"
	}
	return fmt.Sprintf("likely dynamic interpolation: %s", strings.Join(found, ", "))
}

// extractPrefixText concatenates the text of all prefix chunks.
func extractPrefixText(chunks []types.Chunk) string {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}
