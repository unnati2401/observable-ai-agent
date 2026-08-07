package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing(ctx context.Context) (func(context.Context) error, error) {

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint("localhost:4317"),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName("observable-ai-agent"),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.TraceContext{},
	)

	return tp.Shutdown, nil
}
