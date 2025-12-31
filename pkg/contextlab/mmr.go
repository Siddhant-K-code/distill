package contextlab

import (
	"github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// MMRConfig holds Maximal Marginal Relevance parameters.
type MMRConfig struct {
	// Lambda controls the relevance vs diversity tradeoff.
	// 1.0 = pure relevance (no diversity)
	// 0.0 = pure diversity (ignore relevance)
	// 0.5 = balanced (recommended)
	Lambda float64

	// TargetK is the number of chunks to select.
	TargetK int
}

// DefaultMMRConfig returns sensible defaults.
func DefaultMMRConfig() MMRConfig {
	return MMRConfig{
		Lambda:  0.5,
		TargetK: 8,
	}
}

// MMR performs Maximal Marginal Relevance re-ranking.
// It greedily selects chunks that balance relevance and diversity.
type MMR struct {
	cfg MMRConfig
}

// NewMMR creates a new MMR re-ranker with the given config.
func NewMMR(cfg MMRConfig) *MMR {
	if cfg.Lambda < 0 {
		cfg.Lambda = 0
	}
	if cfg.Lambda > 1 {
		cfg.Lambda = 1
	}
	if cfg.TargetK <= 0 {
		cfg.TargetK = 8
	}
	return &MMR{cfg: cfg}
}

// Rerank selects diverse chunks using MMR algorithm.
// Formula: MMR = λ * score(chunk) - (1-λ) * max(similarity(chunk, selected))
func (m *MMR) Rerank(chunks []types.Chunk) []types.Chunk {
	if len(chunks) == 0 {
		return nil
	}

	if len(chunks) <= m.cfg.TargetK {
		return chunks
	}

	// Normalize scores to [0, 1] for fair comparison with similarity
	normalizedScores := m.normalizeScores(chunks)

	// Track selected and remaining indices
	selected := make([]int, 0, m.cfg.TargetK)
	remaining := make(map[int]bool, len(chunks))
	for i := range chunks {
		remaining[i] = true
	}

	// Precompute similarity matrix for efficiency
	simMatrix := m.computeSimilarityMatrix(chunks)

	// Greedy selection
	for len(selected) < m.cfg.TargetK && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := float64(-2) // MMR can be negative

		for idx := range remaining {
			mmrScore := m.computeMMRScore(idx, selected, normalizedScores, simMatrix)
			if mmrScore > bestMMR {
				bestMMR = mmrScore
				bestIdx = idx
			}
		}

		if bestIdx >= 0 {
			selected = append(selected, bestIdx)
			delete(remaining, bestIdx)
		} else {
			break
		}
	}

	// Build result
	result := make([]types.Chunk, len(selected))
	for i, idx := range selected {
		result[i] = chunks[idx]
	}

	return result
}

// normalizeScores normalizes chunk scores to [0, 1].
func (m *MMR) normalizeScores(chunks []types.Chunk) []float64 {
	if len(chunks) == 0 {
		return nil
	}

	minScore := float64(chunks[0].Score)
	maxScore := float64(chunks[0].Score)

	for _, c := range chunks[1:] {
		s := float64(c.Score)
		if s < minScore {
			minScore = s
		}
		if s > maxScore {
			maxScore = s
		}
	}

	normalized := make([]float64, len(chunks))
	scoreRange := maxScore - minScore

	if scoreRange == 0 {
		// All scores are equal
		for i := range normalized {
			normalized[i] = 1.0
		}
	} else {
		for i, c := range chunks {
			normalized[i] = (float64(c.Score) - minScore) / scoreRange
		}
	}

	return normalized
}

// computeSimilarityMatrix computes pairwise cosine similarities.
func (m *MMR) computeSimilarityMatrix(chunks []types.Chunk) [][]float64 {
	n := len(chunks)
	matrix := make([][]float64, n)

	// Initialize all rows first
	for i := 0; i < n; i++ {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0 // Self-similarity
	}

	// Compute similarities
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			// Handle missing embeddings
			if len(chunks[i].Embedding) == 0 || len(chunks[j].Embedding) == 0 {
				matrix[i][j] = 0.0
				matrix[j][i] = 0.0
				continue
			}
			// Similarity = 1 - distance
			sim := 1.0 - math.CosineDistance(chunks[i].Embedding, chunks[j].Embedding)
			matrix[i][j] = sim
			matrix[j][i] = sim
		}
	}

	return matrix
}

// computeMMRScore computes the MMR score for a candidate chunk.
// MMR = λ * relevance - (1-λ) * max_similarity_to_selected
func (m *MMR) computeMMRScore(candidateIdx int, selected []int, scores []float64, simMatrix [][]float64) float64 {
	relevance := scores[candidateIdx]

	// If nothing selected yet, MMR = λ * relevance
	if len(selected) == 0 {
		return m.cfg.Lambda * relevance
	}

	// Find max similarity to any selected chunk
	maxSim := float64(0)
	for _, selIdx := range selected {
		sim := simMatrix[candidateIdx][selIdx]
		if sim > maxSim {
			maxSim = sim
		}
	}

	// MMR formula
	return m.cfg.Lambda*relevance - (1-m.cfg.Lambda)*maxSim
}

// RerankWithQuery performs MMR using query similarity as the relevance signal.
// This is useful when chunk scores are not available or unreliable.
func (m *MMR) RerankWithQuery(chunks []types.Chunk, queryEmbedding []float32) []types.Chunk {
	if len(chunks) == 0 || len(queryEmbedding) == 0 {
		return chunks
	}

	// Compute query similarities as relevance scores
	for i := range chunks {
		sim := 1.0 - math.CosineDistance(chunks[i].Embedding, queryEmbedding)
		chunks[i].Score = float32(sim)
	}

	return m.Rerank(chunks)
}

// MMRRerank is a convenience function for one-shot MMR re-ranking.
func MMRRerank(chunks []types.Chunk, lambda float64, targetK int) []types.Chunk {
	cfg := MMRConfig{
		Lambda:  lambda,
		TargetK: targetK,
	}
	return NewMMR(cfg).Rerank(chunks)
}

// DiversityScore computes the average pairwise distance of selected chunks.
// Higher values indicate more diverse selection.
func DiversityScore(chunks []types.Chunk) float64 {
	if len(chunks) < 2 {
		return 0
	}

	var totalDist float64
	pairs := 0

	for i := 0; i < len(chunks)-1; i++ {
		for j := i + 1; j < len(chunks); j++ {
			totalDist += math.CosineDistance(chunks[i].Embedding, chunks[j].Embedding)
			pairs++
		}
	}

	if pairs == 0 {
		return 0
	}

	return totalDist / float64(pairs)
}

// CoverageScore estimates how well the selected chunks cover the original set.
// For each original chunk, finds the minimum distance to any selected chunk.
// Lower average distance = better coverage.
func CoverageScore(selected, original []types.Chunk) float64 {
	if len(selected) == 0 || len(original) == 0 {
		return 0
	}

	var totalMinDist float64

	for _, orig := range original {
		minDist := float64(2.0)
		for _, sel := range selected {
			dist := math.CosineDistance(orig.Embedding, sel.Embedding)
			if dist < minDist {
				minDist = dist
			}
		}
		totalMinDist += minDist
	}

	return totalMinDist / float64(len(original))
}
