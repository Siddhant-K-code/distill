package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/Siddhant-K-code/distill/pkg/contextlab"
	"github.com/Siddhant-K-code/distill/pkg/embedding/openai"
	"github.com/Siddhant-K-code/distill/pkg/retriever"
	pcretriever "github.com/Siddhant-K-code/distill/pkg/retriever/pinecone"
	qdretriever "github.com/Siddhant-K-code/distill/pkg/retriever/qdrant"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start GoVectorSync as an MCP server",
	Long: `Starts GoVectorSync as a Model Context Protocol (MCP) server.

This allows AI assistants like Claude, Amp, and Cursor to use GoVectorSync's
semantic deduplication capabilities directly.

Transports:
  stdio (default) - For local desktop apps (Claude Desktop, Cursor)
  http            - For remote/cloud deployments (hosted MCP server)

Tools exposed:
  deduplicate_chunks    - Deduplicate chunks with embeddings
  retrieve_deduplicated - Query vector DB with deduplication
  analyze_redundancy    - Analyze chunks for redundancy stats

Resources exposed:
  govectorsync://system-prompt - System prompt for AI assistants

Example:
  # Local stdio server (Claude Desktop, Cursor, Amp)
  govs mcp

  # Remote HTTP server (hosted deployment)
  govs mcp --transport http --port 8081

  # With vector DB backend
  govs mcp --backend pinecone --index my-index

Configure in Claude Desktop (claude_desktop_config.json):
  {
    "mcpServers": {
      "govectorsync": {
        "command": "govs",
        "args": ["mcp"]
      }
    }
  }

For remote MCP server:
  {
    "mcpServers": {
      "govectorsync": {
        "url": "https://your-server.fly.dev/mcp"
      }
    }
  }`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)

	// Transport settings
	mcpCmd.Flags().String("transport", "stdio", "Transport type: stdio or http")
	mcpCmd.Flags().Int("port", 8081, "HTTP server port (for http transport)")
	mcpCmd.Flags().String("host", "0.0.0.0", "HTTP server host (for http transport)")

	// Backend settings (optional - only needed for retrieve_deduplicated)
	mcpCmd.Flags().String("backend", "", "Vector DB backend (pinecone, qdrant)")
	mcpCmd.Flags().StringP("index", "i", "", "Index/collection name")
	mcpCmd.Flags().String("api-key", "", "Vector DB API key (or use PINECONE_API_KEY)")
	mcpCmd.Flags().String("db-host", "", "Vector DB host (for Qdrant)")
	mcpCmd.Flags().StringP("namespace", "n", "", "Default namespace")

	// Embedding settings
	mcpCmd.Flags().String("openai-key", "", "OpenAI API key for embeddings (or use OPENAI_API_KEY)")
	mcpCmd.Flags().String("embedding-model", "text-embedding-3-small", "OpenAI embedding model")

	// Default deduplication settings
	mcpCmd.Flags().Int("over-fetch-k", 50, "Default over-fetch count")
	mcpCmd.Flags().Int("target-k", 8, "Default target chunk count")
	mcpCmd.Flags().Float64("threshold", 0.15, "Default clustering threshold")
	mcpCmd.Flags().Float64("lambda", 0.5, "Default MMR lambda")
}

