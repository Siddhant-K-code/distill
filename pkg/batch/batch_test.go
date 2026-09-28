package batch

import (
	"sync"
	"testing"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/pipeline"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

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

	_, _ = p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "a", Text: "hello"}}, Options: pipeline.Options{}})
	_, _ = p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "b", Text: "world"}}, Options: pipeline.Options{}})

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
	_, _ = p.Submit(SubmitRequest{Chunks: []types.Chunk{{ID: "a", Text: "x"}}, Options: pipeline.Options{}})

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
	ids := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		id := generateID()
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestGenerateID_ConcurrentUnique(t *testing.T) {
	const goroutines = 100
	const perGoroutine = 1000

	ids := make(chan string, goroutines*perGoroutine)
	var workers sync.WaitGroup
	for range goroutines {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range perGoroutine {
				ids <- generateID()
			}
		}()
	}
	workers.Wait()
	close(ids)

	seen := make(map[string]struct{}, goroutines*perGoroutine)
	for id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate concurrent ID generated: %s", id)
		}
		seen[id] = struct{}{}
	}
}
