package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Siddhant-K-code/distill/pkg/embedding/openai"
	"github.com/Siddhant-K-code/distill/pkg/memory"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage the persistent context memory store",
	Long: `Store, recall, and manage persistent context memories.

Memories are deduplicated on write, compressed over time, and
ranked by relevance + recency on recall.

Examples:
  distill memory store --text "Auth uses JWT with RS256" --tags auth
  distill memory recall --query "How does auth work?" --max-results 5
  distill memory forget --tags deprecated
  distill memory stats`,
}

var memoryStoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Store a memory entry",
	RunE:  runMemoryStore,
}

var memoryRecallCmd = &cobra.Command{
	Use:   "recall",
	Short: "Recall memories matching a query",
	RunE:  runMemoryRecall,
}

var memoryForgetCmd = &cobra.Command{
	Use:   "forget",
	Short: "Remove memories matching criteria",
	RunE:  runMemoryForget,
}

var memoryStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show memory store statistics",
	RunE:  runMemoryStats,
}

func init() {
	rootCmd.AddCommand(memoryCmd)
	memoryCmd.AddCommand(memoryStoreCmd)
	memoryCmd.AddCommand(memoryRecallCmd)
	memoryCmd.AddCommand(memoryForgetCmd)
	memoryCmd.AddCommand(memoryStatsCmd)

	// Shared flags
	memoryCmd.PersistentFlags().String("db", "distill-memory.db", "SQLite database path")
	memoryCmd.PersistentFlags().Float64("dedup-threshold", 0.15, "Cosine distance threshold for dedup")

	// Store flags
	memoryStoreCmd.Flags().String("text", "", "Text to store")
	memoryStoreCmd.Flags().String("source", "", "Source of the memory (e.g., code_review, docs)")
	memoryStoreCmd.Flags().StringSlice("tags", nil, "Tags for the memory")
	memoryStoreCmd.Flags().String("session-id", "", "Session ID")
	memoryStoreCmd.Flags().String("openai-key", "", "OpenAI API key for embeddings (or OPENAI_API_KEY)")

	// Recall flags
	memoryRecallCmd.Flags().String("query", "", "Query text")
	memoryRecallCmd.Flags().StringSlice("tags", nil, "Filter by tags")
	memoryRecallCmd.Flags().Int("max-results", 10, "Maximum results to return")
	memoryRecallCmd.Flags().Int("max-tokens", 0, "Maximum token budget (0 = unlimited)")
	memoryRecallCmd.Flags().Float64("recency-weight", 0.3, "Weight for recency vs relevance (0-1)")
	memoryRecallCmd.Flags().String("openai-key", "", "OpenAI API key for embeddings (or OPENAI_API_KEY)")

	// Forget flags
	memoryForgetCmd.Flags().StringSlice("tags", nil, "Remove memories with these tags")
	memoryForgetCmd.Flags().StringSlice("ids", nil, "Remove memories with these IDs")
}

func openMemoryStore(cmd *cobra.Command) (*memory.SQLiteStore, error) {
	dbPath, _ := cmd.Flags().GetString("db")
	threshold, _ := cmd.Flags().GetFloat64("dedup-threshold")

	cfg := memory.DefaultConfig()
	cfg.DedupThreshold = threshold

	return memory.NewSQLiteStore(dbPath, cfg)
}

func runMemoryStore(cmd *cobra.Command, args []string) error {
	text, _ := cmd.Flags().GetString("text")
	if text == "" {
		return fmt.Errorf("--text is required")
	}

	source, _ := cmd.Flags().GetString("source")
	tags, _ := cmd.Flags().GetStringSlice("tags")
	sessionID, _ := cmd.Flags().GetString("session-id")
	openaiKey, _ := cmd.Flags().GetString("openai-key")
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	store, err := openMemoryStore(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	entry := memory.StoreEntry{
		Text:   text,
		Source: source,
		Tags:   tags,
	}

	// Generate embedding if OpenAI key is available
	if openaiKey != "" {
		model := viper.GetString("embedding.model")
		if model == "" {
			model = "text-embedding-3-small"
		}
		embedder, err := openai.NewClient(openai.Config{APIKey: openaiKey, Model: model})
		if err != nil {
			return fmt.Errorf("create embedder: %w", err)
		}
		emb, err := embedder.Embed(context.Background(), text)
		if err != nil {
			return fmt.Errorf("embed text: %w", err)
		}
		entry.Embedding = emb
	}

	result, err := store.Store(context.Background(), memory.StoreRequest{
		SessionID: sessionID,
		Entries:   []memory.StoreEntry{entry},
	})
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

func runMemoryRecall(cmd *cobra.Command, args []string) error {
	query, _ := cmd.Flags().GetString("query")
	if query == "" {
		return fmt.Errorf("--query is required")
	}

	tags, _ := cmd.Flags().GetStringSlice("tags")
	maxResults, _ := cmd.Flags().GetInt("max-results")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	recencyWeight, _ := cmd.Flags().GetFloat64("recency-weight")
	openaiKey, _ := cmd.Flags().GetString("openai-key")
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	store, err := openMemoryStore(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	req := memory.RecallRequest{
		Query:         query,
		Tags:          tags,
		MaxResults:    maxResults,
		MaxTokens:     maxTokens,
		RecencyWeight: recencyWeight,
	}

	// Generate query embedding if OpenAI key is available
	if openaiKey != "" {
		model := viper.GetString("embedding.model")
		if model == "" {
			model = "text-embedding-3-small"
		}
		embedder, err := openai.NewClient(openai.Config{APIKey: openaiKey, Model: model})
		if err != nil {
			return fmt.Errorf("create embedder: %w", err)
		}
		emb, err := embedder.Embed(context.Background(), query)
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}
		req.QueryEmbedding = emb
	}

	result, err := store.Recall(context.Background(), req)
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

func runMemoryForget(cmd *cobra.Command, args []string) error {
	tags, _ := cmd.Flags().GetStringSlice("tags")
	ids, _ := cmd.Flags().GetStringSlice("ids")

	if len(tags) == 0 && len(ids) == 0 {
		return fmt.Errorf("at least one of --tags or --ids is required")
	}

	store, err := openMemoryStore(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	result, err := store.Forget(context.Background(), memory.ForgetRequest{
		Tags: tags,
		IDs:  ids,
	})
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

func runMemoryStats(cmd *cobra.Command, args []string) error {
	store, err := openMemoryStore(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	stats, err := store.Stats(context.Background())
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(out))
	return nil
}

// memoryStoreFromConfig creates a memory store from the API server config.
// Used by the API server and MCP server.
func memoryStoreFromConfig(dbPath string, threshold float64) (*memory.SQLiteStore, error) {
	if dbPath == "" {
		dbPath = "distill-memory.db"
	}
	cfg := memory.DefaultConfig()
	cfg.DedupThreshold = threshold
	return memory.NewSQLiteStore(dbPath, cfg)
}
