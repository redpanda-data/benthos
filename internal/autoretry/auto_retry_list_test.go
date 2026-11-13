// Copyright 2025 Redpanda Data, Inc.

package autoretry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCustomEOF = errors.New("custom EOF")

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

func TestRetryListCancelledDuringBackoffKeepsRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
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
		}, nil)

		v, fooFn, err := l.Shift(tCtx, true)
		require.NoError(t, err)
		assert.Equal(t, "foo", v)

		_, _, err = l.Shift(tCtx, true)
		require.Equal(t, errCustomEOF, err)

		assert.NoError(t, fooFn(tCtx, errors.New("foo nope")))

		// Fast forward the pending retry into its backoff phase with a
		// backoff long enough that the next shift is still waiting when its
		// context expires.
		l.cond.L.Lock()
		require.Len(t, l.pendingRetry, 1)
		l.pendingRetry[0].attempts = 2
		l.pendingRetry[0].boff = backoff.NewConstantBackOff(time.Millisecond * 200)
		l.cond.L.Unlock()

		// Each finished shift leaves a goroutine that takes the lock to
		// broadcast. Let those exit before a shift sleeps in its backoff with
		// the lock held: a goroutine waiting on a mutex is not durably
		// blocked, so the bubble's clock would never advance.
		synctest.Wait()

		cancelledCtx, cancelledDone := context.WithTimeout(tCtx, time.Millisecond*20)
		defer cancelledDone()

		_, _, err = l.Shift(cancelledCtx, false)
		assert.Equal(t, cancelledCtx.Err(), err)

		synctest.Wait()

		// The message must be back in the queue, and the cancelled attempt
		// must not count as one.
		l.cond.L.Lock()
		require.Len(t, l.pendingRetry, 1)
		assert.Equal(t, 2, l.pendingRetry[0].attempts)
		l.cond.L.Unlock()

		// The next shift only waits out the rest of the backoff.
		start := time.Now()
		v, fooFn, err = l.Shift(tCtx, false)
		require.NoError(t, err)
		assert.Equal(t, "foo", v)
		assert.Equal(t, time.Millisecond*180, time.Since(start))

		assert.NoError(t, fooFn(tCtx, nil))
		assert.Equal(t, []string{"foo"}, acked)

		_, _, err = l.Shift(tCtx, false)
		assert.Equal(t, ErrExhausted, err)

		require.NoError(t, l.Close(tCtx))
	})
}

func TestRetryListCancelledDuringBackoffKeepsPendingRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tCtx, done := context.WithTimeout(t.Context(), time.Second*5)
		defer done()

		var acked []string

		data := []string{"foo", "bar"}
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

		assert.NoError(t, fooFn(tCtx, errors.New("foo nope")))

		// Shifting the retry with reads enabled dispatches a read of bar,
		// which is stored as the pending read.
		v, fooFn, err = l.Shift(tCtx, true)
		require.NoError(t, err)
		assert.Equal(t, "foo", v)

		synctest.Wait()
		l.cond.L.Lock()
		require.NotNil(t, l.pendingRead)
		assert.Equal(t, "bar", l.pendingRead.t)
		l.cond.L.Unlock()

		assert.NoError(t, fooFn(tCtx, errors.New("foo nope again")))

		l.cond.L.Lock()
		require.Len(t, l.pendingRetry, 1)
		l.pendingRetry[0].attempts = 2
		l.pendingRetry[0].boff = backoff.NewConstantBackOff(time.Millisecond * 200)
		l.cond.L.Unlock()

		// A shift whose context expires during the backoff must return the
		// context error rather than the pending read.
		// Each finished shift leaves a goroutine that takes the lock to
		// broadcast. Let those exit before a shift sleeps in its backoff with
		// the lock held: a goroutine waiting on a mutex is not durably
		// blocked, so the bubble's clock would never advance.
		synctest.Wait()

		cancelledCtx, cancelledDone := context.WithTimeout(tCtx, time.Millisecond*20)
		defer cancelledDone()

		_, _, err = l.Shift(cancelledCtx, true)
		assert.Equal(t, cancelledCtx.Err(), err)

		synctest.Wait()

		// The retry comes first, then the read.
		v, fooFn, err = l.Shift(tCtx, false)
		require.NoError(t, err)
		assert.Equal(t, "foo", v)

		v, barFn, err := l.Shift(tCtx, false)
		require.NoError(t, err)
		assert.Equal(t, "bar", v)

		assert.NoError(t, fooFn(tCtx, nil))
		assert.NoError(t, barFn(tCtx, nil))
		assert.Equal(t, []string{"foo", "bar"}, acked)

		_, _, err = l.Shift(tCtx, false)
		assert.Equal(t, ErrExhausted, err)

		require.NoError(t, l.Close(tCtx))
	})
}
