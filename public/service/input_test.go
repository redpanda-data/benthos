// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

type fnInput struct {
	connect func() error
	read    func() (*Message, AckFunc, error)
	closed  bool
}

func (f *fnInput) Connect(ctx context.Context) error {
	return f.connect()
}

func (f *fnInput) Read(ctx context.Context) (*Message, AckFunc, error) {
	return f.read()
}

func (f *fnInput) Close(ctx context.Context) error {
	f.closed = true
	return nil
}

func TestInputAirGapShutdown(t *testing.T) {
	i := &fnInput{}
	agi := newAirGapReader(mock.NewManager(), i)

	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	require.NoError(t, agi.Close(ctx))
	assert.True(t, i.closed)
}

func TestInputAirGapSad(t *testing.T) {
	i := &fnInput{
		connect: func() error {
			return errors.New("bad connect")
		},
		read: func() (*Message, AckFunc, error) {
			return nil, nil, errors.New("bad read")
		},
	}
	agi := newAirGapReader(mock.NewManager(), i)

	err := agi.Connect(t.Context())
	assert.EqualError(t, err, "bad connect")

	_, _, err = agi.ReadBatch(t.Context())
	assert.EqualError(t, err, "bad read")

	i.read = func() (*Message, AckFunc, error) {
		return nil, nil, ErrNotConnected
	}

	_, _, err = agi.ReadBatch(t.Context())
	assert.Equal(t, component.ErrNotConnected, err)

	i.read = func() (*Message, AckFunc, error) {
		return nil, nil, ErrEndOfInput
	}

	_, _, err = agi.ReadBatch(t.Context())
	assert.Equal(t, component.ErrTypeClosed, err)
}

func TestInputAirGapHappy(t *testing.T) {
	var ackErr error
	ackFn := func(ctx context.Context, err error) error {
		ackErr = err
		return nil
	}
	i := &fnInput{
		connect: func() error {
			return nil
		},
		read: func() (*Message, AckFunc, error) {
			m := &Message{
				part: message.NewPart([]byte("hello world")),
			}
			return m, ackFn, nil
		},
	}
	agi := newAirGapReader(mock.NewManager(), i)

	err := agi.Connect(t.Context())
	assert.NoError(t, err)

	outMsg, outAckFn, err := agi.ReadBatch(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 1, outMsg.Len())
	assert.Equal(t, "hello world", string(outMsg.Get(0).AsBytes()))

	assert.NoError(t, outAckFn(t.Context(), errors.New("foobar")))
	assert.EqualError(t, ackErr, "foobar")
}

type fnBatchInput struct {
	connect func() error
	read    func() (MessageBatch, AckFunc, error)
	closed  bool
}

func (f *fnBatchInput) Connect(ctx context.Context) error {
	return f.connect()
}

func (f *fnBatchInput) ReadBatch(ctx context.Context) (MessageBatch, AckFunc, error) {
	return f.read()
}

func (f *fnBatchInput) Close(ctx context.Context) error {
	f.closed = true
	return nil
}

func TestBatchInputAirGapShutdown(t *testing.T) {
	i := &fnBatchInput{}
	agi := newAirGapBatchReader(mock.NewManager(), i)

	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	require.NoError(t, agi.Close(ctx))
	assert.True(t, i.closed)
}

func TestBatchInputAirGapSad(t *testing.T) {
	i := &fnBatchInput{
		connect: func() error {
			return errors.New("bad connect")
		},
		read: func() (MessageBatch, AckFunc, error) {
			return nil, nil, errors.New("bad read")
		},
	}
	agi := newAirGapBatchReader(mock.NewManager(), i)

	err := agi.Connect(t.Context())
	assert.EqualError(t, err, "bad connect")

	_, _, err = agi.ReadBatch(t.Context())
	assert.EqualError(t, err, "bad read")

	i.read = func() (MessageBatch, AckFunc, error) {
		return nil, nil, ErrNotConnected
	}

	_, _, err = agi.ReadBatch(t.Context())
	assert.Equal(t, component.ErrNotConnected, err)

	i.read = func() (MessageBatch, AckFunc, error) {
		return nil, nil, ErrEndOfInput
	}

	_, _, err = agi.ReadBatch(t.Context())
	assert.Equal(t, component.ErrTypeClosed, err)
}

