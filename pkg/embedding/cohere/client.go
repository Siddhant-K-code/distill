// Package cohere provides an embedding.Provider backed by the Cohere API.
package cohere

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/embedding"
)

const (
	defaultBaseURL = "https://api.cohere.ai/v1"
	defaultModel   = "embed-english-v3.0"
	defaultTimeout = 30 * time.Second
)

// InputType controls how Cohere classifies the input for retrieval tasks.
type InputType string

const (
	InputTypeSearchDocument InputType = "search_document"
	InputTypeSearchQuery    InputType = "search_query"
	InputTypeClassification InputType = "classification"
	InputTypeClustering     InputType = "clustering"
)

// Model dimensions for common Cohere embedding models.
var modelDimensions = map[string]int{
	"embed-english-v3.0":       1024,
	"embed-multilingual-v3.0":  1024,
	"embed-english-light-v3.0": 384,
}

// Config holds Cohere client configuration.
type Config struct {
	// APIKey is the Cohere API key (required).
	APIKey string

	// Model is the embedding model. Default: embed-english-v3.0
	Model string

	// InputType controls retrieval optimisation. Default: search_document
	InputType InputType

	// Timeout for API requests. Default: 30s
	Timeout time.Duration
}

// Client implements embedding.Provider for Cohere.
type Client struct {
	cfg        Config
	httpClient *http.Client
	dimension  int
}

// NewClient creates a new Cohere embedding client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Cohere API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.InputType == "" {
		cfg.InputType = InputTypeSearchDocument
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	dim := modelDimensions[cfg.Model]
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
		dimension:  dim,
	}, nil
}

type embedRequest struct {
	Texts     []string  `json:"texts"`
	Model     string    `json:"model"`
	InputType InputType `json:"input_type"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns the embedding for a single text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, embedding.ErrEmptyInput
	}
	results, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// EmbedBatch embeds multiple texts in a single API call.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(embedRequest{
		Texts:     texts,
		Model:     c.cfg.Model,
		InputType: c.cfg.InputType,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		defaultBaseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cohere request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, embedding.ErrRateLimited
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, embedding.ErrInvalidAPIKey
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cohere %d: %s", resp.StatusCode, string(b))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}
	return result.Embeddings, nil
}

// Dimension returns the embedding dimension for the configured model.
func (c *Client) Dimension() int { return c.dimension }

// ModelName returns the configured model name.
func (c *Client) ModelName() string { return c.cfg.Model }
