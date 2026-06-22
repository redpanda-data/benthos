// Copyright 2026 Redpanda Data, Inc.

package output

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/batch"
	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// recordingWriter is an always-connected AsyncSink that records the content of
// every batch it is asked to write.
type recordingWriter struct {
	mu       sync.Mutex
	written  [][]string
	writeErr error
}

func (w *recordingWriter) ConnectionTest(context.Context) component.ConnectionTestResults {
	return nil
}
func (w *recordingWriter) Connect(context.Context) error { return nil }

func (w *recordingWriter) WriteBatch(_ context.Context, msg message.Batch) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var got []string
	_ = msg.Iter(func(_ int, p *message.Part) error {
		got = append(got, string(p.AsBytes()))
		return nil
	})
	w.written = append(w.written, got)
	return w.writeErr
}
func (w *recordingWriter) Close(context.Context) error { return nil }

func (w *recordingWriter) writes() [][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([][]string, len(w.written))
	copy(out, w.written)
	return out
}

func sendForAck(t *testing.T, tChan chan message.Transaction, b message.Batch) error {
	t.Helper()
	resChan := make(chan error)
	select {
	case tChan <- message.NewTransaction(b, resChan):
	case <-time.After(time.Second):
		t.Fatal("timed out sending transaction")
	}
	select {
	case err := <-resChan:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ack")
	}
	return nil
}

func startWriter(t *testing.T, strict bool, w AsyncSink) (Streamed, chan message.Transaction) {
	t.Helper()
	aw, err := NewAsyncWriter("foo", 1, strict, w, component.NoopObservability())
	require.NoError(t, err)
	tChan := make(chan message.Transaction)
	require.NoError(t, aw.Consume(tChan))
	aw.TriggerStartConsuming()
	t.Cleanup(func() {
		aw.TriggerCloseNow()
		ctx, done := context.WithTimeout(context.Background(), time.Second*5)
		defer done()
		_ = aw.WaitForClose(ctx)
	})
	return aw, tChan
}

// In strict mode a mixed batch writes only the messages that did not fail, and
// the failed message is nacked via a batch error.
func TestAsyncWriterStrictRejectsMixedBatch(t *testing.T) {
	w := &recordingWriter{}
	_, tChan := startWriter(t, true, w)

	b := message.QuickBatch([][]byte{[]byte("a"), []byte("b"), []byte("c")})
	b.Get(1).ErrorSet(errors.New("boom"))

	ackErr := sendForAck(t, tChan, b)

	require.Error(t, ackErr, "the failed message must be nacked")
	var bErr *batch.Error
	require.ErrorAs(t, ackErr, &bErr)
	assert.Equal(t, 1, bErr.IndexedErrors(), "exactly one message rejected")

	assert.Equal(t, [][]string{{"a", "c"}}, w.writes(), "only the non-failed messages are written")
}

// In strict mode, when the write of the surviving (non-failed) messages itself
// fails, that failure is merged with the rejections so the whole batch is
// nacked.
func TestAsyncWriterStrictMixedBatchWriteFails(t *testing.T) {
	w := &recordingWriter{writeErr: errors.New("sink unavailable")}
	_, tChan := startWriter(t, true, w)

	b := message.QuickBatch([][]byte{[]byte("a"), []byte("b"), []byte("c")})
	b.Get(1).ErrorSet(errors.New("boom")) // rejected by processing

	ackErr := sendForAck(t, tChan, b)

	require.Error(t, ackErr)
	var bErr *batch.Error
	require.ErrorAs(t, ackErr, &bErr)
	assert.Equal(t, 3, bErr.IndexedErrors(), "all three messages nacked: one rejected, two failed to write")
	assert.Equal(t, [][]string{{"a", "c"}}, w.writes(), "only the non-rejected messages were attempted")
}

// In strict mode a batch in which every message has failed is nacked without
// any write taking place.
func TestAsyncWriterStrictRejectsWholeBatch(t *testing.T) {
	w := &recordingWriter{}
	_, tChan := startWriter(t, true, w)

	b := message.QuickBatch([][]byte{[]byte("a"), []byte("b")})
	_ = b.Iter(func(_ int, p *message.Part) error {
		p.ErrorSet(errors.New("boom"))
		return nil
	})

	ackErr := sendForAck(t, tChan, b)

	require.Error(t, ackErr)
	assert.Empty(t, w.writes(), "no write should occur when every message failed")
}

// Without strict mode, a message that arrives flagged as failed is still written
// (the historical behaviour) and the batch is acked.
func TestAsyncWriterNonStrictWritesErrored(t *testing.T) {
	w := &recordingWriter{}
	_, tChan := startWriter(t, false, w)

	b := message.QuickBatch([][]byte{[]byte("a"), []byte("b")})
	b.Get(0).ErrorSet(errors.New("boom"))

	ackErr := sendForAck(t, tChan, b)

	require.NoError(t, ackErr)
	assert.Equal(t, [][]string{{"a", "b"}}, w.writes(), "without strict the errored message is still written")
}
