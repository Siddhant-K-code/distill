//go:build postgres

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	distillmath "github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/sensitivity"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore uses a connection pool (pgxpool) and relies on Postgres's MVCC for concurrency safety.
// Deduplication uses a full-scan cosine distance search (fine for < 10K rows; consider pgvector at scale).
type PostgresStore struct {
	dbPool     *pgxpool.Pool
	cfg        Config
	handlersMu sync.RWMutex
	handlers   []MemoryEventHandler
	classifier *sensitivity.Classifier

	decayCancel context.CancelFunc
	decayDone   chan struct{}
}

// NewPostgresStore creates a new Postgres-backed memory store.
func NewPostgresStore(dsn string, cfg Config) (*PostgresStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty DSN")
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	ps := &PostgresStore{
		dbPool:     pool,
		cfg:        cfg,
		classifier: sensitivity.New(sensitivity.DefaultConfig()),
		decayDone:  make(chan struct{}),
	}

	if err := ps.migrate(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}

	if cfg.DecayEnabled {
		dctx, cancel := context.WithCancel(context.Background())
		ps.decayCancel = cancel
		go ps.decayWorker(dctx)
	}

	return ps, nil
}

func (ps *PostgresStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id             TEXT PRIMARY KEY,
		text           TEXT NOT NULL,
		embedding      BYTEA,
		source         TEXT DEFAULT '',
		session_id     TEXT DEFAULT '',
		metadata       TEXT DEFAULT '{}',
		decay_level    INTEGER DEFAULT 0,
		sensitivity    INTEGER DEFAULT 0,
		created_at     TIMESTAMPTZ NOT NULL,
		last_referenced TIMESTAMPTZ NOT NULL,
		access_count   INTEGER DEFAULT 0,
		expired        BOOLEAN DEFAULT FALSE,
		expired_at     TIMESTAMPTZ,
		superseded_by  TEXT DEFAULT '',
		expires_at     TIMESTAMPTZ
	);
	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id TEXT NOT NULL,
		tag       TEXT NOT NULL,
		PRIMARY KEY (memory_id, tag)
	);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_tag    ON memory_tags(tag);
	CREATE INDEX IF NOT EXISTS idx_memories_decay     ON memories(decay_level);
	CREATE INDEX IF NOT EXISTS idx_memories_created   ON memories(created_at);
	CREATE INDEX IF NOT EXISTS idx_memories_referenced ON memories(last_referenced);
	CREATE INDEX IF NOT EXISTS idx_memories_expired   ON memories(expired);
	`
	_, err := ps.dbPool.Exec(context.Background(), schema)
	return err
}

// Store adds entries with write-time deduplication.
func (ps *PostgresStore) Store(ctx context.Context, req StoreRequest) (*StoreResult, error) {
	result := &StoreResult{}

	for _, entry := range req.Entries {
		if entry.Text == "" {
			continue
		}

		if len(entry.Embedding) > 0 {
			similar, err := ps.findSimilar(ctx, entry.Embedding)
			if err != nil {
				return nil, fmt.Errorf("find similar: %w", err)
			}

			isDup := false
			for _, sim := range similar {
				if sim.isDup {
					_, err := ps.dbPool.Exec(ctx,
						`UPDATE memories SET last_referenced = NOW(), access_count = access_count + 1 WHERE id = $1`,
						sim.id,
					)
					if err != nil {
						return nil, fmt.Errorf("postgres: update duplicate: %w", err)
					}
					result.Deduplicated++
					isDup = true
					break
				}
			}

			if isDup {
				continue
			}

			for _, sim := range similar {
				result.Conflicts = append(result.Conflicts, Conflict{
					NewText:      entry.Text,
					ExistingID:   sim.id,
					ExistingText: sim.text,
					Distance:     sim.distance,
				})
			}
		}

		id := generateID()
		metaJSON, _ := json.Marshal(entry.Metadata)
		embBlob := encodeEmbedding(entry.Embedding)

		sens := entry.Sensitivity
		if entry.AutoClassify {
			classified := ps.classifier.Classify(entry.Text)
			if classified.Level > sens {
				sens = classified.Level
			}
		}

		_, err := ps.dbPool.Exec(ctx, `
			INSERT INTO memories
				(id, text, embedding, source, session_id, metadata, decay_level, sensitivity,
				 created_at, last_referenced, access_count, expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,0,$7,NOW(),NOW(),0,$8)`,
			id, entry.Text, embBlob, entry.Source, req.SessionID,
			string(metaJSON), int(sens), entry.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: insert memory: %w", err)
		}

		for _, tag := range entry.Tags {
			_, err := ps.dbPool.Exec(ctx,
				`INSERT INTO memory_tags (memory_id, tag) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
				id, tag,
			)
			if err != nil {
				return nil, fmt.Errorf("postgres: insert tag: %w", err)
			}
		}

		for i := range result.Conflicts {
			if result.Conflicts[i].NewID == "" {
				result.Conflicts[i].NewID = id
			}
		}

		result.Stored++
	}

	var total int
	if err := ps.dbPool.QueryRow(ctx, `SELECT COUNT(*) FROM memories`).Scan(&total); err != nil {
		return nil, err
	}
	result.TotalMemories = total

	return result, nil
}

