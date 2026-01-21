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
	SpanID    string
	Parent    *RecordedSpan
	StartTime time.Time
	EndTime   time.Time
	Ended     bool
	Events    []string
}

// Duration returns the duration of the span. If the span has not ended, it returns 0.
func (rs *RecordedSpan) Duration() time.Duration {
	if !rs.Ended {
		return 0
	}
	return rs.EndTime.Sub(rs.StartTime)
}

// IsChildOf returns true if this span is a direct child of the given parent span.
func (rs *RecordedSpan) IsChildOf(parent *RecordedSpan) bool {
	return rs.Parent == parent
}

// HasParent returns true if this span has a parent span.
func (rs *RecordedSpan) HasParent() bool {
	return rs.Parent != nil
}

// IsRoot returns true if this span has no parent (is a root span).
func (rs *RecordedSpan) IsRoot() bool {
	return rs.Parent == nil
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

func (sr *SpanRecorder) record(name, spanID string, parent *RecordedSpan) *RecordedSpan {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	span := &RecordedSpan{
		Name:      name,
		SpanID:    spanID,
		Parent:    parent,
		StartTime: time.Now(),
		Ended:     false,
		Events:    make([]string, 0),
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

// FindSpansByName returns all recorded spans with the given name.
func (sr *SpanRecorder) FindSpansByName(name string) []*RecordedSpan {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	var result []*RecordedSpan
	for _, span := range sr.spans {
		if span.Name == name {
			result = append(result, span)
		}
	}
	return result
}

// FindSpan returns the first recorded span with the given name, or nil if not found.
func (sr *SpanRecorder) FindSpan(name string) *RecordedSpan {
	spans := sr.FindSpansByName(name)
	if len(spans) > 0 {
		return spans[0]
	}
	return nil
}

// GetChildren returns all child spans of the given parent span.
func (sr *SpanRecorder) GetChildren(parent *RecordedSpan) []*RecordedSpan {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	var result []*RecordedSpan
	for _, span := range sr.spans {
		if span.Parent == parent {
			result = append(result, span)
		}
	}
	return result
}

// recordingSpan wraps a trace.Span to record its lifecycle.
type recordingSpan struct {
	trace.Span
	recorded *RecordedSpan
	ctx      context.Context
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
	var parent *RecordedSpan
	if p := trace.SpanFromContext(ctx); p != nil {
		if s, ok := p.(*recordingSpan); ok {
			parent = s.recorded
		}
	}

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

	// Generate a span ID from the span context if available
	spanID := ""
	if baseSpan != nil && baseSpan.SpanContext().IsValid() {
		spanID = baseSpan.SpanContext().SpanID().String()
	}

	newSpan := &recordingSpan{
		Span:     baseSpan,
		recorded: rt.recorder.record(spanName, spanID, parent),
		ctx:      baseCtx,
	}

	return trace.ContextWithSpan(baseCtx, newSpan), newSpan
}

// RecordingTracerProvider is a TracerProvider that records all spans for testing.
type RecordingTracerProvider struct {
	trace.TracerProvider
	*SpanRecorder
}

// NewInMemoryRecordingTracerProvider creates a new noop tracer provider for
// testing. All spans created by this provider will be recorded and can be
// accessed via the [Spans] method.
func NewInMemoryRecordingTracerProvider() *RecordingTracerProvider {
	return &RecordingTracerProvider{
		TracerProvider: noop.NewTracerProvider(),
		SpanRecorder:   NewSpanRecorder(),
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
		recorder: p.SpanRecorder,
	}
}
