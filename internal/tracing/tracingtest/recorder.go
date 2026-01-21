// Copyright 2026 Redpanda Data, Inc.

package tracingtest

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// RecordedSpan captures span information for testing.
type RecordedSpan struct {
	Name      string
	Ended     bool
	Events    []string
	StartTime time.Time
	EndTime   time.Time
}

// Duration returns the duration of the span. If the span has not ended, it returns 0.
func (rs *RecordedSpan) Duration() time.Duration {
	if !rs.Ended {
		return 0
	}
	return rs.EndTime.Sub(rs.StartTime)
}

// SpanRecorder records spans for testing purposes.
type SpanRecorder struct {
	mu    sync.Mutex
	spans []*RecordedSpan
}

// NewSpanRecorder creates a new span recorder for testing.
func NewSpanRecorder() *SpanRecorder {
	return &SpanRecorder{
		spans: make([]*RecordedSpan, 0),
	}
}

func (sr *SpanRecorder) record(name string) *RecordedSpan {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	span := &RecordedSpan{
		Name:      name,
		Ended:     false,
		Events:    make([]string, 0),
		StartTime: time.Now(),
	}
	sr.spans = append(sr.spans, span)
	return span
}

// Spans returns a copy of all recorded spans.
func (sr *SpanRecorder) Spans() []*RecordedSpan {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	res := make([]*RecordedSpan, len(sr.spans))
	copy(res, sr.spans)
	return res
}

// Reset clears all recorded spans.
func (sr *SpanRecorder) Reset() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.spans = make([]*RecordedSpan, 0)
}

// recordingSpan wraps a trace.Span to record its lifecycle.
type recordingSpan struct {
	trace.Span
	recorded *RecordedSpan
}

func (rs *recordingSpan) End(options ...trace.SpanEndOption) {
	rs.recorded.Ended = true
	rs.recorded.EndTime = time.Now()
	if rs.Span != nil {
		rs.Span.End(options...)
	}
}

func (rs *recordingSpan) AddEvent(name string, options ...trace.EventOption) {
	rs.recorded.Events = append(rs.recorded.Events, name)
	if rs.Span != nil {
		rs.Span.AddEvent(name, options...)
	}
}

// recordingTracer wraps a tracer to record span creation.
type recordingTracer struct {
	trace.Tracer
	recorder *SpanRecorder
}

func (rt *recordingTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	recorded := rt.recorder.record(spanName)

	var (
		baseCtx  context.Context
		baseSpan trace.Span
	)
	if rt.Tracer != nil {
		baseCtx, baseSpan = rt.Tracer.Start(ctx, spanName, opts...)
	} else {
		baseCtx = ctx
		baseSpan = trace.SpanFromContext(ctx)
	}

	return baseCtx, &recordingSpan{
		Span:     baseSpan,
		recorded: recorded,
	}
}

// RecordingTracerProvider is a TracerProvider that records all spans for testing.
type RecordingTracerProvider struct {
	trace.TracerProvider
	recorder *SpanRecorder
}

// NewInMemoryRecordingTracerProvider creates a new noop tracer provider for
// testing. All spans created by this provider will be recorded and can be
// accessed via the [Spans] method.
func NewInMemoryRecordingTracerProvider() *RecordingTracerProvider {
	return &RecordingTracerProvider{
		TracerProvider: noop.NewTracerProvider(),
		recorder:       NewSpanRecorder(),
	}
}

// Tracer returns a tracer that records all spans.
func (p *RecordingTracerProvider) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	var baseTracer trace.Tracer
	if p.TracerProvider != nil {
		baseTracer = p.TracerProvider.Tracer(name, options...)
	}

	return &recordingTracer{
		Tracer:   baseTracer,
		recorder: p.recorder,
	}
}

// Spans returns a copy of all recorded spans.
func (p *RecordingTracerProvider) Spans() []*RecordedSpan {
	return p.recorder.Spans()
}

// Reset clears all recorded spans.
func (p *RecordingTracerProvider) Reset() {
	p.recorder.Reset()
}
