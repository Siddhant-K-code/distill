package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studylocalv2"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "distill-local-context-control-v2:", err)
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
		output := flags.String("output", "", "new offline package directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *output == "" {
			return fmt.Errorf("--output is required")
		}
		_, err := studylocalv2.Prepare(*output)
		return err
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		input := flags.String("input", "", "offline package directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		_, err := studylocalv2.ValidatePackage(*input)
		return err
	case "summarize":
		flags := flag.NewFlagSet("summarize", flag.ContinueOnError)
		input := flags.String("input", "", "offline package directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		summary, err := studylocalv2.Summarize(*input)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(summary)
	case "model-manifest":
		flags := flag.NewFlagSet("model-manifest", flag.ContinueOnError)
		modelPath := flags.String("model-path", "", "verified local model directory")
		output := flags.String("output", "", "new owner-only model manifest")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *modelPath == "" || *output == "" {
			return fmt.Errorf("--model-path and --output are required")
		}
		_, err := studylocalv2.WriteModelManifest(*modelPath, *output)
		return err
	case "runtime-manifest":
		flags := flag.NewFlagSet("runtime-manifest", flag.ContinueOnError)
		runtimePython := flags.String("runtime-python", "", "fresh trusted venv bin/python")
		adapter := flags.String("adapter", "tools/local-context-control-mlx-v2.py", "committed trusted adapter")
		repository := flags.String("repository", ".", "clean merged repository root")
		commit := flags.String("expected-commit", "", "exact merged implementation commit")
		runtimeTree := flags.String("runtime-tree-manifest", "", "private exact venv tree manifest")
		baseTree := flags.String("base-tree-manifest", "", "private exact base CPython tree manifest")
		wheelVerification := flags.String("wheel-verification", "", "private wheel RECORD verification record")
		output := flags.String("output", "", "new owner-only runtime manifest")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *runtimePython == "" || *commit == "" || *runtimeTree == "" || *baseTree == "" ||
			*wheelVerification == "" || *output == "" {
			return fmt.Errorf("--runtime-python, --expected-commit, --runtime-tree-manifest, --base-tree-manifest, --wheel-verification, and --output are required")
		}
		repoAbs, err := filepath.Abs(*repository)
		if err != nil {
			return err
		}
		adapterAbs, err := filepath.Abs(*adapter)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		_, err = studylocalv2.WriteRuntimeManifest(ctx, studylocalv2.RuntimeManifestOptions{
			RuntimePython: *runtimePython, AdapterPath: adapterAbs,
			RepositoryRoot: repoAbs, ImplementationCommit: *commit, OutputPath: *output,
			RuntimeTreeManifestPath: *runtimeTree, BaseTreeManifestPath: *baseTree,
			WheelVerificationPath: *wheelVerification,
		})
		return err
	case "preflight":
		flags := flag.NewFlagSet("preflight", flag.ContinueOnError)
		outputParent := flags.String("output-parent", "", "existing parent for the future private run")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *outputParent == "" {
			return fmt.Errorf("--output-parent is required")
		}
		protocol, err := studylocalv2.BuildProtocol()
		if err != nil {
			return err
		}
		snapshot, err := studylocalv2.CollectHostPreflight(context.Background(), *outputParent, protocol.Isolation)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(os.Stdout).Encode(snapshot); err != nil {
			return err
		}
		if !snapshot.Eligible {
			return fmt.Errorf("host is not execution-eligible")
		}
		return nil
	case "adapter-check":
		flags := flag.NewFlagSet("adapter-check", flag.ContinueOnError)
		input := flags.String("input", "", "validated v2 offline package")
		modelPath := flags.String("model-path", "", "verified local model directory")
		modelManifest := flags.String("model-manifest", "", "owner-only v2 model manifest")
		runtimePython := flags.String("runtime-python", "", "fresh trusted venv bin/python")
		runtimeManifest := flags.String("runtime-manifest", "", "owner-only fresh v2 runtime manifest")
		runtimeTree := flags.String("runtime-tree-manifest", "", "private exact venv tree manifest")
		baseTree := flags.String("base-tree-manifest", "", "private exact base CPython tree manifest")
		wheelVerification := flags.String("wheel-verification", "", "private wheel RECORD verification record")
		adapter := flags.String("adapter", "tools/local-context-control-mlx-v2.py", "committed trusted v2 adapter")
		repository := flags.String("repository", ".", "clean merged repository root")
		mainRef := flags.String("main-ref", "origin/main", "reviewed merged main ref")
		trustedCommit := flags.String("trusted-implementation-commit", "", "reviewed merged v2 commit")
		trustedTree := flags.String("trusted-implementation-tree", "", "reviewed merged v2 tree")
		runOutput := flags.String("run-output", "", "absent private v2 run namespace")
		output := flags.String("output", "", "new owner-only adapter-check directory")
		expiresAt := flags.String("expires-at", "", "RFC3339 expiry no more than two hours after check start")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *modelPath == "" || *modelManifest == "" || *runtimePython == "" ||
			*runtimeManifest == "" || *runtimeTree == "" || *baseTree == "" ||
			*wheelVerification == "" || *trustedCommit == "" || *trustedTree == "" ||
			*runOutput == "" || *output == "" || *expiresAt == "" {
			return fmt.Errorf("all adapter-check identity, runtime, model, output, and expiry flags are required")
		}
		expiry, err := time.Parse(time.RFC3339Nano, *expiresAt)
		if err != nil {
			return fmt.Errorf("--expires-at: %w", err)
		}
		repoAbs, err := filepath.Abs(*repository)
		if err != nil {
			return err
		}
		adapterAbs, err := filepath.Abs(*adapter)
		if err != nil {
			return err
		}
		_, err = studylocalv2.RunAdapterCheck(context.Background(), studylocalv2.AdapterCheckOptions{
			PackageDirectory: *input, ModelDirectory: *modelPath,
			ModelManifestPath: *modelManifest, RuntimePython: *runtimePython,
			RuntimeManifestPath: *runtimeManifest, RuntimeTreeManifestPath: *runtimeTree,
			BaseTreeManifestPath: *baseTree, WheelVerificationPath: *wheelVerification,
			AdapterPath: adapterAbs, RepositoryRoot: repoAbs, MainRef: *mainRef,
			TrustedImplementationCommit: *trustedCommit, TrustedImplementationTree: *trustedTree,
			RunOutputDirectory: *runOutput, OutputDirectory: *output, ExpiresAt: expiry,
		})
		return err
	case "authorize":
		flags := flag.NewFlagSet("authorize", flag.ContinueOnError)
		input := flags.String("input", "", "validated offline package")
		modelPath := flags.String("model-path", "", "verified local model directory")
		modelManifest := flags.String("model-manifest", "", "owner-only model manifest")
		runtimePython := flags.String("runtime-python", "", "fresh trusted venv bin/python")
		runtimeManifest := flags.String("runtime-manifest", "", "owner-only fresh runtime manifest")
		runtimeTree := flags.String("runtime-tree-manifest", "", "private exact venv tree manifest")
		baseTree := flags.String("base-tree-manifest", "", "private exact base CPython tree manifest")
		wheelVerification := flags.String("wheel-verification", "", "private wheel RECORD verification record")
		adapter := flags.String("adapter", "tools/local-context-control-mlx-v2.py", "committed trusted adapter")
		repository := flags.String("repository", ".", "clean merged repository root")
		mainRef := flags.String("main-ref", "origin/main", "reviewed merged main ref")
		trustedCommit := flags.String("trusted-implementation-commit", "", "externally reviewed merged implementation commit")
		trustedTree := flags.String("trusted-implementation-tree", "", "externally reviewed merged implementation tree")
		runOutput := flags.String("run-output", "", "absent private run namespace bound by authorization")
		adapterCheck := flags.String("adapter-check", "", "successful unexpired owner-only adapter-check directory")
		output := flags.String("output", "", "new owner-only authorization directory")
		acknowledgement := flags.String("acknowledgement", "", "exact local-only execution acknowledgement")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *modelPath == "" || *modelManifest == "" || *runtimePython == "" ||
			*runtimeManifest == "" || *runtimeTree == "" || *baseTree == "" || *wheelVerification == "" ||
			*runOutput == "" || *adapterCheck == "" || *output == "" || *trustedCommit == "" || *trustedTree == "" {
			return fmt.Errorf("--input, --model-path, --model-manifest, --runtime-python, --runtime-manifest, --runtime-tree-manifest, --base-tree-manifest, --wheel-verification, --trusted-implementation-commit, --trusted-implementation-tree, --run-output, --adapter-check, and --output are required")
		}
		repoAbs, err := filepath.Abs(*repository)
		if err != nil {
			return err
		}
		adapterAbs, err := filepath.Abs(*adapter)
		if err != nil {
			return err
		}
		_, err = studylocalv2.Authorize(context.Background(), studylocalv2.AuthorizeOptions{
			PackageDirectory: *input, ModelDirectory: *modelPath,
			ModelManifestPath: *modelManifest, RuntimePython: *runtimePython,
			RuntimeManifestPath: *runtimeManifest, AdapterPath: adapterAbs,
			RuntimeTreeManifestPath: *runtimeTree, BaseTreeManifestPath: *baseTree,
			WheelVerificationPath: *wheelVerification,
			RepositoryRoot:        repoAbs, MainRef: *mainRef, RunOutputDirectory: *runOutput,
			TrustedImplementationCommit: *trustedCommit, TrustedImplementationTree: *trustedTree,
			AdapterCheckDirectory: *adapterCheck, OutputDirectory: *output,
			Acknowledgement: *acknowledgement,
		})
		return err
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		input := flags.String("input", "", "validated offline package")
		authorization := flags.String("authorization", "", "owner-only authorization directory")
		runOutput := flags.String("run-output", "", "authorized absent private run namespace")
		modelPath := flags.String("model-path", "", "verified local model directory")
		runtimePython := flags.String("runtime-python", "", "fresh trusted venv bin/python")
		runtimeTree := flags.String("runtime-tree-manifest", "", "private exact venv tree manifest")
		baseTree := flags.String("base-tree-manifest", "", "private exact base CPython tree manifest")
		wheelVerification := flags.String("wheel-verification", "", "private wheel RECORD verification record")
		adapter := flags.String("adapter", "tools/local-context-control-mlx-v2.py", "committed trusted adapter")
		repository := flags.String("repository", ".", "clean merged repository root")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *authorization == "" || *runOutput == "" || *modelPath == "" || *runtimePython == "" ||
			*runtimeTree == "" || *baseTree == "" || *wheelVerification == "" {
			return fmt.Errorf("--input, --authorization, --run-output, --model-path, --runtime-python, --runtime-tree-manifest, --base-tree-manifest, and --wheel-verification are required")
		}
		repoAbs, err := filepath.Abs(*repository)
		if err != nil {
			return err
		}
		adapterAbs, err := filepath.Abs(*adapter)
		if err != nil {
			return err
		}
		summary, err := studylocalv2.Run(context.Background(), studylocalv2.RunOptions{
			PackageDirectory: *input, AuthorizationDirectory: *authorization,
			RunDirectory: *runOutput, ModelDirectory: *modelPath,
			RuntimePython: *runtimePython, RuntimeTreeManifestPath: *runtimeTree,
			BaseTreeManifestPath: *baseTree, WheelVerificationPath: *wheelVerification,
			AdapterPath: adapterAbs, RepositoryRoot: repoAbs,
		})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(summary)
	case "verify-results":
		flags := flag.NewFlagSet("verify-results", flag.ContinueOnError)
		input := flags.String("input", "", "validated offline package")
		runDirectory := flags.String("run", "", "completed private run directory")
		authorization := flags.String("authorization", "", "original consumed authorization directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *authorization == "" || *runDirectory == "" {
			return fmt.Errorf("--input, --authorization, and --run are required")
		}
		return studylocalv2.VerifyResults(*input, *authorization, *runDirectory)
	case "summarize-results":
		flags := flag.NewFlagSet("summarize-results", flag.ContinueOnError)
		input := flags.String("input", "", "validated offline package")
		runDirectory := flags.String("run", "", "completed private run directory")
		authorization := flags.String("authorization", "", "original consumed authorization directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *authorization == "" || *runDirectory == "" {
			return fmt.Errorf("--input, --authorization, and --run are required")
		}
		summary, err := studylocalv2.SummarizeResults(*input, *authorization, *runDirectory)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(summary)
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: distill-local-context-control-v2 <prepare|validate|summarize|model-manifest|runtime-manifest|preflight|adapter-check|authorize|run|verify-results|summarize-results>")
}
