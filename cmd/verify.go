package cmd

import (
	distilllock "github.com/Siddhant-K-code/distill/pkg/lock"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <directory>",
	Short: "Verify a Distill Lock artifact offline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := distilllock.Verify(args[0])
		if err != nil {
			return err
		}
		return printLockSummary(cmd, "verified", summary)
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
