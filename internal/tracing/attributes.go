// Copyright 2026 Redpanda Data, Inc.

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// WithAttributes returns a tracer provider that adds the given attributes to
// every span started through it, leaving the provided provider untouched. This
// allows a manager to annotate spans with the context it already adds to its
// logs and metrics, such as the stream a component belongs to.
func WithAttributes(prov trace.TracerProvider, attrs ...attribute.KeyValue) trace.TracerProvider {
	if prov == nil || len(attrs) == 0 {
		return prov
	}
	return &attributedTracerProvider{TracerProvider: prov, attrs: attrs}
}

type attributedTracerProvider struct {
	trace.TracerProvider
	attrs []attribute.KeyValue
}

func (p *attributedTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &attributedTracer{
		Tracer: p.TracerProvider.Tracer(name, opts...),
		attrs:  p.attrs,
	}
}

type attributedTracer struct {
	trace.Tracer
	attrs []attribute.KeyValue
}

func (t *attributedTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	// The attributes go in as start options rather than being set on the span
	// afterwards so that samplers can see them.
	withAttrs := make([]trace.SpanStartOption, 0, len(opts)+1)
	withAttrs = append(withAttrs, opts...)
	withAttrs = append(withAttrs, trace.WithAttributes(t.attrs...))
	return t.Tracer.Start(ctx, name, withAttrs...)
}
