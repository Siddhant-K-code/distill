package memory

import (
	"context"
	"testing"
)

func TestRecall_BoostTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Store two entries with different tags at nearly equal distance from query
	// angle 0.3 from query(0.3): auth at 0 → dist=0.045, db at 0.6 → dist=0.045
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Auth uses JWT", Embedding: makeEmbedding(0, 8), Tags: []string{"auth"}},
			{Text: "DB uses Postgres", Embedding: makeEmbedding(0.6, 8), Tags: []string{"database"}},
		},
	})

	// Query equidistant from both; boost on "database" should tip the ranking
	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "infrastructure",
		QueryEmbedding: makeEmbedding(0.3, 8), // equidistant from 0 and 0.6
		MaxResults:     10,
		BoostTags:      []string{"database"},
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(recall.Memories))
	}
	// With the boost, database entry should be ranked first
	if recall.Memories[0].Tags[0] != "database" {
		t.Errorf("expected database entry first (boosted), got tags=%v", recall.Memories[0].Tags)
	}
}

func TestRecall_MinRelevance(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Highly relevant", Embedding: makeEmbedding(0, 8)},
			{Text: "Somewhat relevant", Embedding: makeEmbedding(0.6, 8)},
			{Text: "Not relevant", Embedding: makeEmbedding(2.0, 8)},
		},
	})

	// Query with high min relevance — should filter out low-scoring entries
	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "test",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
		MinRelevance:   0.8,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	// Only the highly relevant entry (cosine similarity ~1.0) should pass
	if len(recall.Memories) == 0 {
		t.Fatal("expected at least 1 memory above min relevance")
	}
	for _, m := range recall.Memories {
		if m.Relevance < 0.8 {
			t.Errorf("memory %s has relevance %.3f, below min 0.8", m.ID, m.Relevance)
		}
	}
}

func TestRecall_MinRelevance_Zero_NoFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Entry A", Embedding: makeEmbedding(0, 8)},
			{Text: "Entry B", Embedding: makeEmbedding(2.0, 8)},
		},
	})

	// MinRelevance=0 should return all entries
	recall, _ := s.Recall(ctx, RecallRequest{
		Query:          "test",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
		MinRelevance:   0,
	})
	if len(recall.Memories) != 2 {
		t.Errorf("expected 2 memories with no min filter, got %d", len(recall.Memories))
	}
}

func TestRecall_TaskContext_SourceBoost(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Use angles far enough apart to avoid dedup (>0.555 rad)
	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "JWT validation logic", Embedding: makeEmbedding(0, 8), Source: "code_review"},
			{Text: "JWT token format", Embedding: makeEmbedding(0.6, 8), Source: "docs"},
		},
	})

	// Query equidistant; task context mentions "code_review" — should boost that source
	recall, err := s.Recall(ctx, RecallRequest{
		Query:          "JWT",
		QueryEmbedding: makeEmbedding(0.3, 8), // equidistant from 0 and 0.6
		MaxResults:     10,
		TaskContext:    "reviewing code_review findings",
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Memories) < 2 {
		t.Fatalf("expected 2 memories, got %d", len(recall.Memories))
	}
	if recall.Memories[0].Source != "code_review" {
		t.Errorf("expected code_review source first (boosted), got %s", recall.Memories[0].Source)
	}
}

func TestRecall_RelevanceClamped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Perfect match", Embedding: makeEmbedding(0, 8), Tags: []string{"auth"}},
		},
	})

	// Exact embedding match + boost tag + task context = would exceed 1.0
	recall, _ := s.Recall(ctx, RecallRequest{
		Query:          "auth",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
		BoostTags:      []string{"auth"},
		TaskContext:    "auth",
	})
	if len(recall.Memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(recall.Memories))
	}
	if recall.Memories[0].Relevance > 1.0 {
		t.Errorf("relevance should be clamped to 1.0, got %.3f", recall.Memories[0].Relevance)
	}
}

func TestRecall_BoostTags_Empty_NoEffect(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Entry A", Embedding: makeEmbedding(0, 8), Tags: []string{"a"}},
			{Text: "Entry B", Embedding: makeEmbedding(0.6, 8), Tags: []string{"b"}},
		},
	})

	// No boost tags — ranking should be purely by similarity
	recall, _ := s.Recall(ctx, RecallRequest{
		Query:          "test",
		QueryEmbedding: makeEmbedding(0, 8),
		MaxResults:     10,
	})
	if len(recall.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(recall.Memories))
	}
	// Entry A is closer to query (angle 0 vs 0.6)
	if recall.Memories[0].Text != "Entry A" {
		t.Errorf("expected Entry A first (closer), got %s", recall.Memories[0].Text)
	}
}
