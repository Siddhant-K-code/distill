package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInit_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	if p.Tracer() == nil {
		t.Fatal("tracer should not be nil even when disabled")
	}

	// Should create no-op spans without error
	ctx, span := p.StartRequest(context.Background(), "/v1/dedupe")
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
	span.End()
}

func TestInit_ExporterNone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "none"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	if p.Tracer() == nil {
		t.Fatal("tracer should not be nil")
	}
}

func TestInit_ExporterStdout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	if p.tp == nil {
		t.Fatal("TracerProvider should not be nil for stdout exporter")
	}
}

func TestInit_InvalidExporter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "invalid"

	_, err := Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid exporter")
	}
}

func TestInit_SampleRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"
	cfg.SampleRate = 0.5

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()
}

func TestShutdown_NilProvider(t *testing.T) {
	p := &Provider{
		tracer: noop.NewTracerProvider().Tracer(tracerName),
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should not error on nil provider: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("tracing should be disabled by default")
	}
	if cfg.Exporter != "otlp" {
		t.Errorf("expected default exporter otlp, got %s", cfg.Exporter)
	}
	if cfg.Endpoint != "localhost:4317" {
		t.Errorf("expected default endpoint localhost:4317, got %s", cfg.Endpoint)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("expected default sample rate 1.0, got %f", cfg.SampleRate)
	}
	if cfg.ServiceName != "distill" {
		t.Errorf("expected default service name distill, got %s", cfg.ServiceName)
	}
}

func TestSpanHelpers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	ctx := context.Background()

	// All span helpers should work without panicking
	tests := []struct {
		name string
		fn   func() (context.Context, trace.Span)
	}{
		{"StartRequest", func() (context.Context, trace.Span) { return p.StartRequest(ctx, "/v1/dedupe") }},
		{"StartEmbedding", func() (context.Context, trace.Span) { return p.StartEmbedding(ctx, 10) }},
		{"StartClustering", func() (context.Context, trace.Span) { return p.StartClustering(ctx, 10, 0.15) }},
		{"StartSelection", func() (context.Context, trace.Span) { return p.StartSelection(ctx, 5) }},
		{"StartMMR", func() (context.Context, trace.Span) { return p.StartMMR(ctx, 5, 0.5) }},
		{"StartCompress", func() (context.Context, trace.Span) { return p.StartCompress(ctx, 5, "extractive") }},
		{"StartCacheLookup", func() (context.Context, trace.Span) { return p.StartCacheLookup(ctx, "test-key") }},
		{"StartRetrieval", func() (context.Context, trace.Span) { return p.StartRetrieval(ctx, 50, "pinecone") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, span := tt.fn()
			if c == nil {
				t.Error("context should not be nil")
			}
			if span == nil {
				t.Error("span should not be nil")
			}
			span.End()
		})
	}
}

func TestRecordResult(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	_, span := p.StartRequest(context.Background(), "/v1/dedupe")
	// Should not panic
	RecordResult(span, 10, 6, 6, 12*time.Millisecond)
	span.End()
}

func TestRecordResult_ZeroInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	_, span := p.StartRequest(context.Background(), "/v1/dedupe")
	// Should not panic on zero input
	RecordResult(span, 0, 0, 0, 0)
	span.End()
}

func TestRecordError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Exporter = "stdout"

	p, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = p.Shutdown(context.Background()) }()

	_, span := p.StartRequest(context.Background(), "/v1/dedupe")
	RecordError(span, fmt.Errorf("test error"))
	span.End()
}

// Verify attribute is importable (compile-time check used in span helpers)
var _ = attribute.String("test", "value")
