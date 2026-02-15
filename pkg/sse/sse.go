// Package sse provides Server-Sent Events support for streaming
// pipeline progress to clients.
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Stage identifies a pipeline processing stage.
type Stage string

const (
	StageEmbedding  Stage = "embedding"
	StageClustering Stage = "clustering"
	StageSelection  Stage = "selection"
	StageCompress   Stage = "compress"
	StageMMR        Stage = "mmr"
)

// ProgressEvent is sent during processing to report stage progress.
type ProgressEvent struct {
	Stage    Stage            `json:"stage"`
	Progress float64          `json:"progress"`
	Stats    *json.RawMessage `json:"stats,omitempty"`
}

// CompleteEvent is sent when processing finishes.
type CompleteEvent struct {
	Chunks json.RawMessage `json:"chunks"`
	Stats  json.RawMessage `json:"stats"`
}

// ErrorEvent is sent when processing fails.
type ErrorEvent struct {
	Error string `json:"error"`
	Stage Stage  `json:"stage,omitempty"`
}

// Writer wraps an http.ResponseWriter for SSE output.
// It sets the required headers and provides methods to send typed events.
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewWriter prepares the response for SSE streaming.
// Returns nil if the ResponseWriter does not support flushing.
func NewWriter(w http.ResponseWriter) *Writer {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &Writer{w: w, flusher: flusher}
}

// SendProgress emits a progress event for the given stage.
func (s *Writer) SendProgress(stage Stage, progress float64) error {
	evt := ProgressEvent{Stage: stage, Progress: progress}
	return s.sendEvent("progress", evt)
}

// SendProgressWithStats emits a progress event that includes stage-level stats.
func (s *Writer) SendProgressWithStats(stage Stage, progress float64, stats interface{}) error {
	raw, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}
	rawMsg := json.RawMessage(raw)
	evt := ProgressEvent{Stage: stage, Progress: progress, Stats: &rawMsg}
	return s.sendEvent("progress", evt)
}

// SendComplete emits the final complete event with chunks and stats.
func (s *Writer) SendComplete(chunks interface{}, stats interface{}) error {
	chunksJSON, err := json.Marshal(chunks)
	if err != nil {
		return fmt.Errorf("marshal chunks: %w", err)
	}
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}
	evt := CompleteEvent{
		Chunks: json.RawMessage(chunksJSON),
		Stats:  json.RawMessage(statsJSON),
	}
	return s.sendEvent("complete", evt)
}

// SendError emits an error event.
func (s *Writer) SendError(stage Stage, errMsg string) error {
	evt := ErrorEvent{Error: errMsg, Stage: stage}
	return s.sendEvent("error", evt)
}

// sendEvent writes a single SSE event and flushes.
func (s *Writer) sendEvent(eventType string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, payload)
	if err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	s.flusher.Flush()
	return nil
}

// StageTimer tracks elapsed time for a pipeline stage.
type StageTimer struct {
	Stage   Stage
	started time.Time
}

// NewStageTimer starts timing a stage.
func NewStageTimer(stage Stage) *StageTimer {
	return &StageTimer{Stage: stage, started: time.Now()}
}

// Elapsed returns the duration since the timer started.
func (t *StageTimer) Elapsed() time.Duration {
	return time.Since(t.started)
}

// ElapsedMs returns elapsed milliseconds.
func (t *StageTimer) ElapsedMs() int64 {
	return t.Elapsed().Milliseconds()
}
