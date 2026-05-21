//go:build postgres

package memory

import (
	"context"
	"math"
	"os"
	"testing"
	"time"
)

// Compile-time assertion that PostgresStore satisfies the Store interface.
var _ Store = (*PostgresStore)(nil)

func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	return newTestPostgresStoreWithConfig(t, func(cfg *Config) {})
}

func newTestPostgresStoreWithConfig(t *testing.T, modify func(*Config)) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set — skipping postgres tests")
	}

	cfg := DefaultConfig()
	cfg.DedupThreshold = 0.15
	cfg.DecayEnabled = false // tests call runDecaySweep directly; avoid background worker
	modify(&cfg)

	ps, err := NewPostgresStore(dsn, cfg)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = ps.dbPool.Exec(ctx, "TRUNCATE memories, memory_tags")
		_ = ps.Close()
	})
	return ps
}

// ---------------------------------------------------------------------------
// Core store/recall tests
// ---------------------------------------------------------------------------

func TestStoreAndRecall_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	result, err := ps.Store(ctx, StoreRequest{
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

	recall, err := ps.Recall(ctx, RecallRequest{
		Query:          "How does authentication work?",
		QueryEmbedding: makeEmbedding(0.05, 8),
		MaxResults:     5,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Memories) == 0 {
		t.Fatal("expected at least 1 memory")
	}
	if recall.Memories[0].Source != "code_review" {
		t.Errorf("expected auth entry first, got source=%s", recall.Memories[0].Source)
	}
}

func TestWriteTimeDedup_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	emb := makeEmbedding(0, 8)

	r1, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "JWT uses RS256 for signing", Embedding: emb, Source: "docs"}},
	})
	if err != nil {
		t.Fatalf("Store 1: %v", err)
	}
	if r1.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", r1.Stored)
	}

	r2, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Auth tokens are signed with RS256", Embedding: emb, Source: "code"}},
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

func TestForget_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Old deprecated info", Tags: []string{"deprecated"}},
			{Text: "Current auth info", Tags: []string{"auth"}},
			{Text: "Another deprecated item", Tags: []string{"deprecated"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	result, err := ps.Forget(ctx, ForgetRequest{Tags: []string{"deprecated"}})
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

func TestForgetByAge_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	_, err := ps.dbPool.Exec(ctx,
		`INSERT INTO memories (id, text, source, metadata, decay_level, created_at, last_referenced, access_count)
		 VALUES ($1, $2, '', '{}', 0, $3, $4, 0)`,
		"old-1", "Old memory", old, old,
	)
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}

	_, err = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Recent memory"}},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	result, err := ps.Forget(ctx, ForgetRequest{
		OlderThan: now.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}
}

func TestStats_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Entry from code review", Source: "code_review"},
			{Text: "Entry from docs", Source: "docs"},
			{Text: "Another code review entry", Source: "code_review"},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	stats, err := ps.Stats(ctx)
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

