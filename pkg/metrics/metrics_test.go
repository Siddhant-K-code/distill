package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.registry == nil {
		t.Fatal("registry is nil")
	}
}

func TestRecordRequest(t *testing.T) {
	m := New()
	m.RecordRequest("/v1/dedupe", 200, 50*time.Millisecond)
	m.RecordRequest("/v1/dedupe", 200, 100*time.Millisecond)
	m.RecordRequest("/v1/dedupe", 400, 5*time.Millisecond)

	// Check counter
	val := counterValue(t, m.RequestsTotal, "endpoint", "/v1/dedupe", "status", "200")
	if val != 2 {
		t.Errorf("expected 2 requests with status 200, got %f", val)
	}

	val = counterValue(t, m.RequestsTotal, "endpoint", "/v1/dedupe", "status", "400")
	if val != 1 {
		t.Errorf("expected 1 request with status 400, got %f", val)
	}
}

func TestRecordDedup(t *testing.T) {
	m := New()
	m.RecordDedup("/v1/dedupe", 10, 6, 6)

	inputVal := counterValue(t, m.ChunksProcessed, "direction", "input")
	if inputVal != 10 {
		t.Errorf("expected 10 input chunks, got %f", inputVal)
	}

	outputVal := counterValue(t, m.ChunksProcessed, "direction", "output")
	if outputVal != 6 {
		t.Errorf("expected 6 output chunks, got %f", outputVal)
	}

	clusterVal := counterValue(t, m.ClustersFormed, "endpoint", "/v1/dedupe")
	if clusterVal != 6 {
		t.Errorf("expected 6 clusters, got %f", clusterVal)
	}
}

func TestRecordDedup_ZeroInput(t *testing.T) {
	m := New()
	// Should not panic on zero input
	m.RecordDedup("/v1/dedupe", 0, 0, 0)
}