func TestBatchInputAirGapSadWithBackOff(t *testing.T) {
	i := &fnBatchInput{
		connect: func() error {
			return NewErrBackOff(errors.New("bad connect"), time.Second*2)
		},
		read: func() (MessageBatch, AckFunc, error) {
			return nil, nil, NewErrBackOff(errors.New("bad read"), time.Second*3)
		},
	}
	agi := newAirGapBatchReader(mock.NewManager(), i)

	err := agi.Connect(t.Context())
	assert.EqualError(t, err, "bad connect")

	var e *component.ErrBackOff
	assert.ErrorAs(t, err, &e)
	assert.Equal(t, time.Second*2, e.Wait)
	assert.EqualError(t, e.Err, "bad connect")
	assert.Equal(t, "bad connect", e.Error())

	_, _, err = agi.ReadBatch(t.Context())
	assert.EqualError(t, err, "bad read")

	assert.ErrorAs(t, err, &e)
	assert.Equal(t, time.Second*3, e.Wait)
	assert.EqualError(t, e.Err, "bad read")
	assert.Equal(t, "bad read", e.Error())

	i.read = func() (MessageBatch, AckFunc, error) {
		return nil, nil, NewErrBackOff(ErrNotConnected, time.Second*2)
	}

	_, _, err = agi.ReadBatch(t.Context())

	assert.ErrorAs(t, err, &e)
	assert.Equal(t, time.Second*2, e.Wait)
	assert.ErrorIs(t, e.Err, component.ErrNotConnected)

	i.read = func() (MessageBatch, AckFunc, error) {
		return nil, nil, ErrEndOfInput
	}

	_, _, err = agi.ReadBatch(t.Context())
	assert.Equal(t, component.ErrTypeClosed, err)
}

func TestBatchInputAirGapHappy(t *testing.T) {
	var ackErr error
	ackFn := func(ctx context.Context, err error) error {
		ackErr = err
		return nil
	}
	i := &fnBatchInput{
		connect: func() error {
			return nil
		},
		read: func() (MessageBatch, AckFunc, error) {
			m := MessageBatch{
				NewMessage([]byte("hello world")),
				NewMessage([]byte("this is a test message")),
				NewMessage([]byte("and it will work")),
			}
			return m, ackFn, nil
		},
	}
	agi := newAirGapBatchReader(mock.NewManager(), i)

	err := agi.Connect(t.Context())
	assert.NoError(t, err)

	outMsg, outAckFn, err := agi.ReadBatch(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 3, outMsg.Len())
	assert.Equal(t, "hello world", string(outMsg.Get(0).AsBytes()))
	assert.Equal(t, "this is a test message", string(outMsg.Get(1).AsBytes()))
	assert.Equal(t, "and it will work", string(outMsg.Get(2).AsBytes()))

	assert.NoError(t, outAckFn(t.Context(), errors.New("foobar")))
	assert.EqualError(t, ackErr, "foobar")
}

type fnSnapshotBatchInput struct {
	*fnBatchInput
	snapshotRead     func() (MessageBatch, AckFunc, error)
	snapshotComplete func() error
}

func (f *fnSnapshotBatchInput) BackfillReadBatch(ctx context.Context) (MessageBatch, AckFunc, error) {
	return f.snapshotRead()
}

func (f *fnSnapshotBatchInput) BackfillComplete(ctx context.Context) error {
	return f.snapshotComplete()
}

func TestBatchInputAirGapSnapshotHappy(t *testing.T) {
	var snapshotAckErr error
	snapshotAckFn := func(ctx context.Context, err error) error {
		snapshotAckErr = err
		return nil
	}

	snapshotCompleteCalled := false
	i := &fnSnapshotBatchInput{
		fnBatchInput: &fnBatchInput{
			connect: func() error { return nil },
		},
		snapshotRead: func() (MessageBatch, AckFunc, error) {
			return MessageBatch{NewMessage([]byte("snapshot row"))}, snapshotAckFn, nil
		},
		snapshotComplete: func() error {
			snapshotCompleteCalled = true
			return nil
		},
	}

	agi := newAirGapBatchReader(mock.NewManager(), i)

	sa, ok := agi.(input.BackfillAsync)
	require.True(t, ok, "airGapBatchReader must implement input.BackfillAsync when the wrapped BatchInput implements SnapshotBatchInput")

	outMsg, outAckFn, err := sa.BackfillReadBatch(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, outMsg.Len())
	assert.Equal(t, "snapshot row", string(outMsg.Get(0).AsBytes()))

	assert.NoError(t, outAckFn(t.Context(), errors.New("snapshot nack")))
	assert.EqualError(t, snapshotAckErr, "snapshot nack")

	i.snapshotRead = func() (MessageBatch, AckFunc, error) {
		return nil, nil, ErrBackfillComplete
	}
	_, _, err = sa.BackfillReadBatch(t.Context())
	assert.Equal(t, component.ErrBackfillComplete, err)

	sc, ok := agi.(input.BackfillCompleter)
	require.True(t, ok, "airGapBatchReader must implement input.BackfillCompleter when the wrapped BatchInput implements BackfillCompleter")
	require.NoError(t, sc.BackfillComplete(t.Context()))
	assert.True(t, snapshotCompleteCalled)
}

func TestBatchInputAirGapWithoutSnapshot(t *testing.T) {
	i := &fnBatchInput{
		connect: func() error { return nil },
	}
	agi := newAirGapBatchReader(mock.NewManager(), i)

	_, ok := agi.(input.BackfillAsync)
	assert.False(t, ok, "airGapBatchReader must not implement input.BackfillAsync when the wrapped BatchInput has no snapshot phase")
}
