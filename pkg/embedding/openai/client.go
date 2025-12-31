package openai

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
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "text-embedding-3-small"
	defaultTimeout = 30 * time.Second
)

// Model dimensions for OpenAI embedding models.
var modelDimensions = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// Config holds OpenAI client configuration.
type Config struct {
	// APIKey is the OpenAI API key (required)
	APIKey string

	// Model is the embedding model to use
	Model string

	// BaseURL is the API base URL (default: https://api.openai.com/v1)
	BaseURL string

	// Timeout for API requests
	Timeout time.Duration

	// MaxRetries for transient failures
	MaxRetries int
}

// Client implements the embedding.Provider interface for OpenAI.
type Client struct {
	cfg        Config
	httpClient *http.Client
	dimension  int
}

// NewClient creates a new OpenAI embedding client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	// Get dimension for model
	dimension, ok := modelDimensions[cfg.Model]
	if !ok {
		// Default to 1536 for unknown models
		dimension = 1536
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		dimension: dimension,
	}, nil
}

// embeddingRequest is the OpenAI API request body.
type embeddingRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
}

// embeddingResponse is the OpenAI API response.
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// errorResponse is the OpenAI API error response.
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Embed converts a single text into a vector embedding.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, embedding.ErrEmptyInput
	}

	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return embeddings[0], nil
}

// EmbedBatch converts multiple texts into vector embeddings.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, embedding.ErrEmptyInput
	}

	// Filter empty texts
	validTexts := make([]string, 0, len(texts))
	validIndices := make([]int, 0, len(texts))
	for i, text := range texts {
		if text != "" {
			validTexts = append(validTexts, text)
			validIndices = append(validIndices, i)
		}
	}

	if len(validTexts) == 0 {
		return nil, embedding.ErrEmptyInput
	}

	// Build request
	reqBody := embeddingRequest{
		Input: validTexts,
		Model: c.cfg.Model,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request with retries
	var resp *embeddingResponse
	var lastErr error

	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			time.Sleep(time.Duration(attempt*attempt) * 100 * time.Millisecond)
		}

		resp, lastErr = c.doRequest(ctx, reqJSON)
		if lastErr == nil {
			break
		}

		// Don't retry on certain errors
		if lastErr == embedding.ErrInvalidAPIKey || lastErr == embedding.ErrContextTooLong {
			return nil, lastErr
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	// Build result array preserving original order
	results := make([][]float32, len(texts))
	for _, data := range resp.Data {
		if data.Index < len(validIndices) {
			originalIdx := validIndices[data.Index]
			results[originalIdx] = data.Embedding
		}
	}

	// Fill in empty embeddings for empty input texts
	for i, text := range texts {
		if text == "" {
			results[i] = make([]float32, c.dimension)
		}
	}

	return results, nil
}

// doRequest makes the HTTP request to OpenAI.
func (c *Client) doRequest(ctx context.Context, body []byte) (*embeddingResponse, error) {
	url := c.cfg.BaseURL + "/embeddings"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			switch resp.StatusCode {
			case http.StatusUnauthorized:
				return nil, embedding.ErrInvalidAPIKey
			case http.StatusTooManyRequests:
				return nil, embedding.ErrRateLimited
			case http.StatusBadRequest:
				if errResp.Error.Code == "context_length_exceeded" {
					return nil, embedding.ErrContextTooLong
				}
			}
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse response
	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &embResp, nil
}

// Dimension returns the embedding dimension for this model.
func (c *Client) Dimension() int {
	return c.dimension
}

// ModelName returns the model name.
func (c *Client) ModelName() string {
	return c.cfg.Model
}