func TestMiddleware(t *testing.T) {
	m := New()

	handler := m.Middleware("/v1/dedupe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/dedupe", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	val := counterValue(t, m.RequestsTotal, "endpoint", "/v1/dedupe", "status", "200")
	if val != 1 {
		t.Errorf("expected 1 request recorded, got %f", val)
	}
}

func TestMiddleware_ErrorStatus(t *testing.T) {
	m := New()

	handler := m.Middleware("/v1/dedupe", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/dedupe", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	val := counterValue(t, m.RequestsTotal, "endpoint", "/v1/dedupe", "status", "400")
	if val != 1 {
		t.Errorf("expected 1 request with status 400, got %f", val)
	}
}

func TestHandler(t *testing.T) {
	m := New()
	m.RecordRequest("/v1/dedupe", 200, 10*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "distill_requests_total") {
		t.Error("metrics output missing distill_requests_total")
	}
	if !strings.Contains(body, "distill_request_duration_seconds") {
		t.Error("metrics output missing distill_request_duration_seconds")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("metrics output missing go runtime metrics")
	}
}

func TestActiveRequests(t *testing.T) {
	m := New()

	started := make(chan struct{})
	release := make(chan struct{})

	handler := m.Middleware("/v1/dedupe", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/dedupe", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}()

	<-started

	var metric dto.Metric
	if err := m.ActiveRequests.Write(&metric); err != nil {
		t.Fatalf("failed to read gauge: %v", err)
	}
	if metric.GetGauge().GetValue() != 1 {
		t.Errorf("expected 1 active request, got %f", metric.GetGauge().GetValue())
	}

	close(release)
}

func TestRecordCacheUsage(t *testing.T) {
	m := New()

	m.RecordCacheUsage(UsageRecord{
		SessionID:                "sess-1",
		InputTokens:              100,
		CacheCreationInputTokens: 8000,
		CacheReadInputTokens:     0,
		OutputTokens:             200,
	})

	creationVal := counterValue(t, m.CacheCreationTokens, "session_id", "sess-1")
	if creationVal != 8000 {
		t.Errorf("expected 8000 cache creation tokens, got %f", creationVal)
	}

	uncachedVal := counterValue(t, m.UncachedInputTokens, "session_id", "sess-1")
	if uncachedVal != 100 {
		t.Errorf("expected 100 uncached input tokens, got %f", uncachedVal)
	}

	// Second call: now we have cache reads.
	m.RecordCacheUsage(UsageRecord{
		SessionID:                "sess-1",
		InputTokens:              0,
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     8000,
		OutputTokens:             200,
	})

	readVal := counterValue(t, m.CacheReadTokens, "session_id", "sess-1")
	if readVal != 8000 {
		t.Errorf("expected 8000 cache read tokens, got %f", readVal)
	}

	// Hit rate should be 1.0 (all tokens from cache on second call).
	var hitRateMetric dto.Metric
	if err := m.CacheHitRate.Write(&hitRateMetric); err != nil {
		t.Fatalf("read CacheHitRate: %v", err)
	}
	if hitRateMetric.GetGauge().GetValue() != 1.0 {
		t.Errorf("expected hit rate 1.0, got %f", hitRateMetric.GetGauge().GetValue())
	}
}

func TestRecordCacheUsage_DefaultSessionID(t *testing.T) {
	m := New()
	// Should not panic with empty session ID.
	m.RecordCacheUsage(UsageRecord{
		InputTokens:              50,
		CacheCreationInputTokens: 1000,
	})
	val := counterValue(t, m.CacheCreationTokens, "session_id", "default")
	if val != 1000 {
		t.Errorf("expected 1000, got %f", val)
	}
}

func TestRecordCacheBoundary(t *testing.T) {
	m := New()

	m.RecordCacheBoundary("sess-1", 8192, true, false)
	m.RecordCacheBoundary("sess-1", 16384, true, false)
	m.RecordCacheBoundary("sess-1", 8192, false, true)

	advVal := counterValue(t, m.CacheBoundaryAdvances, "session_id", "sess-1")
	if advVal != 2 {
		t.Errorf("expected 2 advances, got %f", advVal)
	}

	retVal := counterValue(t, m.CacheBoundaryRetreats, "session_id", "sess-1")
	if retVal != 1 {
		t.Errorf("expected 1 retreat, got %f", retVal)
	}

	var posMetric dto.Metric
	pos, err := m.CacheBoundaryPosition.GetMetricWithLabelValues("sess-1")
	if err != nil {
		t.Fatalf("get boundary position: %v", err)
	}
	if err := pos.Write(&posMetric); err != nil {
		t.Fatalf("read boundary position: %v", err)
	}
	if posMetric.GetGauge().GetValue() != 8192 {
		t.Errorf("expected boundary 8192, got %f", posMetric.GetGauge().GetValue())
	}
}

func TestHandler_CacheMetrics(t *testing.T) {
	m := New()
	m.RecordCacheUsage(UsageRecord{
		SessionID:                "sess-1",
		CacheCreationInputTokens: 4096,
		CacheReadInputTokens:     4096,
	})
	m.RecordCacheBoundary("sess-1", 8192, true, false)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, metric := range []string{
		"distill_cache_creation_tokens_total",
		"distill_cache_read_tokens_total",
		"distill_cache_hit_rate",
		"distill_cache_write_efficiency",
		"distill_cache_boundary_position_tokens",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("metrics output missing %s", metric)
		}
	}
}

// counterValue extracts the value of a counter with the given label pairs.
func counterValue(t *testing.T, cv *prometheus.CounterVec, labelPairs ...string) float64 {
	t.Helper()
	labels := prometheus.Labels{}
	for i := 0; i < len(labelPairs); i += 2 {
		labels[labelPairs[i]] = labelPairs[i+1]
	}
	counter, err := cv.GetMetricWith(labels)
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	return metric.GetCounter().GetValue()
}
