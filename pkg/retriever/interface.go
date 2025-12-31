package retriever

import (
	"context"
	"errors"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Common errors returned by retrievers.
var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidQuery   = errors.New("invalid query: must provide query text or embedding")
	ErrConnectionFailed = errors.New("connection to vector database failed")
	ErrRateLimited    = errors.New("rate limited by vector database")
	ErrTimeout        = errors.New("query timeout")
)

// Retriever defines the interface for vector database query operations.
type Retriever interface {
	// Query retrieves chunks similar to the given embedding.
	Query(ctx context.Context, req *types.RetrievalRequest) (*types.RetrievalResult, error)

	// QueryByID retrieves chunks similar to an existing vector by its ID.
	QueryByID(ctx context.Context, id string, topK int, namespace string) (*types.RetrievalResult, error)

	// Close releases any resources held by the retriever.
	Close() error
}

// EmbeddingProvider defines the interface for text embedding services.
type EmbeddingProvider interface {
	// Embed converts a single text into a vector embedding.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts multiple texts into vector embeddings.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension returns the embedding dimension for this provider.
	Dimension() int

	// ModelName returns the name of the embedding model.
	ModelName() string
}

// RetrieverWithEmbedding combines a retriever with an embedding provider
// to support text-based queries.
type RetrieverWithEmbedding struct {
	Retriever Retriever
	Embedder  EmbeddingProvider
}

// Query handles both text and vector queries.
func (r *RetrieverWithEmbedding) Query(ctx context.Context, req *types.RetrievalRequest) (*types.RetrievalResult, error) {
	// If query text is provided but no embedding, generate embedding
	if req.Query != "" && len(req.QueryEmbedding) == 0 {
		if r.Embedder == nil {
			return nil, errors.New("embedding provider required for text queries")
		}

		embedding, err := r.Embedder.Embed(ctx, req.Query)
		if err != nil {
			return nil, err
		}
		req.QueryEmbedding = embedding
	}

	if len(req.QueryEmbedding) == 0 {
		return nil, ErrInvalidQuery
	}

	return r.Retriever.Query(ctx, req)
}

// QueryByID delegates to the underlying retriever.
func (r *RetrieverWithEmbedding) QueryByID(ctx context.Context, id string, topK int, namespace string) (*types.RetrievalResult, error) {
	return r.Retriever.QueryByID(ctx, id, topK, namespace)
}

// Close releases resources.
func (r *RetrieverWithEmbedding) Close() error {
	return r.Retriever.Close()
}

// Config holds common retriever configuration.
type Config struct {
	// APIKey for authentication
	APIKey string

	// Host is the vector database endpoint
	Host string

	// Timeout for operations in seconds
	TimeoutSeconds int

	// MaxRetries for transient failures
	MaxRetries int

	// DefaultNamespace if not specified in requests
	DefaultNamespace string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		TimeoutSeconds: 30,
		MaxRetries:     3,
	}
}