// MCPServer wraps the MCP server with GoVectorSync capabilities
type MCPServer struct {
	broker   *contextlab.Broker
	embedder retriever.EmbeddingProvider
	cfg      contextlab.BrokerConfig
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Get flags
	transport, _ := cmd.Flags().GetString("transport")
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

	// Resolve API keys from environment
	if apiKey == "" {
		apiKey = os.Getenv("PINECONE_API_KEY")
	}
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	ctx := context.Background()

	// Create broker config
	brokerCfg := contextlab.BrokerConfig{
		OverFetchK:        overFetchK,
		TargetK:           targetK,
		ClusterThreshold:  threshold,
		ClusterLinkage:    "average",
		SelectionStrategy: contextlab.SelectByScore,
		EnableMMR:         true,
		MMRLambda:         lambda,
		IncludeMetadata:   true,
	}

	// Create MCP server wrapper
	mcpSrv := &MCPServer{
		cfg: brokerCfg,
	}

	// Create embedding provider if OpenAI key is provided
	if openaiKey != "" {
		embedder, err := openai.NewClient(openai.Config{
			APIKey: openaiKey,
			Model:  embeddingModel,
		})
		if err != nil {
			return fmt.Errorf("failed to create embedding provider: %w", err)
		}
		mcpSrv.embedder = embedder
	}

	// Create retriever if backend is configured
	if backend != "" && index != "" {
		var ret retriever.Retriever
		var err error

		switch backend {
		case "pinecone":
			if apiKey == "" {
				return fmt.Errorf("Pinecone API key required (--api-key or PINECONE_API_KEY)")
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
			ret, err = qdretriever.NewClient(ctx, qdretriever.Config{
				Config: retriever.Config{
					APIKey:           apiKey,
					Host:             dbHost,
					DefaultNamespace: namespace,
				},
				Collection: index,
			})

		default:
			return fmt.Errorf("unsupported backend: %s", backend)
		}

		if err != nil {
			return fmt.Errorf("failed to create retriever: %w", err)
		}
		defer ret.Close()

		// Create broker with retriever
		if mcpSrv.embedder != nil {
			mcpSrv.broker = contextlab.NewBrokerWithEmbedder(ret, mcpSrv.embedder, brokerCfg)
		} else {
			mcpSrv.broker = contextlab.NewBroker(ret, brokerCfg)
		}
		defer mcpSrv.broker.Close()
	}

	// Create MCP server with capabilities
	s := server.NewMCPServer(
		"GoVectorSync",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(true, false),
		server.WithPromptCapabilities(false),
	)

	// Register tools, resources, and prompts
	mcpSrv.registerTools(s)
	mcpSrv.registerResources(s)
	mcpSrv.registerPrompts(s)

	// Start server based on transport
	switch transport {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			return fmt.Errorf("MCP server error: %w", err)
		}

	case "http":
		addr := fmt.Sprintf("%s:%d", host, port)
		fmt.Printf("GoVectorSync MCP server starting on http://%s\n", addr)
		fmt.Printf("  Endpoint: http://%s/mcp\n", addr)
		fmt.Printf("  Health:   http://%s/health\n", addr)
		fmt.Println()

		// Create HTTP handler with stateful session management
		mux := http.NewServeMux()

		// Health check endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","server":"govectorsync-mcp"}`))
		})

		// MCP endpoint with stateful sessions
		mcpHandler := server.NewStreamableHTTPServer(s, server.WithStateful(true))
		mux.Handle("/mcp", mcpHandler)

		// Start HTTP server
		httpServer := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		if err := httpServer.ListenAndServe(); err != nil {
			return fmt.Errorf("HTTP server error: %w", err)
		}

	default:
		return fmt.Errorf("unsupported transport: %s (use 'stdio' or 'http')", transport)
	}

	return nil
}

func (m *MCPServer) registerTools(s *server.MCPServer) {
	// Tool 1: deduplicate_chunks - works without vector DB
	// Description is action-oriented to encourage AI to use it
	deduplicateTool := mcp.NewTool("deduplicate_chunks",
		mcp.WithDescription(`Remove redundant information from RAG chunks before sending to LLM.

WHEN TO USE: Call this tool whenever you have multiple text chunks from retrieval, 
search results, or context assembly. 30-40% of RAG context is typically redundant.

BENEFITS:
- Reduces token usage by removing duplicate information
- Improves response quality by providing diverse perspectives
- Prevents LLM confusion from repetitive content

INPUT: Array of chunks with text and embeddings (from your RAG pipeline)
OUTPUT: Deduplicated chunks with diversity optimization`),
		mcp.WithArray("chunks",
			mcp.Required(),
			mcp.Description("Array of chunk objects. Each chunk must have 'text' (string) and 'embedding' (array of floats). Optional: 'id' (string), 'score' (float), 'metadata' (object)."),
		),
		mcp.WithNumber("target_k",
			mcp.Description("Target number of chunks to return (default: 8)"),
		),
		mcp.WithNumber("threshold",
			mcp.Description("Clustering threshold - lower means more aggressive deduplication (default: 0.15). Use 0.1 for code, 0.2 for prose."),
		),
		mcp.WithNumber("lambda",
			mcp.Description("MMR lambda - 1.0 for pure relevance, 0.0 for pure diversity (default: 0.5)"),
		),
	)

	s.AddTool(deduplicateTool, m.handleDeduplicateChunks)

	// Tool 2: retrieve_deduplicated - requires vector DB
	if m.broker != nil {
		retrieveTool := mcp.NewTool("retrieve_deduplicated",
			mcp.WithDescription(`Query vector database with automatic deduplication.

Fetches 3-5x more results than needed, clusters semantically similar chunks,
selects the best representative from each cluster, and applies MMR for diversity.

USE THIS instead of raw vector DB queries when you need diverse, non-redundant context.`),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("The search query text"),
			),
			mcp.WithString("namespace",
				mcp.Description("Vector DB namespace to search"),
			),
			mcp.WithNumber("target_k",
				mcp.Description("Target number of chunks to return (default: 8)"),
			),
			mcp.WithNumber("over_fetch_k",
				mcp.Description("Number of chunks to fetch before deduplication (default: 50)"),
			),
			mcp.WithNumber("threshold",
				mcp.Description("Clustering threshold (default: 0.15)"),
			),
			mcp.WithNumber("lambda",
				mcp.Description("MMR lambda for relevance vs diversity (default: 0.5)"),
			),
		)

		s.AddTool(retrieveTool, m.handleRetrieveDeduplicated)
	}

	// Tool 3: analyze_redundancy - analyze chunks without deduplicating
	analyzeTool := mcp.NewTool("analyze_redundancy",
		mcp.WithDescription(`Analyze chunks for semantic redundancy without removing any.

Use this to understand how much overlap exists in your context before deciding
whether to deduplicate. Returns cluster information and redundancy percentage.`),
		mcp.WithArray("chunks",
			mcp.Required(),
			mcp.Description("Array of chunk objects with 'text' and 'embedding' fields"),
		),
		mcp.WithNumber("threshold",
			mcp.Description("Clustering threshold (default: 0.15)"),
		),
	)

	s.AddTool(analyzeTool, m.handleAnalyzeRedundancy)
}

