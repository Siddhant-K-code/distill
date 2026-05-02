package cmd

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Siddhant-K-code/distill/pkg/batch"
	"github.com/Siddhant-K-code/distill/pkg/pipeline"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// PipelineRequest is the JSON body for POST /v1/pipeline.
type PipelineRequest struct {
	Chunks  []DedupeChunk   `json:"chunks"`
	Options PipelineOptions `json:"options,omitempty"`
}

// PipelineOptions mirrors pipeline.Options for JSON serialisation.
type PipelineOptions struct {
	Dedup     PipelineDedupOptions     `json:"dedup,omitempty"`
	Compress  PipelineCompressOptions  `json:"compress,omitempty"`
	Summarize PipelineSummarizeOptions `json:"summarize,omitempty"`
}

type PipelineDedupOptions struct {
	Enabled   bool    `json:"enabled"`
	Threshold float64 `json:"threshold,omitempty"`
	Lambda    float64 `json:"lambda,omitempty"`
	TargetK   int     `json:"target_k,omitempty"`
}

type PipelineCompressOptions struct {
	Enabled         bool    `json:"enabled"`
	TargetReduction float64 `json:"target_reduction,omitempty"`
}

type PipelineSummarizeOptions struct {
	Enabled    bool `json:"enabled"`
	MaxTokens  int  `json:"max_tokens,omitempty"`
	KeepRecent int  `json:"keep_recent,omitempty"`
}

// PipelineResponse is the JSON response for POST /v1/pipeline.
type PipelineResponse struct {
	Chunks []DedupeChunk        `json:"chunks"`
	Stats  PipelineStatsPayload `json:"stats"`
}

// PipelineStatsPayload is the serialisable form of pipeline.Stats.
type PipelineStatsPayload struct {
	OriginalTokens int                       `json:"original_tokens"`
	FinalTokens    int                       `json:"final_tokens"`
	TotalReduction float64                   `json:"total_reduction"`
	LatencyMs      float64                   `json:"latency_ms"`
	Stages         map[string]StageStatsPL   `json:"stages"`
}

