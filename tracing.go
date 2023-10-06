package main

import (
	"context"

	"github.com/ipfs/boxo/tracing"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	traceapi "go.opentelemetry.io/otel/trace"
)

func newTracerProvider(ctx context.Context) (traceapi.TracerProvider, func(context.Context) error, error) {
	exporters, err := tracing.NewSpanExporters(ctx)
	if err != nil {
		return nil, nil, err
	}

	if len(exporters) == 0 {
		return traceapi.NewNoopTracerProvider(), func(ctx context.Context) error { return nil }, nil
	}

	options := []trace.TracerProviderOption{}

	for _, exporter := range exporters {
		options = append(options, trace.WithBatcher(exporter))
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String("Rainbow"),
			semconv.ServiceVersionKey.String(buildVersion()),
		),
	)
	if err != nil {
		return nil, nil, err
	}
	options = append(options, trace.WithResource(r))

	tp := trace.NewTracerProvider(options...)
	return tp, tp.Shutdown, nil
}
