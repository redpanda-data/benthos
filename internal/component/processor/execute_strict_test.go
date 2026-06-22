// Copyright 2026 Redpanda Data, Inc.

package processor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// recordProc records the order in which it runs and optionally fails messages.
type recordProc struct {
	name string
	ran  *[]string
	fail bool
}

func (p recordProc) ProcessBatch(_ context.Context, b message.Batch) ([]message.Batch, error) {
	*p.ran = append(*p.ran, p.name)
	if p.fail {
		_ = b.Iter(func(_ int, part *message.Part) error {
			part.ErrorSet(errors.New(p.name + " failed"))
			return nil
		})
	}
	return []message.Batch{b}, nil
}

func (p recordProc) Close(context.Context) error { return nil }

// strictProbe records whether the strict signal was present in the context it
// was executed with.
type strictProbe struct {
	saw *bool
}

func (p strictProbe) ProcessBatch(ctx context.Context, b message.Batch) ([]message.Batch, error) {
	*p.saw = processor.IsStrict(ctx)
	return []message.Batch{b}, nil
}

func (p strictProbe) Close(context.Context) error { return nil }

// Without the strict signal, ExecuteAll runs every processor even after one has
// failed a message (mark-and-continue, the historical behaviour).
func TestExecuteAllNonStrictRunsAll(t *testing.T) {
	var ran []string
	procs := []processor.V1{
		recordProc{name: "a", ran: &ran, fail: true},
		recordProc{name: "b", ran: &ran},
	}
	_, err := processor.ExecuteAll(context.Background(), procs, message.QuickBatch([][]byte{[]byte("x")}))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, ran)
}

// With the strict signal, a message that fails a processor skips the remaining
// processors in the chain.
func TestExecuteAllStrictShortCircuits(t *testing.T) {
	var ran []string
	procs := []processor.V1{
		recordProc{name: "a", ran: &ran, fail: true},
		recordProc{name: "b", ran: &ran},
	}
	_, err := processor.ExecuteAll(processor.WithStrict(context.Background()), procs, message.QuickBatch([][]byte{[]byte("x")}))
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, ran, "processor b must be skipped once a fails in strict mode")
}

// The strict signal propagates to nested ExecuteAll calls via the context.
func TestExecuteAllStrictSignalVisible(t *testing.T) {
	var saw bool
	_, err := processor.ExecuteAll(processor.WithStrict(context.Background()), []processor.V1{strictProbe{saw: &saw}}, message.QuickBatch([][]byte{[]byte("x")}))
	require.NoError(t, err)
	assert.True(t, saw)
}

// --- Benchmarks: verify strict adds no material overhead to the hot path -----

type noopProc struct{}

func (noopProc) ProcessBatch(_ context.Context, b message.Batch) ([]message.Batch, error) {
	return []message.Batch{b}, nil
}
func (noopProc) Close(context.Context) error { return nil }

func benchExecuteAll(b *testing.B, ctx context.Context) {
	b.Helper()
	procs := []processor.V1{noopProc{}, noopProc{}, noopProc{}}
	msg := message.QuickBatch([][]byte{[]byte("hello world")})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := processor.ExecuteAll(ctx, procs, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// The default (non-strict) path: this is what every existing pipeline pays. The
// only added cost over the historical implementation is a single IsStrict(ctx)
// lookup per call.
func BenchmarkExecuteAllNonStrict(b *testing.B) {
	// Mirror the pipeline's context shape (a cancel context over Background).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	benchExecuteAll(b, ctx)
}

func BenchmarkExecuteAllStrict(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	benchExecuteAll(b, processor.WithStrict(ctx))
}

func BenchmarkIsStrict(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = processor.IsStrict(ctx)
	}
}
