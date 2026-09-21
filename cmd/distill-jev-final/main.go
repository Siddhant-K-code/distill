package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studyfinal"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "distill-jev-final:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "prepare":
		flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
		output := flags.String("output", "", "new output directory")
		commit := flags.String("llmtracefx-commit", studyfinal.LLMTraceFXCommit, "frozen LLMTraceFX commit or explicit offline-unresolved fixture marker")
		seed := flags.String("seed", studyfinal.DefaultCorpusSeed, "frozen corpus seed")
		agentArtifactPath := flags.String("agenttrace-contamination-artifact", "", "reviewed contamination record; absence forces downgrade")
		agentArtifactSHA256 := flags.String("agenttrace-contamination-sha256", "", "trusted SHA-256 of exact contamination artifact bytes")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *output == "" {
			return fmt.Errorf("--output is required")
		}
		var agentArtifact *studyfinal.ValidatedAgentTraceArtifact
		if *agentArtifactPath != "" {
			if *agentArtifactSHA256 == "" {
				return fmt.Errorf("--agenttrace-contamination-sha256 is required with artifact path")
			}
			raw, err := os.ReadFile(*agentArtifactPath)
			if err != nil {
				return err
			}
			agentArtifact, err = studyfinal.ValidateAgentTraceArtifact(raw, *agentArtifactSHA256)
			if err != nil {
				return err
			}
		} else if *agentArtifactSHA256 != "" {
			return fmt.Errorf("--agenttrace-contamination-artifact is required with expected SHA-256")
		}
		corpus, err := studyfinal.BuildCorpus(studyfinal.Config{
			LLMTraceFXCommit: *commit, Seed: *seed, TokenBound: studyfinal.DefaultMaxInputToken,
			AgentTraceContamination: agentArtifact,
		})
		if err != nil {
			return err
		}
		schedule, err := studyfinal.BuildSchedule(corpus, studyfinal.DefaultScheduleSeed)
		if err != nil {
			return err
		}
		return studyfinal.PreparePackage(*output, corpus, schedule)
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		input := flags.String("input", "", "package directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		_, _, err := studyfinal.ValidatePackage(*input)
		return err
	case "summarize":
		flags := flag.NewFlagSet("summarize", flag.ContinueOnError)
		input := flags.String("input", "", "package directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		corpus, schedule, err := studyfinal.ValidatePackage(*input)
		if err != nil {
			return err
		}
		summary := map[string]any{
			"schema_version": corpus.SchemaVersion, "executable": corpus.Executable,
			"execution_status": "no-go-without-signed-passed-attestation",
			"bases":            len(corpus.Bases), "conditions": len(corpus.Conditions), "scheduled_calls": len(schedule),
			"arms": []string{studyfinal.ArmRaw, studyfinal.ArmDistillLock}, "arm_b": corpus.ArmB, "arm_d": corpus.ArmD,
			"corpus_sha256": corpus.CorpusSHA256, "split_sha256": corpus.SplitSHA256,
			"question_schema_sha256":                   corpus.QuestionSchemaSHA256,
			"agenttrace_contamination_status":          corpus.AgentTraceContamination.Status,
			"agenttrace_contamination_evidence_sha256": corpus.AgentTraceContamination.EvidenceSHA256,
			"agenttrace_contamination_artifact_path":   corpus.AgentTraceContamination.ArtifactPath,
		}
		summary["schedule_sha256"], _ = studyfinal.DigestDomain("run-schedule", schedule)
		registry, err := studyfinal.BuildReceiptRegistry(corpus, schedule)
		if err != nil {
			return err
		}
		summary["receipt_registry_sha256"] = registry.RegistrySHA256
		return json.NewEncoder(os.Stdout).Encode(summary)
	case "authorize":
		flags := flag.NewFlagSet("authorize", flag.ContinueOnError)
		input := flags.String("input", "", "validated package directory")
		output := flags.String("output", "", "new owner-only authorization file")
		analysis := flags.String("analysis-sha256", "", "frozen analysis hash")
		expires := flags.String("expires", "", "RFC3339 expiry")
		attestationPath := flags.String("attestation", "", "signed passed gate-attestation JSON")
		ledgerPath := flags.String("ledger", "", "pre-created witnessed execution ledger")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *output == "" || *analysis == "" || *expires == "" || *attestationPath == "" || *ledgerPath == "" {
			return fmt.Errorf("--input, --output, --analysis-sha256, --expires, --attestation, and --ledger are required")
		}
		expiry, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			return err
		}
		corpus, schedule, err := studyfinal.ValidatePackage(*input)
		if err != nil {
			return err
		}
		if !corpus.Executable {
			return fmt.Errorf("LLMTraceFX registry remains offline-unresolved")
		}
		var attestation studyfinal.GateAttestation
		if err := readStrictJSON(*attestationPath, &attestation); err != nil {
			return err
		}
		ledgerBinding, err := studyfinal.LedgerBindingFromAttestation(*ledgerPath, attestation)
		if err != nil {
			return err
		}
		auth, err := studyfinal.NewAuthorization(corpus, schedule, *analysis, studyfinal.DefaultMaxInputToken, expiry, attestation, ledgerBinding, time.Now().UTC())
		if err != nil {
			return err
		}

		return studyfinal.WriteAuthorization(*output, auth)
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		fake := flags.Bool("fake", false, "acknowledge fake-transport-only scaffold")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if !*fake {
			return fmt.Errorf("real transport is intentionally unavailable; use the in-memory studyfinal.Transport interface")
		}
		_, err := fmt.Fprintln(os.Stdout, "fake transport scaffold ready; no network call made")
		return err
	default:
		return usageError()
	}
}

func readStrictJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func usageError() error {
	return fmt.Errorf("usage: distill-jev-final <prepare|validate|summarize|authorize|run>")
}
