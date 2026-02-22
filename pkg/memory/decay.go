package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	w.store.mu.Lock()
	defer w.store.mu.Unlock()

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
	rows.Close()

	for _, e := range entries {
		compressed := transform(e.text)
		_, _ = w.store.db.ExecContext(ctx,
			"UPDATE memories SET text = ?, decay_level = ? WHERE id = ?",
			compressed, int(toLevel), e.id,
		)
	}

	return nil
}

// extractSummary produces a shortened version of the text.
// Keeps the first and last sentences, targeting ~20% of original length.
func extractSummary(text string) string {
	sentences := splitSentences(text)
	if len(sentences) <= 2 {
		return text
	}

	// Target ~20% of sentences, minimum 2
	target := len(sentences) / 5
	if target < 2 {
		target = 2
	}

	// Keep first half from the beginning, second half from the end
	firstHalf := target / 2
	if firstHalf < 1 {
		firstHalf = 1
	}
	secondHalf := target - firstHalf

	var parts []string
	parts = append(parts, sentences[:firstHalf]...)
	if secondHalf > 0 && len(sentences)-secondHalf > firstHalf {
		parts = append(parts, sentences[len(sentences)-secondHalf:]...)
	}

	return strings.Join(parts, " ")
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

// splitSentences splits text on sentence boundaries.
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for _, r := range text {
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}

	// Remaining text
	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	return sentences
}

// isStopWord returns true for common English stop words.
func isStopWord(w string) bool {
	stops := map[string]bool{
		"that": true, "this": true, "with": true, "from": true,
		"have": true, "been": true, "were": true, "they": true,
		"their": true, "which": true, "would": true, "there": true,
		"about": true, "could": true, "other": true, "into": true,
		"more": true, "some": true, "than": true, "them": true,
		"very": true, "when": true, "what": true, "your": true,
		"also": true, "each": true, "does": true, "will": true,
		"just": true, "should": true, "because": true, "these": true,
	}
	return stops[w]
}
