package session

import (
	"context"
	"math"
	"strings"
	"testing"
)

func makeEmbedding(angle float64, dim int) []float32 {
	emb := make([]float32, dim)
	emb[0] = float32(math.Cos(angle))
	emb[1] = float32(math.Sin(angle))
	return emb
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	cfg := DefaultConfig()
	cfg.DefaultMaxTokens = 1000 // small budget for testing
	cfg.DefaultPreserveRecent = 2
	s, err := NewSQLiteStore(":memory:", cfg)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sess, err := s.Create(ctx, CreateRequest{
		SessionID: "test-1",
		MaxTokens: 5000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID != "test-1" {
		t.Errorf("expected id test-1, got %s", sess.ID)
	}
	if sess.MaxTokens != 5000 {
		t.Errorf("expected 5000 max tokens, got %d", sess.MaxTokens)
	}

	got, err := s.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MaxTokens != 5000 {
		t.Errorf("expected 5000, got %d", got.MaxTokens)
	}
}

func TestCreateAutoID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sess, err := s.Create(ctx, CreateRequest{MaxTokens: 1000})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestCreateDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Create(ctx, CreateRequest{SessionID: "dup"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = s.Create(ctx, CreateRequest{SessionID: "dup"})
	if err != ErrSessionExists {
		t.Errorf("expected ErrSessionExists, got %v", err)
	}
}

func TestPushAndContext(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "s1", MaxTokens: 50000})

	result, err := s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "user", Content: "Fix the JWT validation bug", Importance: 1.0},
			{Role: "tool", Content: "File: auth/jwt.go\nfunc ValidateToken()...", Source: "file_read"},
		},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if result.Accepted != 2 {
		t.Errorf("expected 2 accepted, got %d", result.Accepted)
	}
	if result.CurrentTokens <= 0 {
		t.Error("expected positive token count")
	}

	// Read context
	ctxResult, err := s.Context(ctx, ContextRequest{SessionID: "s1"})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(ctxResult.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(ctxResult.Entries))
	}
	// Entries should be in push order
	if ctxResult.Entries[0].Role != "user" {
		t.Errorf("expected first entry role=user, got %s", ctxResult.Entries[0].Role)
	}
}

func TestPushDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "s1", MaxTokens: 50000})

	emb := makeEmbedding(0, 8)

	r1, err := s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "tool", Content: "File: auth/jwt.go contents...", Embedding: emb},
		},
	})
	if err != nil {
		t.Fatalf("Push 1: %v", err)
	}
	if r1.Accepted != 1 {
		t.Errorf("expected 1 accepted, got %d", r1.Accepted)
	}

	// Push same embedding again - should be deduped
	r2, err := s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "tool", Content: "File: auth/jwt.go (re-read)", Embedding: emb},
		},
	})
	if err != nil {
		t.Fatalf("Push 2: %v", err)
	}
	if r2.Deduplicated != 1 {
		t.Errorf("expected 1 deduplicated, got %d", r2.Deduplicated)
	}
	if r2.Accepted != 0 {
		t.Errorf("expected 0 accepted, got %d", r2.Accepted)
	}
}

func TestBudgetEnforcement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create session with very tight budget
	_, _ = s.Create(ctx, CreateRequest{
		SessionID:      "tight",
		MaxTokens:      50, // ~200 chars
		PreserveRecent: 1,
	})

	// Push entries that exceed budget
	_, err := s.Push(ctx, PushRequest{
		SessionID: "tight",
		Entries: []PushEntry{
			{Role: "user", Content: "First message about authentication and JWT tokens.", Importance: 0.3},
			{Role: "tool", Content: "Second message with file contents from the auth module.", Importance: 0.5},
			{Role: "user", Content: "Third message asking about the bug fix.", Importance: 1.0},
		},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Check that budget is enforced
	sess, _ := s.Get(ctx, "tight")
	if sess.CurrentTokens > 50 {
		t.Errorf("expected tokens <= 50, got %d", sess.CurrentTokens)
	}
}

func TestContextWithRoleFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "s1", MaxTokens: 50000})

	_, _ = s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "user", Content: "Fix the bug"},
			{Role: "tool", Content: "File contents..."},
			{Role: "assistant", Content: "I'll look at that"},
			{Role: "tool", Content: "Test results..."},
		},
	})

	// Filter by tool role
	result, err := s.Context(ctx, ContextRequest{SessionID: "s1", Role: "tool"})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 tool entries, got %d", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.Role != "tool" {
			t.Errorf("expected role=tool, got %s", e.Role)
		}
	}
}

func TestContextWithTokenLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "s1", MaxTokens: 50000})

	_, _ = s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "user", Content: "Short message"},
			{Role: "tool", Content: "This is a much longer message that contains many more tokens and should push us over a small token limit when combined with the first entry"},
		},
	})

	// Request with tight token limit
	result, err := s.Context(ctx, ContextRequest{SessionID: "s1", MaxTokens: 10})
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if result.Stats.TotalTokens > 10 {
		t.Errorf("expected tokens <= 10, got %d", result.Stats.TotalTokens)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "del"})
	_, _ = s.Push(ctx, PushRequest{
		SessionID: "del",
		Entries:   []PushEntry{{Role: "user", Content: "test"}},
	})

	result, err := s.Delete(ctx, "del")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if result.EntriesRemoved != 1 {
		t.Errorf("expected 1 removed, got %d", result.EntriesRemoved)
	}

	_, err = s.Get(ctx, "del")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Delete(ctx, "nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestPushToNonexistentSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Push(ctx, PushRequest{
		SessionID: "nope",
		Entries:   []PushEntry{{Role: "user", Content: "test"}},
	})
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestPushEmptyContent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _ = s.Create(ctx, CreateRequest{SessionID: "s1"})

	result, err := s.Push(ctx, PushRequest{
		SessionID: "s1",
		Entries: []PushEntry{
			{Role: "user", Content: ""},
			{Role: "user", Content: "Valid"},
		},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if result.Accepted != 1 {
		t.Errorf("expected 1 accepted (empty skipped), got %d", result.Accepted)
	}
}

func TestCompressToLevel(t *testing.T) {
	text := "The authentication service uses JWT tokens with RS256 signing. It validates tokens on every request. The token expiry is set to 24 hours. Refresh tokens are stored in Redis with a 7-day TTL. The service also supports OAuth2 for third-party integrations."

	summary := compressToLevel(text, LevelSummary)
	if len(summary) >= len(text) {
		t.Errorf("summary should be shorter than original: %d >= %d", len(summary), len(text))
	}
	if summary == "" {
		t.Error("summary should not be empty")
	}

	sentence := compressToLevel(text, LevelSentence)
	if len(sentence) >= len(text) {
		t.Errorf("sentence should be shorter than original: %d >= %d", len(sentence), len(text))
	}
	// Should end with a sentence delimiter
	last := sentence[len(sentence)-1]
	if last != '.' && last != '!' && last != '?' {
		t.Errorf("sentence should end with delimiter, got %q", string(last))
	}

	keywords := compressToLevel(text, LevelKeywords)
	if keywords == "" {
		t.Error("keywords should not be empty")
	}
	// Keywords should be comma-separated
	if !strings.Contains(keywords, ",") {
		t.Errorf("keywords should be comma-separated, got %q", keywords)
	}
	// Keywords should differ from full text
	if keywords == text {
		t.Error("keywords should differ from original text")
	}
}
