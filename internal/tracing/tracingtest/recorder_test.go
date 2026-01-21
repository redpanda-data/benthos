// Copyright 2026 Redpanda Data, Inc.

package tracingtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingTracerProvider(t *testing.T) {
	tp := NewInMemoryRecordingTracerProvider()
	tracer := tp.Tracer("test")

	// Create a span
	ctx, span := tracer.Start(context.Background(), "test-span")
	require.NotNil(t, span)
	require.NotNil(t, ctx)

	// End the span
	span.End()

	// Verify span was recorded
	spans := tp.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "test-span", spans[0].Name)
	assert.True(t, spans[0].Ended)
}

func TestRecordingTracerProviderMultipleSpans(t *testing.T) {
	tp := NewInMemoryRecordingTracerProvider()
	tracer := tp.Tracer("test")

	// Create multiple spans
	for i := 0; i < 3; i++ {
		_, span := tracer.Start(context.Background(), "span")
		span.End()
	}

	// Verify all spans were recorded
	spans := tp.Spans()
	require.Len(t, spans, 3)
	for i, span := range spans {
		assert.Equal(t, "span", span.Name)
		assert.True(t, span.Ended, "Span %d should be ended", i)
	}
}

func TestRecordingTracerProviderReset(t *testing.T) {
	tp := NewInMemoryRecordingTracerProvider()
	tracer := tp.Tracer("test")

	// Create a span
	_, span := tracer.Start(context.Background(), "span1")
	span.End()

	require.Len(t, tp.Spans(), 1)

	// Reset
	tp.Reset()

	// Verify spans were cleared
	assert.Empty(t, tp.Spans())

	// Create another span after reset
	_, span2 := tracer.Start(context.Background(), "span2")
	span2.End()

	// Verify only new span is recorded
	spans := tp.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "span2", spans[0].Name)
}

func TestRecordingTracerProviderEvents(t *testing.T) {
	tp := NewInMemoryRecordingTracerProvider()
	tracer := tp.Tracer("test")

	// Create a span and add events
	_, span := tracer.Start(context.Background(), "span-with-events")
	span.AddEvent("event1")
	span.AddEvent("event2")
	span.End()

	// Verify events were recorded
	spans := tp.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "span-with-events", spans[0].Name)
	assert.Equal(t, []string{"event1", "event2"}, spans[0].Events)
	assert.True(t, spans[0].Ended)
}

func TestRecordingTracerProviderNestedSpans(t *testing.T) {
	tp := NewInMemoryRecordingTracerProvider()
	tracer := tp.Tracer("test")

	// Create parent span
	ctx, parentSpan := tracer.Start(context.Background(), "parent")

	// Create child span
	_, childSpan := tracer.Start(ctx, "child")
	childSpan.End()

	parentSpan.End()

	// Verify both spans were recorded
	spans := tp.Spans()
	require.Len(t, spans, 2)
	assert.Equal(t, "parent", spans[0].Name)
	assert.Equal(t, "child", spans[1].Name)
	assert.True(t, spans[0].Ended)
	assert.True(t, spans[1].Ended)
}
