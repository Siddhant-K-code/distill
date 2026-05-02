// Package metrics provides Prometheus instrumentation for Distill.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric collectors for Distill.
type Metrics struct {
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	ChunksProcessed  *prometheus.CounterVec
	ReductionRatio   *prometheus.HistogramVec
	ActiveRequests   prometheus.Gauge
	ClustersFormed   *prometheus.CounterVec

	// Cache cost accounting (issue #52).
	// These track Anthropic API usage fields returned in every response.
	CacheCreationTokens *prometheus.CounterVec
	CacheReadTokens     *prometheus.CounterVec
	UncachedInputTokens *prometheus.CounterVec
	CacheHitRate        prometheus.Gauge
	CacheWriteEfficiency prometheus.Gauge

	// Cache boundary metrics (issue #51).
	CacheBoundaryPosition  *prometheus.GaugeVec
	CacheBoundaryAdvances  *prometheus.CounterVec
	CacheBoundaryRetreats  *prometheus.CounterVec
	CacheEstimatedSavings  *prometheus.CounterVec

	registry *prometheus.Registry
}

// New creates and registers all Distill metrics.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	// Include default Go and process collectors
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_requests_total",
				Help: "Total HTTP requests by endpoint and status code.",
			},
			[]string{"endpoint", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "distill_request_duration_seconds",
				Help:    "HTTP request latency distribution.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"endpoint"},
		),
		ChunksProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_chunks_processed_total",
				Help: "Total chunks processed by direction (input/output).",
			},
			[]string{"direction"},
		),
		ReductionRatio: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "distill_reduction_ratio",
				Help:    "Chunk reduction ratio per request (0=no reduction, 1=all removed).",
				Buckets: []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"endpoint"},
		),
		ActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "distill_active_requests",
				Help: "Number of requests currently being processed.",
			},
		),
		ClustersFormed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_clusters_formed_total",
				Help: "Total clusters formed during deduplication.",
			},
			[]string{"endpoint"},
		),

		// Cache cost accounting.
		CacheCreationTokens: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_cache_creation_tokens_total",
				Help: "Tokens written to Anthropic prompt cache (charged at 1.25x input price).",
			},
			[]string{"session_id"},
		),
		CacheReadTokens: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_cache_read_tokens_total",
				Help: "Tokens read from Anthropic prompt cache (charged at 0.10x input price).",
			},
			[]string{"session_id"},
		),
		UncachedInputTokens: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_uncached_input_tokens_total",
				Help: "Input tokens not served from cache (charged at 1.00x input price).",
			},
			[]string{"session_id"},
		),
		CacheHitRate: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "distill_cache_hit_rate",
				Help: "Rolling cache hit rate: cache_read / (cache_read + cache_creation + input).",
			},
		),
		CacheWriteEfficiency: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "distill_cache_write_efficiency",
				Help: "Cache read/write ratio. Values < 1.0 indicate writes that expire before being read.",
			},
		),

		// Cache boundary metrics.
		CacheBoundaryPosition: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "distill_cache_boundary_position_tokens",
				Help: "Current cache boundary position in tokens for a session.",
			},
			[]string{"session_id"},
		),
		CacheBoundaryAdvances: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_cache_boundary_advances_total",
				Help: "Number of times the cache boundary advanced (more content became stable).",
			},
			[]string{"session_id"},
		),
		CacheBoundaryRetreats: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_cache_boundary_retreats_total",
				Help: "Number of times the cache boundary retreated (content changed or was evicted).",
			},
			[]string{"session_id"},
		),
		CacheEstimatedSavings: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "distill_cache_estimated_savings_tokens_total",
				Help: "Estimated tokens saved by prompt caching across all sessions.",
			},
			[]string{"session_id"},
		),

		registry: reg,
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.ChunksProcessed,
		m.ReductionRatio,
		m.ActiveRequests,
		m.ClustersFormed,
		m.CacheCreationTokens,
		m.CacheReadTokens,
		m.UncachedInputTokens,
		m.CacheHitRate,
		m.CacheWriteEfficiency,
		m.CacheBoundaryPosition,
		m.CacheBoundaryAdvances,
		m.CacheBoundaryRetreats,
		m.CacheEstimatedSavings,
	)

	return m
}

// Handler returns an http.Handler that serves the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordRequest records a completed request's metrics.
func (m *Metrics) RecordRequest(endpoint string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	m.RequestsTotal.WithLabelValues(endpoint, status).Inc()
	m.RequestDuration.WithLabelValues(endpoint).Observe(duration.Seconds())
}

// RecordDedup records deduplication-specific metrics.
func (m *Metrics) RecordDedup(endpoint string, inputCount, outputCount, clusterCount int) {
	m.ChunksProcessed.WithLabelValues("input").Add(float64(inputCount))
	m.ChunksProcessed.WithLabelValues("output").Add(float64(outputCount))
	m.ClustersFormed.WithLabelValues(endpoint).Add(float64(clusterCount))

	if inputCount > 0 {
		ratio := 1.0 - float64(outputCount)/float64(inputCount)
		m.ReductionRatio.WithLabelValues(endpoint).Observe(ratio)
	}
}

// UsageRecord holds the token counts returned by the Anthropic API in the
// usage block of every response. Pass this to RecordCacheUsage after each
// API call to keep the cache cost metrics up to date.
type UsageRecord struct {
	// SessionID is optional; use "" for non-session requests.
	SessionID string

	InputTokens              int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	OutputTokens             int
}

// RecordCacheUsage records Anthropic API usage fields and updates the derived
// cache hit rate and write efficiency gauges.
func (m *Metrics) RecordCacheUsage(u UsageRecord) {
	sid := u.SessionID
	if sid == "" {
		sid = "default"
	}

	if u.CacheCreationInputTokens > 0 {
		m.CacheCreationTokens.WithLabelValues(sid).Add(float64(u.CacheCreationInputTokens))
	}
	if u.CacheReadInputTokens > 0 {
		m.CacheReadTokens.WithLabelValues(sid).Add(float64(u.CacheReadInputTokens))
	}
	if u.InputTokens > 0 {
		m.UncachedInputTokens.WithLabelValues(sid).Add(float64(u.InputTokens))
	}

	// Update derived gauges using the values from this single request.
	total := float64(u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens)
	if total > 0 {
		hitRate := float64(u.CacheReadInputTokens) / total
		m.CacheHitRate.Set(hitRate)
	}

	if u.CacheCreationInputTokens > 0 {
		efficiency := float64(u.CacheReadInputTokens) / float64(u.CacheCreationInputTokens)
		m.CacheWriteEfficiency.Set(efficiency)
	}
}

// RecordCacheBoundary records a cache boundary evaluation result for a session.
func (m *Metrics) RecordCacheBoundary(sessionID string, boundaryTokens int, advanced, retreated bool) {
	if sessionID == "" {
		sessionID = "default"
	}
	m.CacheBoundaryPosition.WithLabelValues(sessionID).Set(float64(boundaryTokens))
	if advanced {
		m.CacheBoundaryAdvances.WithLabelValues(sessionID).Inc()
	}
	if retreated {
		m.CacheBoundaryRetreats.WithLabelValues(sessionID).Inc()
	}
}

// Middleware returns an HTTP middleware that instruments requests.
func (m *Metrics) Middleware(endpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.ActiveRequests.Inc()
		defer m.ActiveRequests.Dec()

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(rw, r)

		m.RecordRequest(endpoint, rw.statusCode, time.Since(start))
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
