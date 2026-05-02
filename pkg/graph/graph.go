// Package graph builds a code dependency graph from a repository and answers
// blast-radius queries: given a set of changed files, which other files are
// likely affected?
package graph

import (
	"fmt"
	"sort"
	"strings"
)

// NodeType classifies a graph node.
type NodeType string

const (
	NodeTypeFile    NodeType = "file"
	NodeTypePackage NodeType = "package"
	NodeTypeModule  NodeType = "module"
)

// Node represents a file or package in the dependency graph.
type Node struct {
	ID       string   // canonical path (e.g. "pkg/cache/patterns.go")
	Type     NodeType
	Package  string   // Go package name or language-specific module
	Language string   // "go", "python", "typescript", etc.
	Tags     []string // arbitrary labels (e.g. "api", "storage", "test")
}

// Edge represents a directed dependency between two nodes.
type Edge struct {
	From   string // Node.ID
	To     string // Node.ID
	Weight float64 // 1.0 = direct import; < 1.0 = transitive
}

// Graph is an in-memory directed dependency graph.
type Graph struct {
	nodes    map[string]*Node
	outEdges map[string][]Edge // from → edges
	inEdges  map[string][]Edge // to → edges
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{
		nodes:    make(map[string]*Node),
		outEdges: make(map[string][]Edge),
		inEdges:  make(map[string][]Edge),
	}
}

// AddNode adds or replaces a node.
func (g *Graph) AddNode(n Node) {
	g.nodes[n.ID] = &n
}

// AddEdge adds a directed edge from → to.
func (g *Graph) AddEdge(e Edge) {
	g.outEdges[e.From] = append(g.outEdges[e.From], e)
	g.inEdges[e.To] = append(g.inEdges[e.To], e)
}

// Node returns the node with the given ID, or nil.
func (g *Graph) Node(id string) *Node {
	return g.nodes[id]
}

// NodeCount returns the number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// EdgeCount returns the total number of edges.
func (g *Graph) EdgeCount() int {
	n := 0
	for _, edges := range g.outEdges {
		n += len(edges)
	}
	return n
}

// Dependents returns all nodes that directly depend on id (reverse edges).
func (g *Graph) Dependents(id string) []*Node {
	var out []*Node
	for _, e := range g.inEdges[id] {
		if n := g.nodes[e.From]; n != nil {
			out = append(out, n)
		}
	}
	return out
}

// Dependencies returns all nodes that id directly depends on.
func (g *Graph) Dependencies(id string) []*Node {
	var out []*Node
	for _, e := range g.outEdges[id] {
		if n := g.nodes[e.To]; n != nil {
			out = append(out, n)
		}
	}
	return out
}

// BlastRadiusResult is the output of a blast-radius query.
type BlastRadiusResult struct {
	// Changed is the input set of changed file IDs.
	Changed []string

	// Affected is the set of files transitively affected, ranked by impact score.
	Affected []AffectedNode

	// TotalAffected is the count of affected nodes (excluding the changed set).
	TotalAffected int

	// MaxDepth is the deepest transitive level reached.
	MaxDepth int
}

// AffectedNode pairs a node with its impact score and traversal depth.
type AffectedNode struct {
	Node        Node
	ImpactScore float64 // higher = more likely to be affected
	Depth       int     // 1 = direct dependent, 2 = transitive, etc.
	Path        []string // shortest dependency path from changed file
}

// BlastRadius computes the set of files transitively affected by changes to
// the given file IDs. It performs a BFS over reverse edges (dependents).
//
// maxDepth limits traversal depth. 0 = unlimited.
func (g *Graph) BlastRadius(changedIDs []string, maxDepth int) BlastRadiusResult {
	result := BlastRadiusResult{Changed: changedIDs}

	// Seed the BFS with the changed set.
	type qitem struct {
		id    string
		depth int
		path  []string
	}

	visited := map[string]bool{}
	for _, id := range changedIDs {
		visited[id] = true
	}

	queue := make([]qitem, 0, len(changedIDs))
	for _, id := range changedIDs {
		queue = append(queue, qitem{id: id, depth: 0, path: []string{id}})
	}

	// best tracks the best (shallowest) depth and path for each affected node.
	type best struct {
		depth int
		path  []string
	}
	bestMap := map[string]best{}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		for _, e := range g.inEdges[item.id] {
			dep := e.From
			if visited[dep] {
				continue
			}
			newDepth := item.depth + 1
			if maxDepth > 0 && newDepth > maxDepth {
				continue
			}
			visited[dep] = true
			newPath := append(append([]string{}, item.path...), dep)
			bestMap[dep] = best{depth: newDepth, path: newPath}
			if newDepth > result.MaxDepth {
				result.MaxDepth = newDepth
			}
			queue = append(queue, qitem{id: dep, depth: newDepth, path: newPath})
		}
	}

	// Build AffectedNode list.
	for id, b := range bestMap {
		n := g.nodes[id]
		if n == nil {
			n = &Node{ID: id, Type: NodeTypeFile}
		}
		// Impact score: direct dependents score 1.0, each level halves it.
		score := 1.0
		for i := 1; i < b.depth; i++ {
			score *= 0.5
		}
		result.Affected = append(result.Affected, AffectedNode{
			Node:        *n,
			ImpactScore: score,
			Depth:       b.depth,
			Path:        b.path,
		})
	}

	// Sort by impact score descending, then by ID for stability.
	sort.Slice(result.Affected, func(i, j int) bool {
		if result.Affected[i].ImpactScore != result.Affected[j].ImpactScore {
			return result.Affected[i].ImpactScore > result.Affected[j].ImpactScore
		}
		return result.Affected[i].Node.ID < result.Affected[j].Node.ID
	})

	result.TotalAffected = len(result.Affected)
	return result
}

// Summary returns a human-readable summary of a blast-radius result.
func (r *BlastRadiusResult) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Changed: %d file(s), Affected: %d file(s), Max depth: %d\n",
		len(r.Changed), r.TotalAffected, r.MaxDepth)
	for _, a := range r.Affected {
		fmt.Fprintf(&sb, "  [depth=%d score=%.2f] %s\n", a.Depth, a.ImpactScore, a.Node.ID)
	}
	return sb.String()
}

// Stats returns aggregate graph statistics.
type Stats struct {
	NodeCount    int
	EdgeCount    int
	MaxInDegree  int
	MaxOutDegree int
	// TopHubs lists the nodes with the highest in-degree (most depended-upon).
	TopHubs []HubNode
}

// HubNode pairs a node ID with its in-degree.
type HubNode struct {
	ID       string
	InDegree int
}

// Stats computes graph statistics.
func (g *Graph) Stats() Stats {
	s := Stats{
		NodeCount: g.NodeCount(),
		EdgeCount: g.EdgeCount(),
	}

	type degree struct {
		id  string
		in  int
		out int
	}
	degrees := make([]degree, 0, len(g.nodes))
	for id := range g.nodes {
		in := len(g.inEdges[id])
		out := len(g.outEdges[id])
		degrees = append(degrees, degree{id, in, out})
		if in > s.MaxInDegree {
			s.MaxInDegree = in
		}
		if out > s.MaxOutDegree {
			s.MaxOutDegree = out
		}
	}

	sort.Slice(degrees, func(i, j int) bool {
		return degrees[i].in > degrees[j].in
	})
	for i := 0; i < 5 && i < len(degrees); i++ {
		s.TopHubs = append(s.TopHubs, HubNode{degrees[i].id, degrees[i].in})
	}
	return s
}
