// Copyright 2025 Redpanda Data, Inc.

package input_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// mockBackfillAsyncReader implements input.BackfillAsync and
// input.BackfillCompleter, recording event order so tests can assert on the
// ack barrier.
type mockBackfillAsyncReader struct {
	mu sync.Mutex

	backfillBatches [][]byte
	backfillIdx     int
	streamBatches   [][]byte
	streamIdx       int

	events []string

	backfillCompleteCalled chan struct{}
}

func newMockBackfillAsyncReader(snapshotBatches, streamBatches [][]byte) *mockBackfillAsyncReader {
	return &mockBackfillAsyncReader{
		backfillBatches:        snapshotBatches,
		streamBatches:          streamBatches,
		backfillCompleteCalled: make(chan struct{}),
	}
}

func (r *mockBackfillAsyncReader) record(ev string) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}

func (r *mockBackfillAsyncReader) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *mockBackfillAsyncReader) ConnectionTest(ctx context.Context) component.ConnectionTestResults {
	return component.ConnectionTestNotSupported(mock.NewManager()).AsList()
}

func (r *mockBackfillAsyncReader) Connect(ctx context.Context) error { return nil }

func (r *mockBackfillAsyncReader) BackfillReadBatch(ctx context.Context) (message.Batch, input.AsyncAckFn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.backfillIdx >= len(r.backfillBatches) {
		return nil, nil, component.ErrBackfillComplete
	}
	payload := r.backfillBatches[r.backfillIdx]
	idx := r.backfillIdx
	r.backfillIdx++

	batch := message.Batch{message.NewPart(payload)}
	return batch, func(ctx context.Context, err error) error {
		r.record("snapshot-ack-" + string(rune('0'+idx)))
		return nil
	}, nil
}

func (r *mockBackfillAsyncReader) BackfillComplete(ctx context.Context) error {
	r.record("snapshot-complete")
	close(r.backfillCompleteCalled)
	return nil
}

func (r *mockBackfillAsyncReader) ReadBatch(ctx context.Context) (message.Batch, input.AsyncAckFn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.streamIdx >= len(r.streamBatches) {
		// Stall rather than end the stream, so the test controls the
		// component's lifecycle explicitly via TriggerStopConsuming.
		return nil, nil, component.ErrTimeout
	}
	payload := r.streamBatches[r.streamIdx]
	idx := r.streamIdx
	r.streamIdx++

	batch := message.Batch{message.NewPart(payload)}
	return batch, func(ctx context.Context, err error) error {
		r.record("stream-ack-" + string(rune('0'+idx)))
		return nil
	}, nil
}

func (r *mockBackfillAsyncReader) Close(ctx context.Context) error { return nil }

// TestAsyncReaderSnapshotPhaseBarrier proves AsyncReader waits for every
// snapshot batch to be acknowledged, even out of order, before calling
// BackfillComplete.
func TestAsyncReaderSnapshotPhaseBarrier(t *testing.T) {
	readerImpl := newMockBackfillAsyncReader(
		[][]byte{[]byte("snap-0"), []byte("snap-1")},
		[][]byte{[]byte("stream-0")},
	)

	r, err := input.NewAsyncReader("foo", readerImpl, mock.NewManager())
	require.NoError(t, err)
	r.TriggerStartConsuming()
	defer func() {
		r.TriggerStopConsuming()
		require.NoError(t, r.WaitForClose(t.Context()))
	}()

	ctx, done := context.WithTimeout(t.Context(), 10*time.Second)
	defer done()

	tran0, ok := <-r.TransactionChan()
	require.True(t, ok)

	tran1, ok := <-r.TransactionChan()
	require.True(t, ok)
	require.NoError(t, tran1.Ack(ctx, nil))

	select {
	case <-readerImpl.backfillCompleteCalled:
		t.Fatal("BackfillComplete fired while a snapshot batch was still un-acked")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, tran0.Ack(ctx, nil))

	select {
	case <-readerImpl.backfillCompleteCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("BackfillComplete never fired after all snapshot batches were acked")
	}

	// Proves streaming only begins once the snapshot phase has resolved.
	streamTran, ok := <-r.TransactionChan()
	require.True(t, ok)
	require.NoError(t, streamTran.Ack(ctx, nil))

	// ackFn runs in AsyncReader's own goroutine, not synchronously with
	// Ack(), so give it a moment to execute before asserting on events.
	require.Eventually(t, func() bool {
		return len(readerImpl.Events()) == 4
	}, 3*time.Second, 10*time.Millisecond)

	events := readerImpl.Events()
	require.Len(t, events, 4)
	assert.Equal(t, "snapshot-ack-1", events[0])
	assert.Equal(t, "snapshot-ack-0", events[1])
	assert.Equal(t, "snapshot-complete", events[2])
	assert.Equal(t, "stream-ack-0", events[3])
}

// TestAsyncReaderSnapshotPhaseSkippedWhenEmpty proves BackfillComplete still
// fires once, immediately, for a reader with an empty snapshot phase.
func TestAsyncReaderSnapshotPhaseSkippedWhenEmpty(t *testing.T) {
	readerImpl := newMockBackfillAsyncReader(nil, [][]byte{[]byte("stream-0")})

	r, err := input.NewAsyncReader("foo", readerImpl, mock.NewManager())
	require.NoError(t, err)
	r.TriggerStartConsuming()
	defer func() {
		r.TriggerStopConsuming()
		require.NoError(t, r.WaitForClose(t.Context()))
	}()

	ctx, done := context.WithTimeout(t.Context(), 10*time.Second)
	defer done()

	select {
	case <-readerImpl.backfillCompleteCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("BackfillComplete never fired for an empty backfill phase")
	}

	streamTran, ok := <-r.TransactionChan()
	require.True(t, ok)
	require.NoError(t, streamTran.Ack(ctx, nil))

	require.Eventually(t, func() bool {
		return len(readerImpl.Events()) == 2
	}, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{"backfill-complete", "stream-ack-0"}, readerImpl.Events())
}

// TestAsyncReaderBackfillPhaseAbortsOnHardStopBeforeAck proves a hard stop
// with an un-acked backfill batch in flight must not let BackfillComplete
// fire, even though the hard stop also unblocks the ack wait.
func TestAsyncReaderBackfillPhaseAbortsOnHardStopBeforeAck(t *testing.T) {
	readerImpl := newMockBackfillAsyncReader(
		[][]byte{[]byte("snap-0")},
		nil,
	)

	r, err := input.NewAsyncReader("foo", readerImpl, mock.NewManager())
	require.NoError(t, err)
	r.TriggerStartConsuming()

	tran, ok := <-r.TransactionChan()
	require.True(t, ok)
	_ = tran // deliberately never acked - simulates a crash before the ack arrives.

	r.TriggerCloseNow()
	require.NoError(t, r.WaitForClose(context.Background()))

	select {
	case <-readerImpl.backfillCompleteCalled:
		t.Fatal("BackfillComplete must not fire when a backfill batch was cut short by a hard stop instead of being genuinely acknowledged")
	default:
	}
}
