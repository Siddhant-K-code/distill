package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	distillmath "github.com/Siddhant-K-code/distill/pkg/math"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite for local persistent storage.
type SQLiteStore struct {
	db  *sql.DB
	cfg Config
	mu  sync.RWMutex
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

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
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
		tags            TEXT DEFAULT '[]',
		session_id      TEXT DEFAULT '',
		metadata        TEXT DEFAULT '{}',
		decay_level     INTEGER DEFAULT 0,
		created_at      TEXT NOT NULL,
		last_referenced TEXT NOT NULL,
		access_count    INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_memories_tags ON memories(tags);
	CREATE INDEX IF NOT EXISTS idx_memories_decay ON memories(decay_level);
	CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
	CREATE INDEX IF NOT EXISTS idx_memories_referenced ON memories(last_referenced);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Store adds entries with write-time deduplication.
func (s *SQLiteStore) Store(ctx context.Context, req StoreRequest) (*StoreResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

		tagsJSON, _ := json.Marshal(entry.Tags)
		metaJSON, _ := json.Marshal(entry.Metadata)
		embBlob := encodeEmbedding(entry.Embedding)

		sessionID := req.SessionID

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO memories (id, text, embedding, source, tags, session_id, metadata, decay_level, created_at, last_referenced, access_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, 0)`,
			id, entry.Text, embBlob, entry.Source, string(tagsJSON), sessionID, string(metaJSON), now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("insert memory: %w", err)
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	query := "SELECT id, text, embedding, source, tags, decay_level, last_referenced FROM memories"
	var args []interface{}

	if len(req.Tags) > 0 {
		clauses := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			clauses[i] = "tags LIKE ?"
			args = append(args, "%\""+tag+"\"%")
		}
		query += " WHERE (" + strings.Join(clauses, " OR ") + ")"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candidates []scored
	now := time.Now()

	for rows.Next() {
		var (
			id, text, source, tagsStr, refStr string
			embBlob                           []byte
			decayLevel                        int
		)
		if err := rows.Scan(&id, &text, &embBlob, &source, &tagsStr, &decayLevel, &refStr); err != nil {
			return nil, err
		}

		var tags []string
		_ = json.Unmarshal([]byte(tagsStr), &tags)
		lastRef, _ := time.Parse(time.RFC3339Nano, refStr)

		// Compute relevance score from embedding similarity
		var similarity float64
		if len(req.QueryEmbedding) > 0 {
			existing := decodeEmbedding(embBlob)
			if len(existing) > 0 {
				dist := distillmath.CosineDistance(req.QueryEmbedding, existing)
				similarity = 1.0 - dist // Convert distance to similarity
			}
		}

		// Compute recency score (exponential decay, half-life = 24h)
		age := now.Sub(lastRef).Hours()
		recency := 1.0
		if age > 0 {
			recency = 1.0 / (1.0 + age/24.0)
		}

		// Combined score
		relevance := (1.0-recencyWeight)*similarity + recencyWeight*recency

		candidates = append(candidates, scored{
			memory: RecalledMemory{
				ID:             id,
				Text:           text,
				Source:         source,
				Tags:           tags,
				Relevance:      relevance,
				DecayLevel:     DecayLevel(decayLevel),
				LastReferenced: lastRef,
			},
			relevance: relevance,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
	s.mu.Lock()
	defer s.mu.Unlock()

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
		tagClauses := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			tagClauses[i] = "tags LIKE ?"
			args = append(args, "%\""+tag+"\"%")
		}
		conditions = append(conditions, "("+strings.Join(tagClauses, " OR ")+")")
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
func (s *SQLiteStore) Stats(ctx context.Context) (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &Stats{
		ByDecayLevel: make(map[int]int),
		BySource:     make(map[string]int),
	}

	// Total count
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}

	// By decay level
	rows, err := s.db.QueryContext(ctx, "SELECT decay_level, COUNT(*) FROM memories GROUP BY decay_level")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var level, count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		stats.ByDecayLevel[level] = count
	}

	// By source
	rows2, err := s.db.QueryContext(ctx, "SELECT source, COUNT(*) FROM memories WHERE source != '' GROUP BY source")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows2.Close() }()
	for rows2.Next() {
		var source string
		var count int
		if err := rows2.Scan(&source, &count); err != nil {
			return nil, err
		}
		stats.BySource[source] = count
	}

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

// touchMemories updates last_referenced and access_count for the given IDs.
// Called from Recall under a read lock, so we use a separate goroutine.
func (s *SQLiteStore) touchMemories(ctx context.Context, ids []string) {
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		now := time.Now().UTC().Format(time.RFC3339Nano)
		placeholders := make([]string, len(ids))
		args := []interface{}{now}
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query := "UPDATE memories SET last_referenced = ?, access_count = access_count + 1 WHERE id IN (" + strings.Join(placeholders, ",") + ")"
		_, _ = s.db.ExecContext(ctx, query, args...)
	}()
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
