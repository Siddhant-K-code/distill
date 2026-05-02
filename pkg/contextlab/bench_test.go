package contextlab

import (
	"math/rand"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// deterministicEmbedding generates a reproducible embedding for a given seed.
// Using a fixed seed ensures benchmark results are stable across runs.
func deterministicEmbedding(seed int64, dims int) []float32 {
	rng := rand.New(rand.NewSource(seed))
	v := make([]float32, dims)
	for i := range v {
		v[i] = rng.Float32()
	}
	return v
}

// makeBenchChunks builds n chunks with deterministic embeddings.
func makeBenchChunks(n, dims int) []types.Chunk {
	chunks := make([]types.Chunk, n)
	for i := range chunks {
		chunks[i] = types.Chunk{
			ID:        string(rune('A'+i%26)) + string(rune('0'+i/26%10)),
			Text:      "benchmark chunk content for semantic deduplication testing",
			Embedding: deterministicEmbedding(int64(i), dims),
		}
	}
	return chunks
}

func BenchmarkCluster_10Chunks(b *testing.B) {
	chunks := makeBenchChunks(10, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClusterByThreshold(chunks, 0.15)
	}
}

func BenchmarkCluster_50Chunks(b *testing.B) {
	chunks := makeBenchChunks(50, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClusterByThreshold(chunks, 0.15)
	}
}

func BenchmarkCluster_100Chunks(b *testing.B) {
	chunks := makeBenchChunks(100, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClusterByThreshold(chunks, 0.15)
	}
}

func BenchmarkCluster_500Chunks(b *testing.B) {
	chunks := makeBenchChunks(500, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClusterByThreshold(chunks, 0.15)
	}
}

func BenchmarkMMR_10Chunks(b *testing.B) {
	chunks := makeBenchChunks(10, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MMRRerank(chunks, 0.7, 5)
	}
}

func BenchmarkMMR_50Chunks(b *testing.B) {
	chunks := makeBenchChunks(50, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MMRRerank(chunks, 0.7, 10)
	}
}

func BenchmarkSelector_10Clusters(b *testing.B) {
	chunks := makeBenchChunks(10, 128)
	result := ClusterByThreshold(chunks, 0.15)
	sel := NewSelector(DefaultSelectorConfig())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sel.Select(result)
	}
}

func BenchmarkSelector_50Clusters(b *testing.B) {
	chunks := makeBenchChunks(50, 128)
	result := ClusterByThreshold(chunks, 0.15)
	sel := NewSelector(DefaultSelectorConfig())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sel.Select(result)
	}
}
