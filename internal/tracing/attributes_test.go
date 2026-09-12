// Copyright 2026 Redpanda Data, Inc.

package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/redpanda-data/benthos/v4/internal/tracing"
)

type captureTracerProvider struct {
	trace.TracerProvider
	starts []trace.SpanConfig
}

func (p *captureTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &captureTracer{Tracer: p.TracerProvider.Tracer(name, opts...), prov: p}
}

type captureTracer struct {
	trace.Tracer
	prov *captureTracerProvider
}

func (t *captureTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	t.prov.starts = append(t.prov.starts, trace.NewSpanStartConfig(opts...))
	return t.Tracer.Start(ctx, name, opts...)
}

func newCaptureTracerProvider() *captureTracerProvider {
	return &captureTracerProvider{TracerProvider: noop.NewTracerProvider()}
}

func TestWithAttributesAddsToEverySpan(t *testing.T) {
	capture := newCaptureTracerProvider()

	prov := tracing.WithAttributes(capture, attribute.String("stream", "foo"))
	_, span := prov.Tracer("bar").Start(t.Context(), "baz")
	span.End()

	assert.Len(t, capture.starts, 1)
	assert.Equal(t, []attribute.KeyValue{
		attribute.String("stream", "foo"),
	}, capture.starts[0].Attributes())
}

func TestWithAttributesKeepsStartOptions(t *testing.T) {
	capture := newCaptureTracerProvider()

	prov := tracing.WithAttributes(capture, attribute.String("stream", "foo"))
	_, span := prov.Tracer("bar").Start(t.Context(), "baz",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("meow", "woof")),
	)
	span.End()

	assert.Len(t, capture.starts, 1)
	assert.Equal(t, trace.SpanKindConsumer, capture.starts[0].SpanKind())
	assert.Equal(t, []attribute.KeyValue{
		attribute.String("meow", "woof"),
		attribute.String("stream", "foo"),
	}, capture.starts[0].Attributes())
}

func TestWithAttributesDoesNotModifyCallerOptions(t *testing.T) {
	capture := newCaptureTracerProvider()

	// Spare capacity in the caller's slice must not be written to.
	opts := make([]trace.SpanStartOption, 1, 5)
	opts[0] = trace.WithAttributes(attribute.String("meow", "woof"))

	prov := tracing.WithAttributes(capture, attribute.String("stream", "foo"))
	_, span := prov.Tracer("bar").Start(t.Context(), "baz", opts...)
	span.End()

	callerConf := trace.NewSpanStartConfig(opts...)
	assert.Len(t, opts, 1)
	assert.Equal(t, []attribute.KeyValue{
		attribute.String("meow", "woof"),
	}, callerConf.Attributes())
}

func TestWithAttributesNoAttributesIsAPassthrough(t *testing.T) {
	capture := newCaptureTracerProvider()
	assert.Equal(t, trace.TracerProvider(capture), tracing.WithAttributes(capture))
	assert.Nil(t, tracing.WithAttributes(nil, attribute.String("stream", "foo")))
}
