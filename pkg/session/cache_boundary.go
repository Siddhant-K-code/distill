package session

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// minCacheableTokens is Anthropic's minimum prefix size to qualify for prompt
// caching (Claude 3.5+).
const minCacheableTokens = 1024

// maxCacheMarkers is the maximum number of simultaneous cache_control markers
// Anthropic allows per request.
const maxCacheMarkers = 4

// CacheBoundaryConfig controls how the boundary manager classifies stability
// and places cache_control markers.
type CacheBoundaryConfig struct {
	// Enabled turns the boundary manager on or off. Default: true.
	Enabled bool

	// MinStableTurns is the number of consecutive pushes an entry must survive
	// unmodified before it is considered stable. Default: 2.
	MinStableTurns int

	// MinPrefixTokens is the minimum combined token count required before any
	// marker is placed. Matches Anthropic's 1024-token minimum. Default: 1024.
	MinPrefixTokens int

	// MaxMarkers is the maximum number of cache_control markers to place.
	// Anthropic allows up to 4. Default: 4.
	MaxMarkers int
}

// DefaultCacheBoundaryConfig returns sensible defaults.
func DefaultCacheBoundaryConfig() CacheBoundaryConfig {
	return CacheBoundaryConfig{
		Enabled:         true,
		MinStableTurns:  2,
		MinPrefixTokens: minCacheableTokens,
		MaxMarkers:      maxCacheMarkers,
	}
}

// CacheBoundaryMarker describes a single cache_control placement.
type CacheBoundaryMarker struct {
	// EntryID is the session entry that should carry the marker.
	EntryID string

	// TokensUpToHere is the cumulative token count of all entries up to and
	// including this one.
	TokensUpToHere int

	// StableSinceTurn is the push count at which this entry became stable.
	StableSinceTurn int
}

// CacheBoundaryResult is the output of a boundary evaluation.
type CacheBoundaryResult struct {
	// Markers lists the recommended cache_control placements in order.
	Markers []CacheBoundaryMarker

	// TotalStableTokens is the combined token count of all stable entries.
	TotalStableTokens int

	// Advanced is true when the boundary moved forward since the last push.
	Advanced bool

	// Retreated is true when the boundary moved backward (content changed).
	Retreated bool
}

// CacheBoundaryManager evaluates the optimal cache_control placement after
// each session push. It is embedded in SQLiteStore and called automatically
// by Push when boundary management is enabled.
type CacheBoundaryManager struct {
	db  *sql.DB
	cfg CacheBoundaryConfig
}

// newCacheBoundaryManager creates a manager backed by the given database.
func newCacheBoundaryManager(db *sql.DB, cfg CacheBoundaryConfig) *CacheBoundaryManager {
	return &CacheBoundaryManager{db: db, cfg: cfg}
}

// Evaluate computes the current optimal cache boundary for a session.
// It returns the recommended markers and whether the boundary changed.
func (m *CacheBoundaryManager) Evaluate(ctx context.Context, sessionID string) (*CacheBoundaryResult, error) {
	if !m.cfg.Enabled {
		return &CacheBoundaryResult{}, nil
	}

	// Load entries ordered by sequence.
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, tokens, stable_since_turn, content_hash
		 FROM session_entries
		 WHERE session_id = ?
		 ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}

	type entryRow struct {
		id             string
		tokens         int
		stableSince    int
		contentHash    string
	}
	var entries []entryRow
	for rows.Next() {
		var e entryRow
		if err := rows.Scan(&e.id, &e.tokens, &e.stableSince, &e.contentHash); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	minStable := m.cfg.MinStableTurns
	result := &CacheBoundaryResult{}
	cumTokens := 0

	type candidate struct {
		entryID        string
		cumTokens      int
		stableSince    int
	}
	var candidates []candidate

	for _, e := range entries {
		cumTokens += e.tokens
		if e.stableSince > 0 && e.stableSince <= minStable {
			// stable_since_turn is the push count when it first appeared;
			// it is considered stable once it has survived minStable pushes.
			// We store the push count at insertion; the current push count is
			// derived from the max stable_since_turn in the table.
			candidates = append(candidates, candidate{
				entryID:     e.id,
				cumTokens:   cumTokens,
				stableSince: e.stableSince,
			})
		}
	}

	// Filter: only include entries whose cumulative token count meets the
	// minimum prefix requirement.
	var eligible []candidate
	for _, c := range candidates {
		if c.cumTokens >= m.cfg.MinPrefixTokens {
			eligible = append(eligible, c)
		}
	}

	// Sort by cumulative tokens descending to pick the largest stable prefixes.
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].cumTokens > eligible[j].cumTokens
	})

	// Cap at MaxMarkers.
	if len(eligible) > m.cfg.MaxMarkers {
		eligible = eligible[:m.cfg.MaxMarkers]
	}

	// Re-sort by cumTokens ascending so markers are in document order.
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].cumTokens < eligible[j].cumTokens
	})

	for _, c := range eligible {
		result.Markers = append(result.Markers, CacheBoundaryMarker{
			EntryID:         c.entryID,
			TokensUpToHere:  c.cumTokens,
			StableSinceTurn: c.stableSince,
		})
		result.TotalStableTokens = c.cumTokens
	}

	// Detect advance/retreat by comparing with the stored boundary.
	prev, err := m.loadStoredBoundary(ctx, sessionID)
	if err == nil {
		if result.TotalStableTokens > prev {
			result.Advanced = true
		} else if result.TotalStableTokens < prev && prev > 0 {
			result.Retreated = true
		}
	}

	// Persist the new boundary position.
	_ = m.storeBoundary(ctx, sessionID, result.TotalStableTokens)

	return result, nil
}