type pgSimilarEntry struct {
	id       string
	text     string
	distance float64
	isDup    bool
}

// findSimilar performs a full-scan cosine distance search against active memories.
func (ps *PostgresStore) findSimilar(ctx context.Context, embedding []float32) ([]pgSimilarEntry, error) {
	rows, err := ps.dbPool.Query(ctx,
		`SELECT id, text, embedding FROM memories WHERE embedding IS NOT NULL AND expired = FALSE`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conflictThreshold := ps.cfg.ConflictThreshold
	if conflictThreshold <= 0 {
		conflictThreshold = 0.35
	}

	var results []pgSimilarEntry
	for rows.Next() {
		var id, text string
		var embBlob []byte
		if err := rows.Scan(&id, &text, &embBlob); err != nil {
			return nil, err
		}

		existing := decodeEmbedding(embBlob)
		if len(existing) == 0 {
			continue
		}

		dist := distillmath.CosineDistance(embedding, existing)
		if dist < ps.cfg.DedupThreshold {
			return []pgSimilarEntry{{id: id, text: text, distance: dist, isDup: true}}, nil
		}
		if dist < conflictThreshold {
			results = append(results, pgSimilarEntry{id: id, text: text, distance: dist})
		}
	}

	return results, rows.Err()
}

// Recall retrieves memories matching a query, ranked by relevance and recency.
func (ps *PostgresStore) Recall(ctx context.Context, req RecallRequest) (*RecallResult, error) {
	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		return nil, ErrInvalidQuery
	}

	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	recencyWeight := clamp(req.RecencyWeight, 0, 1)

	qb := &pgQueryBuilder{}
	qb.from("memories m")
	qb.selectCols("m.id, m.text, m.embedding, m.source, m.decay_level, m.sensitivity, m.last_referenced")

	if !req.IncludeExpired {
		qb.where("m.expired = FALSE")
		qb.where("(m.expires_at IS NULL OR m.expires_at > NOW())")
	}

	if len(req.Tags) > 0 {
		placeholders := qb.addArgs(tagsToIface(req.Tags)...)
		qb.where(fmt.Sprintf(
			"m.id IN (SELECT memory_id FROM memory_tags WHERE tag = ANY(ARRAY[%s]))",
			placeholders,
		))
	}

	q, args := qb.build()
	rows, err := ps.dbPool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: recall query: %w", err)
	}

	type rawRow struct {
		id, text, source string
		embBlob          []byte
		decayLevel       int
		sensitivityLevel int
		lastRef          time.Time
	}

	var rawRows []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.id, &r.text, &r.embBlob, &r.source, &r.decayLevel, &r.sensitivityLevel, &r.lastRef); err != nil {
			rows.Close()
			return nil, err
		}
		rawRows = append(rawRows, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	boostTagSet := make(map[string]bool, len(req.BoostTags))
	for _, t := range req.BoostTags {
		boostTagSet[t] = true
	}
	taskCtxLower := strings.ToLower(req.TaskContext)

	var candidates []scored
	now := time.Now()

	for _, r := range rawRows {
		tags, _ := ps.loadTags(ctx, r.id)

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

		for _, tag := range tags {
			if boostTagSet[tag] {
				relevance += 0.1
				break
			}
		}

		if taskCtxLower != "" {
			if r.source != "" && strings.Contains(taskCtxLower, strings.ToLower(r.source)) {
				relevance += 0.05
			}
			if strings.Contains(strings.ToLower(r.text), taskCtxLower) {
				relevance += 0.05
			}
		}

		if relevance > 1.0 {
			relevance = 1.0
		}
		if req.MinRelevance > 0 && relevance < req.MinRelevance {
			continue
		}

		candidates = append(candidates, scored{
			memory: RecalledMemory{
				ID:             r.id,
				Text:           r.text,
				Source:         r.source,
				Tags:           tags,
				Relevance:      relevance,
				DecayLevel:     DecayLevel(r.decayLevel),
				Sensitivity:    sensitivity.Level(r.sensitivityLevel),
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
		ps.touchMemories(ctx, ids)
	}

	hint := buildCacheBoundaryHint(results)
	maxSens, sensitiveChunks := buildSensitivityMetadata(results)

	return &RecallResult{
		Memories: results,
		Stats: RecallStats{
			Candidates:   len(candidates),
			Deduplicated: len(candidates) - len(results),
			Returned:     len(results),
			TokenCount:   tokenCount,
		},
		CacheHint:       hint,
		MaxSensitivity:  maxSens,
		SensitiveChunks: sensitiveChunks,
	}, nil
}

// Forget removes memories matching the given criteria.
func (ps *PostgresStore) Forget(ctx context.Context, req ForgetRequest) (*ForgetResult, error) {
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
		conditions = append(conditions, "id = ANY(ARRAY["+strings.Join(placeholders, ",")+"])")
	}

	if len(req.Tags) > 0 {
		placeholders := make([]string, len(req.Tags))
		for i, tag := range req.Tags {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, tag)
			argIdx++
		}
		conditions = append(conditions,
			"id IN (SELECT memory_id FROM memory_tags WHERE tag = ANY(ARRAY["+strings.Join(placeholders, ",")+"]))",
		)
	}

	if !req.OlderThan.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, req.OlderThan.UTC())
	}

	if len(conditions) == 0 {
		return &ForgetResult{}, nil
	}

	query := "DELETE FROM memories WHERE " + strings.Join(conditions, " AND ")
	ct, err := ps.dbPool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: forget: %w", err)
	}

	var total int
	if err := ps.dbPool.QueryRow(ctx, `SELECT COUNT(*) FROM memories`).Scan(&total); err != nil {
		return nil, err
	}

	return &ForgetResult{
		Removed:       int(ct.RowsAffected()),
		TotalMemories: total,
	}, nil
}

