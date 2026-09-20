package cmd

import (
	distilllock "github.com/Siddhant-K-code/distill/pkg/lock"
	"github.com/spf13/cobra"
)

var buildOutput string

var buildCmd = &cobra.Command{
	Use:   "build <lockfile>",
	Short: "Build a deterministic context artifact from unchanged locked inputs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := distilllock.Build(args[0], buildOutput)
		if err != nil {
			return err
		}
		return printLockSummary(cmd, "built", summary)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "fresh output directory")
	_ = buildCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(buildCmd)
}
