package graph

import (
	"os"
	"path/filepath"
	"testing"
)

// buildTestGraph creates a small graph for testing:
//
//	a.go → b.go → d.go
//	a.go → c.go → d.go
//	e.go (isolated)
func buildTestGraph() *Graph {
	g := New()
	for _, id := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
		g.AddNode(Node{ID: id, Type: NodeTypeFile, Language: "go"})
	}
	g.AddEdge(Edge{From: "a.go", To: "b.go", Weight: 1.0})
	g.AddEdge(Edge{From: "a.go", To: "c.go", Weight: 1.0})
	g.AddEdge(Edge{From: "b.go", To: "d.go", Weight: 1.0})
	g.AddEdge(Edge{From: "c.go", To: "d.go", Weight: 1.0})
	return g
}

func TestNodeAndEdgeCount(t *testing.T) {
	g := buildTestGraph()
	if g.NodeCount() != 5 {
		t.Fatalf("want 5 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 4 {
		t.Fatalf("want 4 edges, got %d", g.EdgeCount())
	}
}

func TestDependencies(t *testing.T) {
	g := buildTestGraph()
	deps := g.Dependencies("a.go")
	if len(deps) != 2 {
		t.Fatalf("want 2 deps for a.go, got %d", len(deps))
	}
}

func TestDependents(t *testing.T) {
	g := buildTestGraph()
	// d.go is depended on by b.go and c.go
	deps := g.Dependents("d.go")
	if len(deps) != 2 {
		t.Fatalf("want 2 dependents for d.go, got %d", len(deps))
	}
}

func TestBlastRadius_DirectChange(t *testing.T) {
	g := buildTestGraph()
	// Changing d.go should affect b.go and c.go (depth 1) and a.go (depth 2).
	result := g.BlastRadius([]string{"d.go"}, 0)
	if result.TotalAffected != 3 {
		t.Fatalf("want 3 affected, got %d", result.TotalAffected)
	}
	if result.MaxDepth != 2 {
		t.Fatalf("want max depth 2, got %d", result.MaxDepth)
	}
}

func TestBlastRadius_MaxDepth(t *testing.T) {
	g := buildTestGraph()
	// With maxDepth=1, only direct dependents of d.go (b.go, c.go).
	result := g.BlastRadius([]string{"d.go"}, 1)
	if result.TotalAffected != 2 {
		t.Fatalf("want 2 affected at depth 1, got %d", result.TotalAffected)
	}
}

func TestBlastRadius_MultipleChanged(t *testing.T) {
	g := buildTestGraph()
	// Changing both b.go and c.go: a.go is affected at depth 1 from both.
	result := g.BlastRadius([]string{"b.go", "c.go"}, 0)
	if result.TotalAffected != 1 {
		t.Fatalf("want 1 affected (a.go), got %d", result.TotalAffected)
	}
	if result.Affected[0].Node.ID != "a.go" {
		t.Fatalf("want a.go affected, got %s", result.Affected[0].Node.ID)
	}
}

func TestBlastRadius_IsolatedNode(t *testing.T) {
	g := buildTestGraph()
	result := g.BlastRadius([]string{"e.go"}, 0)
	if result.TotalAffected != 0 {
		t.Fatalf("isolated node should have 0 affected, got %d", result.TotalAffected)
	}
}

func TestBlastRadius_ImpactScoreDecay(t *testing.T) {
	g := buildTestGraph()
	result := g.BlastRadius([]string{"d.go"}, 0)
	// b.go and c.go are at depth 1 → score 1.0; a.go at depth 2 → score 0.5
	for _, a := range result.Affected {
		if a.Depth == 1 && a.ImpactScore != 1.0 {
			t.Errorf("depth-1 node %s: want score 1.0, got %.2f", a.Node.ID, a.ImpactScore)
		}
		if a.Depth == 2 && a.ImpactScore != 0.5 {
			t.Errorf("depth-2 node %s: want score 0.5, got %.2f", a.Node.ID, a.ImpactScore)
		}
	}
}

func TestBlastRadius_SortedByScore(t *testing.T) {
	g := buildTestGraph()
	result := g.BlastRadius([]string{"d.go"}, 0)
	for i := 1; i < len(result.Affected); i++ {
		if result.Affected[i].ImpactScore > result.Affected[i-1].ImpactScore {
			t.Errorf("results not sorted by impact score at index %d", i)
		}
	}
}

func TestStats(t *testing.T) {
	g := buildTestGraph()
	s := g.Stats()
	if s.NodeCount != 5 {
		t.Errorf("want 5 nodes, got %d", s.NodeCount)
	}
	if s.EdgeCount != 4 {
		t.Errorf("want 4 edges, got %d", s.EdgeCount)
	}
	// d.go has in-degree 2 (from b.go and c.go)
	if s.MaxInDegree != 2 {
		t.Errorf("want max in-degree 2, got %d", s.MaxInDegree)
	}
	// a.go has out-degree 2
	if s.MaxOutDegree != 2 {
		t.Errorf("want max out-degree 2, got %d", s.MaxOutDegree)
	}
}

func TestSummary(t *testing.T) {
	g := buildTestGraph()
	result := g.BlastRadius([]string{"d.go"}, 0)
	summary := result.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestBuildFromGoFiles(t *testing.T) {
	// Create a temporary Go module with two files.
	dir := t.TempDir()

	// Write a simple Go file that imports another package.
	mainFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainFile, []byte(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Args)
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(dir, "pkg", "util")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	utilFile := filepath.Join(subDir, "util.go")
	if err := os.WriteFile(utilFile, []byte(`package util

import "strings"

func Upper(s string) string { return strings.ToUpper(s) }
`), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := BuildFromGoFiles(dir)
	if err != nil {
		t.Fatalf("BuildFromGoFiles: %v", err)
	}
	if g.NodeCount() < 2 {
		t.Errorf("want at least 2 nodes, got %d", g.NodeCount())
	}
}

func TestNode_NotFound(t *testing.T) {
	g := New()
	if g.Node("nonexistent") != nil {
		t.Error("expected nil for missing node")
	}
}
