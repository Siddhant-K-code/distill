package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Siddhant-K-code/distill/pkg/pipeline"
	"github.com/Siddhant-K-code/distill/pkg/types"
	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Run the full context optimisation pipeline (dedup → compress → summarize)",
	Long: `Runs the complete context optimisation pipeline on a JSON array of chunks
read from stdin or a file. Outputs the optimised chunks as JSON.

Example (stdin):
  echo '[{"id":"1","text":"..."},{"id":"2","text":"..."}]' | distill pipeline

Example (file):
  distill pipeline --input chunks.json --output optimised.json

Example (disable compress):
  distill pipeline --no-compress --dedup-threshold 0.2`,
	RunE: runPipeline,
}

func init() {
	rootCmd.AddCommand(pipelineCmd)

	pipelineCmd.Flags().String("input", "", "Input JSON file (default: stdin)")
	pipelineCmd.Flags().String("output", "", "Output JSON file (default: stdout)")

	// Dedup flags.
	pipelineCmd.Flags().Bool("no-dedup", false, "Disable deduplication stage")
	pipelineCmd.Flags().Float64("dedup-threshold", 0.15, "Cosine distance threshold for dedup clustering")
	pipelineCmd.Flags().Float64("dedup-lambda", 0.7, "MMR diversity weight")
	pipelineCmd.Flags().Int("dedup-target-k", 0, "Maximum chunks to keep after dedup (0 = no limit)")

	// Compress flags.
	pipelineCmd.Flags().Bool("no-compress", false, "Disable compression stage")
	pipelineCmd.Flags().Float64("compress-ratio", 0.5, "Target compression ratio (0.5 = reduce to 50% of tokens)")

	// Summarize flags.
	pipelineCmd.Flags().Bool("summarize", false, "Enable summarization stage")
	pipelineCmd.Flags().Int("summarize-max-tokens", 4000, "Token budget for summarization output")
	pipelineCmd.Flags().Int("summarize-recent", 10, "Number of recent turns to preserve at full fidelity")

	// Output flags.
	pipelineCmd.Flags().Bool("stats", false, "Print pipeline statistics to stderr")
}

func runPipeline(cmd *cobra.Command, _ []string) error {
	// Read input.
	inputFile, _ := cmd.Flags().GetString("input")
	var raw []byte
	var err error
	if inputFile != "" {
		raw, err = os.ReadFile(inputFile)
	} else {
		raw, err = readStdin()
	}
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	var chunks []types.Chunk
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	// Build options.
	noDedup, _ := cmd.Flags().GetBool("no-dedup")
	noCompress, _ := cmd.Flags().GetBool("no-compress")
	doSummarize, _ := cmd.Flags().GetBool("summarize")

	threshold, _ := cmd.Flags().GetFloat64("dedup-threshold")
	lambda, _ := cmd.Flags().GetFloat64("dedup-lambda")
	targetK, _ := cmd.Flags().GetInt("dedup-target-k")
	compressRatio, _ := cmd.Flags().GetFloat64("compress-ratio")
	maxTokens, _ := cmd.Flags().GetInt("summarize-max-tokens")
	keepRecent, _ := cmd.Flags().GetInt("summarize-recent")

	opts := pipeline.Options{
		DedupEnabled:            !noDedup,
		DedupThreshold:          threshold,
		DedupLambda:             lambda,
		DedupTargetK:            targetK,
		CompressEnabled:         !noCompress,
		CompressTargetReduction: compressRatio,
		SummarizeEnabled:        doSummarize,
		SummarizeMaxTokens:      maxTokens,
		SummarizeRecent:         keepRecent,
	}

	// Run.
	runner := pipeline.New()
	result, stats, err := runner.Run(context.Background(), chunks, opts)
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	// Write output.
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling output: %w", err)
	}

	outputFile, _ := cmd.Flags().GetString("output")
	if outputFile != "" {
		if err := os.WriteFile(outputFile, out, 0644); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	} else {
		fmt.Println(string(out))
	}

	// Print stats if requested.
	printStats, _ := cmd.Flags().GetBool("stats")
	if printStats {
		fmt.Fprintf(os.Stderr, "Pipeline stats:\n")
		fmt.Fprintf(os.Stderr, "  original_tokens: %d\n", stats.OriginalTokens)
		fmt.Fprintf(os.Stderr, "  final_tokens:    %d\n", stats.FinalTokens)
		fmt.Fprintf(os.Stderr, "  total_reduction: %.1f%%\n", stats.TotalReduction*100)
		fmt.Fprintf(os.Stderr, "  latency:         %s\n", stats.TotalLatency)
		for name, s := range stats.Stages {
			if s.Enabled {
				fmt.Fprintf(os.Stderr, "  stage[%s]: reduction=%.1f%% latency=%s\n",
					name, s.Reduction*100, s.Latency)
			}
		}
	}

	return nil
}

// readStdin reads all of stdin.
func readStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}
