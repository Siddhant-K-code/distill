package contextlab

import (
	"context"
	"fmt"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/retriever"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// BrokerConfig holds the configuration for the ContextLab broker.
type BrokerConfig struct {
	// OverFetchK is the number of chunks to retrieve from the vector DB.
	// Should be larger than TargetK to allow for deduplication.
	// Recommended: 3-5x TargetK
	OverFetchK int

	// TargetK is the final number of chunks to return.
	TargetK int

	// ClusterThreshold is the cosine distance threshold for clustering.
	// Lower = more clusters, less aggressive deduplication.
	ClusterThreshold float64

	// ClusterLinkage determines how cluster distances are computed.
	// Options: "single", "complete", "average"
	ClusterLinkage string

	// SelectionStrategy determines how representatives are picked.
	// Options: "score", "centroid", "length", "hybrid"
	SelectionStrategy SelectionStrategy

	// EnableMMR enables Maximal Marginal Relevance re-ranking.
	EnableMMR bool

	// MMRLambda controls relevance vs diversity tradeoff (0-1).
	// 1.0 = pure relevance, 0.0 = pure diversity, 0.5 = balanced
	MMRLambda float64

	// IncludeEmbeddings requests embeddings in retrieval results.
	// Required for clustering - will be enabled automatically if false.
	IncludeEmbeddings bool

	// IncludeMetadata requests metadata in retrieval results.
	IncludeMetadata bool
}

// DefaultBrokerConfig returns sensible defaults.
func DefaultBrokerConfig() BrokerConfig {
	return BrokerConfig{
		OverFetchK:        50,
		TargetK:           8,
		ClusterThreshold:  0.15,
		ClusterLinkage:    "average",
		SelectionStrategy: SelectByScore,
		EnableMMR:         true,
		MMRLambda:         0.5,
		IncludeEmbeddings: true,
		IncludeMetadata:   true,
	}
}

// Broker orchestrates the semantic deduplication pipeline for RAG.
// It retrieves chunks, clusters them, selects representatives, and
// optionally applies MMR for diversity.
type Broker struct {
	cfg       BrokerConfig
	retriever retriever.Retriever
	embedder  retriever.EmbeddingProvider
	clusterer *Clusterer
	selector  *Selector
	mmr       *MMR
}

// NewBroker creates a new ContextLab broker.
func NewBroker(ret retriever.Retriever, cfg BrokerConfig) *Broker {
	// Ensure embeddings are included (required for clustering)
	cfg.IncludeEmbeddings = true

	// Apply defaults
	if cfg.OverFetchK <= 0 {
		cfg.OverFetchK = 50
	}
	if cfg.TargetK <= 0 {
		cfg.TargetK = 8
	}
	if cfg.ClusterThreshold <= 0 {
		cfg.ClusterThreshold = 0.15
	}
	if cfg.MMRLambda < 0 || cfg.MMRLambda > 1 {
		cfg.MMRLambda = 0.5
	}

	// Create sub-components
	clusterer := NewClusterer(ClusterConfig{
		Threshold: cfg.ClusterThreshold,
		Linkage:   cfg.ClusterLinkage,
	})

	selector := NewSelector(SelectorConfig{
		Strategy: cfg.SelectionStrategy,
	})

	var mmr *MMR
	if cfg.EnableMMR {
		mmr = NewMMR(MMRConfig{
			Lambda:  cfg.MMRLambda,
			TargetK: cfg.TargetK,
		})
	}

	return &Broker{
		cfg:       cfg,
		retriever: ret,
		clusterer: clusterer,
		selector:  selector,
		mmr:       mmr,
	}
}

// NewBrokerWithEmbedder creates a broker that can handle text queries.
func NewBrokerWithEmbedder(ret retriever.Retriever, emb retriever.EmbeddingProvider, cfg BrokerConfig) *Broker {
	broker := NewBroker(ret, cfg)
	broker.embedder = emb
	return broker
}

// Retrieve performs the full deduplication pipeline.
func (b *Broker) Retrieve(ctx context.Context, req *types.RetrievalRequest) (*types.BrokerResult, error) {
	totalStart := time.Now()
	stats := types.BrokerStats{}

	// Step 1: Embed query if needed
	if req.Query != "" && len(req.QueryEmbedding) == 0 {
		if b.embedder == nil {
			return nil, fmt.Errorf("embedding provider required for text queries")
		}
		embedding, err := b.embedder.Embed(ctx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to embed query: %w", err)
		}
		req.QueryEmbedding = embedding
	}

	if len(req.QueryEmbedding) == 0 {
		return nil, retriever.ErrInvalidQuery
	}

	// Step 2: Over-fetch from vector DB
	req.TopK = b.cfg.OverFetchK
	req.IncludeEmbeddings = true
	req.IncludeMetadata = b.cfg.IncludeMetadata

	retrievalStart := time.Now()
	result, err := b.retriever.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}
	stats.RetrievalLatency = time.Since(retrievalStart)
	stats.Retrieved = len(result.Chunks)

	if len(result.Chunks) == 0 {
		return &types.BrokerResult{
			Chunks: []types.Chunk{},
			Stats:  stats,
		}, nil
	}

	// Step 3: Cluster retrieved chunks
	clusterStart := time.Now()
	clusterResult := b.clusterer.Cluster(result.Chunks)
	stats.ClusteringLatency = time.Since(clusterStart)
	stats.Clustered = clusterResult.ClusterCount

	// Step 4: Select representatives from each cluster
	representatives := b.selector.Select(clusterResult)

	// Step 5: Apply MMR if enabled
	var finalChunks []types.Chunk
	if b.cfg.EnableMMR && b.mmr != nil && len(representatives) > b.cfg.TargetK {
		finalChunks = b.mmr.Rerank(representatives)
	} else if len(representatives) > b.cfg.TargetK {
		// Just take top K by score
		finalChunks = SelectTopK(clusterResult, b.cfg.TargetK, b.cfg.SelectionStrategy)
	} else {
		finalChunks = representatives
	}

	stats.Returned = len(finalChunks)
	stats.TotalLatency = time.Since(totalStart)

	return &types.BrokerResult{
		Chunks: finalChunks,
		Stats:  stats,
	}, nil
}

