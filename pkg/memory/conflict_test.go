package memory

import (
	"context"
	"testing"
)

// Angle thresholds for makeEmbedding:
// cosine_distance(0, x) = 1 - cos(x)
// dedup threshold 0.15 → angle ~0.55
// conflict threshold 0.35 → angle ~0.84
// So angles in (0.55, 0.84) produce conflicts; angles < 0.55 produce dups.
const (
	conflictAngle = 0.7  // between dedup (0.55) and conflict (0.84) thresholds
	farAngle      = 2.0  // well beyond conflict threshold
)

func TestConflictDetection_SimilarButNotIdentical(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses JWT with RS256 signing", Embedding: makeEmbedding(0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store initial: %v", err)
	}

	// Angle 0.7 → cosine distance ~0.23, above dedup (0.15) but below conflict (0.35)
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses HMAC with HS256 signing", Embedding: makeEmbedding(conflictAngle, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store conflicting: %v", err)
	}

	if result.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", result.Stored)
	}
	if len(result.Conflicts) == 0 {
		t.Fatal("expected at least 1 conflict, got 0")
	}

	c := result.Conflicts[0]
	if c.NewText != "Auth uses HMAC with HS256 signing" {
		t.Errorf("unexpected NewText: %s", c.NewText)
	}
	if c.ExistingText != "Auth uses JWT with RS256 signing" {
		t.Errorf("unexpected ExistingText: %s", c.ExistingText)
	}
	if c.NewID == "" {
		t.Error("expected NewID to be set")
	}
	if c.ExistingID == "" {
		t.Error("expected ExistingID to be set")
	}
	if c.Distance <= 0 {
		t.Errorf("expected positive distance, got %f", c.Distance)
	}
}

func TestConflictDetection_ExactDuplicate_NoConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	emb := makeEmbedding(0, 8)

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Exact duplicate", Embedding: emb}},
	})

	// Same embedding — should be deduped, not flagged as conflict
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Exact duplicate again", Embedding: emb}},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Deduplicated != 1 {
		t.Errorf("expected 1 deduplicated, got %d", result.Deduplicated)
	}
	if len(result.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts for exact dup, got %d", len(result.Conflicts))
	}
}

func TestConflictDetection_FarApart_NoConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses JWT", Embedding: makeEmbedding(0, 8)},
		},
	})

	// Very different embedding — no conflict
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Database uses Postgres", Embedding: makeEmbedding(farAngle, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", result.Stored)
	}
	if len(result.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts for distant entries, got %d", len(result.Conflicts))
	}
}

func TestConflictDetection_MultipleConflicts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Entry1 at angle 0, Entry2 at angle 1.2 (dist=0.64, no dedup between them)
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Deploy to AWS us-east-1", Embedding: makeEmbedding(0, 8)},
		},
	})
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Deploy to AWS us-west-2", Embedding: makeEmbedding(1.2, 8)},
		},
	})

	// Entry3 at angle 0.6 — equidistant from both (dist=0.17 each, in conflict zone)
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Deploy to GCP us-central1", Embedding: makeEmbedding(0.6, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if result.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", result.Stored)
	}
	if len(result.Conflicts) < 2 {
		t.Errorf("expected at least 2 conflicts, got %d", len(result.Conflicts))
	}
}

func TestConflictDetection_StillStoresEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use MySQL", Embedding: makeEmbedding(0, 8)},
		},
	})

	result, _ := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Use Postgres", Embedding: makeEmbedding(conflictAngle, 8)},
		},
	})

	// Both entries should be in the store
	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "database", QueryEmbedding: makeEmbedding(0.35, 8), MaxResults: 10,
	})
	if len(recall.Memories) != 2 {
		t.Errorf("expected 2 memories (conflict doesn't block store), got %d", len(recall.Memories))
	}
	if result.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", result.Stored)
	}
}

func TestConflictDetection_NoEmbedding_NoConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Some fact", Embedding: makeEmbedding(0, 8)}},
	})

	// Store without embedding — no conflict detection possible
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{{Text: "Related fact"}},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts without embedding, got %d", len(result.Conflicts))
	}
}

func TestConflictDetection_ExpiredEntriesIgnored(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Old decision", Embedding: makeEmbedding(0, 8)},
		},
	})

	// Expire the entry
	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "decision", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 1,
	})
	_, _ = s.Expire(ctx, ExpireRequest{IDs: []string{recall.Memories[0].ID}})

	// Store a similar entry — should NOT conflict with expired entry
	result, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "New decision", Embedding: makeEmbedding(conflictAngle, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts (expired entry ignored), got %d", len(result.Conflicts))
	}
}
