package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/contextlab"
	"github.com/Siddhant-K-code/distill/pkg/embedding/openai"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the Distill API server (standalone, no vector DB required)",
	Long: `Starts a standalone HTTP API server for semantic deduplication.
Unlike 'serve', this doesn't require a vector DB connection.
Clients send chunks directly and receive deduplicated results.

Example:
  distill api --port 8080

The server exposes:
  POST /v1/dedupe   - Deduplicate chunks
  GET  /health      - Health check`,
	RunE: runAPI,
}

func init() {
	rootCmd.AddCommand(apiCmd)

	apiCmd.Flags().IntP("port", "p", 8080, "HTTP server port")
	apiCmd.Flags().String("host", "0.0.0.0", "HTTP server host")
	apiCmd.Flags().String("openai-key", "", "OpenAI API key for embeddings (or use OPENAI_API_KEY)")
	apiCmd.Flags().String("embedding-model", "text-embedding-3-small", "OpenAI embedding model")
	apiCmd.Flags().String("api-keys", "", "Comma-separated list of valid API keys (or use DISTILL_API_KEYS)")
}

// DedupeRequest is the JSON request body for /v1/dedupe.
type DedupeRequest struct {
	Chunks    []DedupeChunk `json:"chunks"`
	Threshold float64       `json:"threshold,omitempty"`
	Lambda    float64       `json:"lambda,omitempty"`
	TargetK   int           `json:"target_k,omitempty"`
}

// DedupeChunk represents a chunk in the request.
type DedupeChunk struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Embedding []float32 `json:"embedding,omitempty"`
	Score     float32   `json:"score,omitempty"`
}

// DedupeResponse is the JSON response for /v1/dedupe.
type DedupeResponse struct {
	Chunks []DedupeChunkResponse `json:"chunks"`
	Stats  DedupeStats           `json:"stats"`
}

// DedupeChunkResponse represents a chunk in the response.
type DedupeChunkResponse struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	Score     float32 `json:"score"`
	ClusterID int     `json:"cluster_id"`
}

// DedupeStats contains processing statistics.
type DedupeStats struct {
	InputCount   int   `json:"input_count"`
	OutputCount  int   `json:"output_count"`
	ClusterCount int   `json:"cluster_count"`
	ReductionPct int   `json:"reduction_pct"`
	LatencyMs    int64 `json:"latency_ms"`
}

// APIServer holds the API server state.
type APIServer struct {
	embedder   *openai.Client
	validKeys  map[string]bool
	hasAuth    bool
}

func runAPI(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	openaiKey, _ := cmd.Flags().GetString("openai-key")
	embeddingModel, _ := cmd.Flags().GetString("embedding-model")
	apiKeysStr, _ := cmd.Flags().GetString("api-keys")

	// Resolve from environment
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKeysStr == "" {
		apiKeysStr = os.Getenv("DISTILL_API_KEYS")
	}

	// Parse API keys
	validKeys := make(map[string]bool)
	if apiKeysStr != "" {
		for _, key := range strings.Split(apiKeysStr, ",") {
			key = strings.TrimSpace(key)
			if key != "" {
				validKeys[key] = true
			}
		}
	}

	// Create embedding provider if OpenAI key is provided
	var embedder *openai.Client
	if openaiKey != "" {
		var err error
		embedder, err = openai.NewClient(openai.Config{
			APIKey: openaiKey,
			Model:  embeddingModel,
		})
		if err != nil {
			return fmt.Errorf("failed to create embedding provider: %w", err)
		}
	}

	server := &APIServer{
		embedder:  embedder,
		validKeys: validKeys,
		hasAuth:   len(validKeys) > 0,
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dedupe", server.handleDedupe)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/", server.handleRoot)

	// CORS middleware
	handler := corsMiddleware(mux)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Fprintln(os.Stderr, "\nShutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
		}
		close(done)
	}()

	// Start server
	fmt.Printf("Distill API server starting on %s\n", addr)
	fmt.Printf("  Embeddings: %v\n", embedder != nil)
	fmt.Printf("  Auth: %v (%d keys)\n", server.hasAuth, len(validKeys))
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Printf("  POST http://%s/v1/dedupe\n", addr)
	fmt.Printf("  GET  http://%s/health\n", addr)
	fmt.Println()

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	fmt.Println("Server stopped")
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *APIServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name":    "Distill API",
		"version": "1.0.0",
		"docs":    "https://distill.siddhantkhare.com/docs",
		"endpoints": map[string]string{
			"dedupe": "POST /v1/dedupe",
			"health": "GET /health",
		},
	})
}

