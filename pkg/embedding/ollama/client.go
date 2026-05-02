// Package ollama provides an embedding.Provider backed by a local Ollama server.
package ollama

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
	defaultBaseURL = "http://localhost:11434"
	defaultModel   = "nomic-embed-text"
	defaultTimeout = 60 * time.Second
)

// Config holds Ollama client configuration.
type Config struct {
	// BaseURL is the Ollama server URL. Default: http://localhost:11434
	BaseURL string

	// Model is the embedding model to use. Default: nomic-embed-text
	Model string

	// Timeout for API requests. Default: 60s (local models can be slow).
	Timeout time.Duration
}

// Client implements embedding.Provider for Ollama.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient creates a new Ollama embedding client.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns the embedding for a single text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, embedding.ErrEmptyInput
	}

	body, err := json.Marshal(embedRequest{Model: c.cfg.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(b))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}
	return result.Embedding, nil
}

// EmbedBatch embeds multiple texts sequentially (Ollama has no batch API).
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := c.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed[%d]: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

// Dimension returns the embedding dimension. Ollama models vary; we return
// 0 to indicate it is determined at runtime from the first response.
func (c *Client) Dimension() int { return 0 }

// ModelName returns the configured model name.
func (c *Client) ModelName() string { return c.cfg.Model }
