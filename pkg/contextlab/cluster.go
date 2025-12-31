package contextlab

import (
	"sort"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// ClusterConfig holds clustering parameters.
type ClusterConfig struct {
	// Threshold is the maximum cosine distance for merging clusters.
	// Lower values = more clusters, less aggressive merging.
	// Typical range: 0.10-0.30
	Threshold float64

	// MinClusters is the minimum number of clusters to form (optional).
	// If 0, clustering stops only based on threshold.
	MinClusters int

	// MaxClusters is the maximum number of clusters (optional).
	// If 0, no limit is applied.
	MaxClusters int

	// Linkage determines how inter-cluster distance is computed.
	// Options: "single", "complete", "average" (default: "average")
	Linkage string
}

// DefaultClusterConfig returns sensible defaults.
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		Threshold:   0.15,
		MinClusters: 0,
		MaxClusters: 0,
		Linkage:     "average",
	}
}

// Clusterer performs agglomerative clustering on chunks.
type Clusterer struct {
	cfg ClusterConfig
}

// NewClusterer creates a new clusterer with the given config.
func NewClusterer(cfg ClusterConfig) *Clusterer {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.15
	}
	if cfg.Linkage == "" {
		cfg.Linkage = "average"
	}
	return &Clusterer{cfg: cfg}
}

// clusterNode represents a node in the clustering hierarchy.
type clusterNode struct {
	id       int
	members  []int // indices into original chunk slice
	centroid []float32
	active   bool
}

// Cluster performs agglomerative clustering on the given chunks.
// Returns clusters with assigned members and centroids.
func (c *Clusterer) Cluster(chunks []types.Chunk) *types.ClusterResult {
	start := time.Now()

	n := len(chunks)
	if n == 0 {
		return &types.ClusterResult{
			Clusters:        []types.Cluster{},
			Representatives: []types.Chunk{},
			InputCount:      0,
			ClusterCount:    0,
			Latency:         time.Since(start),
		}
	}

	if n == 1 {
		chunks[0].ClusterID = 0
		return &types.ClusterResult{
			Clusters: []types.Cluster{{
				ID:       0,
				Members:  []types.Chunk{chunks[0]},
				Centroid: chunks[0].Embedding,
			}},
			Representatives: []types.Chunk{chunks[0]},
			InputCount:      1,
			ClusterCount:    1,
			Latency:         time.Since(start),
		}
	}

	// Check if embeddings are present
	hasEmbeddings := false
	for _, chunk := range chunks {
		if len(chunk.Embedding) > 0 {
			hasEmbeddings = true
			break
		}
	}

	// If no embeddings, return all chunks as separate clusters (no dedup possible)
	if !hasEmbeddings {
		clusters := make([]types.Cluster, n)
		for i := range chunks {
			chunks[i].ClusterID = i
			clusters[i] = types.Cluster{
				ID:      i,
				Members: []types.Chunk{chunks[i]},
			}
		}
		return &types.ClusterResult{
			Clusters:        clusters,
			Representatives: chunks,
			InputCount:      n,
			ClusterCount:    n,
			Latency:         time.Since(start),
		}
	}

	// Initialize each chunk as its own cluster
	nodes := make([]*clusterNode, n)
	for i := range chunks {
		centroid := make([]float32, len(chunks[i].Embedding))
		copy(centroid, chunks[i].Embedding)
		nodes[i] = &clusterNode{
			id:       i,
			members:  []int{i},
			centroid: centroid,
			active:   true,
		}
	}

	// Compute initial distance matrix (upper triangular)
	distMatrix := c.computeDistanceMatrix(chunks)

	// Agglomerative merging
	activeCount := n
	for activeCount > 1 {
		// Check stopping conditions
		if c.cfg.MinClusters > 0 && activeCount <= c.cfg.MinClusters {
			break
		}

		// Find closest pair of clusters
		minDist := float64(2.0) // Max cosine distance
		minI, minJ := -1, -1

		for i := 0; i < n; i++ {
			if !nodes[i].active {
				continue
			}
			for j := i + 1; j < n; j++ {
				if !nodes[j].active {
					continue
				}

				dist := c.clusterDistance(nodes[i], nodes[j], chunks, distMatrix)
				if dist < minDist {
					minDist = dist
					minI, minJ = i, j
				}
			}
		}

		// Check if we should stop merging
		if minDist > c.cfg.Threshold {
			break
		}

		// Merge clusters i and j into i
		c.mergeClusters(nodes[minI], nodes[minJ], chunks)
		nodes[minJ].active = false
		activeCount--

		// Check max clusters limit
		if c.cfg.MaxClusters > 0 && activeCount <= c.cfg.MaxClusters {
			break
		}
	}

	// Build result from active clusters
	clusters := make([]types.Cluster, 0, activeCount)
	clusterID := 0

	for _, node := range nodes {
		if !node.active {
			continue
		}

		members := make([]types.Chunk, len(node.members))
		for i, idx := range node.members {
			chunks[idx].ClusterID = clusterID
			members[i] = chunks[idx]
		}

		clusters = append(clusters, types.Cluster{
			ID:       clusterID,
			Members:  members,
			Centroid: node.centroid,
		})
		clusterID++
	}

	return &types.ClusterResult{
		Clusters:     clusters,
		InputCount:   n,
		ClusterCount: len(clusters),
		Latency:      time.Since(start),
	}
}

