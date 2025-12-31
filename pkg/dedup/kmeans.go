package dedup

import (
	"context"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"

	simd "github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Config holds deduplication parameters.
type Config struct {
	// Threshold is the cosine distance below which vectors are considered duplicates.
	// Lower values = stricter deduplication. Typical range: 0.01-0.10
	Threshold float64

	// K is the number of clusters. If 0, defaults to sqrt(N/2).
	K int

	// MaxIterations limits K-Means iterations. Default: 10
	MaxIterations int

	// Workers is the number of parallel workers. Default: NumCPU
	Workers int

	// Seed for reproducible clustering. If 0, uses current time.
	Seed int64
}

// DefaultConfig returns sensible defaults for deduplication.
func DefaultConfig() Config {
	return Config{
		Threshold:     0.05,
		MaxIterations: 10,
		Workers:       runtime.NumCPU(),
		Seed:          0,
	}
}

// Engine performs semantic deduplication using custom K-Means clustering.
type Engine struct {
	cfg Config
	rng *rand.Rand
}

// NewEngine creates a deduplication engine with the given config.
func NewEngine(cfg Config) *Engine {
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 10
	}

	return &Engine{
		cfg: cfg,
		rng: rand.New(rand.NewSource(seed)),
	}
}

// cluster holds vectors assigned to a centroid.
type cluster struct {
	centroid []float32
	members  []int // indices into original vector slice
}

// Deduplicate performs semantic deduplication on the input vectors.
// Returns unique vectors and deduplication statistics.
func (e *Engine) Deduplicate(ctx context.Context, vectors []types.Vector) (*types.DeduplicationResult, error) {
	start := time.Now()

	if len(vectors) == 0 {
		return &types.DeduplicationResult{}, nil
	}

	// Determine K
	k := e.cfg.K
	if k <= 0 {
		k = int(math.Sqrt(float64(len(vectors)) / 2))
		if k < 1 {
			k = 1
		}
	}
	if k > len(vectors) {
		k = len(vectors)
	}

	// Run K-Means clustering
	clusters, err := e.kMeans(ctx, vectors, k)
	if err != nil {
		return nil, err
	}

	// Prune duplicates within each cluster
	uniqueIndices := e.pruneClustersConcurrent(ctx, vectors, clusters)

	// Build result
	uniqueVectors := make([]types.Vector, 0, len(uniqueIndices))
	for _, idx := range uniqueIndices {
		uniqueVectors = append(uniqueVectors, vectors[idx])
	}

	return &types.DeduplicationResult{
		UniqueVectors:    uniqueVectors,
		DuplicateCount:   len(vectors) - len(uniqueVectors),
		TotalProcessed:   len(vectors),
		ClusterCount:     k,
		ProcessingTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// kMeans performs K-Means clustering on vectors.
func (e *Engine) kMeans(ctx context.Context, vectors []types.Vector, k int) ([]cluster, error) {
	if len(vectors) == 0 || k == 0 {
		return nil, nil
	}

	dim := vectors[0].Dimension()

	// Initialize centroids using random selection (K-Means++)
	centroids := e.initCentroids(vectors, k, dim)

	// Cluster assignments: vectorIndex -> clusterIndex
	assignments := make([]int, len(vectors))

	for iter := 0; iter < e.cfg.MaxIterations; iter++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Assignment step: parallel
		changed := e.assignVectorsConcurrent(vectors, centroids, assignments)

		// If no assignments changed, we've converged
		if !changed && iter > 0 {
			break
		}

		// Update step: recalculate centroids
		e.updateCentroids(vectors, assignments, centroids, k, dim)
	}

	// Build cluster structures
	clusters := make([]cluster, k)
	for i := range clusters {
		clusters[i].centroid = centroids[i]
		clusters[i].members = make([]int, 0)
	}

	for vecIdx, clusterIdx := range assignments {
		clusters[clusterIdx].members = append(clusters[clusterIdx].members, vecIdx)
	}

	return clusters, nil
}

// initCentroids selects k random vectors as initial centroids.
func (e *Engine) initCentroids(vectors []types.Vector, k, dim int) [][]float32 {
	centroids := make([][]float32, k)

	// Simple random selection
	perm := e.rng.Perm(len(vectors))
	for i := 0; i < k; i++ {
		centroids[i] = make([]float32, dim)
		copy(centroids[i], vectors[perm[i]].Values)
	}

	return centroids
}

// assignVectorsConcurrent assigns each vector to nearest centroid in parallel.
// Returns true if any assignment changed.
func (e *Engine) assignVectorsConcurrent(vectors []types.Vector, centroids [][]float32, assignments []int) bool {
	n := len(vectors)
	workers := e.cfg.Workers
	if workers > n {
		workers = n
	}

	chunkSize := (n + workers - 1) / workers
	var wg sync.WaitGroup
	changedFlags := make([]bool, workers)

	for w := 0; w < workers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID, start, end int) {
			defer wg.Done()
			changed := false

			for i := start; i < end; i++ {
				nearest := e.findNearestCentroid(vectors[i].Values, centroids)
				if assignments[i] != nearest {
					assignments[i] = nearest
					changed = true
				}
			}

			changedFlags[workerID] = changed
		}(w, start, end)
	}

	wg.Wait()

	for _, c := range changedFlags {
		if c {
			return true
		}
	}
	return false
}

