package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/contextlab"
	"github.com/Siddhant-K-code/distill/pkg/embedding/openai"
	"github.com/Siddhant-K-code/distill/pkg/retriever"
	pcretriever "github.com/Siddhant-K-code/distill/pkg/retriever/pinecone"
	qdretriever "github.com/Siddhant-K-code/distill/pkg/retriever/qdrant"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query [text]",
	Short: "Query the vector database with semantic deduplication",
	Long: `Performs a semantic search with deduplication and displays results.
Useful for testing and tuning ContextLab parameters.

Example:
  distill query "How do I configure authentication?" --index my-index

Requires PINECONE_API_KEY and OPENAI_API_KEY environment variables.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runQuery,
}

func init() {
	rootCmd.AddCommand(queryCmd)

	// Backend settings
	queryCmd.Flags().String("backend", "pinecone", "Vector DB backend (pinecone, qdrant)")
	queryCmd.Flags().StringP("index", "i", "", "Index/collection name (required)")
	queryCmd.Flags().String("api-key", "", "Vector DB API key")
	queryCmd.Flags().String("db-host", "", "Vector DB host (for Qdrant)")
	queryCmd.Flags().StringP("namespace", "n", "", "Namespace")

	// Embedding settings
	queryCmd.Flags().String("openai-key", "", "OpenAI API key")
	queryCmd.Flags().String("embedding-model", "text-embedding-3-small", "Embedding model")

	// ContextLab settings
	queryCmd.Flags().Int("over-fetch-k", 50, "Number of chunks to over-fetch")
	queryCmd.Flags().Int("target-k", 8, "Target number of chunks")
	queryCmd.Flags().Float64("threshold", 0.15, "Clustering threshold")
	queryCmd.Flags().Float64("lambda", 0.5, "MMR lambda")
	queryCmd.Flags().Bool("enable-mmr", true, "Enable MMR re-ranking")
	queryCmd.Flags().Bool("no-dedup", false, "Disable deduplication (raw retrieval)")

	// Output settings
	queryCmd.Flags().Bool("show-text", true, "Show chunk text")
	queryCmd.Flags().Bool("show-metadata", false, "Show chunk metadata")
	queryCmd.Flags().Bool("show-stats", true, "Show processing statistics")
	queryCmd.Flags().Int("text-limit", 200, "Max characters of text to show per chunk")
}

func runQuery(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Get flags
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
	noDedup, _ := cmd.Flags().GetBool("no-dedup")
	showText, _ := cmd.Flags().GetBool("show-text")
	showMetadata, _ := cmd.Flags().GetBool("show-metadata")
	showStats, _ := cmd.Flags().GetBool("show-stats")
	textLimit, _ := cmd.Flags().GetInt("text-limit")

	// Resolve API keys from environment
	if apiKey == "" {
		apiKey = os.Getenv("PINECONE_API_KEY")
	}
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Validate
	if index == "" {
		return fmt.Errorf("index name required (--index)")
	}
	if openaiKey == "" {
		return fmt.Errorf("OpenAI API key required for text queries (--openai-key or OPENAI_API_KEY)")
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nCancelled")
		cancel()
	}()

	// Create retriever
	var ret retriever.Retriever
	var err error

	switch backend {
	case "pinecone":
		if apiKey == "" {
			return fmt.Errorf("Pinecone API key required")
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
	defer func() { _ = ret.Close() }()

	// Create embedding provider
	embedder, err := openai.NewClient(openai.Config{
		APIKey: openaiKey,
		Model:  embeddingModel,
	})
	if err != nil {
		return fmt.Errorf("failed to create embedding provider: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Query: %s\n", query)
	fmt.Fprintf(os.Stderr, "Embedding query...\n")

	// Embed query
	embedding, err := embedder.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to embed query: %w", err)
	}

	var chunks []types.Chunk
	var stats types.BrokerStats

	if noDedup {
		// Raw retrieval without deduplication
		fmt.Fprintf(os.Stderr, "Retrieving (no dedup)...\n")

		req := &types.RetrievalRequest{
			QueryEmbedding:    embedding,
			TopK:              targetK,
			Namespace:         namespace,
			IncludeEmbeddings: true,
			IncludeMetadata:   true,
		}

		start := time.Now()
		result, err := ret.Query(ctx, req)
		if err != nil {
			return fmt.Errorf("retrieval failed: %w", err)
		}

		chunks = result.Chunks
		stats = types.BrokerStats{
			Retrieved:        len(chunks),
			Returned:         len(chunks),
			RetrievalLatency: result.Latency,
			TotalLatency:     time.Since(start),
		}
	} else {
		// Use ContextLab broker
		fmt.Fprintf(os.Stderr, "Retrieving with deduplication...\n")

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

		broker := contextlab.NewBrokerWithEmbedder(ret, embedder, brokerCfg)
		defer func() { _ = broker.Close() }()

		req := &types.RetrievalRequest{
			QueryEmbedding: embedding,
			Namespace:      namespace,
		}

		result, err := broker.Retrieve(ctx, req)
		if err != nil {
			return fmt.Errorf("retrieval failed: %w", err)
		}

		chunks = result.Chunks
		stats = result.Stats
	}

	fmt.Fprintln(os.Stderr)

	// Display results
	if len(chunks) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	fmt.Printf("=== Results (%d chunks) ===\n\n", len(chunks))

	for i, chunk := range chunks {
		fmt.Printf("[%d] ID: %s\n", i+1, chunk.ID)
		fmt.Printf("    Score: %.4f", chunk.Score)
		if chunk.ClusterID >= 0 {
			fmt.Printf("  |  Cluster: %d", chunk.ClusterID)
		}
		fmt.Println()

		if showText && chunk.Text != "" {
			text := chunk.Text
			if textLimit > 0 && len(text) > textLimit {
				text = text[:textLimit] + "..."
			}
			// Clean up whitespace
			text = strings.ReplaceAll(text, "\n", " ")
			text = strings.Join(strings.Fields(text), " ")
			fmt.Printf("    Text: %s\n", text)
		}

		if showMetadata && len(chunk.Metadata) > 0 {
			fmt.Printf("    Metadata: %v\n", chunk.Metadata)
		}

		fmt.Println()
	}

	// Display stats
	if showStats {
		fmt.Println("=== Statistics ===")
		fmt.Printf("Retrieved:    %d chunks\n", stats.Retrieved)
		if stats.Clustered > 0 {
			fmt.Printf("Clusters:     %d\n", stats.Clustered)
		}
		fmt.Printf("Returned:     %d chunks\n", stats.Returned)
		if stats.Retrieved > 0 && stats.Returned > 0 {
			reduction := float64(stats.Retrieved-stats.Returned) / float64(stats.Retrieved) * 100
			fmt.Printf("Reduction:    %.1f%%\n", reduction)
		}
		fmt.Printf("Retrieval:    %dms\n", stats.RetrievalLatency.Milliseconds())
		if stats.ClusteringLatency > 0 {
			fmt.Printf("Clustering:   %dms\n", stats.ClusteringLatency.Milliseconds())
		}
		fmt.Printf("Total:        %dms\n", stats.TotalLatency.Milliseconds())
	}

	return nil
}
