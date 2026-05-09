package memory

import (
	"context"
	"testing"
	"time"
)

func TestExpire(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Decision: use Postgres for persistence", Embedding: makeEmbedding(0, 8), Tags: []string{"arch"}},
			{Text: "Auth uses JWT with RS256", Embedding: makeEmbedding(1, 8), Tags: []string{"auth"}},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Get the IDs
	recall, _ := s.Recall(ctx, RecallRequest{
		Query:          "all",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
	})
	if len(recall.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(recall.Memories))
	}

	// Expire the first entry
	expResult, err := s.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if expResult.Expired != 1 {
		t.Errorf("expected 1 expired, got %d", expResult.Expired)
	}

	// Recall should now return only 1 entry
	recall2, _ := s.Recall(ctx, RecallRequest{
		Query:          "all",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
	})
	if len(recall2.Memories) != 1 {
		t.Errorf("expected 1 active memory after expire, got %d", len(recall2.Memories))
	}

	// Recall with IncludeExpired should return both
	recall3, _ := s.Recall(ctx, RecallRequest{
		Query:          "all",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
		IncludeExpired: true,
	})
	if len(recall3.Memories) != 2 {
		t.Errorf("expected 2 memories with IncludeExpired, got %d", len(recall3.Memories))
	}
}

func TestExpire_AlreadyExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Some fact"}},
	})

	recall, _ := s.Recall(ctx, RecallRequest{Query: "fact", MaxResults: 1})
	id := recall.Memories[0].ID

	// Expire once
	r1, _ := s.Expire(ctx, ExpireRequest{IDs: []string{id}})
	if r1.Expired != 1 {
		t.Errorf("first expire: expected 1, got %d", r1.Expired)
	}

	// Expire again — should affect 0 rows
	r2, _ := s.Expire(ctx, ExpireRequest{IDs: []string{id}})
	if r2.Expired != 0 {
		t.Errorf("second expire: expected 0, got %d", r2.Expired)
	}
}

func TestSupersede(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Use distinct embeddings so dedup doesn't merge them
	oldEmb := makeEmbedding(0, 8)
	newEmb := makeEmbedding(1.5, 8) // far enough apart to avoid dedup

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use MySQL for persistence", Embedding: oldEmb, Tags: []string{"arch"}},
		},
	})

	// Get the old entry ID
	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "persistence", QueryEmbedding: oldEmb, MaxResults: 1,
	})
	oldID := recall.Memories[0].ID

	// Store the replacement with a distinct embedding
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use Postgres for persistence (decision reversed)", Embedding: newEmb, Tags: []string{"arch"}},
		},
	})

	recall2, _ := s.Recall(ctx, RecallRequest{
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

	// Supersede old with new
	supResult, err := s.Supersede(ctx, SupersedeRequest{OldID: oldID, NewID: newID})
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if !supResult.Superseded {
		t.Error("expected Superseded=true")
	}

	// Recall should only return the new entry
	recall3, _ := s.Recall(ctx, RecallRequest{
		Query: "persistence", QueryEmbedding: newEmb, MaxResults: 10,
	})
	if len(recall3.Memories) != 1 {
		t.Fatalf("expected 1 memory after supersede, got %d", len(recall3.Memories))
	}
	if recall3.Memories[0].ID != newID {
		t.Errorf("expected new entry %s, got %s", newID, recall3.Memories[0].ID)
	}
}

func TestSupersede_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Supersede(ctx, SupersedeRequest{OldID: "nonexistent", NewID: "also-nonexistent"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSupersede_AlreadyExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Old decision"}},
	})
	recall, _ := s.Recall(ctx, RecallRequest{Query: "decision", MaxResults: 1})
	id := recall.Memories[0].ID

	// Expire first
	_, _ = s.Expire(ctx, ExpireRequest{IDs: []string{id}})

	// Supersede should fail with ErrAlreadyExpired
	_, err := s.Supersede(ctx, SupersedeRequest{OldID: id, NewID: "new-id"})
	if err != ErrAlreadyExpired {
		t.Errorf("expected ErrAlreadyExpired, got %v", err)
	}
}

func TestExpire_LifecycleEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var events []MemoryEvent
	s.OnLifecycleEvent(func(e MemoryEvent) {
		events = append(events, e)
	})

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Will be expired"}},
	})
	recall, _ := s.Recall(ctx, RecallRequest{Query: "expired", MaxResults: 1})

	_, _ = s.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	if len(events) == 0 {
		t.Fatal("expected lifecycle event for expire")
	}
	if events[0].Type != EventExpired {
		t.Errorf("expected EventExpired, got %s", events[0].Type)
	}
}

func TestStoreWithTTL(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Store with a TTL that has already passed
	pastExpiry := time.Now().Add(-1 * time.Hour)
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Already expired by TTL", ExpiresAt: &pastExpiry},
			{Text: "Still valid"},
		},
	})

	// Recall should only return the valid entry
	recall, _ := s.Recall(ctx, RecallRequest{Query: "entry", MaxResults: 10})
	if len(recall.Memories) != 1 {
		t.Errorf("expected 1 active memory (TTL-expired excluded), got %d", len(recall.Memories))
	}
	if recall.Memories[0].Text != "Still valid" {
		t.Errorf("expected 'Still valid', got %q", recall.Memories[0].Text)
	}
}

func TestStoreWithFutureTTL(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	futureExpiry := time.Now().Add(24 * time.Hour)
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Valid for 24h", ExpiresAt: &futureExpiry},
		},
	})

	recall, _ := s.Recall(ctx, RecallRequest{Query: "valid", MaxResults: 10})
	if len(recall.Memories) != 1 {
		t.Errorf("expected 1 memory with future TTL, got %d", len(recall.Memories))
	}
}

func TestStats_IncludesExpiredCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Active entry"},
			{Text: "Will expire"},
		},
	})

	recall, _ := s.Recall(ctx, RecallRequest{Query: "expire", MaxResults: 10})
	_, _ = s.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	stats, err := s.Stats(ctx)
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

func TestExpire_DedupSkipsExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	emb := makeEmbedding(0, 8)

	// Store and expire an entry
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Original fact", Embedding: emb}},
	})
	recall, _ := s.Recall(ctx, RecallRequest{Query: "fact", QueryEmbedding: emb, MaxResults: 1})
	_, _ = s.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	// Store a new entry with the same embedding — should NOT be deduped
	result, err := s.Store(ctx, StoreRequest{
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

func TestExpire_EmptyRequest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.Expire(ctx, ExpireRequest{})
	if err != nil {
		t.Fatalf("Expire empty: %v", err)
	}
	if result.Expired != 0 {
		t.Errorf("expected 0 expired, got %d", result.Expired)
	}
}

func TestSupersede_EmptyOldID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Supersede(ctx, SupersedeRequest{OldID: "", NewID: "new"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for empty OldID, got %v", err)
	}
}
