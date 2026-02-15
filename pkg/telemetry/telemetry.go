// Package telemetry provides OpenTelemetry distributed tracing for Distill.
// It instruments the deduplication pipeline with spans for each stage,
// supports W3C Trace Context propagation, and exports to OTLP or stdout.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/Siddhant-K-code/distill"

// Config holds tracing configuration.
type Config struct {
	// Enabled turns tracing on/off.
	Enabled bool

	// Exporter selects the trace exporter: "otlp", "stdout", or "none".
	Exporter string

	// Endpoint is the OTLP collector address (e.g., "localhost:4317").
	Endpoint string

	// SampleRate controls the sampling ratio (0.0 to 1.0).
	// 1.0 = sample everything, 0.1 = sample 10%.
	SampleRate float64

	// ServiceName overrides the default service name.
	ServiceName string

	// Insecure disables TLS for the OTLP exporter.
	Insecure bool
}

// DefaultConfig returns tracing defaults (disabled).
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Exporter:    "otlp",
		Endpoint:    "localhost:4317",
		SampleRate:  1.0,
		ServiceName: "distill",
		Insecure:    true,
	}
}

// Provider wraps the OTEL TracerProvider and exposes Distill-specific helpers.
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

// Init sets up the global TracerProvider based on the config.
// Returns a Provider that must be shut down with Shutdown().
func Init(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		// Return a no-op provider
		return &Provider{
			tracer: trace.NewNoopTracerProvider().Tracer(tracerName),
		}, nil
	}

	var exporter sdktrace.SpanExporter
	var err error

	switch cfg.Exporter {
	case "otlp":
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	case "none", "":
		return &Provider{
			tracer: trace.NewNoopTracerProvider().Tracer(tracerName),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported exporter: %q (supported: otlp, stdout, none)", cfg.Exporter)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("0.2.0"),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if cfg.SampleRate < 1.0 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global provider and propagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(tracerName),
	}, nil
}

// Shutdown flushes pending spans and shuts down the provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp == nil {
		return nil
	}
	return p.tp.Shutdown(ctx)
}

// Tracer returns the Distill tracer for creating spans.
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// --- Span helpers for pipeline stages ---

// StartRequest creates a root span for an incoming HTTP request.
func (p *Provider) StartRequest(ctx context.Context, endpoint string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.request",
		trace.WithAttributes(attribute.String("distill.endpoint", endpoint)),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartEmbedding creates a span for the embedding generation stage.
func (p *Provider) StartEmbedding(ctx context.Context, chunkCount int) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.embedding",
		trace.WithAttributes(attribute.Int("distill.embedding.chunk_count", chunkCount)),
	)
}

// StartClustering creates a span for the clustering stage.
func (p *Provider) StartClustering(ctx context.Context, inputCount int, threshold float64) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.clustering",
		trace.WithAttributes(
			attribute.Int("distill.clustering.input_count", inputCount),
			attribute.Float64("distill.clustering.threshold", threshold),
		),
	)
}

// StartSelection creates a span for the representative selection stage.
func (p *Provider) StartSelection(ctx context.Context, clusterCount int) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.selection",
		trace.WithAttributes(attribute.Int("distill.selection.cluster_count", clusterCount)),
	)
}

// StartMMR creates a span for the MMR re-ranking stage.
func (p *Provider) StartMMR(ctx context.Context, inputCount int, lambda float64) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.mmr",
		trace.WithAttributes(
			attribute.Int("distill.mmr.input_count", inputCount),
			attribute.Float64("distill.mmr.lambda", lambda),
		),
	)
}

// StartCompress creates a span for the compression stage.
func (p *Provider) StartCompress(ctx context.Context, chunkCount int, mode string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.compress",
		trace.WithAttributes(
			attribute.Int("distill.compress.chunk_count", chunkCount),
			attribute.String("distill.compress.mode", mode),
		),
	)
}

// StartCacheLookup creates a span for a cache lookup.
func (p *Provider) StartCacheLookup(ctx context.Context, key string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.cache.lookup",
		trace.WithAttributes(attribute.String("distill.cache.key", key)),
	)
}

// StartRetrieval creates a span for vector DB retrieval.
func (p *Provider) StartRetrieval(ctx context.Context, topK int, backend string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "distill.retrieval",
		trace.WithAttributes(
			attribute.Int("distill.retrieval.top_k", topK),
			attribute.String("distill.retrieval.backend", backend),
		),
	)
}

// RecordResult adds result attributes to a span.
func RecordResult(span trace.Span, inputCount, outputCount, clusterCount int, latency time.Duration) {
	span.SetAttributes(
		attribute.Int("distill.result.input_count", inputCount),
		attribute.Int("distill.result.output_count", outputCount),
		attribute.Int("distill.result.cluster_count", clusterCount),
		attribute.Int64("distill.result.latency_ms", latency.Milliseconds()),
	)
	if inputCount > 0 {
		reduction := 1.0 - float64(outputCount)/float64(inputCount)
		span.SetAttributes(attribute.Float64("distill.result.reduction_ratio", reduction))
	}
}

// RecordError records an error on a span.
func RecordError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetAttributes(attribute.Bool("error", true))
}
