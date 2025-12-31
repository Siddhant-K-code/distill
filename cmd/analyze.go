package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/dedup"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze a JSONL file for semantic duplicates",
	Long: `Runs the semantic deduplication engine on a JSONL file and reports
potential duplicates without uploading to Pinecone.

Example:
  govs analyze --file data.jsonl --threshold 0.05

The threshold controls duplicate sensitivity:
  - 0.01: Very strict (only near-identical vectors)
  - 0.05: Balanced (recommended default)
  - 0.10: Loose (more aggressive deduplication)`,
	RunE: runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	analyzeCmd.Flags().StringP("file", "f", "", "path to JSONL file containing vectors (required)")
	analyzeCmd.Flags().Float64P("threshold", "t", 0.05, "cosine distance threshold for duplicates")
	analyzeCmd.Flags().IntP("clusters", "k", 0, "number of clusters (0 = auto: sqrt(N/2))")
	analyzeCmd.Flags().IntP("workers", "w", 0, "number of parallel workers (0 = NumCPU)")
	analyzeCmd.Flags().Int64("seed", 0, "random seed for reproducibility (0 = random)")

	analyzeCmd.MarkFlagRequired("file")

	viper.BindPFlag("analyze.threshold", analyzeCmd.Flags().Lookup("threshold"))
	viper.BindPFlag("analyze.clusters", analyzeCmd.Flags().Lookup("clusters"))
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	clusters, _ := cmd.Flags().GetInt("clusters")
	workers, _ := cmd.Flags().GetInt("workers")
	seed, _ := cmd.Flags().GetInt64("seed")
	verbose := viper.GetBool("verbose")

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

	// Load vectors from file
	if verbose {
		fmt.Fprintf(os.Stderr, "Loading vectors from %s...\n", filePath)
	}

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

	if verbose {
		fmt.Fprintf(os.Stderr, "Loaded %d vectors in %v\n", len(vectors), loadDuration)
		fmt.Fprintf(os.Stderr, "Vector dimension: %d\n", vectors[0].Dimension())
	}

	// Configure deduplication engine
	cfg := dedup.Config{
		Threshold:     threshold,
		K:             clusters,
		MaxIterations: 10,
		Workers:       workers,
		Seed:          seed,
	}

	engine := dedup.NewEngine(cfg)

	// Run deduplication
	if verbose {
		fmt.Fprintln(os.Stderr, "Running semantic deduplication...")
	}

	result, err := engine.Deduplicate(ctx, vectors)
	if err != nil {
		return fmt.Errorf("deduplication failed: %w", err)
	}

	// Print report
	printAnalysisReport(result, verbose)

	return nil
}

func loadVectorsFromFile(filePath string) ([]types.Vector, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var vectors []types.Vector
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var v struct {
			ID       string                 `json:"id"`
			Values   []float32              `json:"values"`
			Metadata map[string]interface{} `json:"metadata,omitempty"`
		}

		if err := json.Unmarshal(line, &v); err != nil {
			// Skip malformed lines but warn
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed line %d: %v\n", lineNum, err)
			continue
		}

		if v.ID == "" || len(v.Values) == 0 {
			continue
		}

		vectors = append(vectors, types.Vector{
			ID:       v.ID,
			Values:   v.Values,
			Metadata: v.Metadata,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return vectors, nil
}

func printAnalysisReport(result *types.DeduplicationResult, verbose bool) {
	fmt.Println()
	fmt.Println("=== Semantic Deduplication Analysis ===")
	fmt.Println()
	fmt.Printf("Total vectors analyzed:  %d\n", result.TotalProcessed)
	fmt.Printf("Unique vectors:          %d\n", len(result.UniqueVectors))
	fmt.Printf("Duplicates found:        %d\n", result.DuplicateCount)
	fmt.Printf("Potential savings:       %.1f%%\n", result.SavingsPercent())
	fmt.Println()
	fmt.Printf("Clusters used:           %d\n", result.ClusterCount)
	fmt.Printf("Processing time:         %dms\n", result.ProcessingTimeMs)
	fmt.Println()

	if result.DuplicateCount > 0 {
		fmt.Println("Recommendation: Use 'govs sync --dedup=true' to upload deduplicated vectors.")
	} else {
		fmt.Println("No duplicates found. Your dataset is already unique.")
	}
}
