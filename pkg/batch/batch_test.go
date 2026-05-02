package batch

import (
	"fmt"
	"testing"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/pipeline"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

func makeChunks(n int) []types.Chunk {
	chunks := make([]types.Chunk, n)
	for i := range chunks {
		chunks[i] = types.Chunk{
			ID:   fmt.Sprintf("chunk-%d", i),
			Text: "test chunk content for batch processing",
		}
	}
	return chunks
}

func TestSubmitAndGet(t *testing.T) {
	p := NewProcessor(Config{Workers: 1, QueueSize: 10, ResultTTL: time.Minute})
	defer p.Stop()

	job, err := p.Submit(SubmitRequest{
		Chunks:  []types.Chunk{{ID: "a", Text: "hello"}},
		Options: pipeline.Options{},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}

	// Poll until done (max 2s).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := p.Get(job.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status == StatusCompleted || got.Status == StatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got, _ := p.Get(job.ID)
	if got.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s (error: %s)", got.Status, got.Error)
	}
}

func TestResults_NotCompleted(t *testing.T) {
	p := NewProcessor(Config{Workers: 0, QueueSize: 10, ResultTTL: time.Minute})
	// Workers=0 means nothing processes; job stays queued.
	defer p.Stop()

	job, _ := p.Submit(SubmitRequest{
		Chunks:  []types.Chunk{{ID: "a", Text: "hello"}},
		Options: pipeline.Options{},
	})

	_, _, err := p.Results(job.ID)
	if err == nil {
		t.Error("expected error for non-completed job")
	}
}

func TestGet_NotFound(t *testing.T) {
	p := NewProcessor(DefaultConfig())
	defer p.Stop()

	_, err := p.Get("nonexistent")
	if err != ErrJobNotFound {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	p := NewProcessor(Config{Workers: 1, QueueSize: 10, ResultTTL: time.Minute})
	defer p.Stop()

	p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "a", Text: "hello"}}, Options: pipeline.Options{}})
	p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "b", Text: "world"}}, Options: pipeline.Options{}})

	// Wait briefly for processing.
	time.Sleep(200 * time.Millisecond)

	all := p.List("")
	if len(all) < 1 {
		t.Error("expected at least one job in list")
	}
}

func TestQueueFull(t *testing.T) {
	// QueueSize=0 means the channel has no buffer — submit should fail.
	p := NewProcessor(Config{Workers: 0, QueueSize: 1, ResultTTL: time.Minute})
	defer p.Stop()

	// Fill the queue.
	p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "a", Text: "x"}}, Options: pipeline.Options{}})

	// This should fail.
	_, err := p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "b", Text: "y"}}, Options: pipeline.Options{}})
	if err == nil {
		t.Error("expected error when queue is full")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Workers <= 0 {
		t.Error("expected positive workers")
	}
	if cfg.ResultTTL <= 0 {
		t.Error("expected positive ResultTTL")
	}
}

func TestGenerateID_Unique(t *testing.T) {
	ids := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := generateID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
		time.Sleep(time.Nanosecond)
	}
}