// Expire marks the given memory IDs as expired.
func (ps *PostgresStore) Expire(ctx context.Context, req ExpireRequest) (*ExpireResult, error) {
	if len(req.IDs) == 0 {
		return &ExpireResult{}, nil
	}

	placeholders := make([]string, len(req.IDs))
	args := make([]interface{}, len(req.IDs))
	for i, id := range req.IDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	ct, err := ps.dbPool.Exec(ctx,
		"UPDATE memories SET expired = TRUE, expired_at = NOW() WHERE expired = FALSE AND id = ANY(ARRAY["+
			strings.Join(placeholders, ",")+"])",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: expire: %w", err)
	}

	now := time.Now().UTC()
	for _, id := range req.IDs {
		ps.emit(MemoryEvent{Type: EventExpired, EntryID: id, OccurredAt: now})
	}

	return &ExpireResult{Expired: int(ct.RowsAffected())}, nil
}

// Supersede marks oldID as expired and records newID as its replacement.
func (ps *PostgresStore) Supersede(ctx context.Context, req SupersedeRequest) (*SupersedeResult, error) {
	if req.OldID == "" {
		return nil, ErrNotFound
	}

	ct, err := ps.dbPool.Exec(ctx,
		`UPDATE memories SET expired = TRUE, expired_at = NOW(), superseded_by = $1 WHERE id = $2 AND expired = FALSE`,
		req.NewID, req.OldID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: supersede: %w", err)
	}

	if ct.RowsAffected() == 0 {
		var count int
		if err := ps.dbPool.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE id = $1`, req.OldID).Scan(&count); err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, ErrNotFound
		}
		return nil, ErrAlreadyExpired
	}

	ps.emit(MemoryEvent{Type: EventExpired, EntryID: req.OldID, OccurredAt: time.Now().UTC()})
	return &SupersedeResult{Superseded: true}, nil
}

// Stats returns memory store statistics.
func (ps *PostgresStore) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		ByDecayLevel: make(map[int]int),
		BySource:     make(map[string]int),
	}

	if err := ps.dbPool.QueryRow(ctx, `SELECT COUNT(*) FROM memories`).Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}
	if err := ps.dbPool.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE expired = TRUE`).Scan(&stats.ExpiredCount); err != nil {
		return nil, err
	}
	stats.ActiveCount = stats.TotalMemories - stats.ExpiredCount

	rows, err := ps.dbPool.Query(ctx, `SELECT decay_level, COUNT(*) FROM memories GROUP BY decay_level`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var level, count int
		if err := rows.Scan(&level, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.ByDecayLevel[level] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = ps.dbPool.Query(ctx, `SELECT source, COUNT(*) FROM memories WHERE source != '' GROUP BY source`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.BySource[source] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var oldest, newest *time.Time
	_ = ps.dbPool.QueryRow(ctx, `SELECT MIN(created_at) FROM memories`).Scan(&oldest)
	_ = ps.dbPool.QueryRow(ctx, `SELECT MAX(created_at) FROM memories`).Scan(&newest)
	if oldest != nil {
		stats.OldestMemory = *oldest
	}
	if newest != nil {
		stats.NewestMemory = *newest
	}

	return stats, nil
}

// OnLifecycleEvent registers a handler called on memory lifecycle transitions.
func (ps *PostgresStore) OnLifecycleEvent(handler MemoryEventHandler) {
	ps.handlersMu.Lock()
	defer ps.handlersMu.Unlock()
	ps.handlers = append(ps.handlers, handler)
}

func (ps *PostgresStore) emit(event MemoryEvent) {
	ps.handlersMu.RLock()
	defer ps.handlersMu.RUnlock()
	for _, h := range ps.handlers {
		h(event)
	}
}

// decayWorker runs periodic decay sweeps until ctx is cancelled.
func (ps *PostgresStore) decayWorker(ctx context.Context) {
	defer close(ps.decayDone)

	interval := ps.cfg.DecayInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ps.runDecaySweep(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ps.runDecaySweep(ctx)
		}
	}
}

// runDecaySweep mirrors the SQLite DecayWorker.runOnce logic:
//  1. Hard-delete TTL-expired entries, emit EventExpired.
//  2. Evict Keywords-level memories older than EvictAge, emit EventEvicted.
//  3. Decay Summary → Keywords for memories older than KeywordsAge, compress text, emit EventCompressed.
//  4. Decay Full → Summary for memories older than SummaryAge, compress text, emit EventCompressed.
func (ps *PostgresStore) runDecaySweep(ctx context.Context) {
	now := time.Now().UTC()

	// Step 1: hard-delete entries whose TTL has elapsed.
	ttlRows, err := ps.dbPool.Query(ctx, `
		DELETE FROM memories
		WHERE expires_at IS NOT NULL AND expires_at <= NOW()
		RETURNING id
	`)
	if err == nil {
		for ttlRows.Next() {
			var id string
			if ttlRows.Scan(&id) == nil {
				ps.emit(MemoryEvent{Type: EventExpired, EntryID: id, OccurredAt: now})
			}
		}
		ttlRows.Close()
	}

	// Step 2: evict Keywords-level memories older than EvictAge.
	if ps.cfg.EvictAge > 0 {
		cutoff := now.Add(-ps.cfg.EvictAge)
		erows, err := ps.dbPool.Query(ctx,
			`SELECT id, LENGTH(text) FROM memories WHERE expired = FALSE AND decay_level >= $1 AND last_referenced < $2`,
			int(DecayKeywords), cutoff,
		)
		if err == nil {
			type evictEntry struct {
				id     string
				length int
			}
			var entries []evictEntry
			for erows.Next() {
				var e evictEntry
				if erows.Scan(&e.id, &e.length) == nil {
					entries = append(entries, e)
				}
			}
			erows.Close()
			for _, e := range entries {
				_, _ = ps.dbPool.Exec(ctx, "DELETE FROM memories WHERE id = $1", e.id)
				ps.emit(MemoryEvent{
					Type:         EventEvicted,
					EntryID:      e.id,
					TokensBefore: (e.length + 3) / 4,
					TokensAfter:  0,
					OccurredAt:   now,
				})
			}
		}
	}

	// Step 3: decay Summary → Keywords.
	if ps.cfg.KeywordsAge > 0 {
		ps.decayRows(ctx, now.Add(-ps.cfg.KeywordsAge), DecaySummary, DecayKeywords, extractKeywords, now)
	}

	// Step 4: decay Full → Summary.
	if ps.cfg.SummaryAge > 0 {
		ps.decayRows(ctx, now.Add(-ps.cfg.SummaryAge), DecayFull, DecaySummary, extractSummary, now)
	}
}

// decayRows fetches memories at fromLevel older than cutoff, applies transform,
// updates them to toLevel, and emits EventCompressed for each.
func (ps *PostgresStore) decayRows(ctx context.Context, cutoff time.Time, fromLevel, toLevel DecayLevel, transform func(string) string, now time.Time) {
	rows, err := ps.dbPool.Query(ctx,
		`SELECT id, text FROM memories WHERE expired = FALSE AND decay_level = $1 AND last_referenced < $2`,
		int(fromLevel), cutoff,
	)
	if err != nil {
		return
	}

	type entry struct{ id, text string }
	var entries []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.id, &e.text) == nil {
			entries = append(entries, e)
		}
	}
	rows.Close()

	for _, e := range entries {
		compressed := transform(e.text)
		_, _ = ps.dbPool.Exec(ctx,
			"UPDATE memories SET text = $1, decay_level = $2 WHERE id = $3",
			compressed, int(toLevel), e.id,
		)
		ps.emit(MemoryEvent{
			Type:             EventCompressed,
			EntryID:          e.id,
			TokensBefore:     (len(e.text) + 3) / 4,
			TokensAfter:      (len(compressed) + 3) / 4,
			CompressionLevel: toLevel,
			OccurredAt:       now,
		})
	}
}

