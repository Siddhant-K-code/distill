package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	pc "github.com/Siddhant-K-code/distill/pkg/pinecone"
	"github.com/Siddhant-K-code/distill/pkg/types"
)

// Config holds ingestion pipeline configuration.
type Config struct {
	// BatchSize is the number of vectors per batch. Pinecone optimal: 100
	BatchSize int

	// Workers is the number of concurrent upload workers.
	Workers int

	// ChannelBuffer is the buffer size for internal channels.
	ChannelBuffer int
}

// DefaultConfig returns sensible defaults for ingestion.
func DefaultConfig() Config {
	return Config{
		BatchSize:     100,
		Workers:       runtime.NumCPU() * 2,
		ChannelBuffer: 1000,
	}
}

// Pipeline orchestrates the ingestion of vectors to Pinecone.
type Pipeline struct {
	cfg    Config
	client *pc.Client
	stats  *Stats
}

// Stats tracks ingestion metrics.
type Stats struct {
	TotalVectors     int64
	UploadedVectors  int64
	FailedVectors    int64
	BatchesProcessed int64
	StartTime        time.Time
	EndTime          time.Time
}

// Duration returns the total processing duration.
func (s *Stats) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// VectorsPerSecond returns the throughput.
func (s *Stats) VectorsPerSecond() float64 {
	d := s.Duration().Seconds()
	if d == 0 {
		return 0
	}
	return float64(s.UploadedVectors) / d
}

// NewPipeline creates a new ingestion pipeline.
func NewPipeline(client *pc.Client, cfg Config) *Pipeline {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU() * 2
	}
	if cfg.ChannelBuffer <= 0 {
		cfg.ChannelBuffer = 1000
	}

	return &Pipeline{
		cfg:    cfg,
		client: client,
		stats:  &Stats{},
	}
}

// ProgressCallback is called periodically with current stats.
type ProgressCallback func(stats Stats)

// IngestFile reads vectors from a JSONL file and uploads them to Pinecone.
func (p *Pipeline) IngestFile(ctx context.Context, filePath string, progress ProgressCallback) (*Stats, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return p.IngestReader(ctx, file, progress)
}

// IngestReader reads vectors from an io.Reader and uploads them to Pinecone.
func (p *Pipeline) IngestReader(ctx context.Context, r io.Reader, progress ProgressCallback) (*Stats, error) {
	p.stats = &Stats{StartTime: time.Now()}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channels for pipeline stages
	vectorCh := make(chan types.Vector, p.cfg.ChannelBuffer)
	batchCh := make(chan []types.Vector, p.cfg.Workers*2)
	errCh := make(chan error, 1)

	var wg sync.WaitGroup

	// Stage 1: Reader - parse JSONL and emit vectors
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(vectorCh)

		if err := p.readVectors(ctx, r, vectorCh); err != nil {
			select {
			case errCh <- err:
			default:
			}
			cancel()
		}
	}()

	// Stage 2: Batcher - accumulate vectors into batches
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(batchCh)

		p.batchVectors(ctx, vectorCh, batchCh)
	}()

	// Stage 3: Workers - upload batches concurrently
	var workerWg sync.WaitGroup
	for i := 0; i < p.cfg.Workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			p.uploadWorker(ctx, batchCh)
		}()
	}

	// Progress reporter
	if progress != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					progress(p.GetStats())
				}
			}
		}()
	}

	// Wait for workers to finish
	workerWg.Wait()
	wg.Wait()

	p.stats.EndTime = time.Now()

	// Check for errors
	select {
	case err := <-errCh:
		return p.GetStatsPtr(), err
	default:
	}

	return p.GetStatsPtr(), nil
}

// IngestVectors uploads pre-loaded vectors to Pinecone.
func (p *Pipeline) IngestVectors(ctx context.Context, vectors []types.Vector, progress ProgressCallback) (*Stats, error) {
	p.stats = &Stats{
		StartTime:    time.Now(),
		TotalVectors: int64(len(vectors)),
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchCh := make(chan []types.Vector, p.cfg.Workers*2)

	// Batcher goroutine
	go func() {
		defer close(batchCh)

		batch := make([]types.Vector, 0, p.cfg.BatchSize)
		for i, v := range vectors {
			select {
			case <-ctx.Done():
				return
			default:
			}

			batch = append(batch, v)
			if len(batch) >= p.cfg.BatchSize || i == len(vectors)-1 {
				batchCopy := make([]types.Vector, len(batch))
				copy(batchCopy, batch)
				batchCh <- batchCopy
				batch = batch[:0]
			}
		}
	}()

	// Workers
	var wg sync.WaitGroup
	for i := 0; i < p.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.uploadWorker(ctx, batchCh)
		}()
	}

	// Progress reporter
	if progress != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					progress(p.GetStats())
				}
			}
		}()
	}

	wg.Wait()
	p.stats.EndTime = time.Now()

	return p.GetStatsPtr(), nil
}

// readVectors parses JSONL and sends vectors to the channel.
func (p *Pipeline) readVectors(ctx context.Context, r io.Reader, out chan<- types.Vector) error {
	scanner := bufio.NewScanner(r)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var v jsonVector
		if err := json.Unmarshal(line, &v); err != nil {
			// Skip malformed lines
			continue
		}

		vec := types.Vector{
			ID:       v.ID,
			Values:   v.Values,
			Metadata: v.Metadata,
		}

		atomic.AddInt64(&p.stats.TotalVectors, 1)

		select {
		case out <- vec:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return scanner.Err()
}

// jsonVector is the expected JSONL format.
type jsonVector struct {
	ID       string                 `json:"id"`
	Values   []float32              `json:"values"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// batchVectors accumulates vectors into batches.
func (p *Pipeline) batchVectors(ctx context.Context, in <-chan types.Vector, out chan<- []types.Vector) {
	batch := make([]types.Vector, 0, p.cfg.BatchSize)

	for {
		select {
		case <-ctx.Done():
			// Flush remaining
			if len(batch) > 0 {
				out <- batch
			}
			return

		case v, ok := <-in:
			if !ok {
				// Channel closed, flush remaining
				if len(batch) > 0 {
					out <- batch
				}
				return
			}

			batch = append(batch, v)
			if len(batch) >= p.cfg.BatchSize {
				out <- batch
				batch = make([]types.Vector, 0, p.cfg.BatchSize)
			}
		}
	}
}

// uploadWorker processes batches from the channel.
func (p *Pipeline) uploadWorker(ctx context.Context, batches <-chan []types.Vector) {
	for batch := range batches {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := p.client.UpsertBatch(ctx, batch)
		if err != nil {
			atomic.AddInt64(&p.stats.FailedVectors, int64(len(batch)))
		} else {
			atomic.AddInt64(&p.stats.UploadedVectors, int64(len(batch)))
		}
		atomic.AddInt64(&p.stats.BatchesProcessed, 1)
	}
}

// GetStats returns current statistics.
func (p *Pipeline) GetStats() Stats {
	return Stats{
		TotalVectors:     atomic.LoadInt64(&p.stats.TotalVectors),
		UploadedVectors:  atomic.LoadInt64(&p.stats.UploadedVectors),
		FailedVectors:    atomic.LoadInt64(&p.stats.FailedVectors),
		BatchesProcessed: atomic.LoadInt64(&p.stats.BatchesProcessed),
		StartTime:        p.stats.StartTime,
		EndTime:          p.stats.EndTime,
	}
}

// GetStatsPtr returns a pointer to current statistics.
func (p *Pipeline) GetStatsPtr() *Stats {
	s := p.GetStats()
	return &s
}