func (s *APIServer) handleDedupe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check auth if enabled
	if s.hasAuth {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if !s.validKeys[token] {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}
	}

	var req DedupeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.Chunks) == 0 {
		http.Error(w, "At least one chunk is required", http.StatusBadRequest)
		return
	}

	start := time.Now()

	// Convert to internal types
	chunks := make([]types.Chunk, len(req.Chunks))
	needsEmbedding := false

	for i, c := range req.Chunks {
		chunks[i] = types.Chunk{
			ID:        c.ID,
			Text:      c.Text,
			Embedding: c.Embedding,
			Score:     c.Score,
		}
		if len(c.Embedding) == 0 {
			needsEmbedding = true
		}
	}

	// Generate embeddings if needed
	if needsEmbedding {
		if s.embedder == nil {
			http.Error(w, "Embeddings required but no embedding provider configured. Either provide embeddings in request or configure OPENAI_API_KEY.", http.StatusBadRequest)
			return
		}

		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Text
		}

		embeddings, err := s.embedder.EmbedBatch(r.Context(), texts)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to generate embeddings: %v", err), http.StatusInternalServerError)
			return
		}

		for i := range chunks {
			chunks[i].Embedding = embeddings[i]
		}
	}

	// Set defaults
	threshold := req.Threshold
	if threshold <= 0 {
		threshold = 0.15
	}
	lambda := req.Lambda
	if lambda <= 0 {
		lambda = 0.5
	}
	targetK := req.TargetK
	if targetK <= 0 {
		targetK = 0 // Will be set to cluster count
	}

	// Cluster
	clusterer := contextlab.NewClusterer(contextlab.ClusterConfig{
		Threshold: threshold,
		Linkage:   "average",
	})
	clusterResult := clusterer.Cluster(chunks)

	// Select representatives
	selectorCfg := contextlab.DefaultSelectorConfig()
	selectorCfg.Strategy = contextlab.SelectByScore
	selector := contextlab.NewSelector(selectorCfg)
	representatives := selector.Select(clusterResult)

	// Apply MMR if we have more representatives than target
	if targetK > 0 && len(representatives) > targetK {
		mmrCfg := contextlab.MMRConfig{
			Lambda:  lambda,
			TargetK: targetK,
		}
		mmr := contextlab.NewMMR(mmrCfg)
		representatives = mmr.Rerank(representatives)
	}

	latency := time.Since(start)

	// Build response
	outputChunks := make([]DedupeChunkResponse, len(representatives))
	for i, c := range representatives {
		outputChunks[i] = DedupeChunkResponse{
			ID:        c.ID,
			Text:      c.Text,
			Score:     c.Score,
			ClusterID: c.ClusterID,
		}
	}

	reductionPct := 0
	if len(req.Chunks) > 0 {
		reductionPct = int((1 - float64(len(representatives))/float64(len(req.Chunks))) * 100)
	}

	resp := DedupeResponse{
		Chunks: outputChunks,
		Stats: DedupeStats{
			InputCount:   len(req.Chunks),
			OutputCount:  len(representatives),
			ClusterCount: clusterResult.ClusterCount,
			ReductionPct: reductionPct,
			LatencyMs:    latency.Milliseconds(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
