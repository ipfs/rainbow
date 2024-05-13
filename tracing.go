package main

import (
	"context"
	"fmt"
	"github.com/ipfs/boxo/tracing"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	traceapi "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"strings"
)

func newTracerProvider(ctx context.Context) (traceapi.TracerProvider, func(context.Context) error, error) {
	exporters, err := tracing.NewSpanExporters(ctx)
	if err != nil {
		return nil, nil, err
	}

	if len(exporters) == 0 {
		return tracenoop.NewTracerProvider(), func(ctx context.Context) error { return nil }, nil
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

	options = append(options, trace.WithResource(r), trace.WithSampler(RootPrefixSampler{RootPrefix: "Gateway", Next: trace.ParentBased(trace.NeverSample())}))

	tp := trace.NewTracerProvider(options...)
	return tp, tp.Shutdown, nil
}

type RootPrefixSampler struct {
	Next       trace.Sampler
	RootPrefix string
}

var _ trace.Sampler = (*RootPrefixSampler)(nil)

func (s RootPrefixSampler) ShouldSample(parameters trace.SamplingParameters) trace.SamplingResult {
	parentSpan := traceapi.SpanContextFromContext(parameters.ParentContext)
	if !parentSpan.IsValid() && strings.HasPrefix(parameters.Name, s.RootPrefix) {
		res := s.Next.ShouldSample(parameters)
		return trace.SamplingResult{
			Decision:   res.Decision,
			Attributes: res.Attributes,
			Tracestate: res.Tracestate,
		}
	}

	return s.Next.ShouldSample(parameters)
}

func (s RootPrefixSampler) Description() string {
	return fmt.Sprintf("root prefix sampler: %s", s.RootPrefix)
}
