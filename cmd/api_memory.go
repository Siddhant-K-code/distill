package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/memory"
	"github.com/Siddhant-K-code/distill/pkg/retriever"
)

// MemoryAPI handles memory-related HTTP endpoints.
type MemoryAPI struct {
	store    *memory.SQLiteStore
	embedder retriever.EmbeddingProvider
}

// RegisterMemoryRoutes adds memory endpoints to the given mux.
func (m *MemoryAPI) RegisterMemoryRoutes(mux *http.ServeMux, mw func(string, http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/v1/memory/store", mw("/v1/memory/store", m.handleStore))
	mux.HandleFunc("/v1/memory/recall", mw("/v1/memory/recall", m.handleRecall))
	mux.HandleFunc("/v1/memory/forget", mw("/v1/memory/forget", m.handleForget))
	mux.HandleFunc("/v1/memory/stats", mw("/v1/memory/stats", m.handleStats))
}

func (m *MemoryAPI) handleStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req memory.StoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Generate embeddings for entries that don't have them
	if m.embedder != nil {
		var textsToEmbed []string
		var indices []int
		for i, e := range req.Entries {
			if len(e.Embedding) == 0 && e.Text != "" {
				textsToEmbed = append(textsToEmbed, e.Text)
				indices = append(indices, i)
			}
		}
		if len(textsToEmbed) > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			embeddings, err := m.embedder.EmbedBatch(ctx, textsToEmbed)
			if err != nil {
				writeJSONError(w, fmt.Sprintf("embedding error: %v", err), http.StatusInternalServerError)
				return
			}
			for i, idx := range indices {
				req.Entries[idx].Embedding = embeddings[i]
			}
		}
	}

	result, err := m.store.Store(r.Context(), req)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (m *MemoryAPI) handleRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req memory.RecallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		writeJSONError(w, "query or query_embedding is required", http.StatusBadRequest)
		return
	}

	// Generate query embedding if not provided
	if len(req.QueryEmbedding) == 0 && m.embedder != nil && req.Query != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		emb, err := m.embedder.Embed(ctx, req.Query)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("embedding error: %v", err), http.StatusInternalServerError)
			return
		}
		req.QueryEmbedding = emb
	}

	result, err := m.store.Recall(r.Context(), req)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (m *MemoryAPI) handleForget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req memory.ForgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := m.store.Forget(r.Context(), req)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (m *MemoryAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := m.store.Stats(r.Context())
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
