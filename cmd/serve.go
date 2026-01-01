package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/contextlab"
	"github.com/Siddhant-K-code/distill/pkg/embedding/openai"
	"github.com/Siddhant-K-code/distill/pkg/retriever"
	pcretriever "github.com/Siddhant-K-code/distill/pkg/retriever/pinecone"
	qdretriever "github.com/Siddhant-K-code/distill/pkg/retriever/qdrant"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ContextLab HTTP server",
	Long: `Starts an HTTP server that provides semantic deduplication
for RAG retrieval queries.

Example:
  distill serve --port 8080 --backend pinecone --index my-index

The server exposes:
  POST /v1/retrieve  - Deduplicated retrieval endpoint
  GET  /health       - Health check
  GET  /metrics      - Basic metrics`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Server settings
	serveCmd.Flags().IntP("port", "p", 8080, "HTTP server port")
	serveCmd.Flags().String("host", "0.0.0.0", "HTTP server host")

	// Backend settings
	serveCmd.Flags().String("backend", "pinecone", "Vector DB backend (pinecone, qdrant)")
	serveCmd.Flags().StringP("index", "i", "", "Index/collection name")
	serveCmd.Flags().String("api-key", "", "Vector DB API key (or use PINECONE_API_KEY)")
	serveCmd.Flags().String("db-host", "", "Vector DB host (for Qdrant)")
	serveCmd.Flags().StringP("namespace", "n", "", "Default namespace")

	// Embedding settings
	serveCmd.Flags().String("openai-key", "", "OpenAI API key for embeddings (or use OPENAI_API_KEY)")
	serveCmd.Flags().String("embedding-model", "text-embedding-3-small", "OpenAI embedding model")

	// ContextLab settings
	serveCmd.Flags().Int("over-fetch-k", 50, "Number of chunks to over-fetch")
	serveCmd.Flags().Int("target-k", 8, "Target number of chunks to return")
	serveCmd.Flags().Float64("threshold", 0.15, "Clustering threshold")
	serveCmd.Flags().Float64("lambda", 0.5, "MMR lambda (relevance vs diversity)")
	serveCmd.Flags().Bool("enable-mmr", true, "Enable MMR re-ranking")

	// Bind to viper
	_ = viper.BindPFlag("serve.port", serveCmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("serve.host", serveCmd.Flags().Lookup("host"))
}

// Server holds the HTTP server state.
type Server struct {
	broker *contextlab.Broker
	cfg    ServerConfig
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Host string
	Port int
}

// RetrieveRequest is the JSON request body for /v1/retrieve.
type RetrieveRequest struct {
	Query          string                 `json:"query,omitempty"`
	QueryEmbedding []float32              `json:"query_embedding,omitempty"`
	Index          string                 `json:"index,omitempty"`
	Namespace      string                 `json:"namespace,omitempty"`
	OverFetchK     int                    `json:"over_fetch_k,omitempty"`
	TargetK        int                    `json:"target_k,omitempty"`
	Threshold      float64                `json:"threshold,omitempty"`
	Lambda         float64                `json:"lambda,omitempty"`
	Filter         map[string]interface{} `json:"filter,omitempty"`
}

// RetrieveResponse is the JSON response for /v1/retrieve.
type RetrieveResponse struct {
	Chunks []ChunkResponse `json:"chunks"`
	Stats  StatsResponse   `json:"stats"`
}

