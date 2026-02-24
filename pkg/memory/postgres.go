package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	distillmath "github.com/Siddhant-K-code/distill/pkg/math"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresStore implements Store using PostgreSQL (Supabase) for persistent storage.
type PostgresStore struct {
	db  *sql.DB
	cfg Config
}

// NewPostgresStore creates a new Postgres-backed memory store.
// dsn should be a Postgres connection string, e.g.:
//
//	"postgres://user:pass@host:5432/dbname?sslmode=require"
func NewPostgresStore(dsn string, cfg Config) (*PostgresStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &PostgresStore{db: db, cfg: cfg}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *PostgresStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id              TEXT PRIMARY KEY,
		text            TEXT NOT NULL,
		embedding       BYTEA,
		source          TEXT DEFAULT '',
		session_id      TEXT DEFAULT '',
		metadata        JSONB DEFAULT '{}',
		decay_level     INTEGER DEFAULT 0,
		created_at      TIMESTAMPTZ NOT NULL,
		last_referenced TIMESTAMPTZ NOT NULL,
		access_count    INTEGER DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
		tag       TEXT NOT NULL,
		PRIMARY KEY (memory_id, tag)
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
func (s *PostgresStore) Store(ctx context.Context, req StoreRequest) (*StoreResult, error) {
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
				_, err := s.db.ExecContext(ctx,
					`UPDATE memories SET last_referenced = $1, access_count = access_count + 1 WHERE id = $2`,
					time.Now().UTC(), dupID,
				)
				if err != nil {
					return nil, fmt.Errorf("update duplicate: %w", err)
				}
				result.Deduplicated++
				continue
			}
		}

		id := generateID()
		now := time.Now().UTC()

		metaJSON, _ := json.Marshal(entry.Metadata)
		embBlob := encodeEmbedding(entry.Embedding)

		sessionID := req.SessionID

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO memories (id, text, embedding, source, session_id, metadata, decay_level, created_at, last_referenced, access_count)
			 VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, 0)`,
			id, entry.Text, embBlob, entry.Source, sessionID, string(metaJSON), now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("insert memory: %w", err)
		}

		for _, tag := range entry.Tags {
			_, err := s.db.ExecContext(ctx,
				"INSERT INTO memory_tags (memory_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				id, tag,
			)
			if err != nil {
				return nil, fmt.Errorf("insert tag: %w", err)
			}
		}

		result.Stored++
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&total); err != nil {
		return nil, err
	}
	result.TotalMemories = total

	return result, nil
}

func (s *PostgresStore) findDuplicate(ctx context.Context, embedding []float32) (string, error) {
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
func (s *PostgresStore) Recall(ctx context.Context, req RecallRequest) (*RecallResult, error) {
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
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, tag)
		}
		query += " WHERE m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (" + strings.Join(placeholders, ",") + "))"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type rawRow struct {
		id, text, source string
		embBlob          []byte
		decayLevel       int
		lastRef          time.Time
	}
	var rawRows []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.id, &r.text, &r.embBlob, &r.source, &r.decayLevel, &r.lastRef); err != nil {
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var candidates []scored
	now := time.Now()

	for _, r := range rawRows {
		tags, _ := s.loadTags(ctx, r.id)

		var similarity float64
		if len(req.QueryEmbedding) > 0 {
			existing := decodeEmbedding(r.embBlob)
			if len(existing) > 0 {
				dist := distillmath.CosineDistance(req.QueryEmbedding, existing)
				similarity = 1.0 - dist
			}
		}

		age := now.Sub(r.lastRef).Hours()
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
				LastReferenced: r.lastRef,
			},
			relevance: relevance,
		})
	}

	sortByRelevance(candidates)

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
func (s *PostgresStore) Forget(ctx context.Context, req ForgetRequest) (*ForgetResult, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if len(req.IDs) > 0 {
		placeholders := make([]string, len(req.IDs))
		for i, id := range req.IDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, id)
			argIdx++
		}
		conditions = append(conditions, "id IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(req.Tags) > 0 {
		placeholders := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, tag)
			argIdx++
		}
		conditions = append(conditions, "id IN (SELECT memory_id FROM memory_tags WHERE tag IN ("+strings.Join(placeholders, ",")+"))")
	}

	if !req.OlderThan.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, req.OlderThan.UTC())
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
func (s *PostgresStore) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		ByDecayLevel: make(map[int]int),
		BySource:     make(map[string]int),
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}

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
	_ = rows.Close()

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
	_ = rows.Close()

	var oldest, newest sql.NullTime
	_ = s.db.QueryRowContext(ctx, "SELECT MIN(created_at) FROM memories").Scan(&oldest)
	_ = s.db.QueryRowContext(ctx, "SELECT MAX(created_at) FROM memories").Scan(&newest)
	if oldest.Valid {
		stats.OldestMemory = oldest.Time
	}
	if newest.Valid {
		stats.NewestMemory = newest.Time
	}

	return stats, nil
}

// Close closes the database connection pool.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) loadTags(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT tag FROM memory_tags WHERE memory_id = $1", memoryID)
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

func (s *PostgresStore) touchMemories(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	placeholders := make([]string, len(ids))
	args := []interface{}{time.Now().UTC()}
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	query := "UPDATE memories SET last_referenced = $1, access_count = access_count + 1 WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, _ = s.db.ExecContext(ctx, query, args...)
}
