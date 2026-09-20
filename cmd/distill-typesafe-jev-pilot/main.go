package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Siddhant-K-code/distill/internal/jevpilot"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "distill-typesafe-jev-pilot:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: distill-typesafe-jev-pilot <authorize|run|validate>")
	}
	switch args[0] {
	case "authorize":
		flags := flag.NewFlagSet("authorize", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		pilotDirectory := flags.String("pilot", "", "validated offline pilot directory")
		outputDirectory := flags.String("output", "", "fresh execution directory")
		modelList := flags.String("model-list", "", "saved authenticated GET /v1/models response")
		modelListRequestID := flags.String("model-list-request-id", "", "saved model-list provider request id")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *pilotDirectory == "" || *outputDirectory == "" || *modelList == "" ||
			*modelListRequestID == "" || flags.NArg() != 0 {
			return fmt.Errorf("usage: distill-typesafe-jev-pilot authorize --pilot <dir> --output <fresh-dir> --model-list <json> --model-list-request-id <file>")
		}
		authorization, err := jevpilot.Prepare(jevpilot.PrepareOptions{
			PilotDirectory: *pilotDirectory, OutputDirectory: *outputDirectory,
			ModelListPath: *modelList, ModelListRequestIDPath: *modelListRequestID,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output,
			"authorized=true excluded=true final_study_eligible=false model=%s calls=%d cap_usd=%s max_scheduled_cost_usd=%s schedule_sha256=%s\n",
			authorization.ModelID, authorization.ScheduledCalls, authorization.AuthorizedBudgetUSD,
			authorization.MaxScheduledCostUSD, authorization.ScheduleSHA256)
		return err
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		pilotDirectory := flags.String("pilot", "", "validated offline pilot directory")
		runDirectory := flags.String("run-dir", "", "prepared execution directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *pilotDirectory == "" || *runDirectory == "" || flags.NArg() != 0 {
			return fmt.Errorf("usage: distill-typesafe-jev-pilot run --pilot <dir> --run-dir <dir>")
		}
		key := os.Getenv("TYPESAFE_API_KEY")
		summary, err := jevpilot.Run(context.Background(), jevpilot.RunOptions{
			PilotDirectory: *pilotDirectory, RunDirectory: *runDirectory, APIKey: key,
		})
		if err != nil {
			return err
		}
		return writeSummary(output, summary)
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		pilotDirectory := flags.String("pilot", "", "validated offline pilot directory")
		runDirectory := flags.String("run-dir", "", "execution directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *pilotDirectory == "" || *runDirectory == "" || flags.NArg() != 0 {
			return fmt.Errorf("usage: distill-typesafe-jev-pilot validate --pilot <dir> --run-dir <dir>")
		}
		summary, err := jevpilot.ValidateRun(*pilotDirectory, *runDirectory)
		if err != nil {
			return err
		}
		return writeSummary(output, summary)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func writeSummary(output io.Writer, summary jevpilot.RunSummary) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "%s\n", data)
	return err
}