func TestRecallWithTokenBudget_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Short entry about auth", Embedding: makeEmbedding(0, 8)},
			{Text: "This is a much longer entry about authentication that contains many more tokens and details about how the JWT system works with RS256 signing", Embedding: makeEmbedding(0.1, 8)},
			{Text: "Another auth entry", Embedding: makeEmbedding(0.2, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, err := ps.Recall(ctx, RecallRequest{
		Query:          "auth",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxTokens:      20,
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if recall.Stats.TokenCount > 20 {
		t.Errorf("expected token count <= 20, got %d", recall.Stats.TokenCount)
	}
}

func TestRecallWithTagFilter_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses JWT", Embedding: makeEmbedding(0, 8), Tags: []string{"auth"}},
			{Text: "Payments use Stripe", Embedding: makeEmbedding(math.Pi/2, 8), Tags: []string{"payments"}},
			{Text: "Auth also uses OAuth", Embedding: makeEmbedding(math.Pi, 8), Tags: []string{"auth"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, err := ps.Recall(ctx, RecallRequest{
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

func TestEmptyStore_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	stats, err := ps.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 total, got %d", stats.TotalMemories)
	}
}

func TestStoreEmptyText_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	result, err := ps.Store(ctx, StoreRequest{
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

func TestRecall_CacheBoundaryHint_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "The auth service uses JWT with RS256 signing algorithm", Embedding: makeEmbedding(0, 8)},
			{Text: "Payment service integrates with Stripe for billing", Embedding: makeEmbedding(math.Pi/2, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	result, err := ps.Recall(ctx, RecallRequest{
		Query:          "auth JWT",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     5,
		RecencyWeight:  0.1,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if result.CacheHint == nil {
		t.Fatal("expected CacheBoundaryHint, got nil")
	}
	if len(result.CacheHint.StableEntryIDs) == 0 {
		t.Error("expected at least one stable entry ID in hint")
	}
	if result.CacheHint.ConfidenceScore <= 0 {
		t.Error("expected positive confidence score")
	}
}

// ---------------------------------------------------------------------------
// Decay worker tests
// ---------------------------------------------------------------------------

func TestDecayWorker_Postgres(t *testing.T) {
	ps := newTestPostgresStoreWithConfig(t, func(cfg *Config) {
		cfg.SummaryAge = 1 * time.Millisecond
		cfg.KeywordsAge = 1 * time.Millisecond
		cfg.EvictAge = 0
	})
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "The authentication service uses JWT tokens with RS256 signing. It validates tokens on every request. The token expiry is set to 24 hours. Refresh tokens are stored in Redis with a 7-day TTL. The service also supports OAuth2 for third-party integrations."},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	past := time.Now().Add(-48 * time.Hour).UTC()
	_, _ = ps.dbPool.Exec(ctx, "UPDATE memories SET last_referenced = $1", past)

	ps.runDecaySweep(ctx)

	stats, _ := ps.Stats(ctx)
	if stats.ByDecayLevel[int(DecaySummary)] != 1 {
		t.Errorf("expected 1 summary-level memory, got decay levels: %v", stats.ByDecayLevel)
	}

	ps.runDecaySweep(ctx)

	stats, _ = ps.Stats(ctx)
	if stats.ByDecayLevel[int(DecayKeywords)] != 1 {
		t.Errorf("expected 1 keywords-level memory, got decay levels: %v", stats.ByDecayLevel)
	}
}

func TestLifecycleEvents_Compression_Postgres(t *testing.T) {
	ps := newTestPostgresStoreWithConfig(t, func(cfg *Config) {
		cfg.SummaryAge = 1 * time.Millisecond
		cfg.KeywordsAge = 1 * time.Millisecond
		cfg.EvictAge = 0
	})
	ctx := context.Background()

	var events []MemoryEvent
	ps.OnLifecycleEvent(func(e MemoryEvent) {
		events = append(events, e)
	})

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "The authentication service uses JWT tokens with RS256 signing. It validates tokens on every request. The token expiry is set to 24 hours."},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	past := time.Now().Add(-48 * time.Hour).UTC()
	_, _ = ps.dbPool.Exec(ctx, "UPDATE memories SET last_referenced = $1", past)

	ps.runDecaySweep(ctx)

	if len(events) == 0 {
		t.Fatal("expected at least one lifecycle event, got none")
	}
	if events[0].Type != EventCompressed {
		t.Errorf("expected EventCompressed, got %s", events[0].Type)
	}
	if events[0].TokensBefore <= events[0].TokensAfter {
		t.Errorf("expected TokensBefore > TokensAfter, got %d <= %d",
			events[0].TokensBefore, events[0].TokensAfter)
	}
	if events[0].CompressionLevel != DecaySummary {
		t.Errorf("expected DecaySummary, got %d", events[0].CompressionLevel)
	}
}

func TestLifecycleEvents_Eviction_Postgres(t *testing.T) {
	ps := newTestPostgresStoreWithConfig(t, func(cfg *Config) {
		cfg.SummaryAge = 0
		cfg.KeywordsAge = 0
		cfg.EvictAge = 1 * time.Millisecond
	})
	ctx := context.Background()

	var events []MemoryEvent
	ps.OnLifecycleEvent(func(e MemoryEvent) {
		events = append(events, e)
	})

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Old keywords-level memory that should be evicted soon."},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	past := time.Now().Add(-48 * time.Hour).UTC()
	_, _ = ps.dbPool.Exec(ctx,
		"UPDATE memories SET decay_level = $1, last_referenced = $2",
		int(DecayKeywords), past,
	)

	ps.runDecaySweep(ctx)

	if len(events) == 0 {
		t.Fatal("expected eviction event, got none")
	}
	if events[0].Type != EventEvicted {
		t.Errorf("expected EventEvicted, got %s", events[0].Type)
	}
	if events[0].TokensAfter != 0 {
		t.Errorf("expected TokensAfter=0 for eviction, got %d", events[0].TokensAfter)
	}
}

// ---------------------------------------------------------------------------
// Expire / Supersede tests
// ---------------------------------------------------------------------------

func TestExpire_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Decision: use Postgres for persistence", Embedding: makeEmbedding(0, 8), Tags: []string{"arch"}},
			{Text: "Auth uses JWT with RS256", Embedding: makeEmbedding(1, 8), Tags: []string{"auth"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := ps.Recall(ctx, RecallRequest{
		Query: "all", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if len(recall.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(recall.Memories))
	}

	expResult, err := ps.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if expResult.Expired != 1 {
		t.Errorf("expected 1 expired, got %d", expResult.Expired)
	}

	recall2, _ := ps.Recall(ctx, RecallRequest{
		Query: "all", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if len(recall2.Memories) != 1 {
		t.Errorf("expected 1 active memory after expire, got %d", len(recall2.Memories))
	}

	recall3, _ := ps.Recall(ctx, RecallRequest{
		Query: "all", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
		IncludeExpired: true,
	})
	if len(recall3.Memories) != 2 {
		t.Errorf("expected 2 memories with IncludeExpired, got %d", len(recall3.Memories))
	}
}

func TestExpire_AlreadyExpired_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Some fact"}},
	})

	recall, _ := ps.Recall(ctx, RecallRequest{Query: "fact", MaxResults: 1})
	id := recall.Memories[0].ID

	r1, _ := ps.Expire(ctx, ExpireRequest{IDs: []string{id}})
	if r1.Expired != 1 {
		t.Errorf("first expire: expected 1, got %d", r1.Expired)
	}

	r2, _ := ps.Expire(ctx, ExpireRequest{IDs: []string{id}})
	if r2.Expired != 0 {
		t.Errorf("second expire: expected 0, got %d", r2.Expired)
	}
}

func TestExpire_EmptyRequest_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	result, err := ps.Expire(ctx, ExpireRequest{})
	if err != nil {
		t.Fatalf("Expire empty: %v", err)
	}
	if result.Expired != 0 {
		t.Errorf("expected 0 expired, got %d", result.Expired)
	}
}

func TestExpire_LifecycleEvent_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	var events []MemoryEvent
	ps.OnLifecycleEvent(func(e MemoryEvent) {
		events = append(events, e)
	})

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Will be expired"}},
	})
	recall, _ := ps.Recall(ctx, RecallRequest{Query: "expired", MaxResults: 1})

	_, _ = ps.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	if len(events) == 0 {
		t.Fatal("expected lifecycle event for expire")
	}
	if events[0].Type != EventExpired {
		t.Errorf("expected EventExpired, got %s", events[0].Type)
	}
}

func TestExpire_DedupSkipsExpired_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	emb := makeEmbedding(0, 8)

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Original fact", Embedding: emb}},
	})
	recall, _ := ps.Recall(ctx, RecallRequest{Query: "fact", QueryEmbedding: emb, MaxResults: 1})
	_, _ = ps.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	result, err := ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Updated fact", Embedding: emb}},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Stored != 1 {
		t.Errorf("expected 1 stored (expired entry should not dedup), got %d", result.Stored)
	}
	if result.Deduplicated != 0 {
		t.Errorf("expected 0 deduplicated, got %d", result.Deduplicated)
	}
}