// System prompt that guides AI assistants to use deduplication
const systemPromptContent = `You have access to GoVectorSync, a semantic deduplication tool for RAG context.

IMPORTANT: When you receive multiple chunks from retrieval or search:
1. Check if chunks might contain overlapping information
2. If yes, use deduplicate_chunks to remove redundancy before reasoning
3. This typically reduces context by 30-40% while improving diversity

The tool clusters semantically similar chunks and picks the best representative
from each cluster, ensuring you see different perspectives rather than the same
information repeated.

Use analyze_redundancy first if you want to see the redundancy stats before deduplicating.`

func (m *MCPServer) registerResources(s *server.MCPServer) {
	// System prompt resource - hosts can include this in context
	systemPrompt := mcp.NewResource(
		"govectorsync://system-prompt",
		"GoVectorSync System Prompt",
		mcp.WithResourceDescription("System prompt that guides AI to use deduplication tools effectively"),
		mcp.WithMIMEType("text/plain"),
	)

	s.AddResource(systemPrompt, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "govectorsync://system-prompt",
				MIMEType: "text/plain",
				Text:     systemPromptContent,
			},
		}, nil
	})

	// Configuration resource - shows current settings
	configResource := mcp.NewResource(
		"govectorsync://config",
		"GoVectorSync Configuration",
		mcp.WithResourceDescription("Current deduplication configuration and defaults"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(configResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		config := map[string]interface{}{
			"defaults": map[string]interface{}{
				"target_k":   m.cfg.TargetK,
				"over_fetch_k": m.cfg.OverFetchK,
				"threshold":  m.cfg.ClusterThreshold,
				"lambda":     m.cfg.MMRLambda,
			},
			"backend_configured": m.broker != nil,
			"embedder_configured": m.embedder != nil,
		}
		configJSON, _ := json.MarshalIndent(config, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "govectorsync://config",
				MIMEType: "application/json",
				Text:     string(configJSON),
			},
		}, nil
	})
}

func (m *MCPServer) registerPrompts(s *server.MCPServer) {
	// Prompt for optimizing RAG context
	optimizePrompt := mcp.NewPrompt(
		"optimize-rag-context",
		mcp.WithPromptDescription("Optimize RAG context by removing redundant chunks before answering a question"),
		mcp.WithArgument("question", mcp.ArgumentDescription("The user's question to answer")),
		mcp.WithArgument("chunks_json", mcp.ArgumentDescription("JSON array of chunks with text and embeddings")),
	)

	s.AddPrompt(optimizePrompt, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		question := request.Params.Arguments["question"]
		chunksJSON := request.Params.Arguments["chunks_json"]

		return &mcp.GetPromptResult{
			Description: "Optimize RAG context before answering",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.RoleUser,
					Content: mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf(`I need to answer this question: %s

I have these chunks from RAG retrieval:
%s

Please:
1. First, call deduplicate_chunks with these chunks to remove redundancy
2. Then answer the question using only the deduplicated chunks
3. Cite which chunks you used in your answer`, question, chunksJSON),
					},
				},
			},
		}, nil
	})
}

