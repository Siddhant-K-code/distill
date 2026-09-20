package jevpilot

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

func loadPilot(directory string) (pilotData, studypilot.Summary, error) {
	summary, err := studypilot.Validate(directory)
	if err != nil {
		return pilotData{}, studypilot.Summary{}, err
	}
	names := []string{"cases.jsonl", "requests.jsonl", "schedule.jsonl", "pilot-manifest.json", "SHA256SUMS"}
	files := make(map[string][]byte, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return pilotData{}, studypilot.Summary{}, fmt.Errorf("read %s: %w", name, err)
		}
		files[name] = data
	}
	cases, err := parseJSONL[studypilot.CaseRecord](files["cases.jsonl"])
	if err != nil {
		return pilotData{}, studypilot.Summary{}, fmt.Errorf("parse cases: %w", err)
	}
	requests, err := parseJSONL[studypilot.RequestRecord](files["requests.jsonl"])
	if err != nil {
		return pilotData{}, studypilot.Summary{}, fmt.Errorf("parse requests: %w", err)
	}
	schedules, err := parseJSONL[studypilot.ScheduleRecord](files["schedule.jsonl"])
	if err != nil {
		return pilotData{}, studypilot.Summary{}, fmt.Errorf("parse schedule: %w", err)
	}
	return pilotData{Cases: cases, Requests: requests, Schedules: schedules, Files: files}, summary, nil
}

func indexPilot(data pilotData) (map[string]studypilot.CaseRecord, map[string]studypilot.RequestRecord, map[string]studypilot.ScheduleRecord, error) {
	cases := make(map[string]studypilot.CaseRecord, len(data.Cases))
	requests := make(map[string]studypilot.RequestRecord, len(data.Requests))
	schedules := make(map[string]studypilot.ScheduleRecord, len(data.Schedules))
	for _, record := range data.Cases {
		if _, exists := cases[record.CaseID]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate case %q", record.CaseID)
		}
		cases[record.CaseID] = record
	}
	for _, record := range data.Requests {
		if _, exists := requests[record.RequestID]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate request %q", record.RequestID)
		}
		requests[record.RequestID] = record
	}
	for _, record := range data.Schedules {
		if _, exists := schedules[record.ScheduledCallID]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate scheduled call %q", record.ScheduledCallID)
		}
		schedules[record.ScheduledCallID] = record
	}
	return cases, requests, schedules, nil
}
