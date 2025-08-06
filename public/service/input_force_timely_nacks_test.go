// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchTimelyNacksConfig(t *testing.T) {
	spec := NewConfigSpec().Field(NewAutoRetryNacksToggleField())
	for conf, hasMaxWait := range map[string]bool{
		`{}`:                            false,
		`timely_nacks_maximum_wait: 0s`: false,
		`timely_nacks_maximum_wait: 1s`: true,
	} {
		inConf, err := spec.ParseYAML(conf, nil)
		require.NoError(t, err, conf)

		readerImpl := newMockBatchInput()
		pres, err := ForceTimelyNacksBatched(inConf, readerImpl)
		require.NoError(t, err, conf)

		_, isWrapped := pres.(*forceTimelyNacksInputBatched)
		assert.Equal(t, hasMaxWait, isWrapped, conf)
	}
}

func TestBatchTimelyNacksClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	spec := NewConfigSpec().Field(NewForceTimelyNacksField())

	inConf, err := spec.ParseYAML(`timely_nacks_maximum_wait: 1s`, nil)
	require.NoError(t, err)

	readerImpl := newMockBatchInput()
	pres, err := ForceTimelyNacksBatched(inConf, readerImpl)
	require.NoError(t, err)

	expErr := errors.New("foo error")

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		err := pres.Connect(ctx)
		require.NoError(t, err)

		assert.Equal(t, expErr, pres.Close(ctx))
	}()

	select {
	case readerImpl.connChan <- nil:
	case <-time.After(time.Second):
		t.Error("Timed out")
	}

	select {
	case readerImpl.closeChan <- expErr:
	case <-time.After(time.Second):
		t.Error("Timed out")
	}

	wg.Wait()
}

func TestForceTimelyNacksBatchedHappy(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	readerImpl := newMockBatchInput()
	readerImpl.msgsToSnd = append(readerImpl.msgsToSnd, MessageBatch{
		NewMessage([]byte("foo")),
		NewMessage([]byte("bar")),
	})

	spec := NewConfigSpec().Field(NewForceTimelyNacksField())

	inConf, err := spec.ParseYAML(`timely_nacks_maximum_wait: 1s`, nil)
	require.NoError(t, err)

	pres, err := ForceTimelyNacksBatched(inConf, readerImpl)
	require.NoError(t, err)

	go func() {
		select {
		case readerImpl.connChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
		select {
		case readerImpl.readChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
		select {
		case readerImpl.ackChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
	}()

	require.NoError(t, pres.Connect(ctx))

	batch, ackFn, err := pres.ReadBatch(ctx)
	require.NoError(t, err)
	require.Len(t, batch, 2)

	act, err := batch[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "foo", string(act))

	act, err = batch[1].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "bar", string(act))

	require.NoError(t, ackFn(ctx, nil))

	readerImpl.ackRcvdMut.Lock()
	assert.Len(t, readerImpl.ackRcvd, 1)
	assert.NoError(t, readerImpl.ackRcvd[0])
	readerImpl.ackRcvdMut.Unlock()
}

func TestForceTimelyNacksBatchedNoAck(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	readerImpl := newMockBatchInput()
	readerImpl.msgsToSnd = append(readerImpl.msgsToSnd, MessageBatch{
		NewMessage([]byte("foo")),
		NewMessage([]byte("bar")),
	})

	spec := NewConfigSpec().Field(NewForceTimelyNacksField())

	inConf, err := spec.ParseYAML(`timely_nacks_maximum_wait: 5ms`, nil)
	require.NoError(t, err)

	pres, err := ForceTimelyNacksBatched(inConf, readerImpl)
	require.NoError(t, err)

	go func() {
		select {
		case readerImpl.connChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
		select {
		case readerImpl.readChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
		select {
		case readerImpl.ackChan <- nil:
		case <-time.After(time.Second):
			t.Error("Timed out")
		}
	}()

	require.NoError(t, pres.Connect(ctx))

	batch, ackFn, err := pres.ReadBatch(ctx)
	require.NoError(t, err)
	require.Len(t, batch, 2)

	act, err := batch[0].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "foo", string(act))

	act, err = batch[1].AsBytes()
	require.NoError(t, err)
	assert.Equal(t, "bar", string(act))

	require.Eventually(t, func() bool {
		readerImpl.ackRcvdMut.Lock()
		ackLen := len(readerImpl.ackRcvd)
		readerImpl.ackRcvdMut.Unlock()
		return ackLen >= 1
	}, time.Second, time.Millisecond*10)

	readerImpl.ackRcvdMut.Lock()
	assert.Len(t, readerImpl.ackRcvd, 1)
	assert.Error(t, readerImpl.ackRcvd[0])
	readerImpl.ackRcvdMut.Unlock()

	require.NoError(t, ackFn(ctx, nil))

	readerImpl.ackRcvdMut.Lock()
	assert.Len(t, readerImpl.ackRcvd, 1)
	assert.Error(t, readerImpl.ackRcvd[0])
	readerImpl.ackRcvdMut.Unlock()
}
