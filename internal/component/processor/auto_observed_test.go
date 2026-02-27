// Copyright 2025 Redpanda Data, Inc.

package processor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/tracing/tracingtest"
)

type fnProcessor struct {
	fn     func(context.Context, *message.Part) ([]*message.Part, error)
	closed bool

	sync.Mutex
}

func (p *fnProcessor) Process(ctx context.Context, msg *message.Part) ([]*message.Part, error) {
	return p.fn(ctx, msg)
}

func (p *fnProcessor) Close(ctx context.Context) error {
	p.Lock()
	p.closed = true
	p.Unlock()
	return nil
}

func TestProcessorAirGapShutdown(t *testing.T) {
	rp := &fnProcessor{}
	agrp := NewAutoObservedProcessor("foo", rp, component.NoopObservability())

	ctx, done := context.WithTimeout(t.Context(), time.Microsecond*5)
	defer done()

	err := agrp.Close(ctx)
	assert.NoError(t, err)
	rp.Lock()
	assert.True(t, rp.closed)
	rp.Unlock()
}

func TestProcessorAirGapOneToOne(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedProcessor("foo", &fnProcessor{
		fn: func(c context.Context, m *message.Part) ([]*message.Part, error) {
			if b := m.AsBytes(); string(b) != "unchanged" {
				return nil, errors.New("nope")
			}
			newPart := m.ShallowCopy()
			newPart.SetBytes([]byte("changed"))
			return []*message.Part{newPart}, nil
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("unchanged")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	assert.Equal(t, 1, msgs[0].Len())
	assert.Equal(t, "changed", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "unchanged", string(msg.Get(0).AsBytes()))
}

func TestProcessorAirGapOneToError(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedProcessor("foo", &fnProcessor{
		fn: func(c context.Context, m *message.Part) ([]*message.Part, error) {
			_, err := m.AsStructuredMut()
			return nil, err
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("not a structured doc")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	assert.Equal(t, 1, msgs[0].Len())
	assert.Equal(t, "not a structured doc", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "not a structured doc", string(msgs[0].Get(0).AsBytes()))
	assert.EqualError(t, msgs[0].Get(0).ErrorGet(), "invalid character 'o' in literal null (expecting 'u')")
}

func TestProcessorAirGapOneToMany(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedProcessor("foo", &fnProcessor{
		fn: func(c context.Context, m *message.Part) ([]*message.Part, error) {
			if b := m.AsBytes(); string(b) != "unchanged" {
				return nil, errors.New("nope")
			}
			first := m.ShallowCopy()
			second := m.ShallowCopy()
			third := m.ShallowCopy()
			first.SetBytes([]byte("changed 1"))
			second.SetBytes([]byte("changed 2"))
			third.SetBytes([]byte("changed 3"))
			return []*message.Part{first, second, third}, nil
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("unchanged")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	assert.Equal(t, 3, msgs[0].Len())
	assert.Equal(t, "changed 1", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "changed 2", string(msgs[0].Get(1).AsBytes()))
	assert.Equal(t, "changed 3", string(msgs[0].Get(2).AsBytes()))
	assert.Equal(t, "unchanged", string(msg.Get(0).AsBytes()))
}

//------------------------------------------------------------------------------

type fnBatchProcessor struct {
	fn     func(*BatchProcContext, message.Batch) ([]message.Batch, error)
	closed bool
}

func (p *fnBatchProcessor) ProcessBatch(ctx *BatchProcContext, batch message.Batch) ([]message.Batch, error) {
	return p.fn(ctx, batch)
}

func (p *fnBatchProcessor) Close(ctx context.Context) error {
	p.closed = true
	return nil
}

func TestBatchProcessorAirGapShutdown(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Millisecond*5)
	defer done()

	rp := &fnBatchProcessor{}
	agrp := NewAutoObservedBatchedProcessor("foo", rp, component.NoopObservability())

	err := agrp.Close(tCtx)
	assert.NoError(t, err)
	assert.True(t, rp.closed)
}

func TestBatchProcessorAirGapOneToOne(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedBatchedProcessor("foo", &fnBatchProcessor{
		fn: func(c *BatchProcContext, msgs message.Batch) ([]message.Batch, error) {
			if b := msgs.Get(0).AsBytes(); string(b) != "unchanged" {
				return nil, errors.New("nope")
			}
			newMsg := msgs.Get(0).ShallowCopy()
			newMsg.SetBytes([]byte("changed"))
			return []message.Batch{{newMsg}}, nil
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("unchanged")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	assert.Equal(t, 1, msgs[0].Len())
	assert.Equal(t, "changed", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "unchanged", string(msg.Get(0).AsBytes()))
}

func TestBatchProcessorAirGapOneToError(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedBatchedProcessor("foo", &fnBatchProcessor{
		fn: func(c *BatchProcContext, msgs message.Batch) ([]message.Batch, error) {
			_, err := msgs.Get(0).AsStructuredMut()
			return nil, err
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("not a structured doc")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	assert.Equal(t, 1, msgs[0].Len())
	assert.Equal(t, "not a structured doc", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "not a structured doc", string(msgs[0].Get(0).AsBytes()))
	assert.EqualError(t, msgs[0].Get(0).ErrorGet(), "invalid character 'o' in literal null (expecting 'u')")
}

func TestBatchProcessorAirGapOneToMany(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedBatchedProcessor("foo", &fnBatchProcessor{
		fn: func(c *BatchProcContext, msgs message.Batch) ([]message.Batch, error) {
			if b := msgs.Get(0).AsBytes(); string(b) != "unchanged" {
				return nil, errors.New("nope")
			}
			first := msgs.Get(0).ShallowCopy()
			second := msgs.Get(0).ShallowCopy()
			third := msgs.Get(0).ShallowCopy()
			first.SetBytes([]byte("changed 1"))
			second.SetBytes([]byte("changed 2"))
			third.SetBytes([]byte("changed 3"))

			firstBatch := message.Batch{first, second}
			secondBatch := message.Batch{third}
			return []message.Batch{firstBatch, secondBatch}, nil
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{[]byte("unchanged")})
	msgs, res := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, res)
	require.Len(t, msgs, 2)
	assert.Equal(t, "unchanged", string(msg.Get(0).AsBytes()))

	assert.Equal(t, 2, msgs[0].Len())
	assert.Equal(t, "changed 1", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "changed 2", string(msgs[0].Get(1).AsBytes()))

	assert.Equal(t, 1, msgs[1].Len())
	assert.Equal(t, "changed 3", string(msgs[1].Get(0).AsBytes()))
}

func TestBatchProcessorAirGapIndividualErrors(t *testing.T) {
	tCtx := t.Context()

	agrp := NewAutoObservedBatchedProcessor("foo", &fnBatchProcessor{
		fn: func(c *BatchProcContext, msgs message.Batch) ([]message.Batch, error) {
			for i, m := range msgs {
				if _, err := m.AsStructuredMut(); err != nil {
					c.OnError(err, i, nil)
				}
			}
			return []message.Batch{msgs}, nil
		},
	}, component.NoopObservability())

	msg := message.QuickBatch([][]byte{
		[]byte("not a structured doc"),
		[]byte(`{"foo":"bar"}`),
		[]byte("abcdefg"),
	})

	msgs, err := agrp.ProcessBatch(tCtx, msg)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0], 3)

	assert.Equal(t, "not a structured doc", string(msgs[0][0].AsBytes()))
	assert.Equal(t, `{"foo":"bar"}`, string(msgs[0][1].AsBytes()))
	assert.Equal(t, "abcdefg", string(msgs[0][2].AsBytes()))

	assert.EqualError(t, msgs[0][0].ErrorGet(), "invalid character 'o' in literal null (expecting 'u')")
	assert.NoError(t, msgs[0][1].ErrorGet())
	assert.EqualError(t, msgs[0][2].ErrorGet(), "invalid character 'a' looking for beginning of value")
}

//------------------------------------------------------------------------------

// passThroughProcessor is a simple processor that passes messages through unchanged
// Used for testing span context isolation
type passThroughProcessor struct {
	name string
}

func (p *passThroughProcessor) Process(ctx context.Context, msg *message.Part) ([]*message.Part, error) {
	return []*message.Part{msg}, nil
}

func (p *passThroughProcessor) Close(ctx context.Context) error {
	return nil
}

// childSpanProcessor is a processor that creates a child span to test nesting
type childSpanProcessor struct {
	name string
	tp   *tracingtest.RecordingTracerProvider
}

func (p *childSpanProcessor) Process(_ context.Context, msg *message.Part) ([]*message.Part, error) {
	// Create a child span within this processor to verify proper nesting
	tracer := p.tp.Tracer("test")
	ctx, span := tracer.Start(message.GetContext(msg), p.name+"_child")
	defer span.End()

	newMsg := msg.ShallowCopy()
	newMsg = newMsg.WithContext(ctx)
	return []*message.Part{newMsg}, nil
}

func (p *childSpanProcessor) Close(ctx context.Context) error {
	return nil
}

func TestProcessorChainTracingIsolation(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	obs := component.MockObservabilityWithTracerProvider(tp)

	// Create a chain of three processors
	proc1 := NewAutoObservedProcessor("processor1", &passThroughProcessor{name: "proc1"}, obs)
	proc2 := NewAutoObservedProcessor("processor2", &childSpanProcessor{name: "proc2", tp: tp}, obs)
	proc3 := NewAutoObservedProcessor("processor3", &passThroughProcessor{name: "proc3"}, obs)

	// Process a message through the chain
	inMsg := message.NewPart([]byte("test message"))
	inBatch := message.Batch{inMsg}

	// First processor
	batch1, res := proc1.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, batch1, 1)

	// Second processor
	batch2, res := proc2.ProcessBatch(t.Context(), batch1[0])
	require.NoError(t, res)
	require.Len(t, batch2, 1)

	// Third processor
	batch3, res := proc3.ProcessBatch(t.Context(), batch2[0])
	require.NoError(t, res)
	require.Len(t, batch3, 1)

	// Verify span hierarchy
	// Expected structure:
	//   processor1 - root (sibling)
	//   processor2 - root (sibling)
	//     proc2_child - child of processor2
	//   processor3 - root (sibling)
	spans := tp.Spans()
	require.Len(t, spans, 4, "should have 4 spans")

	proc1Span := tp.FindSpan("processor1")
	proc2Span := tp.FindSpan("processor2")
	proc2ChildSpan := tp.FindSpan("proc2_child")
	proc3Span := tp.FindSpan("processor3")

	require.NotNil(t, proc1Span)
	require.NotNil(t, proc2Span)
	require.NotNil(t, proc2ChildSpan)
	require.NotNil(t, proc3Span)

	// Verify all processor spans are root spans (siblings, not nested)
	assert.True(t, proc1Span.IsRoot(), "processor1 should be a root span")
	assert.True(t, proc2Span.IsRoot(), "processor2 should be a root span")
	assert.True(t, proc3Span.IsRoot(), "processor3 should be a root span")

	// Verify the child span is nested under processor2, not processor1 or processor3
	assert.True(t, proc2ChildSpan.IsChildOf(proc2Span),
		"proc2_child should be a child of processor2")
	assert.False(t, proc2ChildSpan.IsChildOf(proc1Span),
		"proc2_child should NOT be a child of processor1")
	assert.False(t, proc3Span.IsChildOf(proc2Span),
		"processor3 should NOT be a child of processor2")
	assert.False(t, proc3Span.IsChildOf(proc1Span),
		"processor3 should NOT be a child of processor1")
}

func TestProcessorChainTracingEmptyBatch(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	obs := component.MockObservabilityWithTracerProvider(tp)

	// Create a chain of processors
	proc1 := NewAutoObservedProcessor("processor1", &passThroughProcessor{name: "proc1"}, obs)
	proc2 := NewAutoObservedProcessor("processor2", &childSpanProcessor{name: "proc2", tp: tp}, obs)

	// Process an empty batch through the chain
	emptyBatch := message.Batch{}

	// First processor
	batch1, res := proc1.ProcessBatch(t.Context(), emptyBatch)
	require.NoError(t, res)
	require.Nil(t, batch1, "empty batch should return nil")

	// Second processor with nil input
	batch2, res := proc2.ProcessBatch(t.Context(), emptyBatch)
	require.NoError(t, res)
	require.Nil(t, batch2, "empty batch should return nil")

	// Verify no spans were created for empty batch
	spans := tp.Spans()
	assert.Empty(t, spans, "empty batch should not create any spans")
}

func TestProcessorChainTracingIsolationBatch(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	obs := component.MockObservabilityWithTracerProvider(tp)

	// Create a chain of three processors
	proc1 := NewAutoObservedProcessor("processor1", &passThroughProcessor{name: "proc1"}, obs)
	proc2 := NewAutoObservedProcessor("processor2", &childSpanProcessor{name: "proc2", tp: tp}, obs)
	proc3 := NewAutoObservedProcessor("processor3", &passThroughProcessor{name: "proc3"}, obs)

	// Create batch with multiple messages
	inBatch := message.Batch{
		message.NewPart([]byte("message 1")),
		message.NewPart([]byte("message 2")),
		message.NewPart([]byte("message 3")),
	}

	// First processor
	batch1, res := proc1.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, batch1, 1)
	require.Len(t, batch1[0], 3)

	// Second processor
	batch2, res := proc2.ProcessBatch(t.Context(), batch1[0])
	require.NoError(t, res)
	require.Len(t, batch2, 1)
	require.Len(t, batch2[0], 3)

	// Third processor
	batch3, res := proc3.ProcessBatch(t.Context(), batch2[0])
	require.NoError(t, res)
	require.Len(t, batch3, 1)
	require.Len(t, batch3[0], 3)

	// Verify span hierarchy
	// Expected structure (per message):
	//   processor1 - root (sibling) × 3
	//   processor2 - root (sibling) × 3
	//     proc2_child - child of processor2 × 3
	//   processor3 - root (sibling) × 3
	// Total: 12 spans (3 messages × 4 spans each)
	spans := tp.Spans()
	require.Len(t, spans, 12, "should have 12 spans: 3 × (processor1, processor2, proc2_child, processor3)")

	proc1Spans := tp.FindSpansByName("processor1")
	proc2Spans := tp.FindSpansByName("processor2")
	proc2ChildSpans := tp.FindSpansByName("proc2_child")
	proc3Spans := tp.FindSpansByName("processor3")

	require.Len(t, proc1Spans, 3)
	require.Len(t, proc2Spans, 3)
	require.Len(t, proc2ChildSpans, 3)
	require.Len(t, proc3Spans, 3)

	// Verify all processor spans are root spans (siblings, not nested)
	for i := range 3 {
		assert.True(t, proc1Spans[i].IsRoot(),
			"processor1 span %d should be a root span", i)
		assert.True(t, proc2Spans[i].IsRoot(),
			"processor2 span %d should be a root span", i)
		assert.True(t, proc3Spans[i].IsRoot(),
			"processor3 span %d should be a root span", i)

		// Verify the child span is nested under processor2
		assert.True(t, proc2ChildSpans[i].IsChildOf(proc2Spans[i]),
			"proc2_child span %d should be a child of processor2 span %d", i, i)

		// Verify processor2 child spans are NOT children of processor1
		assert.False(t, proc2ChildSpans[i].IsChildOf(proc1Spans[i]),
			"proc2_child span %d should NOT be a child of processor1 span %d", i, i)

		// Verify processor3 is NOT a child of processor2 or processor1
		assert.False(t, proc3Spans[i].IsChildOf(proc2Spans[i]),
			"processor3 span %d should NOT be a child of processor2 span %d", i, i)
		assert.False(t, proc3Spans[i].IsChildOf(proc1Spans[i]),
			"processor3 span %d should NOT be a child of processor1 span %d", i, i)
	}
}
