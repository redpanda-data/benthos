// Copyright 2025 Redpanda Data, Inc.

package autoretry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCustomEOF = errors.New("custom EOF")

// TestRetryListNotifyAndTryShift pins down the contract of RetryNotifyChan
// and TryShiftRetry: notify fires (coalesced) when a fresh retry is queued,
// never fires on a successful ack, and TryShiftRetry returns retries without
// ever dispatching a fresh read or blocking.
func TestRetryListNotifyAndTryShift(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second)
	defer done()

	var readCalls int
	data := []string{"foo", "bar"}
	l := NewList(func(ctx context.Context) (t string, aFn AckFunc, err error) {
		readCalls++
		if len(data) == 0 {
			err = errCustomEOF
			return
		}
		t = data[0]
		data = data[1:]
		aFn = func(context.Context, error) error { return nil }
		return
	}, nil)

	notify := l.RetryNotifyChan()

	assertNoSignal := func() {
		t.Helper()
		select {
		case <-notify:
			t.Fatal("unexpected retry-notify signal")
		default:
		}
	}
	assertSignal := func() {
		t.Helper()
		select {
		case <-notify:
		case <-tCtx.Done():
			t.Fatal("expected retry-notify signal, got timeout")
		}
	}

	// Empty state: no signal, no retry, no fresh read dispatched.
	assertNoSignal()
	_, _, ok := l.TryShiftRetry(tCtx)
	assert.False(t, ok, "TryShiftRetry returned a retry before any was queued")
	assert.Equal(t, 0, readCalls, "TryShiftRetry must not dispatch a fresh read")

	// Take "foo" off the underlying reader, then nack it. The notify channel
	// must fire and the retry must be drainable via TryShiftRetry.
	v, fooFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	require.Equal(t, "foo", v)

	require.NoError(t, fooFn(tCtx, errors.New("transient")))

	assertSignal()
	assertNoSignal() // single-shot — coalesced

	got, fooRetryFn, ok := l.TryShiftRetry(tCtx)
	require.True(t, ok, "TryShiftRetry returned no retry despite one being queued")
	assert.Equal(t, "foo", got)

	_, _, ok = l.TryShiftRetry(tCtx)
	assert.False(t, ok, "queue must be empty after drain")

	// Successfully ack the retried foo. That must NOT fire notify.
	require.NoError(t, fooRetryFn(tCtx, nil))
	assertNoSignal()

	// Coalescing: queue two retries before the consumer drains the signal.
	// Re-issue Shift for "bar" (the next fresh value), then nack foo (already
	// drained) is impossible — instead nack bar, then nack a second freshly
	// shifted value. We have only "bar" left in data, so use a single nack to
	// confirm coalescing-with-an-already-pending-signal: queue a retry, then
	// queue another via the same path while the channel still has a signal.
	v, barFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	require.Equal(t, "bar", v)
	require.NoError(t, barFn(tCtx, errors.New("transient")))

	// The second nack would attempt a non-blocking send into a buffered (size
	// 1) channel that already has a value — verify the send doesn't block by
	// using a separate retry path. Re-take bar, nack again.
	got, barRetry1, ok := l.TryShiftRetry(tCtx)
	require.True(t, ok)
	require.Equal(t, "bar", got)
	require.NoError(t, barRetry1(tCtx, errors.New("transient2")))

	// Drain at least one signal (could be 1 or 2 depending on timing — the
	// contract is "fires when one or more retries become available since last
	// drain").
	assertSignal()
	// Drain remaining if any, then assert empty.
	select {
	case <-notify:
	default:
	}
	assertNoSignal()

	got, barRetry2, ok := l.TryShiftRetry(tCtx)
	require.True(t, ok, "second nack of bar must be retrievable")
	assert.Equal(t, "bar", got)
	require.NoError(t, barRetry2(tCtx, nil))
	assertNoSignal()
}

