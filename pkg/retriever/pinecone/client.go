package pinecone

import (
	"context"
	"fmt"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/retriever"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
)

// Client implements the Retriever interface for Pinecone.
type Client struct {
	cfg     Config
	pc      *pinecone.Client
	idxConn *pinecone.IndexConnection
}

// Config holds Pinecone-specific configuration.
type Config struct {
	retriever.Config

	// IndexName is the Pinecone index to query
	IndexName string

	// IndexHost is the direct host URL (optional, will be resolved from IndexName)
	IndexHost string
}

// NewClient creates a new Pinecone retriever client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if cfg.IndexName == "" && cfg.IndexHost == "" {
		return nil, fmt.Errorf("index name or host is required")
	}

	// Apply defaults
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	// Create Pinecone client
	pc, err := pinecone.NewClient(pinecone.NewClientParams{
		ApiKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
	}

	// Resolve index host if not provided
	host := cfg.IndexHost
	if host == "" {
		idx, err := pc.DescribeIndex(ctx, cfg.IndexName)
		if err != nil {
			return nil, fmt.Errorf("failed to describe index %q: %w", cfg.IndexName, err)
		}
		host = idx.Host
	}

	// Create index connection
	idxConn, err := pc.Index(pinecone.NewIndexConnParams{
		Host:      host,
		Namespace: cfg.DefaultNamespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to index: %w", err)
	}

	return &Client{
		cfg:     cfg,
		pc:      pc,
		idxConn: idxConn,
	}, nil
}

// Query retrieves chunks similar to the given embedding.
func (c *Client) Query(ctx context.Context, req *types.RetrievalRequest) (*types.RetrievalResult, error) {
	if len(req.QueryEmbedding) == 0 {
		return nil, retriever.ErrInvalidQuery
	}

	start := time.Now()

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	// Build query request
	queryReq := &pinecone.QueryByVectorValuesRequest{
		Vector:          req.QueryEmbedding,
		TopK:            uint32(topK),
		IncludeValues:   req.IncludeEmbeddings,
		IncludeMetadata: req.IncludeMetadata,
	}

	// Use namespace from request or default
	namespace := req.Namespace
	if namespace == "" {
		namespace = c.cfg.DefaultNamespace
	}

	// Execute query
	resp, err := c.idxConn.QueryByVectorValues(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Convert response to chunks
	chunks := make([]types.Chunk, 0, len(resp.Matches))
	for _, match := range resp.Matches {
		chunk := types.Chunk{
			ID:        match.Vector.Id,
			Score:     match.Score,
			ClusterID: -1,
		}

		// Extract embedding if included
		if match.Vector.Values != nil {
			chunk.Embedding = *match.Vector.Values
		}

		// Extract metadata if included
		if match.Vector.Metadata != nil {
			chunk.Metadata = convertMetadataToMap(match.Vector.Metadata)

			// Try to extract text from common metadata fields
			if text, ok := chunk.Metadata["text"].(string); ok {
				chunk.Text = text
			} else if text, ok := chunk.Metadata["content"].(string); ok {
				chunk.Text = text
			} else if text, ok := chunk.Metadata["chunk_text"].(string); ok {
				chunk.Text = text
			}
		}

		chunks = append(chunks, chunk)
	}

	return &types.RetrievalResult{
		Chunks:         chunks,
		QueryEmbedding: req.QueryEmbedding,
		TotalMatches:   len(chunks),
		Latency:        time.Since(start),
	}, nil
}

// QueryByID retrieves chunks similar to an existing vector by its ID.
func (c *Client) QueryByID(ctx context.Context, id string, topK int, namespace string) (*types.RetrievalResult, error) {
	start := time.Now()

	if topK <= 0 {
		topK = 10
	}

	// Build query request
	queryReq := &pinecone.QueryByVectorIdRequest{
		VectorId:        id,
		TopK:            uint32(topK),
		IncludeValues:   true,
		IncludeMetadata: true,
	}

	// Execute query
	resp, err := c.idxConn.QueryByVectorId(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("query by ID failed: %w", err)
	}

	// Convert response to chunks
	chunks := make([]types.Chunk, 0, len(resp.Matches))
	for _, match := range resp.Matches {
		chunk := types.Chunk{
			ID:        match.Vector.Id,
			Score:     match.Score,
			ClusterID: -1,
		}

		if match.Vector.Values != nil {
			chunk.Embedding = *match.Vector.Values
		}

		if match.Vector.Metadata != nil {
			chunk.Metadata = convertMetadataToMap(match.Vector.Metadata)

			if text, ok := chunk.Metadata["text"].(string); ok {
				chunk.Text = text
			} else if text, ok := chunk.Metadata["content"].(string); ok {
				chunk.Text = text
			}
		}

		chunks = append(chunks, chunk)
	}

	return &types.RetrievalResult{
		Chunks:       chunks,
		TotalMatches: len(chunks),
		Latency:      time.Since(start),
	}, nil
}

// Close releases resources.
func (c *Client) Close() error {
	if c.idxConn != nil {
		return c.idxConn.Close()
	}
	return nil
}

// convertMetadataToMap converts Pinecone Struct metadata to a Go map.
func convertMetadataToMap(s *pinecone.Metadata) map[string]interface{} {
	if s == nil {
		return nil
	}

	// Pinecone Metadata is a protobuf Struct
	return s.AsMap()
}
