package obs

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupTracing configures the global OpenTelemetry tracer provider with an OTLP
// gRPC exporter (the in-cluster collector, e.g. otel-collector:4317) plus the W3C
// trace-context + baggage propagators. After this, otelgin and any otel.Tracer()
// spans export for real — without it the global provider is a no-op and spans go
// nowhere.
//
// When otlpEndpoint is empty it is a no-op (dev/local/test boot without a
// collector) and returns a no-op shutdown, so callers can unconditionally
// `defer shutdown(ctx)`. The returned shutdown flushes the batch span processor.
//
// It takes plain strings rather than *config.Config to keep the platform layer
// decoupled from app config. otlpEndpoint must be host:port with no scheme.
func SetupTracing(ctx context.Context, serviceName, env, otlpEndpoint string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if otlpEndpoint == "" {
		return noop, nil
	}

	// otlptracegrpc.New is lazy: it does not dial until the first export, so a
	// temporarily-unreachable collector does not block or fail startup.
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(), // in-cluster hop; TLS is terminated at the collector/mesh
	)
	if err != nil {
		return noop, fmt.Errorf("obs: otlp trace exporter: %w", err)
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", serviceName),
		attribute.String("deployment.environment", env),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