// ChunkResponse represents a chunk in the response.
type ChunkResponse struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text,omitempty"`
	Score     float32                `json:"score"`
	ClusterID int                    `json:"cluster_id"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// StatsResponse contains processing statistics.
type StatsResponse struct {
	Retrieved         int     `json:"retrieved"`
	Clustered         int     `json:"clustered"`
	Returned          int     `json:"returned"`
	RetrievalLatencyMs int64  `json:"retrieval_latency_ms"`
	ClusteringLatencyMs int64 `json:"clustering_latency_ms"`
	TotalLatencyMs    int64   `json:"total_latency_ms"`
}

func runServe(cmd *cobra.Command, args []string) error {
	// Get flags
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	backend, _ := cmd.Flags().GetString("backend")
	index, _ := cmd.Flags().GetString("index")
	apiKey, _ := cmd.Flags().GetString("api-key")
	dbHost, _ := cmd.Flags().GetString("db-host")
	namespace, _ := cmd.Flags().GetString("namespace")
	openaiKey, _ := cmd.Flags().GetString("openai-key")
	embeddingModel, _ := cmd.Flags().GetString("embedding-model")
	overFetchK, _ := cmd.Flags().GetInt("over-fetch-k")
	targetK, _ := cmd.Flags().GetInt("target-k")
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	lambda, _ := cmd.Flags().GetFloat64("lambda")
	enableMMR, _ := cmd.Flags().GetBool("enable-mmr")

	// Resolve API keys from environment
	if apiKey == "" {
		apiKey = os.Getenv("PINECONE_API_KEY")
	}
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	ctx := context.Background()

	// Create retriever based on backend
	var ret retriever.Retriever
	var err error

	switch backend {
	case "pinecone":
		if apiKey == "" {
			return fmt.Errorf("Pinecone API key required (--api-key or PINECONE_API_KEY)")
		}
		if index == "" {
			return fmt.Errorf("index name required (--index)")
		}
		ret, err = pcretriever.NewClient(ctx, pcretriever.Config{
			Config: retriever.Config{
				APIKey:           apiKey,
				DefaultNamespace: namespace,
			},
			IndexName: index,
		})

	case "qdrant":
		if dbHost == "" {
			return fmt.Errorf("Qdrant host required (--db-host)")
		}
		if index == "" {
			return fmt.Errorf("collection name required (--index)")
		}
		ret, err = qdretriever.NewClient(ctx, qdretriever.Config{
			Config: retriever.Config{
				APIKey:           apiKey,
				Host:             dbHost,
				DefaultNamespace: namespace,
			},
			Collection: index,
		})

	default:
		return fmt.Errorf("unsupported backend: %s (use 'pinecone' or 'qdrant')", backend)
	}

	if err != nil {
		return fmt.Errorf("failed to create retriever: %w", err)
	}
	defer func() { _ = ret.Close() }()

	// Create embedding provider if OpenAI key is provided
	var embedder retriever.EmbeddingProvider
	if openaiKey != "" {
		embedder, err = openai.NewClient(openai.Config{
			APIKey: openaiKey,
			Model:  embeddingModel,
		})
		if err != nil {
			return fmt.Errorf("failed to create embedding provider: %w", err)
		}
	}

	// Create broker
	brokerCfg := contextlab.BrokerConfig{
		OverFetchK:        overFetchK,
		TargetK:           targetK,
		ClusterThreshold:  threshold,
		ClusterLinkage:    "average",
		SelectionStrategy: contextlab.SelectByScore,
		EnableMMR:         enableMMR,
		MMRLambda:         lambda,
		IncludeMetadata:   true,
	}

	var broker *contextlab.Broker
	if embedder != nil {
		broker = contextlab.NewBrokerWithEmbedder(ret, embedder, brokerCfg)
	} else {
		broker = contextlab.NewBroker(ret, brokerCfg)
	}
	defer func() { _ = broker.Close() }()

	// Create server
	server := &Server{
		broker: broker,
		cfg: ServerConfig{
			Host: host,
			Port: port,
		},
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/retrieve", server.handleRetrieve)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/metrics", server.handleMetrics)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
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
	fmt.Printf("ContextLab server starting on %s\n", addr)
	fmt.Printf("  Backend: %s\n", backend)
	fmt.Printf("  Index: %s\n", index)
	fmt.Printf("  Embeddings: %v\n", embedder != nil)
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Printf("  POST http://%s/v1/retrieve\n", addr)
	fmt.Printf("  GET  http://%s/health\n", addr)
	fmt.Println()

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	fmt.Println("Server stopped")
	return nil
}

func (s *Server) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RetrieveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Query == "" && len(req.QueryEmbedding) == 0 {
		http.Error(w, "Either 'query' or 'query_embedding' is required", http.StatusBadRequest)
		return
	}

	// Build retrieval request
	retrievalReq := &types.RetrievalRequest{
		Query:          req.Query,
		QueryEmbedding: req.QueryEmbedding,
		Namespace:      req.Namespace,
		Filter:         req.Filter,
	}

	// Override broker config if specified in request
	if req.OverFetchK > 0 || req.TargetK > 0 || req.Threshold > 0 || req.Lambda > 0 {
		cfg := s.broker.GetConfig()
		if req.OverFetchK > 0 {
			cfg.OverFetchK = req.OverFetchK
		}
		if req.TargetK > 0 {
			cfg.TargetK = req.TargetK
		}
		if req.Threshold > 0 {
			cfg.ClusterThreshold = req.Threshold
		}
		if req.Lambda > 0 {
			cfg.MMRLambda = req.Lambda
		}
		s.broker.SetConfig(cfg)
	}

	// Execute retrieval
	result, err := s.broker.Retrieve(r.Context(), retrievalReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Retrieval failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response
	chunks := make([]ChunkResponse, len(result.Chunks))
	for i, c := range result.Chunks {
		chunks[i] = ChunkResponse{
			ID:        c.ID,
			Text:      c.Text,
			Score:     c.Score,
			ClusterID: c.ClusterID,
			Metadata:  c.Metadata,
		}
	}

	resp := RetrieveResponse{
		Chunks: chunks,
		Stats: StatsResponse{
			Retrieved:           result.Stats.Retrieved,
			Clustered:           result.Stats.Clustered,
			Returned:            result.Stats.Returned,
			RetrievalLatencyMs:  result.Stats.RetrievalLatency.Milliseconds(),
			ClusteringLatencyMs: result.Stats.ClusteringLatency.Milliseconds(),
			TotalLatencyMs:      result.Stats.TotalLatency.Milliseconds(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	cfg := s.broker.GetConfig()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"config": map[string]interface{}{
			"over_fetch_k": cfg.OverFetchK,
			"target_k":     cfg.TargetK,
			"threshold":    cfg.ClusterThreshold,
			"lambda":       cfg.MMRLambda,
			"mmr_enabled":  cfg.EnableMMR,
		},
	})
}
