package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	distillmath "github.com/Siddhant-K-code/distill/pkg/math"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite for local persistent storage.
// Uses a single connection (SetMaxOpenConns(1)) so SQLite's internal
// serialization handles concurrency. No application-level mutex needed.
type SQLiteStore struct {
	db  *sql.DB
	cfg Config
}

// NewSQLiteStore creates a new SQLite-backed memory store.
// Use ":memory:" for in-memory storage or a file path for persistence.
func NewSQLiteStore(dsn string, cfg Config) (*SQLiteStore, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite doesn't support concurrent connections well with in-memory DBs
	// and PRAGMAs are per-connection, so pin to a single connection.
	db.SetMaxOpenConns(1)

	// WAL mode for better read performance if pool size increases later.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys for CASCADE deletes on memory_tags.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db, cfg: cfg}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id              TEXT PRIMARY KEY,
		text            TEXT NOT NULL,
		embedding       BLOB,
		source          TEXT DEFAULT '',
		session_id      TEXT DEFAULT '',
		metadata        TEXT DEFAULT '{}',
		decay_level     INTEGER DEFAULT 0,
		created_at      TEXT NOT NULL,
		last_referenced TEXT NOT NULL,
		access_count    INTEGER DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id TEXT NOT NULL,
		tag       TEXT NOT NULL,
		PRIMARY KEY (memory_id, tag),
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);
	CREATE INDEX IF NOT EXISTS idx_memories_decay ON memories(decay_level);
	CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
	CREATE INDEX IF NOT EXISTS idx_memories_referenced ON memories(last_referenced);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Store adds entries with write-time deduplication.