// computeDistanceMatrix computes pairwise cosine distances.
func (c *Clusterer) computeDistanceMatrix(chunks []types.Chunk) [][]float64 {
	n := len(chunks)
	matrix := make([][]float64, n)

	// Initialize all rows first
	for i := 0; i < n; i++ {
		matrix[i] = make([]float64, n)
	}

	// Compute distances
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			// Handle missing embeddings gracefully
			if len(chunks[i].Embedding) == 0 || len(chunks[j].Embedding) == 0 {
				matrix[i][j] = 2.0 // Max distance
				matrix[j][i] = 2.0
				continue
			}
			dist := math.CosineDistance(chunks[i].Embedding, chunks[j].Embedding)
			matrix[i][j] = dist
			matrix[j][i] = dist
		}
	}

	return matrix
}

// clusterDistance computes distance between two clusters based on linkage type.
func (c *Clusterer) clusterDistance(a, b *clusterNode, chunks []types.Chunk, distMatrix [][]float64) float64 {
	switch c.cfg.Linkage {
	case "single":
		// Minimum distance between any pair
		minDist := float64(2.0)
		for _, i := range a.members {
			for _, j := range b.members {
				if distMatrix[i][j] < minDist {
					minDist = distMatrix[i][j]
				}
			}
		}
		return minDist

	case "complete":
		// Maximum distance between any pair
		maxDist := float64(0.0)
		for _, i := range a.members {
			for _, j := range b.members {
				if distMatrix[i][j] > maxDist {
					maxDist = distMatrix[i][j]
				}
			}
		}
		return maxDist

	case "average":
		fallthrough
	default:
		// Average distance between all pairs
		var sum float64
		count := 0
		for _, i := range a.members {
			for _, j := range b.members {
				sum += distMatrix[i][j]
				count++
			}
		}
		if count == 0 {
			return 2.0
		}
		return sum / float64(count)
	}
}

// mergeClusters merges cluster b into cluster a.
func (c *Clusterer) mergeClusters(a, b *clusterNode, chunks []types.Chunk) {
	// Merge members
	a.members = append(a.members, b.members...)

	// Recompute centroid as mean of all member embeddings
	if len(chunks) > 0 && len(chunks[0].Embedding) > 0 {
		dim := len(chunks[0].Embedding)
		newCentroid := make([]float32, dim)

		for _, idx := range a.members {
			for d := 0; d < dim; d++ {
				newCentroid[d] += chunks[idx].Embedding[d]
			}
		}

		invN := float32(1.0 / float64(len(a.members)))
		for d := 0; d < dim; d++ {
			newCentroid[d] *= invN
		}

		a.centroid = newCentroid
	}
}

// ClusterByThreshold is a convenience function for one-shot clustering.
func ClusterByThreshold(chunks []types.Chunk, threshold float64) *types.ClusterResult {
	cfg := DefaultClusterConfig()
	cfg.Threshold = threshold
	return NewClusterer(cfg).Cluster(chunks)
}

// SortClustersBySize sorts clusters by member count (descending).
func SortClustersBySize(clusters []types.Cluster) {
	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i].Members) > len(clusters[j].Members)
	})
}

// SortClustersByMaxScore sorts clusters by highest member score (descending).
func SortClustersByMaxScore(clusters []types.Cluster) {
	sort.Slice(clusters, func(i, j int) bool {
		maxI := maxScore(clusters[i].Members)
		maxJ := maxScore(clusters[j].Members)
		return maxI > maxJ
	})
}

func maxScore(chunks []types.Chunk) float32 {
	if len(chunks) == 0 {
		return 0
	}
	max := chunks[0].Score
	for _, c := range chunks[1:] {
		if c.Score > max {
			max = c.Score
		}
	}
	return max
}
