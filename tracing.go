package main

import (
	"context"
	"github.com/ipfs/boxo/tracing"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	traceapi "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"strings"
)

func newTracerProvider(ctx context.Context, traceFraction float64) (traceapi.TracerProvider, func(context.Context) error, error) {
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

	var baseSampler trace.Sampler
	if traceFraction == 0 {
		baseSampler = trace.NeverSample()
	} else {
		baseSampler = trace.TraceIDRatioBased(traceFraction)
	}

	// Sample all children whose parents are sampled
	// Probabilistically sample if the span is a root which is a Gateway request
	sampler := trace.ParentBased(
		CascadingSamplerFunc(func(parameters trace.SamplingParameters) bool {
			return !traceapi.SpanContextFromContext(parameters.ParentContext).IsValid()
		}, "root sampler",
			CascadingSamplerFunc(func(parameters trace.SamplingParameters) bool {
				return strings.HasPrefix(parameters.Name, "Gateway")
			}, "gateway request sampler",
				baseSampler)))
	options = append(options, trace.WithResource(r), trace.WithSampler(sampler))

	tp := trace.NewTracerProvider(options...)
	return tp, tp.Shutdown, nil
}

type funcSampler struct {
	next        trace.Sampler
	fn          func(trace.SamplingParameters) trace.SamplingResult
	description string
}

func (f funcSampler) ShouldSample(parameters trace.SamplingParameters) trace.SamplingResult {
	return f.fn(parameters)
}

func (f funcSampler) Description() string {
	return f.description
}

// CascadingSamplerFunc will sample with the next tracer if the condition is met, otherwise the sample will be dropped
func CascadingSamplerFunc(shouldSample func(parameters trace.SamplingParameters) bool, description string, next trace.Sampler) trace.Sampler {
	return funcSampler{
		next: next,
		fn: func(parameters trace.SamplingParameters) trace.SamplingResult {
			if shouldSample(parameters) {
				return next.ShouldSample(parameters)
			}
			return trace.SamplingResult{
				Decision:   trace.Drop,
				Tracestate: traceapi.SpanContextFromContext(parameters.ParentContext).TraceState(),
			}
		},
		description: description,
	}
}