// loadStoredBoundary retrieves the last recorded boundary token count.
func (m *CacheBoundaryManager) loadStoredBoundary(ctx context.Context, sessionID string) (int, error) {
	var tokens int
	err := m.db.QueryRowContext(ctx,
		"SELECT cache_boundary_tokens FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&tokens)
	if err != nil {
		return 0, err
	}
	return tokens, nil
}

// storeBoundary persists the current boundary token count.
func (m *CacheBoundaryManager) storeBoundary(ctx context.Context, sessionID string, tokens int) error {
	_, err := m.db.ExecContext(ctx,
		"UPDATE sessions SET cache_boundary_tokens = ? WHERE id = ?",
		tokens, sessionID,
	)
	return err
}

// RecordPush increments the push counter for a session and updates
// stable_since_turn for entries that have now survived minStableTurns pushes.
func (m *CacheBoundaryManager) RecordPush(ctx context.Context, sessionID string) error {
	if !m.cfg.Enabled {
		return nil
	}

	// Increment push count.
	_, err := m.db.ExecContext(ctx,
		"UPDATE sessions SET push_count = push_count + 1 WHERE id = ?",
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("increment push count: %w", err)
	}

	// Fetch current push count.
	var pushCount int
	if err := m.db.QueryRowContext(ctx,
		"SELECT push_count FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&pushCount); err != nil {
		return fmt.Errorf("read push count: %w", err)
	}

	// Mark entries as stable when they have survived minStableTurns pushes
	// without modification (stable_since_turn == 0 means not yet stable).
	stableThreshold := pushCount - m.cfg.MinStableTurns
	if stableThreshold > 0 {
		_, err = m.db.ExecContext(ctx,
			`UPDATE session_entries
			 SET stable_since_turn = inserted_at_push
			 WHERE session_id = ?
			   AND stable_since_turn = 0
			   AND inserted_at_push <= ?`,
			sessionID, stableThreshold,
		)
		if err != nil {
			return fmt.Errorf("mark stable entries: %w", err)
		}
	}

	return nil
}

// InvalidateEntry marks an entry as no longer stable (e.g. content changed).
// The boundary will retreat on the next Evaluate call.
func (m *CacheBoundaryManager) InvalidateEntry(ctx context.Context, entryID string) error {
	_, err := m.db.ExecContext(ctx,
		"UPDATE session_entries SET stable_since_turn = 0, content_hash = '' WHERE id = ?",
		entryID,
	)
	return err
}

// BoundaryStats returns a snapshot of the current boundary state for a session.
type BoundaryStats struct {
	SessionID         string
	PushCount         int
	BoundaryTokens    int
	StableEntryCount  int
	LastEvaluatedAt   time.Time
}

// Stats returns boundary statistics for a session.
func (m *CacheBoundaryManager) Stats(ctx context.Context, sessionID string) (*BoundaryStats, error) {
	var stats BoundaryStats
	stats.SessionID = sessionID

	err := m.db.QueryRowContext(ctx,
		"SELECT push_count, cache_boundary_tokens FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&stats.PushCount, &stats.BoundaryTokens)
	if err != nil {
		return nil, fmt.Errorf("read boundary stats: %w", err)
	}

	_ = m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM session_entries WHERE session_id = ? AND stable_since_turn > 0",
		sessionID,
	).Scan(&stats.StableEntryCount)

	stats.LastEvaluatedAt = time.Now()
	return &stats, nil
}