func (ps *PostgresStore) loadTags(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := ps.dbPool.Query(ctx, `SELECT tag FROM memory_tags WHERE memory_id = $1`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (ps *PostgresStore) touchMemories(ctx context.Context, ids []string) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	_, _ = ps.dbPool.Exec(ctx,
		"UPDATE memories SET last_referenced = NOW(), access_count = access_count + 1 WHERE id = ANY(ARRAY["+
			strings.Join(placeholders, ",")+"])",
		args...,
	)
}

// Close stops the decay worker and closes the connection pool.
func (ps *PostgresStore) Close() error {
	if ps.decayCancel != nil {
		ps.decayCancel()
		<-ps.decayDone
	}
	ps.dbPool.Close()
	return nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func tagsToIface(tags []string) []interface{} {
	out := make([]interface{}, len(tags))
	for i, t := range tags {
		out[i] = t
	}
	return out
}

type pgQueryBuilder struct {
	cols       string
	fromClause string
	wheres     []string
	args       []interface{}
}

func (b *pgQueryBuilder) selectCols(cols string) { b.cols = cols }
func (b *pgQueryBuilder) from(f string)          { b.fromClause = f }
func (b *pgQueryBuilder) where(cond string)      { b.wheres = append(b.wheres, cond) }

// addArgs appends args and returns a comma-separated $N placeholder string.
func (b *pgQueryBuilder) addArgs(vals ...interface{}) string {
	start := len(b.args) + 1
	placeholders := make([]string, len(vals))
	for i, v := range vals {
		b.args = append(b.args, v)
		placeholders[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(placeholders, ",")
}

func (b *pgQueryBuilder) build() (string, []interface{}) {
	q := "SELECT " + b.cols + " FROM " + b.fromClause
	if len(b.wheres) > 0 {
		q += " WHERE " + strings.Join(b.wheres, " AND ")
	}
	return q, b.args
}

// Ensure pgx is imported when the build tag is active.
var _ = pgx.ErrNoRows
