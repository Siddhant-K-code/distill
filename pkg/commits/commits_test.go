package commits

import (
	"context"
	"math"
	"testing"
	"time"
)

func makeCommit(hash, msg string, insertions, deletions int, files []string) Commit {
	return Commit{
		Hash:         hash,
		ShortHash:    hash[:7],
		Message:      msg,
		Timestamp:    time.Now(),
		FilesChanged: files,
		Insertions:   insertions,
		Deletions:    deletions,
	}
}

func unitVec(dim int, idx int) []float32 {
	v := make([]float32, dim)
	v[idx] = 1.0
	return v
}

// ── Classification ────────────────────────────────────────────────────────────

func TestClassify_ConventionalCommits(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	tests := []struct {
		msg       string
		wantType  CommitType
		wantScope string
		wantBreak bool
	}{
		{"feat(auth): add JWT support", CommitTypeFeat, "auth", false},
		{"fix: nil pointer in handler", CommitTypeFix, "", false},
		{"feat!: remove legacy API", CommitTypeFeat, "", true},
		{"chore(deps): bump go version", CommitTypeChore, "deps", false},
		{"BREAKING CHANGE: drop v1 endpoint", CommitTypeUnknown, "", true},
		{"random commit message", CommitTypeUnknown, "", false},
		{"revert: undo bad deploy", CommitTypeRevert, "", false},
	}

	for _, tt := range tests {
		c := Commit{Message: tt.msg}
		a.Classify(&c)
		if c.Type != tt.wantType {
			t.Errorf("%q: type got %q want %q", tt.msg, c.Type, tt.wantType)
		}
		if c.Scope != tt.wantScope {
			t.Errorf("%q: scope got %q want %q", tt.msg, c.Scope, tt.wantScope)
		}
		if c.Breaking != tt.wantBreak {
			t.Errorf("%q: breaking got %v want %v", tt.msg, c.Breaking, tt.wantBreak)
		}
	}
}

// ── Risk scoring ──────────────────────────────────────────────────────────────

func TestScoreRisk_BreakingIsHigh(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	c := makeCommit("abc1234", "feat!: remove v1 API", 10, 5, []string{"api.go"})
	c.Breaking = true
	a.ScoreRisk(&c)
	if c.Risk != RiskHigh {
		t.Errorf("expected RiskHigh for breaking change, got %s", c.Risk)
	}
}

func TestScoreRisk_LargeDiff(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	c := makeCommit("abc1234", "refactor: rewrite core", 400, 200, []string{"core.go"})
	a.ScoreRisk(&c)
	if c.Risk == RiskLow {
		t.Error("expected at least RiskMedium for large diff")
	}
}

func TestScoreRisk_RevertIsHigh(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	c := makeCommit("abc1234", "revert: undo bad deploy", 5, 5, []string{"main.go"})
	c.Type = CommitTypeRevert
	a.ScoreRisk(&c)
	if c.Risk != RiskHigh {
		t.Errorf("expected RiskHigh for revert, got %s", c.Risk)
	}
}

func TestScoreRisk_SmallFix_Low(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	c := makeCommit("abc1234", "fix: typo in README", 1, 1, []string{"README.md"})
	c.Type = CommitTypeFix
	a.ScoreRisk(&c)
	if c.Risk != RiskLow {
		t.Errorf("expected RiskLow for small fix, got %s", c.Risk)
	}
}

// ── Similarity search ─────────────────────────────────────────────────────────

func TestFindSimilar_ReturnsTopK(t *testing.T) {
	a := NewAnalyzer(AnalyzerConfig{TopK: 2, MinSimilarity: 0.0})
	ctx := context.Background()

	query := unitVec(4, 0) // [1,0,0,0]
	corpus := []Commit{
		{Hash: "a", Embedding: unitVec(4, 0)}, // sim=1.0
		{Hash: "b", Embedding: unitVec(4, 1)}, // sim=0.0
		{Hash: "c", Embedding: unitVec(4, 0)}, // sim=1.0
		{Hash: "d", Embedding: unitVec(4, 2)}, // sim=0.0
	}

	results := a.FindSimilar(ctx, query, corpus)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if math.Abs(r.Similarity-1.0) > 0.001 {
			t.Errorf("expected similarity 1.0, got %f", r.Similarity)
		}
	}
}

func TestFindSimilar_MinSimilarityFilter(t *testing.T) {
	a := NewAnalyzer(AnalyzerConfig{TopK: 10, MinSimilarity: 0.9})
	ctx := context.Background()

	query := unitVec(4, 0)
	corpus := []Commit{
		{Hash: "a", Embedding: unitVec(4, 0)}, // sim=1.0 — passes
		{Hash: "b", Embedding: unitVec(4, 1)}, // sim=0.0 — filtered
	}

	results := a.FindSimilar(ctx, query, corpus)
	if len(results) != 1 {
		t.Errorf("expected 1 result above threshold, got %d", len(results))
	}
}

func TestFindSimilar_NoEmbeddings(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	ctx := context.Background()
	corpus := []Commit{{Hash: "a"}, {Hash: "b"}}
	results := a.FindSimilar(ctx, unitVec(4, 0), corpus)
	if len(results) != 0 {
		t.Errorf("expected 0 results for commits without embeddings, got %d", len(results))
	}
}

// ── Pattern detection ─────────────────────────────────────────────────────────

func TestDetectPatterns_RepeatedType(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	commits := []Commit{
		{ShortHash: "aaa", Type: CommitTypeFix},
		{ShortHash: "bbb", Type: CommitTypeFix},
		{ShortHash: "ccc", Type: CommitTypeFix},
		{ShortHash: "ddd", Type: CommitTypeFeat},
	}
	patterns := a.DetectPatterns(commits)
	found := false
	for _, p := range patterns {
		if p.Count == 3 {
			found = true
		}
	}
	if !found {
		t.Error("expected pattern with count=3 for repeated fix commits")
	}
}

func TestDetectPatterns_HighChurnFile(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	commits := []Commit{
		{ShortHash: "a", FilesChanged: []string{"hot.go", "other.go"}},
		{ShortHash: "b", FilesChanged: []string{"hot.go"}},
		{ShortHash: "c", FilesChanged: []string{"hot.go"}},
	}
	patterns := a.DetectPatterns(commits)
	found := false
	for _, p := range patterns {
		if p.Count >= 3 {
			found = true
		}
	}
	if !found {
		t.Error("expected high-churn file pattern")
	}
}

// ── Risk summary ──────────────────────────────────────────────────────────────

func TestSummarize_OverallRisk(t *testing.T) {
	a := NewAnalyzer(DefaultAnalyzerConfig())
	similar := []SimilarCommit{
		{Commit: Commit{Risk: RiskHigh, RiskReasons: []string{"breaking change"}}, Similarity: 0.9},
		{Commit: Commit{Risk: RiskLow}, Similarity: 0.7},
	}
	s := a.Summarize(similar)
	if s.OverallRisk != RiskHigh {
		t.Errorf("expected RiskHigh overall, got %s", s.OverallRisk)
	}
	if s.HighRiskCount != 1 {
		t.Errorf("expected 1 high-risk commit, got %d", s.HighRiskCount)
	}
}