// StageStatsPL is the serialisable form of pipeline.StageStats.
type StageStatsPL struct {
	Enabled      bool    `json:"enabled"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Reduction    float64 `json:"reduction"`
	LatencyMs    float64 `json:"latency_ms"`
}

// BatchSubmitRequest is the JSON body for POST /v1/batch.
type BatchSubmitRequest struct {
	Chunks  []DedupeChunk   `json:"chunks"`
	Options PipelineOptions `json:"options,omitempty"`
}

// BatchSubmitResponse is the JSON response for POST /v1/batch.
type BatchSubmitResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// BatchStatusResponse is the JSON response for GET /v1/batch/{id}.
type BatchStatusResponse struct {
	JobID       string  `json:"job_id"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	Error       string  `json:"error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	StartedAt   string  `json:"started_at,omitempty"`
	CompletedAt string  `json:"completed_at,omitempty"`
}

// BatchResultsResponse is the JSON response for GET /v1/batch/{id}/results.
type BatchResultsResponse struct {
	JobID  string               `json:"job_id"`
	Status string               `json:"status"`
	Chunks []DedupeChunk        `json:"chunks"`
	Stats  PipelineStatsPayload `json:"stats"`
}

// PipelineAPI holds the pipeline runner and batch processor.
type PipelineAPI struct {
	processor *batch.Processor
}

// NewPipelineAPI creates a PipelineAPI with a default batch processor.
func NewPipelineAPI() *PipelineAPI {
	return &PipelineAPI{
		processor: batch.NewProcessor(batch.DefaultConfig()),
	}
}

// RegisterPipelineRoutes wires up /v1/pipeline and /v1/batch/* routes.
func (a *PipelineAPI) RegisterPipelineRoutes(mux *http.ServeMux, middleware func(string, http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/v1/pipeline", middleware("/v1/pipeline", a.handlePipeline))
	mux.HandleFunc("/v1/batch", middleware("/v1/batch", a.handleBatchSubmit))
	mux.HandleFunc("/v1/batch/", middleware("/v1/batch/", a.handleBatchLookup))
}

// handlePipeline runs the full pipeline synchronously.
func (a *PipelineAPI) handlePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	chunks := dedupeChunksToTypes(req.Chunks)
	opts := pipelineOptsFromRequest(req.Options)

	runner := pipeline.New()
	result, stats, err := runner.Run(r.Context(), chunks, opts)
	if err != nil {
		http.Error(w, "pipeline error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := PipelineResponse{
		Chunks: typesToDedupeChunks(result),
		Stats:  marshalStats(stats),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleBatchSubmit accepts a new batch job.
func (a *PipelineAPI) handleBatchSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	job, err := a.processor.Submit(batch.SubmitRequest{
		Chunks:  dedupeChunksToTypes(req.Chunks),
		Options: pipelineOptsFromRequest(req.Options),
	})
	if err != nil {
		http.Error(w, "submit error: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(BatchSubmitResponse{
		JobID:  job.ID,
		Status: string(job.Status),
	})
}

// handleBatchLookup handles GET /v1/batch/{id} and GET /v1/batch/{id}/results.
func (a *PipelineAPI) handleBatchLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /v1/batch/{id} or /v1/batch/{id}/results
	path := strings.TrimPrefix(r.URL.Path, "/v1/batch/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	if sub == "results" {
		a.handleBatchResults(w, r, id)
		return
	}
	a.handleBatchStatus(w, r, id)
}

func (a *PipelineAPI) handleBatchStatus(w http.ResponseWriter, _ *http.Request, id string) {
	job, err := a.processor.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	resp := BatchStatusResponse{
		JobID:    job.ID,
		Status:   string(job.Status),
		Progress: job.Progress,
		Error:    job.Error,
	}
	if !job.CreatedAt.IsZero() {
		resp.CreatedAt = job.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !job.StartedAt.IsZero() {
		resp.StartedAt = job.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !job.CompletedAt.IsZero() {
		resp.CompletedAt = job.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *PipelineAPI) handleBatchResults(w http.ResponseWriter, _ *http.Request, id string) {
	chunks, stats, err := a.processor.Results(id)
	if err != nil {
		code := http.StatusNotFound
		if err == batch.ErrJobNotFound {
			code = http.StatusNotFound
		} else {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
		return
	}
	resp := BatchResultsResponse{
		JobID:  id,
		Status: string(batch.StatusCompleted),
		Chunks: typesToDedupeChunks(chunks),
		Stats:  marshalStats(stats),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func dedupeChunksToTypes(in []DedupeChunk) []types.Chunk {
	out := make([]types.Chunk, len(in))
	for i, c := range in {
		out[i] = types.Chunk{
			ID:        c.ID,
			Text:      c.Text,
			Embedding: c.Embedding,
			Score:     c.Score,
		}
	}
	return out
}

func typesToDedupeChunks(in []types.Chunk) []DedupeChunk {
	out := make([]DedupeChunk, len(in))
	for i, c := range in {
		out[i] = DedupeChunk{
			ID:        c.ID,
			Text:      c.Text,
			Embedding: c.Embedding,
			Score:     c.Score,
		}
	}
	return out
}

func pipelineOptsFromRequest(o PipelineOptions) pipeline.Options {
	return pipeline.Options{
		DedupEnabled:            o.Dedup.Enabled,
		DedupThreshold:          o.Dedup.Threshold,
		DedupLambda:             o.Dedup.Lambda,
		DedupTargetK:            o.Dedup.TargetK,
		CompressEnabled:         o.Compress.Enabled,
		CompressTargetReduction: o.Compress.TargetReduction,
		SummarizeEnabled:        o.Summarize.Enabled,
		SummarizeMaxTokens:      o.Summarize.MaxTokens,
		SummarizeRecent:         o.Summarize.KeepRecent,
	}
}

func marshalStats(s pipeline.Stats) PipelineStatsPayload {
	stages := make(map[string]StageStatsPL, len(s.Stages))
	for k, v := range s.Stages {
		stages[k] = StageStatsPL{
			Enabled:      v.Enabled,
			InputTokens:  v.InputTokens,
			OutputTokens: v.OutputTokens,
			Reduction:    v.Reduction,
			LatencyMs:    float64(v.Latency.Microseconds()) / 1000.0,
		}
	}
	return PipelineStatsPayload{
		OriginalTokens: s.OriginalTokens,
		FinalTokens:    s.FinalTokens,
		TotalReduction: s.TotalReduction,
		LatencyMs:      float64(s.TotalLatency.Microseconds()) / 1000.0,
		Stages:         stages,
	}
}
