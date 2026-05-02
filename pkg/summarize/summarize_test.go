package summarize

import (
	"context"
	"strings"
	"testing"
	"time"
)

func makeTurn(id, role, content string, age time.Duration, importance float64) Turn {
	return Turn{
		ID:         id,
		Role:       role,
		Content:    content,
		Original:   content,
		Timestamp:  time.Now().Add(-age),
		Level:      LevelFull,
		Importance: importance,
		TokenCount: estimateTokens(content),
	}
}

func TestHierarchicalSummarizer_PreservesRecentTurns(t *testing.T) {
	s := NewHierarchicalSummarizer()
	ctx := context.Background()

	turns := []Turn{
		makeTurn("1", "user", "Old message from yesterday", 25*time.Hour, 0.3),
		makeTurn("2", "assistant", "Old reply from yesterday", 25*time.Hour, 0.3),
		makeTurn("3", "user", "Recent message", 1*time.Minute, 0.5),
		makeTurn("4", "assistant", "Recent reply", 1*time.Minute, 0.5),
	}

	opts := DefaultOptions()
	opts.PreserveRecent = 2

	result, stats, err := s.Summarize(ctx, turns, opts)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	// Recent turns (3, 4) must be at LevelFull.
	if result[2].Level != LevelFull {
		t.Errorf("turn 3: expected LevelFull, got %d", result[2].Level)
	}
	if result[3].Level != LevelFull {
		t.Errorf("turn 4: expected LevelFull, got %d", result[3].Level)
	}

	// Old turns should be compressed.
	if result[0].Level == LevelFull {
		t.Errorf("turn 1: expected compression, still at LevelFull")
	}

	if stats.CompressedTurns == 0 {
		t.Error("expected at least one compressed turn")
	}
}

func TestHierarchicalSummarizer_HighImportanceResistsCompression(t *testing.T) {
	s := NewHierarchicalSummarizer()
	ctx := context.Background()

	turns := []Turn{
		makeTurn("1", "assistant", "We decided to use PostgreSQL for the database. This is the final architecture decision.", 25*time.Hour, 0.9),
		makeTurn("2", "user", "Some old low-importance message", 25*time.Hour, 0.2),
	}

	opts := DefaultOptions()
	opts.PreserveRecent = 0
	opts.ImportanceThreshold = 0.7

	result, _, err := s.Summarize(ctx, turns, opts)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	// High-importance turn should not exceed LevelParagraph.
	if result[0].Level > LevelParagraph {
		t.Errorf("high-importance turn compressed beyond LevelParagraph: level=%d", result[0].Level)
	}
}

func TestHierarchicalSummarizer_TokenBudget(t *testing.T) {
	s := NewHierarchicalSummarizer()
	ctx := context.Background()

	// Create many old turns that exceed a small budget.
	var turns []Turn
	for i := 0; i < 20; i++ {
		turns = append(turns, makeTurn(
			"t", "user",
			strings.Repeat("This is a long message with lots of content. ", 10),
			25*time.Hour, 0.3,
		))
	}

	opts := DefaultOptions()
	opts.MaxTokens = 200
	opts.PreserveRecent = 2

	result, stats, err := s.Summarize(ctx, turns, opts)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if stats.OutputTokens > opts.MaxTokens {
		t.Errorf("output tokens %d exceeds budget %d", stats.OutputTokens, opts.MaxTokens)
	}
	_ = result
}

func TestHierarchicalSummarizer_SystemRolePreserved(t *testing.T) {
	s := NewHierarchicalSummarizer()
	ctx := context.Background()

	turns := []Turn{
		makeTurn("sys", "system", "You are a helpful assistant.", 48*time.Hour, 0),
		makeTurn("old", "user", "Old message", 48*time.Hour, 0.2),
	}

	opts := DefaultOptions()
	opts.PreserveRecent = 0

	result, _, err := s.Summarize(ctx, turns, opts)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	// System turn has importance=1.0 after scoring, should not exceed LevelParagraph.
	if result[0].Level > LevelParagraph {
		t.Errorf("system turn compressed beyond LevelParagraph: level=%d", result[0].Level)
	}
}

func TestScoreImportance(t *testing.T) {
	tests := []struct {
		turn    Turn
		wantMin float64
	}{
		{Turn{Role: "system", Content: "You are helpful."}, 1.0},
		{Turn{Role: "assistant", Content: "```go\nfunc main() {}\n```"}, 0.8},
		{Turn{Role: "user", Content: "error: nil pointer dereference"}, 0.7},
		{Turn{Role: "user", Content: "ok"}, 0.0},
	}

	for _, tt := range tests {
		score := ScoreImportance(tt.turn)
		if score < tt.wantMin {
			t.Errorf("role=%s content=%q: score %.2f < min %.2f",
				tt.turn.Role, tt.turn.Content, score, tt.wantMin)
		}
	}
}

func TestExtractParagraphSummary(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph that should be excluded."
	result := extractParagraphSummary(text)
	if strings.Contains(result, "Second paragraph") {
		t.Errorf("paragraph summary should not include second paragraph: %q", result)
	}
}

func TestExtractSentenceSummary(t *testing.T) {
	text := "This is the first sentence. This is the second. This is the third."
	result := extractSentenceSummary(text)
	if strings.Contains(result, "third") {
		t.Errorf("sentence summary should not include third sentence: %q", result)
	}
}

func TestExtractKeywordSummary(t *testing.T) {
	text := "The authentication service uses JWT tokens with RS256 signing algorithm for security."
	result := extractKeywordSummary(text)
	if result == "" {
		t.Error("expected non-empty keyword summary")
	}
	// Should not contain stop words.
	for _, stop := range []string{"the", "with", "for"} {
		if strings.Contains(strings.ToLower(result), " "+stop+" ") {
			t.Errorf("keyword summary contains stop word %q: %q", stop, result)
		}
	}
}

func TestTotalTokens(t *testing.T) {
	turns := []Turn{
		{TokenCount: 10},
		{TokenCount: 20},
		{TokenCount: 30},
	}
	if TotalTokens(turns) != 60 {
		t.Errorf("expected 60, got %d", TotalTokens(turns))
	}
}

func TestSummarizeStats_ReductionPct(t *testing.T) {
	s := NewHierarchicalSummarizer()
	ctx := context.Background()

	longContent := strings.Repeat("This is a detailed message about the system architecture. ", 20)
	turns := []Turn{
		makeTurn("1", "user", longContent, 25*time.Hour, 0.2),
		makeTurn("2", "assistant", longContent, 25*time.Hour, 0.2),
	}

	opts := DefaultOptions()
	opts.PreserveRecent = 0

	_, stats, err := s.Summarize(ctx, turns, opts)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if stats.ReductionPct <= 0 {
		t.Errorf("expected positive reduction, got %.1f%%", stats.ReductionPct)
	}
}
