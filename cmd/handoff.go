package cmd

import (
	"fmt"

	"github.com/Siddhant-K-code/distill/pkg/handoff"
	"github.com/spf13/cobra"
)

var handoffCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Prepare and verify experimental review-only decision handoffs",
	Args:  cobra.NoArgs,
	Long: `Handoff v0 alpha freezes a Markdown conversation and docs tree for an
external agent, then deterministically verifies its review-only proposal.

Distill never calls a model, mutates source docs, applies a patch, or approves a
change. Every accepted proposal route requires human review.`,
}

func newHandoffPrepareCommand() *cobra.Command {
	var conversationPath string
	var docsDirectory string
	var outputDirectory string
	command := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare a deterministic request bundle for an external agent",
		Example: `  distill handoff prepare \
    --conversation export.md \
    --docs ./docs \
    --out ./handoff-request`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			summary, err := handoff.Prepare(conversationPath, docsDirectory, outputDirectory)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"prepared request_id=%s request_sha256=%s documents=%d route=require_review\n",
				summary.RequestID,
				summary.RequestSHA256,
				summary.DocumentCount,
			)
			return err
		},
	}
	command.Flags().StringVar(&conversationPath, "conversation", "", "exported Markdown conversation file")
	command.Flags().StringVar(&docsDirectory, "docs", "", "local Markdown documentation tree")
	command.Flags().StringVar(&outputDirectory, "out", "", "fresh request bundle directory")
	_ = command.MarkFlagRequired("conversation")
	_ = command.MarkFlagRequired("docs")
	_ = command.MarkFlagRequired("out")
	return command
}

func newHandoffVerifyCommand() *cobra.Command {
	var requestPath string
	var proposalPath string
	var outputDirectory string
	var expectedRequestSHA256 string
	command := &cobra.Command{
		Use:   "verify",
		Short: "Verify and package an external proposal for human review",
		Example: `  distill handoff verify \
    --request ./handoff-request/handoff.request.json \
    --expected-request-sha256 <trusted-prepare-digest> \
    --proposal ./proposal.json \
    --out ./handoff-review`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			summary, err := handoff.VerifyWithExpectedRequest(
				requestPath,
				proposalPath,
				outputDirectory,
				expectedRequestSHA256,
			)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"verified request_id=%s proposal_sha256=%s receipt_sha256=%s outcome=%s candidates=%d route=require_review\n",
				summary.RequestID,
				summary.ProposalSHA256,
				summary.ReceiptSHA256,
				summary.Outcome,
				summary.CandidateCount,
			)
			return err
		},
	}
	command.Flags().StringVar(&requestPath, "request", "", "prepared handoff.request.json path")
	command.Flags().StringVar(
		&expectedRequestSHA256,
		"expected-request-sha256",
		"",
		"trusted request SHA-256 printed by handoff prepare",
	)
	command.Flags().StringVar(&proposalPath, "proposal", "", "canonical external proposal JSON path")
	command.Flags().StringVar(&outputDirectory, "out", "", "fresh review package directory")
	_ = command.MarkFlagRequired("request")
	_ = command.MarkFlagRequired("expected-request-sha256")
	_ = command.MarkFlagRequired("proposal")
	_ = command.MarkFlagRequired("out")
	return command
}

func init() {
	handoffCmd.AddCommand(newHandoffPrepareCommand(), newHandoffVerifyCommand())
	rootCmd.AddCommand(handoffCmd)
}
