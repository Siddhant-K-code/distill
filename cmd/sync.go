package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/dedup"
	"github.com/Siddhant-K-code/distill/pkg/ingest"
	pc "github.com/Siddhant-K-code/distill/pkg/pinecone"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync vectors to Pinecone with optional deduplication",
	Long: `Reads vectors from a JSONL file, optionally deduplicates them,
and uploads to a Pinecone index using parallel workers.

Example:
  distill sync --file data.jsonl --index my-index --dedup=true

Environment Variables:
  PINECONE_API_KEY    Your Pinecone API key (required)`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)

	// File input
	syncCmd.Flags().StringP("file", "f", "", "path to JSONL file containing vectors (required)")
	_ = syncCmd.MarkFlagRequired("file")

	// Pinecone settings
	syncCmd.Flags().StringP("index", "i", "", "Pinecone index name (required)")
	syncCmd.Flags().StringP("namespace", "n", "", "Pinecone namespace (optional)")
	syncCmd.Flags().String("api-key", "", "Pinecone API key (or use PINECONE_API_KEY env)")

	// Deduplication settings
	syncCmd.Flags().Bool("dedup", true, "enable semantic deduplication before upload")
	syncCmd.Flags().Float64P("threshold", "t", 0.05, "cosine distance threshold for duplicates")
	syncCmd.Flags().IntP("clusters", "k", 0, "number of clusters (0 = auto)")

	// Performance settings
	syncCmd.Flags().IntP("workers", "w", 0, "number of upload workers (0 = NumCPU*2)")
	syncCmd.Flags().IntP("batch-size", "b", 100, "vectors per batch (Pinecone optimal: 100)")

	// Bind to viper
	_ = viper.BindPFlag("api_key", syncCmd.Flags().Lookup("api-key"))
	_ = viper.BindPFlag("index", syncCmd.Flags().Lookup("index"))
	_ = viper.BindPFlag("namespace", syncCmd.Flags().Lookup("namespace"))
}

func runSync(cmd *cobra.Command, args []string) error {
	// Get flags
	filePath, _ := cmd.Flags().GetString("file")
	indexName, _ := cmd.Flags().GetString("index")
	namespace, _ := cmd.Flags().GetString("namespace")
	apiKey, _ := cmd.Flags().GetString("api-key")
	dedupEnabled, _ := cmd.Flags().GetBool("dedup")
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	clusters, _ := cmd.Flags().GetInt("clusters")
	workers, _ := cmd.Flags().GetInt("workers")
	batchSize, _ := cmd.Flags().GetInt("batch-size")
	verbose := viper.GetBool("verbose")

	// Resolve API key from env if not provided
	if apiKey == "" {
		apiKey = viper.GetString("api_key")
	}
	if apiKey == "" {
		apiKey = os.Getenv("PINECONE_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("pinecone API key is required: set PINECONE_API_KEY or use --api-key")
	}

	// Resolve index from env if not provided
	if indexName == "" {
		indexName = viper.GetString("index")
	}
	if indexName == "" {
		return fmt.Errorf("pinecone index name is required: use --index flag")
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, cleaning up...")
		cancel()
	}()

	// Load vectors
	fmt.Fprintf(os.Stderr, "Loading vectors from %s...\n", filePath)
	loadStart := time.Now()
	vectors, err := loadVectorsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to load vectors: %w", err)
	}
	loadDuration := time.Since(loadStart)

	if len(vectors) == 0 {
		fmt.Println("No vectors found in file.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Loaded %d vectors in %v\n", len(vectors), loadDuration)

	// Deduplication phase
	var uploadVectors = vectors
	if dedupEnabled {
		fmt.Fprintln(os.Stderr, "Running semantic deduplication...")

		cfg := dedup.Config{
			Threshold:     threshold,
			K:             clusters,
			MaxIterations: 10,
			Workers:       workers,
		}

		engine := dedup.NewEngine(cfg)
		result, err := engine.Deduplicate(ctx, vectors)
		if err != nil {
			return fmt.Errorf("deduplication failed: %w", err)
		}

		uploadVectors = result.UniqueVectors

		fmt.Fprintf(os.Stderr, "Deduplication complete: %d unique vectors (removed %d duplicates, %.1f%% savings)\n",
			len(uploadVectors), result.DuplicateCount, result.SavingsPercent())
	}

	// Connect to Pinecone
	fmt.Fprintf(os.Stderr, "Connecting to Pinecone index %q...\n", indexName)

	pcCfg := pc.Config{
		APIKey:    apiKey,
		IndexName: indexName,
		Namespace: namespace,
	}

	client, err := pc.NewClient(ctx, pcCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to Pinecone: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Create ingestion pipeline
	ingestCfg := ingest.Config{
		BatchSize: batchSize,
		Workers:   workers,
	}

	pipeline := ingest.NewPipeline(client, ingestCfg)

	// Create progress bar
	bar := progressbar.NewOptions64(
		int64(len(uploadVectors)),
		progressbar.OptionSetDescription("Uploading"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("vectors"),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	// Progress callback
	var lastUploaded int64
	progressFn := func(stats ingest.Stats) {
		current := stats.UploadedVectors + stats.FailedVectors
		delta := current - lastUploaded
		if delta > 0 {
			_ = bar.Add64(delta)
			lastUploaded = current
		}
	}

	// Run ingestion
	fmt.Fprintln(os.Stderr, "Starting upload...")
	stats, err := pipeline.IngestVectors(ctx, uploadVectors, progressFn)
	if err != nil {
		return fmt.Errorf("ingestion failed: %w", err)
	}

	_ = bar.Finish()
	fmt.Fprintln(os.Stderr)

	// Print summary
	printSyncSummary(stats, verbose)

	if stats.FailedVectors > 0 {
		return fmt.Errorf("%d vectors failed to upload", stats.FailedVectors)
	}

	return nil
}

func printSyncSummary(stats *ingest.Stats, verbose bool) {
	fmt.Println()
	fmt.Println("=== Sync Complete ===")
	fmt.Println()
	fmt.Printf("Vectors uploaded:    %d\n", stats.UploadedVectors)
	fmt.Printf("Vectors failed:      %d\n", stats.FailedVectors)
	fmt.Printf("Batches processed:   %d\n", stats.BatchesProcessed)
	fmt.Printf("Duration:            %v\n", stats.Duration().Round(time.Millisecond))
	fmt.Printf("Throughput:          %.0f vectors/sec\n", stats.VectorsPerSecond())
	fmt.Println()
}
