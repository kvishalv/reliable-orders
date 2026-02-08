package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer initializes the OpenTelemetry tracer with OTLP exporter
// This enables distributed tracing across microservices using W3C Trace Context
func InitTracer(serviceName, collectorEndpoint string) (func(context.Context) error, error) {
	ctx := context.Background()

	// Create OTLP trace exporter that sends spans to otel-collector
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorEndpoint),
		otlptracegrpc.WithInsecure(), // Using insecure for demo; production should use TLS
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service metadata
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider with batch span processor for efficiency
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Sample all traces for demo
	)

	// Set global tracer provider and propagator
	otel.SetTracerProvider(tp)
	// W3C Trace Context propagation ensures trace IDs flow across service boundaries
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Return cleanup function to flush remaining spans on shutdown
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(ctx)
	}, nil
}

// GetTracer returns a tracer for the given instrumentation scope
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
