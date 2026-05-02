// Package batch provides async batch processing for large deduplication workloads.
// Jobs are queued in-memory, processed by a background worker pool, and results
// are retained for a configurable TTL.
package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/pipeline"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Status represents the lifecycle state of a batch job.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Job holds the input, state, and result of a single batch job.
type Job struct {
	ID          string
	Status      Status
	Chunks      []types.Chunk
	Options     pipeline.Options
	Result      []types.Chunk
	Stats       pipeline.Stats
	Error       string
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	Progress    float64 // 0–1
}

// SubmitRequest is the input for submitting a new batch job.
type SubmitRequest struct {
	Chunks  []types.Chunk
	Options pipeline.Options
}

// ErrJobNotFound is returned when a job ID does not exist.
var ErrJobNotFound = errors.New("job not found")

// ErrResultExpired is returned when a job's result has been evicted.
var ErrResultExpired = errors.New("job result has expired")

// Processor manages the job queue, worker pool, and result store.
type Processor struct {
	mu         sync.RWMutex
	jobs       map[string]*Job
	queue      chan string
	resultTTL  time.Duration
	runner     *pipeline.Runner
	wg         sync.WaitGroup
	cancelFunc context.CancelFunc
}

// Config controls processor behaviour.
type Config struct {
	// Workers is the number of concurrent processing goroutines. Default: 4.
	Workers int
	// QueueSize is the maximum number of queued jobs. Default: 1000.
	QueueSize int
	// ResultTTL is how long completed job results are retained. Default: 24h.
	ResultTTL time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Workers:   4,
		QueueSize: 1000,
		ResultTTL: 24 * time.Hour,
	}
}

// NewProcessor creates and starts a Processor with the given config.
func NewProcessor(cfg Config) *Processor {
	if cfg.Workers < 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.ResultTTL <= 0 {
		cfg.ResultTTL = 24 * time.Hour
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Processor{
		jobs:       make(map[string]*Job),
		queue:      make(chan string, cfg.QueueSize),
		resultTTL:  cfg.ResultTTL,
		runner:     pipeline.New(),
		cancelFunc: cancel,
	}

	for i := 0; i < cfg.Workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}

	go p.evictLoop(ctx)

	return p
}

// Submit enqueues a new batch job and returns its ID.
func (p *Processor) Submit(req SubmitRequest) (*Job, error) {
	id := generateID()
	job := &Job{
		ID:        id,
		Status:    StatusQueued,
		Chunks:    req.Chunks,
		Options:   req.Options,
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.jobs[id] = job
	p.mu.Unlock()

	select {
	case p.queue <- id:
	default:
		p.mu.Lock()
		delete(p.jobs, id)
		p.mu.Unlock()
		return nil, fmt.Errorf("job queue is full")
	}

	return job, nil
}

// Get returns the current state of a job.
func (p *Processor) Get(id string) (*Job, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.jobs[id]
	if !ok {
		return nil, ErrJobNotFound
	}
	// Return a copy to avoid data races.
	cp := *job
	return &cp, nil
}

// Results returns the deduplicated chunks for a completed job.
func (p *Processor) Results(id string) ([]types.Chunk, pipeline.Stats, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.jobs[id]
	if !ok {
		return nil, pipeline.Stats{}, ErrJobNotFound
	}
	if job.Status != StatusCompleted {
		return nil, pipeline.Stats{}, fmt.Errorf("job %s is %s, not completed", id, job.Status)
	}
	return job.Result, job.Stats, nil
}

// List returns all jobs, optionally filtered by status ("" = all).
func (p *Processor) List(status Status) []*Job {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []*Job
	for _, j := range p.jobs {
		if status == "" || j.Status == status {
			cp := *j
			out = append(out, &cp)
		}
	}
	return out
}

// Stop gracefully shuts down the processor, waiting for in-flight jobs.
func (p *Processor) Stop() {
	p.cancelFunc()
	close(p.queue)
	p.wg.Wait()
}

// worker processes jobs from the queue.
func (p *Processor) worker(ctx context.Context) {
	defer p.wg.Done()
	for id := range p.queue {
		if ctx.Err() != nil {
			return
		}
		p.process(ctx, id)
	}
}

// process runs the pipeline for a single job.
func (p *Processor) process(ctx context.Context, id string) {
	p.mu.Lock()
	job, ok := p.jobs[id]
	if !ok {
		p.mu.Unlock()
		return
	}
	job.Status = StatusProcessing
	job.StartedAt = time.Now()
	job.Progress = 0.0
	p.mu.Unlock()

	result, stats, err := p.runner.Run(ctx, job.Chunks, job.Options)

	p.mu.Lock()
	defer p.mu.Unlock()
	job, ok = p.jobs[id]
	if !ok {
		return
	}
	job.CompletedAt = time.Now()
	job.Progress = 1.0
	if err != nil {
		job.Status = StatusFailed
		job.Error = err.Error()
	} else {
		job.Status = StatusCompleted
		job.Result = result
		job.Stats = stats
	}
}

// evictLoop removes completed/failed jobs whose results have expired.
func (p *Processor) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.evict()
		}
	}
}

func (p *Processor) evict() {
	cutoff := time.Now().Add(-p.resultTTL)
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, job := range p.jobs {
		if (job.Status == StatusCompleted || job.Status == StatusFailed) &&
			job.CompletedAt.Before(cutoff) {
			delete(p.jobs, id)
		}
	}
}

// generateID returns a simple time-based unique ID.
func generateID() string {
	return fmt.Sprintf("batch_%d", time.Now().UnixNano())
}
