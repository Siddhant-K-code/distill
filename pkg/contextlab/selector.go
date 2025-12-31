package contextlab

import (
	"github.com/Siddhant-K-code/distill/pkg/math"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// SelectionStrategy defines how to pick a representative from a cluster.
type SelectionStrategy string

const (
	// SelectByScore picks the chunk with the highest retrieval score.
	// Best for preserving relevance ranking.
	SelectByScore SelectionStrategy = "score"

	// SelectByCentroid picks the chunk closest to the cluster centroid.
	// Best for finding the most "typical" chunk.
	SelectByCentroid SelectionStrategy = "centroid"

	// SelectByLength picks the chunk with the longest text.
	// Best when longer chunks contain more information.
	SelectByLength SelectionStrategy = "length"

	// SelectByHybrid uses a weighted combination of score and centroid distance.
	SelectByHybrid SelectionStrategy = "hybrid"
)

// SelectorConfig holds selection parameters.
type SelectorConfig struct {
	// Strategy determines the selection method.
	Strategy SelectionStrategy

	// ScoreWeight is the weight for score in hybrid selection (0-1).
	// Higher values favor relevance over typicality.
	ScoreWeight float64

	// CentroidWeight is the weight for centroid proximity in hybrid selection (0-1).
	CentroidWeight float64

	// LengthWeight is the weight for text length in hybrid selection (0-1).
	LengthWeight float64
}

// DefaultSelectorConfig returns sensible defaults.
func DefaultSelectorConfig() SelectorConfig {
	return SelectorConfig{
		Strategy:       SelectByScore,
		ScoreWeight:    0.7,
		CentroidWeight: 0.3,
		LengthWeight:   0.0,
	}
}

// Selector picks representative chunks from clusters.
type Selector struct {
	cfg SelectorConfig
}

// NewSelector creates a new selector with the given config.
func NewSelector(cfg SelectorConfig) *Selector {
	if cfg.Strategy == "" {
		cfg.Strategy = SelectByScore
	}
	return &Selector{cfg: cfg}
}

// Select picks representatives from all clusters.
func (s *Selector) Select(result *types.ClusterResult) []types.Chunk {
	if result == nil || len(result.Clusters) == 0 {
		return nil
	}

	representatives := make([]types.Chunk, 0, len(result.Clusters))

	for i := range result.Clusters {
		rep := s.SelectFromCluster(&result.Clusters[i])
		if rep != nil {
			representatives = append(representatives, *rep)
			result.Clusters[i].Representative = rep
		}
	}

	result.Representatives = representatives
	return representatives
}

// SelectFromCluster picks a single representative from a cluster.
func (s *Selector) SelectFromCluster(cluster *types.Cluster) *types.Chunk {
	if cluster == nil || len(cluster.Members) == 0 {
		return nil
	}

	if len(cluster.Members) == 1 {
		return &cluster.Members[0]
	}

	switch s.cfg.Strategy {
	case SelectByScore:
		return s.selectByScore(cluster)
	case SelectByCentroid:
		return s.selectByCentroid(cluster)
	case SelectByLength:
		return s.selectByLength(cluster)
	case SelectByHybrid:
		return s.selectByHybrid(cluster)
	default:
		return s.selectByScore(cluster)
	}
}

// selectByScore picks the chunk with the highest retrieval score.
func (s *Selector) selectByScore(cluster *types.Cluster) *types.Chunk {
	best := &cluster.Members[0]
	for i := 1; i < len(cluster.Members); i++ {
		if cluster.Members[i].Score > best.Score {
			best = &cluster.Members[i]
		}
	}
	return best
}

// selectByCentroid picks the chunk closest to the cluster centroid.
func (s *Selector) selectByCentroid(cluster *types.Cluster) *types.Chunk {
	if len(cluster.Centroid) == 0 {
		return s.selectByScore(cluster)
	}

	best := &cluster.Members[0]
	bestDist := math.CosineDistance(best.Embedding, cluster.Centroid)

	for i := 1; i < len(cluster.Members); i++ {
		dist := math.CosineDistance(cluster.Members[i].Embedding, cluster.Centroid)
		if dist < bestDist {
			bestDist = dist
			best = &cluster.Members[i]
		}
	}
	return best
}

// selectByLength picks the chunk with the longest text.
func (s *Selector) selectByLength(cluster *types.Cluster) *types.Chunk {
	best := &cluster.Members[0]
	for i := 1; i < len(cluster.Members); i++ {
		if len(cluster.Members[i].Text) > len(best.Text) {
			best = &cluster.Members[i]
		}
	}
	return best
}

// selectByHybrid uses a weighted combination of factors.
func (s *Selector) selectByHybrid(cluster *types.Cluster) *types.Chunk {
	if len(cluster.Centroid) == 0 {
		return s.selectByScore(cluster)
	}

	// Normalize weights
	totalWeight := s.cfg.ScoreWeight + s.cfg.CentroidWeight + s.cfg.LengthWeight
	if totalWeight == 0 {
		return s.selectByScore(cluster)
	}

	scoreW := s.cfg.ScoreWeight / totalWeight
	centroidW := s.cfg.CentroidWeight / totalWeight
	lengthW := s.cfg.LengthWeight / totalWeight

	// Find min/max for normalization
	minScore, maxScore := cluster.Members[0].Score, cluster.Members[0].Score
	minDist, maxDist := float64(2.0), float64(0.0)
	minLen, maxLen := len(cluster.Members[0].Text), len(cluster.Members[0].Text)

	distances := make([]float64, len(cluster.Members))
	for i := range cluster.Members {
		if cluster.Members[i].Score < minScore {
			minScore = cluster.Members[i].Score
		}
		if cluster.Members[i].Score > maxScore {
			maxScore = cluster.Members[i].Score
		}

		distances[i] = math.CosineDistance(cluster.Members[i].Embedding, cluster.Centroid)
		if distances[i] < minDist {
			minDist = distances[i]
		}
		if distances[i] > maxDist {
			maxDist = distances[i]
		}

		textLen := len(cluster.Members[i].Text)
		if textLen < minLen {
			minLen = textLen
		}
		if textLen > maxLen {
			maxLen = textLen
		}
	}

	// Compute hybrid scores
	best := &cluster.Members[0]
	bestHybrid := float64(-1)

	scoreRange := float64(maxScore - minScore)
	distRange := maxDist - minDist
	lenRange := float64(maxLen - minLen)

	for i := range cluster.Members {
		var hybridScore float64

		// Normalized score (higher is better)
		if scoreRange > 0 {
			hybridScore += scoreW * float64(cluster.Members[i].Score-minScore) / scoreRange
		} else {
			hybridScore += scoreW
		}

		// Normalized centroid proximity (lower distance is better, so invert)
		if distRange > 0 {
			hybridScore += centroidW * (1.0 - (distances[i]-minDist)/distRange)
		} else {
			hybridScore += centroidW
		}

		// Normalized length (longer is better)
		if lenRange > 0 {
			hybridScore += lengthW * float64(len(cluster.Members[i].Text)-minLen) / lenRange
		} else {
			hybridScore += lengthW
		}

		if hybridScore > bestHybrid {
			bestHybrid = hybridScore
			best = &cluster.Members[i]
		}
	}

	return best
}

// SelectTopK selects representatives and returns the top K by score.
func SelectTopK(result *types.ClusterResult, k int, strategy SelectionStrategy) []types.Chunk {
	cfg := DefaultSelectorConfig()
	cfg.Strategy = strategy

	selector := NewSelector(cfg)
	reps := selector.Select(result)

	if len(reps) <= k {
		return reps
	}

	// Sort by score descending
	for i := 0; i < len(reps)-1; i++ {
		for j := i + 1; j < len(reps); j++ {
			if reps[j].Score > reps[i].Score {
				reps[i], reps[j] = reps[j], reps[i]
			}
		}
	}

	return reps[:k]
}