func TestSupersede_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	oldEmb := makeEmbedding(0, 8)
	newEmb := makeEmbedding(1.5, 8)

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use MySQL for persistence", Embedding: oldEmb, Tags: []string{"arch"}},
		},
	})

	recall, _ := ps.Recall(ctx, RecallRequest{
		Query: "persistence", QueryEmbedding: oldEmb, MaxResults: 1,
	})
	oldID := recall.Memories[0].ID

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use Postgres for persistence (decision reversed)", Embedding: newEmb, Tags: []string{"arch"}},
		},
	})

	recall2, _ := ps.Recall(ctx, RecallRequest{
		Query: "persistence", QueryEmbedding: newEmb, MaxResults: 10,
	})
	var newID string
	for _, m := range recall2.Memories {
		if m.ID != oldID {
			newID = m.ID
			break
		}
	}
	if newID == "" {
		t.Fatal("could not find new entry ID")
	}

	supResult, err := ps.Supersede(ctx, SupersedeRequest{OldID: oldID, NewID: newID})
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if !supResult.Superseded {
		t.Error("expected Superseded=true")
	}

	recall3, _ := ps.Recall(ctx, RecallRequest{
		Query: "persistence", QueryEmbedding: newEmb, MaxResults: 10,
	})
	if len(recall3.Memories) != 1 {
		t.Fatalf("expected 1 memory after supersede, got %d", len(recall3.Memories))
	}
	if recall3.Memories[0].ID != newID {
		t.Errorf("expected new entry %s, got %s", newID, recall3.Memories[0].ID)
	}
}

