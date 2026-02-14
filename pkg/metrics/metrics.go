// Package metrics provides Prometheus instrumentation for Distill.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

	registry *prometheus.Registry
}

// New creates and registers all Distill metrics.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	// Include default Go and process collectors
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

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
		registry: reg,
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.ChunksProcessed,
		m.ReductionRatio,
		m.ActiveRequests,
		m.ClustersFormed,
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
