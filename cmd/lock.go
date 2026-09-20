package cmd

import (
	"fmt"

	distilllock "github.com/Siddhant-K-code/distill/pkg/lock"
	"github.com/spf13/cobra"
)

var lockOutput string

var lockCmd = &cobra.Command{
	Use:   "lock <config>",
	Short: "Freeze deterministic context inputs without model or network calls",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := distilllock.Create(args[0], lockOutput)
		if err != nil {
			return err
		}
		return printLockSummary(cmd, "locked", summary)
	},
}

func init() {
	lockCmd.Flags().StringVarP(&lockOutput, "output", "o", "", "canonical lockfile path")
	_ = lockCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(lockCmd)
}

func printLockSummary(cmd *cobra.Command, action string, summary distilllock.Summary) error {
	line := fmt.Sprintf(
		"%s sources=%d duplicates=%d selected=%d tokens=%d",
		action,
		summary.SourceCount,
		summary.DuplicateCount,
		summary.SelectedCount,
		summary.SelectedTokens,
	)
	if summary.BundleSHA256 != "" {
		line += fmt.Sprintf(" bundle_sha256=%s", summary.BundleSHA256)
	}
	if summary.LockSHA256 != "" {
		line += fmt.Sprintf(" lock_sha256=%s", summary.LockSHA256)
	}
	if summary.ManifestSHA256 != "" {
		line += fmt.Sprintf(" manifest_sha256=%s", summary.ManifestSHA256)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
	return err
}