// ChunkInput represents a chunk in the MCP request
type ChunkInput struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text"`
	Embedding []float64              `json:"embedding"`
	Score     float64                `json:"score"`
	Metadata  map[string]interface{} `json:"metadata"`
}

func (m *MCPServer) handleDeduplicateChunks(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse chunks from request
	args := request.GetArguments()
	chunksRaw, ok := args["chunks"]
	if !ok {
		return mcp.NewToolResultError("chunks parameter is required"), nil
	}

	// Convert to JSON and back to parse properly
	chunksJSON, err := json.Marshal(chunksRaw)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid chunks format: %v", err)), nil
	}

	var inputChunks []ChunkInput
	if err := json.Unmarshal(chunksJSON, &inputChunks); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse chunks: %v", err)), nil
	}

	if len(inputChunks) == 0 {
		return mcp.NewToolResultError("chunks array is empty"), nil
	}

	// Validate chunks have embeddings
	for i, c := range inputChunks {
		if len(c.Embedding) == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("chunk %d missing embedding", i)), nil
		}
	}

	// Convert to internal types
	chunks := make([]types.Chunk, len(inputChunks))
	for i, c := range inputChunks {
		embedding := make([]float32, len(c.Embedding))
		for j, v := range c.Embedding {
			embedding[j] = float32(v)
		}

		id := c.ID
		if id == "" {
			id = fmt.Sprintf("chunk_%d", i)
		}

		chunks[i] = types.Chunk{
			ID:        id,
			Text:      c.Text,
			Embedding: embedding,
			Score:     float32(c.Score),
			Metadata:  c.Metadata,
			ClusterID: -1,
		}
	}

	// Get optional parameters
	cfg := m.cfg
	if targetK := request.GetFloat("target_k", 0); targetK > 0 {
		cfg.TargetK = int(targetK)
	}
	if threshold := request.GetFloat("threshold", 0); threshold > 0 {
		cfg.ClusterThreshold = threshold
	}
	if lambda := request.GetFloat("lambda", -1); lambda >= 0 && lambda <= 1 {
		cfg.MMRLambda = lambda
	}

	// Create a temporary broker for processing
	clusterer := contextlab.NewClusterer(contextlab.ClusterConfig{
		Threshold: cfg.ClusterThreshold,
		Linkage:   cfg.ClusterLinkage,
	})
	selector := contextlab.NewSelector(contextlab.SelectorConfig{
		Strategy: cfg.SelectionStrategy,
	})
	mmr := contextlab.NewMMR(contextlab.MMRConfig{
		Lambda:  cfg.MMRLambda,
		TargetK: cfg.TargetK,
	})

	// Process chunks
	clusterResult := clusterer.Cluster(chunks)
	representatives := selector.Select(clusterResult)

	var finalChunks []types.Chunk
	if len(representatives) > cfg.TargetK {
		finalChunks = mmr.Rerank(representatives)
	} else {
		finalChunks = representatives
	}

	// Build response
	result := map[string]interface{}{
		"chunks": formatChunksForResponse(finalChunks),
		"stats": map[string]interface{}{
			"input_count":    len(inputChunks),
			"cluster_count":  clusterResult.ClusterCount,
			"output_count":   len(finalChunks),
			"reduction_pct":  clusterResult.ReductionPercent(),
			"threshold_used": cfg.ClusterThreshold,
			"lambda_used":    cfg.MMRLambda,
		},
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (m *MCPServer) handleRetrieveDeduplicated(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.broker == nil {
		return mcp.NewToolResultError("vector DB not configured - start with --backend and --index flags"), nil
	}

	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	namespace := request.GetString("namespace", "")

	// Get optional parameters and update config
	cfg := m.broker.GetConfig()
	if targetK := request.GetFloat("target_k", 0); targetK > 0 {
		cfg.TargetK = int(targetK)
	}
	if overFetchK := request.GetFloat("over_fetch_k", 0); overFetchK > 0 {
		cfg.OverFetchK = int(overFetchK)
	}
	if threshold := request.GetFloat("threshold", 0); threshold > 0 {
		cfg.ClusterThreshold = threshold
	}
	if lambda := request.GetFloat("lambda", -1); lambda >= 0 && lambda <= 1 {
		cfg.MMRLambda = lambda
	}
	m.broker.SetConfig(cfg)

	// Execute retrieval
	brokerResult, err := m.broker.RetrieveByText(ctx, query, namespace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("retrieval failed: %v", err)), nil
	}

	// Build response
	result := map[string]interface{}{
		"chunks": formatChunksForResponse(brokerResult.Chunks),
		"stats": map[string]interface{}{
			"retrieved":            brokerResult.Stats.Retrieved,
			"clustered":            brokerResult.Stats.Clustered,
			"returned":             brokerResult.Stats.Returned,
			"retrieval_latency_ms": brokerResult.Stats.RetrievalLatency.Milliseconds(),
			"clustering_latency_ms": brokerResult.Stats.ClusteringLatency.Milliseconds(),
			"total_latency_ms":     brokerResult.Stats.TotalLatency.Milliseconds(),
		},
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (m *MCPServer) handleAnalyzeRedundancy(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse chunks
	args := request.GetArguments()
	chunksRaw, ok := args["chunks"]
	if !ok {
		return mcp.NewToolResultError("chunks parameter is required"), nil
	}

	chunksJSON, err := json.Marshal(chunksRaw)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid chunks format: %v", err)), nil
	}

	var inputChunks []ChunkInput
	if err := json.Unmarshal(chunksJSON, &inputChunks); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse chunks: %v", err)), nil
	}

	if len(inputChunks) == 0 {
		return mcp.NewToolResultError("chunks array is empty"), nil
	}

	// Convert to internal types
	chunks := make([]types.Chunk, len(inputChunks))
	for i, c := range inputChunks {
		embedding := make([]float32, len(c.Embedding))
		for j, v := range c.Embedding {
			embedding[j] = float32(v)
		}

		id := c.ID
		if id == "" {
			id = fmt.Sprintf("chunk_%d", i)
		}

		chunks[i] = types.Chunk{
			ID:        id,
			Text:      c.Text,
			Embedding: embedding,
			Score:     float32(c.Score),
			Metadata:  c.Metadata,
			ClusterID: -1,
		}
	}

	// Get threshold
	threshold := m.cfg.ClusterThreshold
	if t := request.GetFloat("threshold", 0); t > 0 {
		threshold = t
	}

	// Cluster without selecting
	clusterer := contextlab.NewClusterer(contextlab.ClusterConfig{
		Threshold: threshold,
		Linkage:   "average",
	})
	clusterResult := clusterer.Cluster(chunks)

	// Build cluster details
	clusterDetails := make([]map[string]interface{}, len(clusterResult.Clusters))
	for i, cluster := range clusterResult.Clusters {
		memberIDs := make([]string, len(cluster.Members))
		memberTexts := make([]string, len(cluster.Members))
		for j, member := range cluster.Members {
			memberIDs[j] = member.ID
			if len(member.Text) > 100 {
				memberTexts[j] = member.Text[:100] + "..."
			} else {
				memberTexts[j] = member.Text
			}
		}

		clusterDetails[i] = map[string]interface{}{
			"cluster_id":    cluster.ID,
			"size":          cluster.Size(),
			"member_ids":    memberIDs,
			"member_texts":  memberTexts,
			"is_redundant":  cluster.Size() > 1,
		}
	}

	// Calculate redundancy stats
	redundantChunks := 0
	for _, cluster := range clusterResult.Clusters {
		if cluster.Size() > 1 {
			redundantChunks += cluster.Size() - 1
		}
	}

	result := map[string]interface{}{
		"summary": map[string]interface{}{
			"total_chunks":      len(inputChunks),
			"cluster_count":     clusterResult.ClusterCount,
			"redundant_chunks":  redundantChunks,
			"redundancy_pct":    float64(redundantChunks) / float64(len(inputChunks)) * 100,
			"unique_concepts":   clusterResult.ClusterCount,
			"threshold_used":    threshold,
		},
		"clusters": clusterDetails,
		"recommendation": fmt.Sprintf(
			"Found %d clusters from %d chunks. %.1f%% redundancy detected. Consider using deduplicate_chunks to reduce to %d unique chunks.",
			clusterResult.ClusterCount,
			len(inputChunks),
			float64(redundantChunks)/float64(len(inputChunks))*100,
			clusterResult.ClusterCount,
		),
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func formatChunksForResponse(chunks []types.Chunk) []map[string]interface{} {
	result := make([]map[string]interface{}, len(chunks))
	for i, c := range chunks {
		chunk := map[string]interface{}{
			"id":         c.ID,
			"text":       c.Text,
			"score":      c.Score,
			"cluster_id": c.ClusterID,
		}
		if len(c.Metadata) > 0 {
			chunk["metadata"] = c.Metadata
		}
		result[i] = chunk
	}
	return result
}