// findNearestCentroid returns the index of the closest centroid.
func (e *Engine) findNearestCentroid(vec []float32, centroids [][]float32) int {
	minDist := math.MaxFloat64
	minIdx := 0

	for i, c := range centroids {
		dist := simd.CosineDistance(vec, c)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// updateCentroids recalculates centroids as mean of assigned vectors.
func (e *Engine) updateCentroids(vectors []types.Vector, assignments []int, centroids [][]float32, k, dim int) {
	// Accumulate sums and counts per cluster
	sums := make([][]float64, k)
	counts := make([]int, k)

	for i := range sums {
		sums[i] = make([]float64, dim)
	}

	for vecIdx, clusterIdx := range assignments {
		counts[clusterIdx]++
		for d := 0; d < dim; d++ {
			sums[clusterIdx][d] += float64(vectors[vecIdx].Values[d])
		}
	}

	// Compute means
	for i := 0; i < k; i++ {
		if counts[i] == 0 {
			continue
		}
		invCount := 1.0 / float64(counts[i])
		for d := 0; d < dim; d++ {
			centroids[i][d] = float32(sums[i][d] * invCount)
		}
	}
}

// pruneClustersConcurrent identifies unique vectors within each cluster.
func (e *Engine) pruneClustersConcurrent(ctx context.Context, vectors []types.Vector, clusters []cluster) []int {
	var mu sync.Mutex
	uniqueIndices := make([]int, 0, len(vectors))

	var wg sync.WaitGroup
	sem := make(chan struct{}, e.cfg.Workers)

	for _, cl := range clusters {
		if len(cl.members) == 0 {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(c cluster) {
			defer wg.Done()
			defer func() { <-sem }()

			unique := e.pruneCluster(vectors, c)

			mu.Lock()
			uniqueIndices = append(uniqueIndices, unique...)
			mu.Unlock()
		}(cl)
	}

	wg.Wait()
	return uniqueIndices
}

// pruneCluster identifies unique vectors within a single cluster.
// Uses medoid-based comparison for efficiency.
func (e *Engine) pruneCluster(vectors []types.Vector, cl cluster) []int {
	if len(cl.members) == 0 {
		return nil
	}

	if len(cl.members) == 1 {
		return cl.members
	}

	// Find medoid: vector closest to centroid
	medoidIdx := cl.members[0]
	minDist := simd.CosineDistance(vectors[medoidIdx].Values, cl.centroid)

	for _, idx := range cl.members[1:] {
		dist := simd.CosineDistance(vectors[idx].Values, cl.centroid)
		if dist < minDist {
			minDist = dist
			medoidIdx = idx
		}
	}

	// Mark duplicates: vectors too similar to medoid
	unique := make([]int, 0, len(cl.members))
	unique = append(unique, medoidIdx) // Medoid is always kept

	medoidVec := vectors[medoidIdx].Values

	for _, idx := range cl.members {
		if idx == medoidIdx {
			continue
		}

		dist := simd.CosineDistance(vectors[idx].Values, medoidVec)
		if dist >= e.cfg.Threshold {
			// Not a duplicate - distance exceeds threshold
			unique = append(unique, idx)
		}
	}

	return unique
}