// RetrieveByText is a convenience method for text queries.
func (b *Broker) RetrieveByText(ctx context.Context, query string, namespace string) (*types.BrokerResult, error) {
	req := &types.RetrievalRequest{
		Query:     query,
		Namespace: namespace,
	}
	return b.Retrieve(ctx, req)
}

// RetrieveByVector is a convenience method for vector queries.
func (b *Broker) RetrieveByVector(ctx context.Context, embedding []float32, namespace string) (*types.BrokerResult, error) {
	req := &types.RetrievalRequest{
		QueryEmbedding: embedding,
		Namespace:      namespace,
	}
	return b.Retrieve(ctx, req)
}

// RetrieveWithFilter adds metadata filtering to the query.
func (b *Broker) RetrieveWithFilter(ctx context.Context, req *types.RetrievalRequest, filter map[string]interface{}) (*types.BrokerResult, error) {
	req.Filter = filter
	return b.Retrieve(ctx, req)
}

// SetConfig updates the broker configuration.
func (b *Broker) SetConfig(cfg BrokerConfig) {
	b.cfg = cfg
	b.cfg.IncludeEmbeddings = true

	b.clusterer = NewClusterer(ClusterConfig{
		Threshold: cfg.ClusterThreshold,
		Linkage:   cfg.ClusterLinkage,
	})

	b.selector = NewSelector(SelectorConfig{
		Strategy: cfg.SelectionStrategy,
	})

	if cfg.EnableMMR {
		b.mmr = NewMMR(MMRConfig{
			Lambda:  cfg.MMRLambda,
			TargetK: cfg.TargetK,
		})
	} else {
		b.mmr = nil
	}
}

// GetConfig returns the current configuration.
func (b *Broker) GetConfig() BrokerConfig {
	return b.cfg
}

// Close releases resources.
func (b *Broker) Close() error {
	if b.retriever != nil {
		return b.retriever.Close()
	}
	return nil
}

// ProcessChunks applies deduplication to pre-fetched chunks.
// Useful when you want to use the broker's logic without retrieval.
func (b *Broker) ProcessChunks(chunks []types.Chunk) *types.BrokerResult {
	totalStart := time.Now()
	stats := types.BrokerStats{
		Retrieved: len(chunks),
	}

	if len(chunks) == 0 {
		return &types.BrokerResult{
			Chunks: []types.Chunk{},
			Stats:  stats,
		}
	}

	// Cluster
	clusterStart := time.Now()
	clusterResult := b.clusterer.Cluster(chunks)
	stats.ClusteringLatency = time.Since(clusterStart)
	stats.Clustered = clusterResult.ClusterCount

	// Select representatives
	representatives := b.selector.Select(clusterResult)

	// Apply MMR if enabled
	var finalChunks []types.Chunk
	if b.cfg.EnableMMR && b.mmr != nil && len(representatives) > b.cfg.TargetK {
		finalChunks = b.mmr.Rerank(representatives)
	} else if len(representatives) > b.cfg.TargetK {
		finalChunks = SelectTopK(clusterResult, b.cfg.TargetK, b.cfg.SelectionStrategy)
	} else {
		finalChunks = representatives
	}

	stats.Returned = len(finalChunks)
	stats.TotalLatency = time.Since(totalStart)

	return &types.BrokerResult{
		Chunks: finalChunks,
		Stats:  stats,
	}
}
