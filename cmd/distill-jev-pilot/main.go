package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "distill-jev-pilot:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: distill-jev-pilot <prepare|validate|summarize>")
	}
	switch args[0] {
	case "prepare":
		flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		outputDirectory := flags.String("output", "", "fresh output directory")
		if err := flags.Parse(args[1:]); err != nil {
			return fmt.Errorf("prepare accepts only --output: %w", err)
		}
		if *outputDirectory == "" || flags.NArg() != 0 {
			return fmt.Errorf("usage: distill-jev-pilot prepare --output <nonexistent-directory>")
		}
		summary, err := studypilot.Prepare(*outputDirectory)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "prepared=%s cases=%d provider_calls=0 final_study_eligible=false\n", *outputDirectory, summary.CaseCount)
		return err
	case "validate":
		if len(args) != 2 {
			return fmt.Errorf("usage: distill-jev-pilot validate <artifact-directory>")
		}
		summary, err := studypilot.Validate(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "valid=true cases=%d provider_calls=0 final_study_eligible=false\n", summary.CaseCount)
		return err
	case "summarize":
		if len(args) != 2 {
			return fmt.Errorf("usage: distill-jev-pilot summarize <artifact-directory>")
		}
		summary, err := studypilot.Summarize(args[1])
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, summary)
		return err
	default:
		return fmt.Errorf("unknown command %q; no provider, model, API, credential, or network options are supported", args[0])
	}
}
