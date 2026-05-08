// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// TestBatchAutoRetryReadAheadOrdering is a regression test for the read-ahead
// reordering that occurs when an AsyncReader-wrapped batch input with
// auto_replay_nacks is paired with a downstream that processes batches
// sequentially. The expected behavior is that when a batch is nacked, its
// retry is presented to the downstream BEFORE any batch the AsyncReader has
// already read but not yet pushed past the channel pipeline. The current code
// fails this — AsyncReader pushes a fresh-read batch into the unbuffered
// transactions channel without checking for an incoming retry, so the retry
// of the nacked batch ends up scheduled behind the fresh read.
//
// Scenario:
//
//  1. Mock batch input yields batches A, then B, then blocks.
//  2. AsyncReader iter 1 reads A, pushes to transactions channel.
//  3. Test pulls A. AsyncReader iter 2 reads B, blocks pushing to channel.
//  4. Test acks A with an error → autoretry queues A for retry.
//  5. Test pulls next from transactions channel.
//
// With the current bug, step 5 produces B (the read-ahead won the race against
// the retry-queue signal). With the splicing fix, step 5 produces A (the
// retry preempts the in-flight fresh push).
func TestBatchAutoRetryReadAheadOrdering(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	readerImpl := newMockBatchInput()
	readerImpl.msgsToSnd = []MessageBatch{
		{NewMessage([]byte("A"))},
		{NewMessage([]byte("B"))},
	}

	// Wrap with autoretry, then bridge to internal Async, then AsyncReader.
	pres := AutoRetryNacksBatched(readerImpl)
	nm := mock.NewManager()
	rdr := newAirGapBatchReader(nm, pres)
	asyncReader, err := input.NewAsyncReader("test", rdr, nm)
	require.NoError(t, err)

	asyncReader.TriggerStartConsuming()

	// Drive the mock: connect, then yield A, then yield B.  Subsequent
	// ReadBatch calls block on readChan because we never send again.
	go func() {
		select {
		case readerImpl.connChan <- nil:
		case <-ctx.Done():
			return
		}
		select {
		case readerImpl.readChan <- nil: // returns batch A
		case <-ctx.Done():
			return
		}
		select {
		case readerImpl.readChan <- nil: // returns batch B
		case <-ctx.Done():
			return
		}
	}()

	// Drain the mock's ack channel in a goroutine: the underlying ackFn for a
	// batch is only invoked by autoretry when the batch is finally acked
	// successfully (via wrapPendingAck with err == nil). Each successful ack
	// blocks on this chan, so we keep it drained.
	t.Cleanup(func() { go drainAckChan(readerImpl.ackChan) })
	t.Cleanup(func() { go drainCloseChan(readerImpl.closeChan) })
	t.Cleanup(asyncReader.TriggerCloseNow)

	txnCh := asyncReader.TransactionChan()

	// Pull batch A.
	tranA := pullTxn(t, ctx, txnCh)
	require.Equal(t, "A", payloadString(t, tranA), "first batch should be A")

	// Wait briefly for AsyncReader to advance into iter 2 and block pushing
	// batch B onto the unbuffered transactions channel.  This is the timing
	// window where the read-ahead reordering becomes observable.
	time.Sleep(200 * time.Millisecond)

	// Ack A with an error → autoretry queues A for retry.
	require.NoError(t, tranA.Ack(ctx, errors.New("transient")))

	// Give autoretry's wrapPendingAck a moment to run and append A to its
	// pendingRetry list.
	time.Sleep(200 * time.Millisecond)

	// Pull the next batch.  Per the contract that a queued retry should
	// preempt an in-flight fresh batch, this must be A's retry, not B.
	tranNext := pullTxn(t, ctx, txnCh)
	got := payloadString(t, tranNext)

	assert.Equal(t, "A", got,
		"expected A's retry to be delivered before fresh B; got %q "+
			"(read-ahead reordering: AsyncReader pushed batch B onto the unbuffered "+
			"transactions channel before the retry of A was queued in autoretry, "+
			"so the retry is now scheduled BEHIND B)", got)

	// Ack the retry success so autoretry releases A's underlying ack and the
	// stream can drain.
	require.NoError(t, tranNext.Ack(ctx, nil))

	// Pull and ack B so we exit cleanly.
	tranB := pullTxn(t, ctx, txnCh)
	assert.Equal(t, "B", payloadString(t, tranB), "third delivery should be B")
	require.NoError(t, tranB.Ack(ctx, nil))
}

func pullTxn(t *testing.T, ctx context.Context, ch <-chan message.Transaction) message.Transaction {
	t.Helper()
	select {
	case tran := <-ch:
		return tran
	case <-ctx.Done():
		t.Fatalf("timed out pulling next transaction: %v", ctx.Err())
		return message.Transaction{}
	}
}

func payloadString(t *testing.T, tran message.Transaction) string {
	t.Helper()
	require.Equal(t, 1, tran.Payload.Len(), "expected single-record batch")
	return string(tran.Payload[0].AsBytes())
}

func drainAckChan(ch chan error) {
	for {
		select {
		case ch <- nil:
		case <-time.After(2 * time.Second):
			return
		}
	}
}

func drainCloseChan(ch chan error) {
	select {
	case ch <- nil:
	case <-time.After(2 * time.Second):
	}
}
