// Package commits provides semantic analysis of git commit history.
// It finds semantically similar past changes, classifies commit intent,
// and surfaces patterns that correlate with incidents or regressions.
package commits

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// CommitType classifies the intent of a commit.
type CommitType string

const (
	CommitTypeFeat     CommitType = "feat"
	CommitTypeFix      CommitType = "fix"
	CommitTypeRefactor CommitType = "refactor"
	CommitTypeTest     CommitType = "test"
	CommitTypeDocs     CommitType = "docs"
	CommitTypeChore    CommitType = "chore"
	CommitTypePerf     CommitType = "perf"
	CommitTypeRevert   CommitType = "revert"
	CommitTypeUnknown  CommitType = "unknown"
)

// RiskLevel indicates the estimated risk of a commit.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Commit represents a single git commit with metadata.
type Commit struct {
	Hash      string
	ShortHash string
	Author    string
	Email     string
	Message   string
	Body      string
	Timestamp time.Time

	// Derived fields (populated by Analyzer).
	Type        CommitType
	Scope       string
	Breaking    bool
	FilesChanged []string
	Insertions  int
	Deletions   int
	Embedding   []float32
	Risk        RiskLevel
	RiskReasons []string
}

// SimilarCommit pairs a commit with its similarity score to a query.
type SimilarCommit struct {
	Commit     Commit
	Similarity float64 // cosine similarity 0–1
}

// AnalysisResult is the output of a semantic commit analysis.
type AnalysisResult struct {
	Query          string
	Similar        []SimilarCommit
	RiskSummary    RiskSummary
	Patterns       []Pattern
	AnalyzedAt     time.Time
	CommitsScanned int
}

// RiskSummary aggregates risk signals across similar commits.
type RiskSummary struct {
	HighRiskCount   int
	MediumRiskCount int
	LowRiskCount    int
	TopRiskReasons  []string
	OverallRisk     RiskLevel
}

// Pattern describes a recurring commit pattern in the history.
type Pattern struct {
	Description string
	Count       int
	Examples    []string // short hashes
	RiskLevel   RiskLevel
}

// Analyzer performs semantic analysis on a commit corpus.
type Analyzer struct {
	cfg AnalyzerConfig
}

// AnalyzerConfig controls analysis behaviour.
type AnalyzerConfig struct {
	// TopK is the maximum number of similar commits to return. Default: 10.
	TopK int

	// MinSimilarity is the minimum cosine similarity threshold. Default: 0.5.
	MinSimilarity float64

	// IncludeRiskAnalysis enables heuristic risk scoring. Default: true.
	IncludeRiskAnalysis bool
}

// DefaultAnalyzerConfig returns sensible defaults.
func DefaultAnalyzerConfig() AnalyzerConfig {
	return AnalyzerConfig{
		TopK:                10,
		MinSimilarity:       0.5,
		IncludeRiskAnalysis: true,
	}
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(cfg AnalyzerConfig) *Analyzer {
	if cfg.TopK <= 0 {
		cfg.TopK = 10
	}
	if cfg.MinSimilarity <= 0 {
		cfg.MinSimilarity = 0.5
	}
	return &Analyzer{cfg: cfg}
}

// Classify sets the Type, Scope, and Breaking fields on a commit by parsing
// its message using Conventional Commits conventions.
func (a *Analyzer) Classify(c *Commit) {
	c.Type, c.Scope, c.Breaking = parseConventionalCommit(c.Message)
}

// ClassifyAll classifies all commits in the slice.
func (a *Analyzer) ClassifyAll(commits []Commit) {
	for i := range commits {
		a.Classify(&commits[i])
	}
}

// ScoreRisk assigns a RiskLevel and RiskReasons to a commit based on
// heuristic signals (no LLM required).
func (a *Analyzer) ScoreRisk(c *Commit) {
	var reasons []string
	score := 0

	// Breaking changes are always high risk.
	if c.Breaking {
		score += 3
		reasons = append(reasons, "breaking change")
	}

	// Large diffs are riskier.
	totalLines := c.Insertions + c.Deletions
	if totalLines > 500 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("large diff (%d lines)", totalLines))
	} else if totalLines > 200 {
		score++
		reasons = append(reasons, fmt.Sprintf("medium diff (%d lines)", totalLines))
	}

	// Many files changed increases blast radius.
	if len(c.FilesChanged) > 20 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("%d files changed", len(c.FilesChanged)))
	} else if len(c.FilesChanged) > 10 {
		score++
	}

	// Reverts indicate a previous problem and are always high risk.
	if c.Type == CommitTypeRevert {
		score += 3
		reasons = append(reasons, "revert commit")
	}

	// Fix commits touching many files may indicate systemic issues.
	if c.Type == CommitTypeFix && len(c.FilesChanged) > 5 {
		score++
		reasons = append(reasons, "broad fix")
	}

	// Risk keywords in message body.
	lower := strings.ToLower(c.Message + " " + c.Body)
	for _, kw := range riskKeywords {
		if strings.Contains(lower, kw) {
			score++
			reasons = append(reasons, "risk keyword: "+kw)
			break
		}
	}

	switch {
	case score >= 3:
		c.Risk = RiskHigh
	case score >= 1:
		c.Risk = RiskMedium
	default:
		c.Risk = RiskLow
	}
	c.RiskReasons = reasons
}

