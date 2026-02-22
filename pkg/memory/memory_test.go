package memory

import (
	"context"
	"math"
	"testing"
	"time"
)

// makeEmbedding creates a simple unit vector for testing.
// angle controls the direction in the first two dimensions.
func makeEmbedding(angle float64, dim int) []float32 {
	emb := make([]float32, dim)
	emb[0] = float32(math.Cos(angle))
	emb[1] = float32(math.Sin(angle))
	return emb
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	cfg := DefaultConfig()
	cfg.DedupThreshold = 0.15
	s, err := NewSQLiteStore(":memory:", cfg)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStoreAndRecall(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Store two distinct entries
	result, err := s.Store(ctx, StoreRequest{
		SessionID: "test-session",
		Entries: []StoreEntry{
			{Text: "The auth service uses JWT with RS256", Embedding: makeEmbedding(0, 8), Source: "code_review", Tags: []string{"auth"}},
			{Text: "The payment service uses Stripe API", Embedding: makeEmbedding(math.Pi/2, 8), Source: "docs", Tags: []string{"payments"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Stored != 2 {
		t.Errorf("expected 2 stored, got %d", result.Stored)
	}
	if result.TotalMemories != 2 {
		t.Errorf("expected 2 total, got %d", result.TotalMemories)
	}

	// Recall with embedding similar to auth entry
	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "How does authentication work?",
		QueryEmbedding: makeEmbedding(0.05, 8), // Very close to auth entry
		MaxResults:     5,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Memories) == 0 {
		t.Fatal("expected at least 1 memory")
	}
	// Auth entry should be most relevant (closest embedding)
	if recall.Memories[0].Source != "code_review" {
		t.Errorf("expected auth entry first, got source=%s", recall.Memories[0].Source)
	}
}

func TestWriteTimeDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	emb := makeEmbedding(0, 8)

	// Store first entry
	r1, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "JWT uses RS256 for signing", Embedding: emb, Source: "docs"},
		},
	})
	if err != nil {
		t.Fatalf("Store 1: %v", err)
	}
	if r1.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", r1.Stored)
	}

	// Store near-duplicate (same embedding)
	r2, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth tokens are signed with RS256", Embedding: emb, Source: "code"},
		},
	})
	if err != nil {
		t.Fatalf("Store 2: %v", err)
	}
	if r2.Deduplicated != 1 {
		t.Errorf("expected 1 deduplicated, got %d", r2.Deduplicated)
	}
	if r2.Stored != 0 {
		t.Errorf("expected 0 stored, got %d", r2.Stored)
	}
	if r2.TotalMemories != 1 {
		t.Errorf("expected 1 total, got %d", r2.TotalMemories)
	}
}

func TestForget(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Old deprecated info", Tags: []string{"deprecated"}},
			{Text: "Current auth info", Tags: []string{"auth"}},
			{Text: "Another deprecated item", Tags: []string{"deprecated"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Forget by tag
	result, err := s.Forget(ctx, ForgetRequest{Tags: []string{"deprecated"}})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.Removed != 2 {
		t.Errorf("expected 2 removed, got %d", result.Removed)
	}
	if result.TotalMemories != 1 {
		t.Errorf("expected 1 remaining, got %d", result.TotalMemories)
	}
}

func TestForgetByAge(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert an entry with a manually backdated created_at
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, text, source, metadata, decay_level, created_at, last_referenced, access_count)
		 VALUES (?, ?, '', '{}', 0, ?, ?, 0)`,
		"old-1", "Old memory", old, old,
	)
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}

	_, err = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Recent memory"}},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Forget entries older than 24h
	result, err := s.Forget(ctx, ForgetRequest{
		OlderThan: now.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Entry from code review", Source: "code_review"},
			{Text: "Entry from docs", Source: "docs"},
			{Text: "Another code review entry", Source: "code_review"},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalMemories != 3 {
		t.Errorf("expected 3 total, got %d", stats.TotalMemories)
	}
	if stats.BySource["code_review"] != 2 {
		t.Errorf("expected 2 code_review, got %d", stats.BySource["code_review"])
	}
	if stats.BySource["docs"] != 1 {
		t.Errorf("expected 1 docs, got %d", stats.BySource["docs"])
	}
}

func TestRecallWithTokenBudget(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Store entries with embeddings at different angles
	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Short entry about auth", Embedding: makeEmbedding(0, 8)},
			{Text: "This is a much longer entry about authentication that contains many more tokens and details about how the JWT system works with RS256 signing", Embedding: makeEmbedding(0.1, 8)},
			{Text: "Another auth entry", Embedding: makeEmbedding(0.2, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Recall with a tight token budget
	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "auth",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxTokens:      20, // Very tight budget
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if recall.Stats.TokenCount > 20 {
		t.Errorf("expected token count <= 20, got %d", recall.Stats.TokenCount)
	}
}

func TestRecallWithTagFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses JWT", Embedding: makeEmbedding(0, 8), Tags: []string{"auth"}},
			{Text: "Payments use Stripe", Embedding: makeEmbedding(math.Pi/2, 8), Tags: []string{"payments"}},
			{Text: "Auth also uses OAuth", Embedding: makeEmbedding(math.Pi, 8), Tags: []string{"auth"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "how does it work",
		QueryEmbedding: makeEmbedding(0, 8),
		Tags:           []string{"auth"},
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Memories) != 2 {
		t.Errorf("expected 2 auth memories, got %d", len(recall.Memories))
	}
	for _, m := range recall.Memories {
		found := false
		for _, tag := range m.Tags {
			if tag == "auth" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected auth tag, got tags=%v", m.Tags)
		}
	}
}

func TestDecayWorker(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SummaryAge = 1 * time.Millisecond  // Near-immediate decay for testing
	cfg.KeywordsAge = 1 * time.Millisecond // Near-immediate decay for testing
	cfg.EvictAge = 0                       // Disable eviction for this test
	cfg.DedupThreshold = 0.15

	s, err := NewSQLiteStore(":memory:", cfg)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Store a multi-sentence entry
	_, err = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "The authentication service uses JWT tokens with RS256 signing. It validates tokens on every request. The token expiry is set to 24 hours. Refresh tokens are stored in Redis with a 7-day TTL. The service also supports OAuth2 for third-party integrations."},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Backdate the entry so it qualifies for decay
	past := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx, "UPDATE memories SET last_referenced = ?", past)

	// Run decay
	w := NewDecayWorker(s, cfg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Check that the entry was compressed to summary
	stats, _ := s.Stats(ctx)
	if stats.ByDecayLevel[int(DecaySummary)] != 1 {
		t.Errorf("expected 1 summary-level memory, got decay levels: %v", stats.ByDecayLevel)
	}

	// Run decay again - should compress to keywords
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce 2: %v", err)
	}

	stats, _ = s.Stats(ctx)
	if stats.ByDecayLevel[int(DecayKeywords)] != 1 {
		t.Errorf("expected 1 keywords-level memory, got decay levels: %v", stats.ByDecayLevel)
	}
}

func TestEmbeddingRoundtrip(t *testing.T) {
	original := []float32{0.1, 0.2, 0.3, -0.5, 1.0}
	encoded := encodeEmbedding(original)
	decoded := decodeEmbedding(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("index %d: expected %f, got %f", i, original[i], decoded[i])
		}
	}
}

func TestEmptyStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 total, got %d", stats.TotalMemories)
	}
}

func TestStoreEmptyText(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: ""},
			{Text: "Valid entry"},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Stored != 1 {
		t.Errorf("expected 1 stored (empty skipped), got %d", result.Stored)
	}
}
