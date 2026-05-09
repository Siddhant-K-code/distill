package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/embedding"
)

func fakeOllamaServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func okHandler(dim int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		emb := make([]float32, dim)
		for i := range emb {
			emb[i] = float32(i) * 0.01
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: emb})
	}
}

func TestEmbed_Success(t *testing.T) {
	srv := fakeOllamaServer(t, okHandler(768))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, Model: "nomic-embed-text"})
	emb, err := client.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emb) != 768 {
		t.Errorf("expected 768 dimensions, got %d", len(emb))
	}
}

func TestEmbed_EmptyInput(t *testing.T) {
	client := NewClient(Config{})
	_, err := client.Embed(context.Background(), "")
	if err != embedding.ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

func TestEmbed_ServerError(t *testing.T) {
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestEmbed_EmptyEmbeddingResponse(t *testing.T) {
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: nil})
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}

func TestEmbed_InvalidJSON(t *testing.T) {
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{invalid"))
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestEmbed_ContextCancelled(t *testing.T) {
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, Timeout: 100 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Embed(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestEmbed_RequestBody(t *testing.T) {
	var received embedRequest
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: []float32{0.1}})
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, Model: "test-model"})
	_, _ = client.Embed(context.Background(), "hello")

	if received.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", received.Model)
	}
	if received.Prompt != "hello" {
		t.Errorf("expected prompt hello, got %s", received.Prompt)
	}
}

func TestEmbedBatch_Success(t *testing.T) {
	srv := fakeOllamaServer(t, okHandler(384))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	texts := []string{"hello", "world", "test"}
	results, err := client.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	for i, emb := range results {
		if len(emb) != 384 {
			t.Errorf("result[%d]: expected 384 dimensions, got %d", i, len(emb))
		}
	}
}

func TestEmbedBatch_PartialFailure(t *testing.T) {
	callCount := 0
	srv := fakeOllamaServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embedResponse{Embedding: []float32{0.1}})
	})
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	_, err := client.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected error when one embed in batch fails")
	}
}

func TestEmbedBatch_Empty(t *testing.T) {
	client := NewClient(Config{})
	results, err := client.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestNewClient_Defaults(t *testing.T) {
	client := NewClient(Config{})
	if client.cfg.BaseURL != defaultBaseURL {
		t.Errorf("expected default base URL %s, got %s", defaultBaseURL, client.cfg.BaseURL)
	}
	if client.cfg.Model != defaultModel {
		t.Errorf("expected default model %s, got %s", defaultModel, client.cfg.Model)
	}
	if client.cfg.Timeout != defaultTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultTimeout, client.cfg.Timeout)
	}
}

func TestNewClient_CustomConfig(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "http://custom:1234",
		Model:   "mxbai-embed-large",
		Timeout: 30 * time.Second,
	})
	if client.cfg.BaseURL != "http://custom:1234" {
		t.Errorf("expected custom base URL, got %s", client.cfg.BaseURL)
	}
	if client.cfg.Model != "mxbai-embed-large" {
		t.Errorf("expected custom model, got %s", client.cfg.Model)
	}
	if client.cfg.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", client.cfg.Timeout)
	}
}

func TestDimension(t *testing.T) {
	client := NewClient(Config{})
	if client.Dimension() != 0 {
		t.Errorf("expected 0 (runtime-determined), got %d", client.Dimension())
	}
}

func TestModelName(t *testing.T) {
	client := NewClient(Config{Model: "custom-model"})
	if client.ModelName() != "custom-model" {
		t.Errorf("expected custom-model, got %s", client.ModelName())
	}
}

func TestEmbed_ConnectionRefused(t *testing.T) {
	client := NewClient(Config{
		BaseURL: "http://127.0.0.1:1", // nothing listening
		Timeout: 1 * time.Second,
	})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}