func TestSupersede_NotFound_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Supersede(ctx, SupersedeRequest{OldID: "nonexistent", NewID: "also-nonexistent"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSupersede_AlreadyExpired_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Old decision"}},
	})
	recall, _ := ps.Recall(ctx, RecallRequest{Query: "decision", MaxResults: 1})
	id := recall.Memories[0].ID

	_, _ = ps.Expire(ctx, ExpireRequest{IDs: []string{id}})

	_, err := ps.Supersede(ctx, SupersedeRequest{OldID: id, NewID: "new-id"})
	if err != ErrAlreadyExpired {
		t.Errorf("expected ErrAlreadyExpired, got %v", err)
	}
}

func TestSupersede_EmptyOldID_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, err := ps.Supersede(ctx, SupersedeRequest{OldID: "", NewID: "new"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for empty OldID, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TTL tests
// ---------------------------------------------------------------------------

func TestStoreWithTTL_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	pastExpiry := time.Now().Add(-1 * time.Hour)
	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Already expired by TTL", ExpiresAt: &pastExpiry},
			{Text: "Still valid"},
		},
	})

	recall, _ := ps.Recall(ctx, RecallRequest{Query: "entry", MaxResults: 10})
	if len(recall.Memories) != 1 {
		t.Errorf("expected 1 active memory (TTL-expired excluded), got %d", len(recall.Memories))
	}
	if recall.Memories[0].Text != "Still valid" {
		t.Errorf("expected 'Still valid', got %q", recall.Memories[0].Text)
	}
}

func TestStoreWithFutureTTL_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	futureExpiry := time.Now().Add(24 * time.Hour)
	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Valid for 24h", ExpiresAt: &futureExpiry},
		},
	})

	recall, _ := ps.Recall(ctx, RecallRequest{Query: "valid", MaxResults: 10})
	if len(recall.Memories) != 1 {
		t.Errorf("expected 1 memory with future TTL, got %d", len(recall.Memories))
	}
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestStats_IncludesExpiredCount_Postgres(t *testing.T) {
	ps := newTestPostgresStore(t)
	ctx := context.Background()

	_, _ = ps.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Active entry"},
			{Text: "Will expire"},
		},
	})

	recall, _ := ps.Recall(ctx, RecallRequest{Query: "expire", MaxResults: 10})
	_, _ = ps.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	stats, err := ps.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalMemories != 2 {
		t.Errorf("expected 2 total, got %d", stats.TotalMemories)
	}
	if stats.ExpiredCount != 1 {
		t.Errorf("expected 1 expired, got %d", stats.ExpiredCount)
	}
	if stats.ActiveCount != 1 {
		t.Errorf("expected 1 active, got %d", stats.ActiveCount)
	}
}
