package cmd

import (
	distilllock "github.com/Siddhant-K-code/distill/pkg/lock"
	"github.com/spf13/cobra"
)

var verifyExpectedLockSHA256 string

var verifyCmd = &cobra.Command{
	Use:   "verify <directory>",
	Short: "Verify a Distill Lock artifact offline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, err := distilllock.VerifyWithExpectedLock(args[0], verifyExpectedLockSHA256)
		if err != nil {
			return err
		}
		return printLockSummary(cmd, "verified", summary)
	},
}

func init() {
	verifyCmd.Flags().StringVar(
		&verifyExpectedLockSHA256,
		"expected-lock-sha256",
		"",
		"trusted out-of-band context.lock.json SHA-256",
	)
	rootCmd.AddCommand(verifyCmd)
}
