package sse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)
	if sw == nil {
		t.Fatal("expected non-nil Writer from httptest.ResponseRecorder")
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if conn := rec.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, want keep-alive", conn)
	}
}

// nonFlushWriter does not implement http.Flusher.
type nonFlushWriter struct {
	http.ResponseWriter
}

func TestNewWriter_NoFlusher(t *testing.T) {
	sw := NewWriter(&nonFlushWriter{})
	if sw != nil {
		t.Error("expected nil Writer when ResponseWriter does not support Flusher")
	}
}

func TestSendProgress(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)

	if err := sw.SendProgress(StageClustering, 0.5); err != nil {
		t.Fatalf("SendProgress: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: progress") {
		t.Error("missing 'event: progress' line")
	}

	// Extract data line
	data := extractData(t, body, "progress")
	var evt ProgressEvent
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		t.Fatalf("unmarshal progress event: %v", err)
	}
	if evt.Stage != StageClustering {
		t.Errorf("stage = %q, want %q", evt.Stage, StageClustering)
	}
	if evt.Progress != 0.5 {
		t.Errorf("progress = %v, want 0.5", evt.Progress)
	}
	if evt.Stats != nil {
		t.Error("expected nil stats for basic progress event")
	}
}

func TestSendProgressWithStats(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)

	stats := map[string]int{"clusters": 5}
	if err := sw.SendProgressWithStats(StageSelection, 1.0, stats); err != nil {
		t.Fatalf("SendProgressWithStats: %v", err)
	}

	data := extractData(t, rec.Body.String(), "progress")
	var evt ProgressEvent
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if evt.Stats == nil {
		t.Fatal("expected non-nil stats")
	}

	var parsed map[string]int
	if err := json.Unmarshal(*evt.Stats, &parsed); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if parsed["clusters"] != 5 {
		t.Errorf("clusters = %d, want 5", parsed["clusters"])
	}
}

func TestSendComplete(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)

	chunks := []map[string]string{{"id": "a", "text": "hello"}}
	stats := map[string]int{"input_count": 3, "output_count": 1}

	if err := sw.SendComplete(chunks, stats); err != nil {
		t.Fatalf("SendComplete: %v", err)
	}

	data := extractData(t, rec.Body.String(), "complete")
	var evt CompleteEvent
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var parsedChunks []map[string]string
	if err := json.Unmarshal(evt.Chunks, &parsedChunks); err != nil {
		t.Fatalf("unmarshal chunks: %v", err)
	}
	if len(parsedChunks) != 1 || parsedChunks[0]["id"] != "a" {
		t.Errorf("unexpected chunks: %v", parsedChunks)
	}
}

func TestSendError(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)

	if err := sw.SendError(StageEmbedding, "API key missing"); err != nil {
		t.Fatalf("SendError: %v", err)
	}

	data := extractData(t, rec.Body.String(), "error")
	var evt ErrorEvent
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if evt.Error != "API key missing" {
		t.Errorf("error = %q, want %q", evt.Error, "API key missing")
	}
	if evt.Stage != StageEmbedding {
		t.Errorf("stage = %q, want %q", evt.Stage, StageEmbedding)
	}
}

func TestMultipleEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewWriter(rec)

	_ = sw.SendProgress(StageEmbedding, 0)
	_ = sw.SendProgress(StageEmbedding, 1.0)
	_ = sw.SendProgress(StageClustering, 0)
	_ = sw.SendProgress(StageClustering, 1.0)
	_ = sw.SendComplete([]string{}, map[string]int{})

	body := rec.Body.String()
	progressCount := strings.Count(body, "event: progress")
	if progressCount != 4 {
		t.Errorf("progress events = %d, want 4", progressCount)
	}
	completeCount := strings.Count(body, "event: complete")
	if completeCount != 1 {
		t.Errorf("complete events = %d, want 1", completeCount)
	}
}

func TestStageTimer(t *testing.T) {
	timer := NewStageTimer(StageClustering)
	time.Sleep(10 * time.Millisecond)

	if timer.Stage != StageClustering {
		t.Errorf("stage = %q, want %q", timer.Stage, StageClustering)
	}
	if timer.Elapsed() < 10*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= 10ms", timer.Elapsed())
	}
	if timer.ElapsedMs() < 10 {
		t.Errorf("elapsed ms = %d, expected >= 10", timer.ElapsedMs())
	}
}

func TestStageConstants(t *testing.T) {
	stages := []Stage{StageEmbedding, StageClustering, StageSelection, StageCompress, StageMMR}
	seen := make(map[Stage]bool)
	for _, s := range stages {
		if s == "" {
			t.Error("empty stage constant")
		}
		if seen[s] {
			t.Errorf("duplicate stage: %s", s)
		}
		seen[s] = true
	}
}

// extractData finds the data line for the first occurrence of the given event type.
func extractData(t *testing.T, body, eventType string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == "event: "+eventType {
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
				return strings.TrimPrefix(lines[i+1], "data: ")
			}
		}
	}
	t.Fatalf("no data found for event type %q in:\n%s", eventType, body)
	return ""
}
