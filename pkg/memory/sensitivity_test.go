package memory

import (
	"context"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/sensitivity"
)

func TestStore_ExplicitSensitivity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Q3 pricing: customer A at $120k", Sensitivity: sensitivity.InternalIP, Embedding: makeEmbedding(0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, err := s.Recall(ctx, RecallRequest{
		Query: "pricing", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if recall.MaxSensitivity != sensitivity.InternalIP {
		t.Errorf("expected MaxSensitivity=InternalIP, got %s", recall.MaxSensitivity)
	}
	if len(recall.SensitiveChunks) != 1 {
		t.Fatalf("expected 1 sensitive chunk, got %d", len(recall.SensitiveChunks))
	}
	if recall.SensitiveChunks[0].Sensitivity != sensitivity.InternalIP {
		t.Errorf("expected chunk sensitivity InternalIP, got %s", recall.SensitiveChunks[0].Sensitivity)
	}
}

func TestStore_AutoClassify_Credentials(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "API key: sk-proj-abc123def456ghi789jkl012", AutoClassify: true, Embedding: makeEmbedding(0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "key", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if recall.MaxSensitivity != sensitivity.Credentials {
		t.Errorf("expected MaxSensitivity=Credentials, got %s", recall.MaxSensitivity)
	}
}

func TestStore_AutoClassify_PII(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Contact alice@example.com for the report", AutoClassify: true, Embedding: makeEmbedding(0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "contact", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if recall.MaxSensitivity != sensitivity.PII {
		t.Errorf("expected MaxSensitivity=PII, got %s", recall.MaxSensitivity)
	}
}

func TestStore_AutoClassify_NoMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "The service uses REST APIs", AutoClassify: true, Embedding: makeEmbedding(0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "service", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	if recall.MaxSensitivity != sensitivity.None {
		t.Errorf("expected MaxSensitivity=None, got %s", recall.MaxSensitivity)
	}
	if len(recall.SensitiveChunks) != 0 {
		t.Errorf("expected 0 sensitive chunks, got %d", len(recall.SensitiveChunks))
	}
}

func TestStore_AutoClassify_ExplicitOverride(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Explicit sensitivity is higher than what auto-classify would find
	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{
				Text:         "Normal text with no patterns",
				Sensitivity:  sensitivity.Credentials,
				AutoClassify: true,
				Embedding:    makeEmbedding(0, 8),
			},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "text", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	// Explicit Credentials should be preserved even though auto-classify finds None
	if recall.MaxSensitivity != sensitivity.Credentials {
		t.Errorf("expected MaxSensitivity=Credentials (explicit), got %s", recall.MaxSensitivity)
	}
}

func TestRecall_MaxSensitivity_MultipleEntries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Normal architecture notes", Embedding: makeEmbedding(0, 8)},
			{Text: "Contact bob@company.com", Sensitivity: sensitivity.PII, Embedding: makeEmbedding(1.5, 8)},
			{Text: "AWS key AKIAIOSFODNN7EXAMPLE", Sensitivity: sensitivity.Credentials, Embedding: makeEmbedding(3.0, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "all", QueryEmbedding: makeEmbedding(0.5, 8), MaxResults: 10,
	})
	if recall.MaxSensitivity != sensitivity.Credentials {
		t.Errorf("expected MaxSensitivity=Credentials, got %s", recall.MaxSensitivity)
	}
	if len(recall.SensitiveChunks) != 2 {
		t.Errorf("expected 2 sensitive chunks, got %d", len(recall.SensitiveChunks))
	}
}

func TestRecall_SensitivityDoesNotAffectRanking(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Store(ctx, StoreRequest{
		Entries: []StoreEntry{
			{Text: "Highly relevant auth info", Embedding: makeEmbedding(0, 8), Sensitivity: sensitivity.Credentials},
			{Text: "Less relevant payment info", Embedding: makeEmbedding(1.5, 8)},
		},
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	recall, _ := s.Recall(ctx, RecallRequest{
		Query: "auth", QueryEmbedding: makeEmbedding(0, 8), MaxResults: 10,
	})
	// The sensitive entry should still be ranked first (closest embedding)
	if len(recall.Memories) < 1 {
		t.Fatal("expected at least 1 memory")
	}
	if recall.Memories[0].Sensitivity != sensitivity.Credentials {
		t.Errorf("expected first result to have Credentials sensitivity, got %s", recall.Memories[0].Sensitivity)
	}
}
