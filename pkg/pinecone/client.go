package pinecone

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/pinecone-io/go-pinecone/v3/pinecone"
	"google.golang.org/protobuf/types/known/structpb"
)

// Config holds Pinecone client configuration.
type Config struct {
	APIKey    string
	IndexName string
	Namespace string

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
	}
}

// Client wraps the Pinecone gRPC client for vector operations.
type Client struct {
	cfg     Config
	pc      *pinecone.Client
	idxConn *pinecone.IndexConnection
	stats   *Stats
}

// Stats tracks client operation metrics.
type Stats struct {
	UpsertedVectors int64
	FailedVectors   int64
	RetryCount      int64
	BatchCount      int64
}

// NewClient creates a new Pinecone client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if cfg.IndexName == "" {
		return nil, fmt.Errorf("index name is required")
	}

	// Apply defaults
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	// Create Pinecone client
	pc, err := pinecone.NewClient(pinecone.NewClientParams{
		ApiKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Pinecone client: %w", err)
	}

	// Get index connection (uses gRPC for data operations)
	idx, err := pc.DescribeIndex(ctx, cfg.IndexName)
	if err != nil {
		return nil, fmt.Errorf("failed to describe index %q: %w", cfg.IndexName, err)
	}

	idxConn, err := pc.Index(pinecone.NewIndexConnParams{
		Host:      idx.Host,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to index: %w", err)
	}

	return &Client{
		cfg:     cfg,
		pc:      pc,
		idxConn: idxConn,
		stats:   &Stats{},
	}, nil
}

// UpsertBatch upserts a batch of vectors with retry logic.
func (c *Client) UpsertBatch(ctx context.Context, vectors []types.Vector) error {
	if len(vectors) == 0 {
		return nil
	}

	// Convert to Pinecone vectors
	pcVectors := make([]*pinecone.Vector, len(vectors))
	for i, v := range vectors {
		values := v.Values // Create a copy for pointer
		pcVectors[i] = &pinecone.Vector{
			Id:       v.ID,
			Values:   &values,
			Metadata: convertMetadata(v.Metadata),
		}
	}

	// Retry loop with exponential backoff
	var lastErr error
	backoff := c.cfg.InitialBackoff

	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			atomic.AddInt64(&c.stats.RetryCount, 1)
			time.Sleep(backoff)
			backoff = time.Duration(math.Min(float64(backoff*2), float64(c.cfg.MaxBackoff)))
		}

		_, err := c.idxConn.UpsertVectors(ctx, pcVectors)
		if err == nil {
			atomic.AddInt64(&c.stats.UpsertedVectors, int64(len(vectors)))
			atomic.AddInt64(&c.stats.BatchCount, 1)
			return nil
		}

		lastErr = err

		// Check if error is retryable (429 or 503)
		if !isRetryableError(err) {
			break
		}
	}

	atomic.AddInt64(&c.stats.FailedVectors, int64(len(vectors)))
	return fmt.Errorf("upsert failed after %d retries: %w", c.cfg.MaxRetries, lastErr)
}

// GetStats returns current operation statistics.
func (c *Client) GetStats() Stats {
	return Stats{
		UpsertedVectors: atomic.LoadInt64(&c.stats.UpsertedVectors),
		FailedVectors:   atomic.LoadInt64(&c.stats.FailedVectors),
		RetryCount:      atomic.LoadInt64(&c.stats.RetryCount),
		BatchCount:      atomic.LoadInt64(&c.stats.BatchCount),
	}
}

// DescribeIndexStats returns index statistics.
func (c *Client) DescribeIndexStats(ctx context.Context) (*pinecone.DescribeIndexStatsResponse, error) {
	return c.idxConn.DescribeIndexStats(ctx)
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.idxConn != nil {
		return c.idxConn.Close()
	}
	return nil
}

// convertMetadata converts map[string]interface{} to Pinecone Struct metadata.
func convertMetadata(m map[string]interface{}) *structpb.Struct {
	if m == nil || len(m) == 0 {
		return nil
	}

	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return s
}

// isRetryableError checks if an error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for rate limiting (429) or service unavailable (503)
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "unavailable") ||
		strings.Contains(errStr, "temporarily")
}