func TestRetryListAllAcks(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second)
	defer done()

	var acked []string

	data := []string{"foo", "bar", "baz"}
	l := NewList(func(ctx context.Context) (t string, aFn AckFunc, err error) {
		if len(data) == 0 {
			err = errCustomEOF
			return
		}
		next := data[0]
		data = data[1:]
		return next, func(ctx context.Context, err error) error {
			acked = append(acked, next)
			return nil
		}, nil
	}, nil)

	res, fooFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "foo", res)

	res, barFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "bar", res)

	res, bazFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "baz", res)

	_, _, err = l.Shift(tCtx, true)
	require.Equal(t, errCustomEOF, err)

	assert.NoError(t, bazFn(tCtx, nil))
	assert.NoError(t, barFn(tCtx, nil))
	assert.NoError(t, fooFn(tCtx, nil))

	assert.Equal(t, []string{
		"baz", "bar", "foo",
	}, acked)

	fmt.Println("last shift")
	_, _, err = l.Shift(tCtx, false)
	assert.Equal(t, ErrExhausted, err)

	require.NoError(t, l.Close(tCtx))
}

func TestRetryListNacks(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second)
	defer done()

	var acked []string

	data := []string{"foo", "bar", "baz"}
	l := NewList(func(ctx context.Context) (t string, aFn AckFunc, err error) {
		if len(data) == 0 {
			err = errCustomEOF
			return
		}
		next := data[0]
		data = data[1:]
		return next, func(ctx context.Context, err error) error {
			acked = append(acked, next)
			return nil
		}, nil
	}, nil)

	v, fooFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "foo", v)

	v, barFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "bar", v)

	v, bazFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "baz", v)

	_, _, err = l.Shift(tCtx, true)
	require.Equal(t, errCustomEOF, err)

	assert.NoError(t, bazFn(tCtx, errors.New("baz nope")))
	assert.NoError(t, barFn(tCtx, errors.New("bar nope")))
	assert.NoError(t, fooFn(tCtx, errors.New("foo nope")))

	assert.Equal(t, []string(nil), acked)

	v, bazFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "baz", v)

	v, barFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "bar", v)
	assert.NoError(t, barFn(tCtx, errors.New("bar nope again")))

	v, fooFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "foo", v)

	assert.NoError(t, fooFn(tCtx, nil))
	assert.NoError(t, bazFn(tCtx, nil))

	assert.Equal(t, []string{
		"foo", "baz",
	}, acked)

	v, barFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "bar", v)

	cancelledCtx, done := context.WithTimeout(tCtx, time.Millisecond*50)
	defer done()

	_, _, err = l.Shift(cancelledCtx, false)
	assert.Equal(t, cancelledCtx.Err(), err)

	assert.NoError(t, barFn(tCtx, nil))

	assert.Equal(t, []string{
		"foo", "baz", "bar",
	}, acked)

	_, _, err = l.Shift(tCtx, false)
	assert.Equal(t, ErrExhausted, err)

	require.NoError(t, l.Close(tCtx))
}

func TestRetryListNackMutator(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second)
	defer done()

	var acked []string

	data := []string{"foo"}
	l := NewList(func(ctx context.Context) (t string, aFn AckFunc, err error) {
		if len(data) == 0 {
			err = errCustomEOF
			return
		}
		next := data[0]
		data = data[1:]
		return next, func(ctx context.Context, err error) error {
			acked = append(acked, next)
			return nil
		}, nil
	}, func(t string, err error) string {
		return t + " and " + err.Error()
	})

	v, fooFn, err := l.Shift(tCtx, true)
	require.NoError(t, err)
	assert.Equal(t, "foo", v)

	_, _, err = l.Shift(tCtx, true)
	require.Equal(t, errCustomEOF, err)

	assert.NoError(t, fooFn(tCtx, errors.New("first error")))
	assert.Equal(t, []string(nil), acked)

	v, fooFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "foo and first error", v)

	assert.NoError(t, fooFn(tCtx, errors.New("second error")))
	assert.Equal(t, []string(nil), acked)

	v, fooFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "foo and first error and second error", v)

	assert.NoError(t, fooFn(tCtx, errors.New("third error")))
	assert.Equal(t, []string(nil), acked)

	v, fooFn, err = l.Shift(tCtx, false)
	require.NoError(t, err)
	assert.Equal(t, "foo and first error and second error and third error", v)

	assert.NoError(t, fooFn(tCtx, nil))

	assert.Equal(t, []string{
		"foo",
	}, acked)

	_, _, err = l.Shift(tCtx, false)
	assert.Equal(t, ErrExhausted, err)

	require.NoError(t, l.Close(tCtx))
}
