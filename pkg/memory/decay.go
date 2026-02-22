package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/compress"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// DecayWorker runs periodic compression of aging memories.
// Memories progress through decay levels based on age and access patterns:
//
//	full text -> summary -> keywords -> evicted
type DecayWorker struct {
	store  *SQLiteStore
	cfg    Config
	stopCh chan struct{}
}

// NewDecayWorker creates a decay worker for the given store.
func NewDecayWorker(store *SQLiteStore, cfg Config) *DecayWorker {
	return &DecayWorker{
		store:  store,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins the periodic decay loop. Call Stop() to terminate.
func (w *DecayWorker) Start() {
	go w.run()
}

// Stop terminates the decay worker.
func (w *DecayWorker) Stop() {
	close(w.stopCh)
}

func (w *DecayWorker) run() {
	ticker := time.NewTicker(w.cfg.DecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = w.runOnce(ctx)
			cancel()
		}
	}
}

// RunOnce executes a single decay pass. Exported for testing.
func (w *DecayWorker) RunOnce(ctx context.Context) error {
	return w.runOnce(ctx)
}

func (w *DecayWorker) runOnce(ctx context.Context) error {

	now := time.Now().UTC()

	// Evict: remove very old, unreferenced memories
	if w.cfg.EvictAge > 0 {
		evictBefore := now.Add(-w.cfg.EvictAge).Format(time.RFC3339Nano)
		_, _ = w.store.db.ExecContext(ctx,
			"DELETE FROM memories WHERE last_referenced < ? AND decay_level >= ?",
			evictBefore, int(DecayKeywords),
		)
	}

	// Decay to keywords: compress old summaries
	if w.cfg.KeywordsAge > 0 {
		keywordsBefore := now.Add(-w.cfg.KeywordsAge).Format(time.RFC3339Nano)
		if err := w.decayRows(ctx, keywordsBefore, DecaySummary, DecayKeywords, extractKeywords); err != nil {
			return err
		}
	}

	// Decay to summary: compress old full-text memories
	if w.cfg.SummaryAge > 0 {
		summaryBefore := now.Add(-w.cfg.SummaryAge).Format(time.RFC3339Nano)
		if err := w.decayRows(ctx, summaryBefore, DecayFull, DecaySummary, extractSummary); err != nil {
			return err
		}
	}

	return nil
}

// decayRows queries for memories at fromLevel older than cutoff,
// applies the transform function, and updates them to toLevel.
func (w *DecayWorker) decayRows(ctx context.Context, cutoff string, fromLevel, toLevel DecayLevel, transform func(string) string) error {
	rows, err := w.store.db.QueryContext(ctx,
		"SELECT id, text FROM memories WHERE last_referenced < ? AND decay_level = ?",
		cutoff, int(fromLevel),
	)
	if err != nil {
		return fmt.Errorf("query for decay level %d: %w", fromLevel, err)
	}

	type entry struct {
		id, text string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.text); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	_ = rows.Close()

	for _, e := range entries {
		compressed := transform(e.text)
		_, _ = w.store.db.ExecContext(ctx,
			"UPDATE memories SET text = ?, decay_level = ? WHERE id = ?",
			compressed, int(toLevel), e.id,
		)
	}

	return nil
}

// summaryCompressor is reused across decay passes to avoid per-call allocation.
var summaryCompressor = compress.NewExtractiveCompressor()

// extractSummary produces a shortened version of the text using the
// extractive compressor's sentence scorer for better quality summaries.
func extractSummary(text string) string {
	chunks := []types.Chunk{{ID: "decay", Text: text}}
	opts := compress.Options{
		TargetReduction: 0.2, // keep ~20% of content
		MinChunkLength:  20,
	}
	result, _, _ := summaryCompressor.Compress(context.Background(), chunks, opts)
	if len(result) > 0 && result[0].Text != "" {
		return result[0].Text
	}
	return text
}

// extractKeywords produces a keyword-only representation.
// Keeps words that appear significant (longer words, capitalized, numbers).
func extractKeywords(text string) string {
	words := strings.Fields(text)
	seen := make(map[string]bool)
	var keywords []string

	for _, w := range words {
		lower := strings.ToLower(strings.Trim(w, ".,;:!?\"'()[]{}"))
		if lower == "" || len(lower) < 4 {
			continue
		}
		if isStopWord(lower) {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, lower)
	}

	// Limit to ~20 keywords
	if len(keywords) > 20 {
		keywords = keywords[:20]
	}

	return strings.Join(keywords, ", ")
}

// stopWords is the set of common English stop words filtered during keyword extraction.
var stopWords = map[string]bool{
	"that": true, "this": true, "with": true, "from": true,
	"have": true, "been": true, "were": true, "they": true,
	"their": true, "which": true, "would": true, "there": true,
	"about": true, "could": true, "other": true, "into": true,
	"more": true, "some": true, "than": true, "them": true,
	"very": true, "when": true, "what": true, "your": true,
	"also": true, "each": true, "does": true, "will": true,
	"just": true, "should": true, "because": true, "these": true,
}

// isStopWord returns true for common English stop words.
func isStopWord(w string) bool {
	return stopWords[w]
}
