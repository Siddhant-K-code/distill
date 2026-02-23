package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/compress"
	distillmath "github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/types"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite.
// Single connection (SetMaxOpenConns(1)) - SQLite handles serialization.
type SQLiteStore struct {
	db  *sql.DB
	cfg Config
}

// NewSQLiteStore creates a new SQLite-backed session store.
func NewSQLiteStore(dsn string, cfg Config) (*SQLiteStore, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
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
	CREATE TABLE IF NOT EXISTS sessions (
		id               TEXT PRIMARY KEY,
		max_tokens       INTEGER NOT NULL,
		dedup_threshold  REAL NOT NULL DEFAULT 0.15,
		preserve_recent  INTEGER NOT NULL DEFAULT 10,
		created_at       TEXT NOT NULL,
		updated_at       TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS session_entries (
		id               TEXT PRIMARY KEY,
		session_id       TEXT NOT NULL,
		role             TEXT NOT NULL DEFAULT '',
		content          TEXT NOT NULL,
		original_content TEXT NOT NULL,
		source           TEXT DEFAULT '',
		embedding        BLOB,
		importance       REAL NOT NULL DEFAULT 0.5,
		compression_level INTEGER NOT NULL DEFAULT 0,
		tokens           INTEGER NOT NULL DEFAULT 0,
		seq              INTEGER NOT NULL,
		created_at       TEXT NOT NULL,
		compressed_at    TEXT DEFAULT '',
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_entries_session ON session_entries(session_id);
	CREATE INDEX IF NOT EXISTS idx_entries_seq ON session_entries(session_id, seq);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Create creates a new session.
func (s *SQLiteStore) Create(ctx context.Context, req CreateRequest) (*Session, error) {
	id := req.SessionID
	if id == "" {
		id = generateID()
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = s.cfg.DefaultMaxTokens
	}

	threshold := req.DedupThreshold
	if threshold <= 0 {
		threshold = s.cfg.DefaultDedupThreshold
	}

	preserveRecent := req.PreserveRecent
	if preserveRecent <= 0 {
		preserveRecent = s.cfg.DefaultPreserveRecent
	}

	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, max_tokens, dedup_threshold, preserve_recent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, maxTokens, threshold, preserveRecent, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, ErrSessionExists
		}
		return nil, fmt.Errorf("insert session: %w", err)
	}

	return &Session{
		ID:            id,
		MaxTokens:     maxTokens,
		CurrentTokens: 0,
		EntryCount:    0,
		CreatedAt:     nowTime,
		UpdatedAt:     nowTime,
	}, nil
}

// Push adds entries to a session with dedup and budget enforcement.
func (s *SQLiteStore) Push(ctx context.Context, req PushRequest) (*PushResult, error) {
	// Load session config
	sess, err := s.loadSessionConfig(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	result := &PushResult{SessionID: req.SessionID}

	// Get current max seq
	var maxSeq int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) FROM session_entries WHERE session_id = ?",
		req.SessionID,
	).Scan(&maxSeq)

	for _, entry := range req.Entries {
		if entry.Content == "" {
			continue
		}

		importance := entry.Importance
		if importance <= 0 {
			importance = 0.5
		}

		// Check for duplicates
		if len(entry.Embedding) > 0 {
			isDup, err := s.isDuplicate(ctx, req.SessionID, entry.Embedding, sess.dedupThreshold)
			if err != nil {
				return nil, fmt.Errorf("dedup check: %w", err)
			}
			if isDup {
				result.Deduplicated++
				continue
			}
		}

		tokens := estimateTokens(entry.Content)

		// Reject single entries that exceed the entire budget
		if tokens > sess.maxTokens {
			return nil, ErrOverBudget
		}

		maxSeq++
		id := generateID()
		now := time.Now().UTC().Format(time.RFC3339Nano)

		_, err := s.db.ExecContext(ctx,
			`INSERT INTO session_entries
			 (id, session_id, role, content, original_content, source, embedding, importance, compression_level, tokens, seq, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
			id, req.SessionID, entry.Role, entry.Content, entry.Content,
			entry.Source, encodeEmbedding(entry.Embedding), importance,
			tokens, maxSeq, now,
		)
		if err != nil {
			return nil, fmt.Errorf("insert entry: %w", err)
		}

		result.Accepted++
	}

	// Enforce token budget - loop until within budget or no progress
	for {
		c, e, err := s.enforceBudget(ctx, req.SessionID, sess)
		if err != nil {
			return nil, fmt.Errorf("enforce budget: %w", err)
		}
		result.Compressed += c
		result.Evicted += e
		if c == 0 && e == 0 {
			break // no progress possible
		}
	}

	// Update session timestamp
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx,
		"UPDATE sessions SET updated_at = ? WHERE id = ?",
		now, req.SessionID,
	)

	// Compute current tokens
	var currentTokens int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(tokens), 0) FROM session_entries WHERE session_id = ?",
		req.SessionID,
	).Scan(&currentTokens)

	result.CurrentTokens = currentTokens
	result.BudgetRemaining = sess.maxTokens - currentTokens

	return result, nil
}

// Context returns the current context window for a session.
func (s *SQLiteStore) Context(ctx context.Context, req ContextRequest) (*ContextResult, error) {
	// Verify session exists
	var exists int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE id = ?", req.SessionID,
	).Scan(&exists)
	if err != nil || exists == 0 {
		return nil, ErrSessionNotFound
	}

	query := "SELECT id, role, content, source, compression_level, tokens, created_at FROM session_entries WHERE session_id = ?"
	args := []interface{}{req.SessionID}

	if req.Role != "" {
		query += " AND role = ?"
		args = append(args, req.Role)
	}

	query += " ORDER BY seq ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}

	type rawEntry struct {
		id, role, content, source, createdAt string
		level, tokens                        int
	}
	var raw []rawEntry
	for rows.Next() {
		var r rawEntry
		if err := rows.Scan(&r.id, &r.role, &r.content, &r.source, &r.level, &r.tokens, &r.createdAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	now := time.Now()
	levels := make(map[int]int)
	var entries []ContextEntry
	tokenCount := 0

	for _, r := range raw {
		if req.MaxTokens > 0 && tokenCount+r.tokens > req.MaxTokens {
			break
		}

		created, _ := time.Parse(time.RFC3339Nano, r.createdAt)
		age := formatAge(now.Sub(created))

		entries = append(entries, ContextEntry{
			ID:      r.id,
			Role:    r.role,
			Content: r.content,
			Source:  r.source,
			Level:   CompressionLevel(r.level),
			Tokens:  r.tokens,
			Age:     age,
		})
		tokenCount += r.tokens
		levels[r.level]++
	}

	// Compute compression savings (original tokens - current tokens)
	var totalOriginalTokens int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(LENGTH(original_content)+3)/4, 0) FROM session_entries WHERE session_id = ?",
		req.SessionID,
	).Scan(&totalOriginalTokens)

	return &ContextResult{
		Entries: entries,
		Stats: ContextStats{
			TotalEntries:       len(entries),
			TotalTokens:        tokenCount,
			CompressionLevels:  levels,
			CompressionSavings: totalOriginalTokens - tokenCount,
		},
	}, nil
}

// Get returns session metadata.
func (s *SQLiteStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	var sess Session
	var createdStr, updatedStr string

	err := s.db.QueryRowContext(ctx,
		"SELECT id, max_tokens, created_at, updated_at FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&sess.ID, &sess.MaxTokens, &createdStr, &updatedStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)

	_ = s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(tokens), 0), COUNT(*) FROM session_entries WHERE session_id = ?",
		sessionID,
	).Scan(&sess.CurrentTokens, &sess.EntryCount)

	return &sess, nil
}

// Delete removes a session and all its entries.
func (s *SQLiteStore) Delete(ctx context.Context, sessionID string) (*DeleteResult, error) {
	var count int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM session_entries WHERE session_id = ?",
		sessionID,
	).Scan(&count)

	res, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", sessionID)
	if err != nil {
		return nil, fmt.Errorf("delete session: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrSessionNotFound
	}

	return &DeleteResult{
		SessionID:      sessionID,
		EntriesRemoved: count,
	}, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- internal ---

type sessionConfig struct {
	maxTokens      int
	dedupThreshold float64
	preserveRecent int
}

func (s *SQLiteStore) loadSessionConfig(ctx context.Context, sessionID string) (*sessionConfig, error) {
	var cfg sessionConfig
	err := s.db.QueryRowContext(ctx,
		"SELECT max_tokens, dedup_threshold, preserve_recent FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&cfg.maxTokens, &cfg.dedupThreshold, &cfg.preserveRecent)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &cfg, nil
}

// isDuplicate checks if an embedding is within threshold of any existing entry.
//
// TODO: Full table scan (O(n) per entry). Fine for typical session sizes
// (< 1K entries). For larger sessions, consider caching embeddings in memory.
func (s *SQLiteStore) isDuplicate(ctx context.Context, sessionID string, embedding []float32, threshold float64) (bool, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT embedding FROM session_entries WHERE session_id = ? AND embedding IS NOT NULL",
		sessionID,
	)
	if err != nil {
		return false, err
	}

	// Scan all then close - single connection pattern.
	var blobs [][]byte
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			_ = rows.Close()
			return false, err
		}
		blobs = append(blobs, blob)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	_ = rows.Close()

	for _, blob := range blobs {
		existing := decodeEmbedding(blob)
		if len(existing) == 0 {
			continue
		}
		dist := distillmath.CosineDistance(embedding, existing)
		if dist < threshold {
			return true, nil
		}
	}
	return false, nil
}

// compressor reused across calls.
var compressor = compress.NewExtractiveCompressor()

// enforceBudget compresses and evicts entries until within token budget.
// Returns (compressed count, evicted count).
func (s *SQLiteStore) enforceBudget(ctx context.Context, sessionID string, cfg *sessionConfig) (int, int, error) {
	var currentTokens int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(tokens), 0) FROM session_entries WHERE session_id = ?",
		sessionID,
	).Scan(&currentTokens)

	if currentTokens <= cfg.maxTokens {
		return 0, 0, nil
	}

	compressed := 0
	evicted := 0

	// Get total entry count to determine which are "recent"
	var totalEntries int
	_ = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM session_entries WHERE session_id = ?",
		sessionID,
	).Scan(&totalEntries)

	// Load compressible entries (oldest first, skip the most recent N)
	// We need entries ordered by seq, and we skip the last preserveRecent.
	limit := totalEntries - cfg.preserveRecent
	if limit <= 0 {
		// All entries are "recent" - nothing to compress, but still over budget.
		// Evict the oldest non-recent entry as a last resort.
		return s.evictOldest(ctx, sessionID, cfg, currentTokens)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, original_content, compression_level, importance, tokens
		 FROM session_entries WHERE session_id = ?
		 ORDER BY seq ASC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return 0, 0, err
	}

	var candidates []compressCandidate
	for rows.Next() {
		var c compressCandidate
		if err := rows.Scan(&c.id, &c.originalContent, &c.level, &c.importance, &c.tokens); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	_ = rows.Close()

	// Process candidates from oldest, lowest importance first.
	// Sort: by importance ASC, then by position (already ordered by seq ASC).
	sortCandidates(candidates)

	for _, c := range candidates {
		if currentTokens <= cfg.maxTokens {
			break
		}

		nextLevel := c.level + 1

		if nextLevel > int(LevelKeywords) {
			// Already at keywords - evict
			_, err := s.db.ExecContext(ctx,
				"DELETE FROM session_entries WHERE id = ?", c.id,
			)
			if err != nil {
				return compressed, evicted, err
			}
			currentTokens -= c.tokens
			evicted++
			continue
		}

		// Compress to next level
		newContent := compressToLevel(c.originalContent, CompressionLevel(nextLevel))
		newTokens := estimateTokens(newContent)
		now := time.Now().UTC().Format(time.RFC3339Nano)

		_, err := s.db.ExecContext(ctx,
			`UPDATE session_entries SET content = ?, compression_level = ?, tokens = ?, compressed_at = ? WHERE id = ?`,
			newContent, nextLevel, newTokens, now, c.id,
		)
		if err != nil {
			return compressed, evicted, err
		}

		currentTokens -= (c.tokens - newTokens)
		compressed++
	}

	return compressed, evicted, nil
}

// evictOldest is a fallback when all entries are "recent" but still over budget.
func (s *SQLiteStore) evictOldest(ctx context.Context, sessionID string, cfg *sessionConfig, currentTokens int) (int, int, error) {
	evicted := 0
	for currentTokens > cfg.maxTokens {
		var id string
		var tokens int
		err := s.db.QueryRowContext(ctx,
			"SELECT id, tokens FROM session_entries WHERE session_id = ? ORDER BY seq ASC LIMIT 1",
			sessionID,
		).Scan(&id, &tokens)
		if err != nil {
			break
		}
		_, _ = s.db.ExecContext(ctx, "DELETE FROM session_entries WHERE id = ?", id)
		currentTokens -= tokens
		evicted++
	}
	return 0, evicted, nil
}

// compressToLevel applies compression for the given level.
func compressToLevel(text string, level CompressionLevel) string {
	switch level {
	case LevelSummary:
		// Use extractive compressor to keep ~20%
		chunks := []types.Chunk{{ID: "sess", Text: text}}
		opts := compress.Options{TargetReduction: 0.2, MinChunkLength: 20}
		result, _, _ := compressor.Compress(context.Background(), chunks, opts)
		if len(result) > 0 && result[0].Text != "" {
			return result[0].Text
		}
		return text
	case LevelSentence:
		// Keep first sentence only
		for i, r := range text {
			if r == '.' || r == '!' || r == '?' {
				return text[:i+1]
			}
		}
		// No sentence boundary - truncate at word boundary near 50 chars
		if len(text) > 50 {
			cut := 50
			for cut > 0 && text[cut] != ' ' {
				cut--
			}
			if cut == 0 {
				cut = 50 // no space found, hard cut
			}
			return strings.TrimSpace(text[:cut]) + "..."
		}
		return text
	case LevelKeywords:
		return extractKeywords(text)
	default:
		return text
	}
}

// extractKeywords produces a keyword-only representation.
func extractKeywords(text string) string {
	words := strings.Fields(text)
	seen := make(map[string]bool)
	var keywords []string

	for _, w := range words {
		lower := strings.ToLower(strings.Trim(w, ".,;:!?\"'()[]{}"))
		if lower == "" || len(lower) < 4 || stopWords[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, lower)
	}

	if len(keywords) > 15 {
		keywords = keywords[:15]
	}
	return strings.Join(keywords, ", ")
}

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

// compressCandidate is an entry eligible for compression or eviction.
type compressCandidate struct {
	id              string
	originalContent string
	level           int
	importance      float64
	tokens          int
}

// sortCandidates sorts by importance ASC (least important first).
func sortCandidates(c []compressCandidate) {
	sort.Slice(c, func(i, j int) bool {
		return c[i].importance < c[j].importance
	})
}

// --- helpers ---

func generateID() string {
	b := make([]byte, 12)
	ts := uint32(time.Now().Unix())
	b[0] = byte(ts >> 24)
	b[1] = byte(ts >> 16)
	b[2] = byte(ts >> 8)
	b[3] = byte(ts)
	_, _ = rand.Read(b[4:])
	return hex.EncodeToString(b)
}

func encodeEmbedding(emb []float32) []byte {
	if len(emb) == 0 {
		return nil
	}
	buf := make([]byte, len(emb)*4)
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeEmbedding(buf []byte) []float32 {
	if len(buf) == 0 || len(buf)%4 != 0 {
		return nil
	}
	emb := make([]float32, len(buf)/4)
	for i := range emb {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return emb
}

func estimateTokens(text string) int {
	return (len(text) + 3) / 4
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