func (s *SQLiteStore) Store(ctx context.Context, req StoreRequest) (*StoreResult, error) {

	result := &StoreResult{}

	for _, entry := range req.Entries {
		if entry.Text == "" {
			continue
		}

		// Check for semantic duplicates if embedding is provided
		if len(entry.Embedding) > 0 {
			dupID, err := s.findDuplicate(ctx, entry.Embedding)
			if err != nil {
				return nil, fmt.Errorf("find duplicate: %w", err)
			}
			if dupID != "" {
				// Update the existing memory's last_referenced and access_count
				_, err := s.db.ExecContext(ctx,
					`UPDATE memories SET last_referenced = ?, access_count = access_count + 1 WHERE id = ?`,
					time.Now().UTC().Format(time.RFC3339Nano), dupID,
				)
				if err != nil {
					return nil, fmt.Errorf("update duplicate: %w", err)
				}
				result.Deduplicated++
				continue
			}
		}

		// Insert new memory
		id := generateID()
		now := time.Now().UTC().Format(time.RFC3339Nano)

		metaJSON, _ := json.Marshal(entry.Metadata)
		embBlob := encodeEmbedding(entry.Embedding)

		sessionID := req.SessionID

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO memories (id, text, embedding, source, session_id, metadata, decay_level, created_at, last_referenced, access_count)
			 VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, 0)`,
			id, entry.Text, embBlob, entry.Source, sessionID, string(metaJSON), now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("insert memory: %w", err)
		}

		// Insert tags into junction table
		for _, tag := range entry.Tags {
			_, err := s.db.ExecContext(ctx,
				"INSERT OR IGNORE INTO memory_tags (memory_id, tag) VALUES (?, ?)",
				id, tag,
			)
			if err != nil {
				return nil, fmt.Errorf("insert tag: %w", err)
			}
		}

		result.Stored++
	}

	// Get total count
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&total); err != nil {
		return nil, err
	}
	result.TotalMemories = total

	return result, nil
}

// findDuplicate scans existing embeddings and returns the ID of the first
// entry within the dedup threshold. Returns "" if no duplicate found.
//
// TODO: This does a full table scan (O(n) per insert). Fine for < 10K entries.
// At larger scale, consider an approximate nearest-neighbor index or caching
// embeddings in memory.
func (s *SQLiteStore) findDuplicate(ctx context.Context, embedding []float32) (string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, embedding FROM memories WHERE embedding IS NOT NULL")
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id string
		var embBlob []byte
		if err := rows.Scan(&id, &embBlob); err != nil {
			return "", err
		}

		existing := decodeEmbedding(embBlob)
		if len(existing) == 0 {
			continue
		}

		dist := distillmath.CosineDistance(embedding, existing)
		if dist < s.cfg.DedupThreshold {
			return id, nil
		}
	}

	return "", rows.Err()
}

// Recall retrieves memories matching a query, ranked by relevance and recency.
func (s *SQLiteStore) Recall(ctx context.Context, req RecallRequest) (*RecallResult, error) {

	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		return nil, ErrInvalidQuery
	}

	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	recencyWeight := req.RecencyWeight
	if recencyWeight < 0 {
		recencyWeight = 0
	}
	if recencyWeight > 1 {
		recencyWeight = 1
	}

	// Build query with optional tag filter
	query := "SELECT m.id, m.text, m.embedding, m.source, m.decay_level, m.last_referenced FROM memories m"
	var args []interface{}

	if len(req.Tags) > 0 {
		placeholders := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			placeholders[i] = "?"
			args = append(args, tag)
		}
		query += " WHERE m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (" + strings.Join(placeholders, ",") + "))"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}

	// Scan all rows first, then close before issuing more queries.
	// SQLite with MaxOpenConns(1) requires the connection to be free.
	type rawRow struct {
		id, text, source, refStr string
		embBlob                  []byte
		decayLevel               int
	}
	var rawRows []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.id, &r.text, &r.embBlob, &r.source, &r.decayLevel, &r.refStr); err != nil {
			_ = rows.Close()
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	var candidates []scored
	now := time.Now()

	for _, r := range rawRows {
		tags, _ := s.loadTags(ctx, r.id)
		lastRef, _ := time.Parse(time.RFC3339Nano, r.refStr)

		// Compute relevance score from embedding similarity
		var similarity float64
		if len(req.QueryEmbedding) > 0 {
			existing := decodeEmbedding(r.embBlob)
			if len(existing) > 0 {
				dist := distillmath.CosineDistance(req.QueryEmbedding, existing)
				similarity = 1.0 - dist
			}
		}

		// Compute recency score (exponential decay, half-life = 24h)
		age := now.Sub(lastRef).Hours()
		recency := 1.0
		if age > 0 {
			recency = 1.0 / (1.0 + age/24.0)
		}

		relevance := (1.0-recencyWeight)*similarity + recencyWeight*recency

		candidates = append(candidates, scored{
			memory: RecalledMemory{
				ID:             r.id,
				Text:           r.text,
				Source:         r.source,
				Tags:           tags,
				Relevance:      relevance,
				DecayLevel:     DecayLevel(r.decayLevel),
				LastReferenced: lastRef,
			},
			relevance: relevance,
		})
	}

	// Sort by relevance descending
	sortByRelevance(candidates)

	// Apply token budget or max results limit
	var results []RecalledMemory
	tokenCount := 0
	for _, c := range candidates {
		if len(results) >= maxResults {
			break
		}
		tokens := estimateTokens(c.memory.Text)
		if req.MaxTokens > 0 && tokenCount+tokens > req.MaxTokens {
			break
		}
		results = append(results, c.memory)
		tokenCount += tokens
	}

	// Update last_referenced for returned memories
	if len(results) > 0 {
		ids := make([]string, len(results))
		for i, m := range results {
			ids[i] = m.ID
		}
		s.touchMemories(ctx, ids)
	}

	return &RecallResult{
		Memories: results,
		Stats: RecallStats{
			Candidates:   len(candidates),
			Deduplicated: len(candidates) - len(results),
			Returned:     len(results),
			TokenCount:   tokenCount,
		},
	}, nil
}

// Forget removes memories matching the given criteria.
func (s *SQLiteStore) Forget(ctx context.Context, req ForgetRequest) (*ForgetResult, error) {

	var conditions []string
	var args []interface{}

	if len(req.IDs) > 0 {
		placeholders := make([]string, len(req.IDs))
		for i, id := range req.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, "id IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(req.Tags) > 0 {
		placeholders := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			placeholders[i] = "?"
			args = append(args, tag)
		}
		conditions = append(conditions, "id IN (SELECT memory_id FROM memory_tags WHERE tag IN ("+strings.Join(placeholders, ",")+"))")
	}

	if !req.OlderThan.IsZero() {
		conditions = append(conditions, "created_at < ?")
		args = append(args, req.OlderThan.UTC().Format(time.RFC3339Nano))
	}

	if len(conditions) == 0 {
		return &ForgetResult{}, nil
	}

	query := "DELETE FROM memories WHERE " + strings.Join(conditions, " AND ")
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("delete memories: %w", err)
	}

	removed, _ := res.RowsAffected()

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&total); err != nil {
		return nil, err
	}

	return &ForgetResult{
		Removed:       int(removed),
		TotalMemories: total,
	}, nil
}

// Stats returns memory store statistics.
// Each query is scanned and closed before the next to avoid holding
// the single SQLite connection across multiple result sets.
func (s *SQLiteStore) Stats(ctx context.Context) (*Stats, error) {

	stats := &Stats{
		ByDecayLevel: make(map[int]int),
		BySource:     make(map[string]int),
	}

	// Total count
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}

	// By decay level - scan and close before next query
	rows, err := s.db.QueryContext(ctx, "SELECT decay_level, COUNT(*) FROM memories GROUP BY decay_level")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var level, count int
		if err := rows.Scan(&level, &count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		stats.ByDecayLevel[level] = count
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// By source - scan and close before next query
	rows, err = s.db.QueryContext(ctx, "SELECT source, COUNT(*) FROM memories WHERE source != '' GROUP BY source")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		stats.BySource[source] = count
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// Oldest and newest
	var oldest, newest sql.NullString
	_ = s.db.QueryRowContext(ctx, "SELECT MIN(created_at) FROM memories").Scan(&oldest)
	_ = s.db.QueryRowContext(ctx, "SELECT MAX(created_at) FROM memories").Scan(&newest)
	if oldest.Valid {
		stats.OldestMemory, _ = time.Parse(time.RFC3339Nano, oldest.String)
	}
	if newest.Valid {
		stats.NewestMemory, _ = time.Parse(time.RFC3339Nano, newest.String)
	}

	return stats, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// loadTags returns the tags for a given memory ID.
func (s *SQLiteStore) loadTags(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT tag FROM memory_tags WHERE memory_id = ?", memoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// touchMemories updates last_referenced and access_count for the given IDs.
func (s *SQLiteStore) touchMemories(ctx context.Context, ids []string) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(ids))
	args := []interface{}{now}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := "UPDATE memories SET last_referenced = ?, access_count = access_count + 1 WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, _ = s.db.ExecContext(ctx, query, args...)
}

// scored pairs a recalled memory with its computed relevance.
type scored struct {
	memory    RecalledMemory
	relevance float64
}

// sortByRelevance sorts scored candidates by relevance descending.
func sortByRelevance(candidates []scored) {
	// Simple insertion sort - typically small N
	for i := 1; i < len(candidates); i++ {
		key := candidates[i]
		j := i - 1
		for j >= 0 && candidates[j].relevance < key.relevance {
			candidates[j+1] = candidates[j]
			j--
		}
		candidates[j+1] = key
	}
}