// ScoreRiskAll scores all commits in the slice.
func (a *Analyzer) ScoreRiskAll(commits []Commit) {
	for i := range commits {
		a.ScoreRisk(&commits[i])
	}
}

// FindSimilar returns the top-K commits most semantically similar to query,
// using pre-computed embeddings. Commits without embeddings are skipped.
func (a *Analyzer) FindSimilar(_ context.Context, query []float32, corpus []Commit) []SimilarCommit {
	type scored struct {
		idx   int
		score float64
	}
	var candidates []scored

	for i, c := range corpus {
		if len(c.Embedding) == 0 || len(query) == 0 {
			continue
		}
		sim := cosineSimilarity(query, c.Embedding)
		if sim >= a.cfg.MinSimilarity {
			candidates = append(candidates, scored{i, sim})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	k := a.cfg.TopK
	if len(candidates) < k {
		k = len(candidates)
	}

	result := make([]SimilarCommit, k)
	for i := 0; i < k; i++ {
		result[i] = SimilarCommit{
			Commit:     corpus[candidates[i].idx],
			Similarity: candidates[i].score,
		}
	}
	return result
}

// DetectPatterns identifies recurring patterns in a commit slice.
func (a *Analyzer) DetectPatterns(commits []Commit) []Pattern {
	// Count by type.
	typeCounts := map[CommitType][]string{}
	for _, c := range commits {
		typeCounts[c.Type] = append(typeCounts[c.Type], c.ShortHash)
	}

	var patterns []Pattern
	for ct, hashes := range typeCounts {
		if len(hashes) < 2 {
			continue
		}
		risk := RiskLow
		if ct == CommitTypeFix {
			risk = RiskMedium
		}
		if ct == CommitTypeRevert {
			risk = RiskHigh
		}
		ex := hashes
		if len(ex) > 3 {
			ex = ex[:3]
		}
		patterns = append(patterns, Pattern{
			Description: fmt.Sprintf("repeated %s commits", ct),
			Count:       len(hashes),
			Examples:    ex,
			RiskLevel:   risk,
		})
	}

	// Detect high-churn files.
	fileCounts := map[string]int{}
	for _, c := range commits {
		for _, f := range c.FilesChanged {
			fileCounts[f]++
		}
	}
	for f, count := range fileCounts {
		if count >= 3 {
			patterns = append(patterns, Pattern{
				Description: fmt.Sprintf("high-churn file: %s (%d changes)", f, count),
				Count:       count,
				RiskLevel:   RiskMedium,
			})
		}
	}

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})
	return patterns
}

// Summarize builds a RiskSummary from a set of similar commits.
func (a *Analyzer) Summarize(similar []SimilarCommit) RiskSummary {
	var s RiskSummary
	reasonCounts := map[string]int{}

	for _, sc := range similar {
		switch sc.Commit.Risk {
		case RiskHigh:
			s.HighRiskCount++
		case RiskMedium:
			s.MediumRiskCount++
		default:
			s.LowRiskCount++
		}
		for _, r := range sc.Commit.RiskReasons {
			reasonCounts[r]++
		}
	}

	// Top 3 risk reasons by frequency.
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range reasonCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
	for i := 0; i < 3 && i < len(sorted); i++ {
		s.TopRiskReasons = append(s.TopRiskReasons, sorted[i].k)
	}

	switch {
	case s.HighRiskCount > 0:
		s.OverallRisk = RiskHigh
	case s.MediumRiskCount > 0:
		s.OverallRisk = RiskMedium
	default:
		s.OverallRisk = RiskLow
	}
	return s
}

// parseConventionalCommit parses a Conventional Commits message.
// Returns type, scope, and whether it is a breaking change.
func parseConventionalCommit(msg string) (CommitType, string, bool) {
	msg = strings.TrimSpace(msg)
	breaking := strings.Contains(msg, "BREAKING CHANGE") || strings.Contains(msg, "!")

	// Match "type(scope)!: description" or "type: description"
	idx := strings.Index(msg, ":")
	if idx < 0 {
		return CommitTypeUnknown, "", breaking
	}
	prefix := strings.TrimSpace(msg[:idx])
	prefix = strings.TrimSuffix(prefix, "!")

	scope := ""
	if i := strings.Index(prefix, "("); i >= 0 {
		if j := strings.Index(prefix, ")"); j > i {
			scope = prefix[i+1 : j]
			prefix = prefix[:i]
		}
	}

	switch strings.ToLower(prefix) {
	case "feat", "feature":
		return CommitTypeFeat, scope, breaking
	case "fix", "bugfix":
		return CommitTypeFix, scope, breaking
	case "refactor":
		return CommitTypeRefactor, scope, breaking
	case "test", "tests":
		return CommitTypeTest, scope, breaking
	case "docs", "doc":
		return CommitTypeDocs, scope, breaking
	case "chore":
		return CommitTypeChore, scope, breaking
	case "perf":
		return CommitTypePerf, scope, breaking
	case "revert":
		return CommitTypeRevert, scope, breaking
	default:
		return CommitTypeUnknown, scope, breaking
	}
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / math.Sqrt(normA*normB)
}

var riskKeywords = []string{
	"hotfix", "urgent", "critical", "security", "vulnerability",
	"cve", "exploit", "regression", "rollback", "emergency",
}
