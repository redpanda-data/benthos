// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelableStdinReader_ContextCancellation(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)

	ctx, cancel := context.WithCancel(t.Context())
	reader.setContext(ctx)

	// Cancel the context; Read should return promptly.
	cancel()

	buf := make([]byte, 64)
	_, err := reader.Read(buf)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCancelableStdinReader_Close(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)

	reader.Close()

	buf := make([]byte, 64)
	_, err := reader.Read(buf)
	require.ErrorIs(t, err, io.EOF)

	// Double close must not panic.
	reader.Close()
}

func TestCancelableStdinReader_DataBeforeCancellation(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)
	reader.setContext(t.Context())

	go func() {
		relay.ch <- stdinChunk{data: []byte("hello\n")}
	}()

	buf := make([]byte, 64)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(buf[:n]))
}

func TestCancelableStdinReader_RelayError(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)
	reader.setContext(t.Context())

	go func() {
		relay.ch <- stdinChunk{err: io.EOF}
	}()

	buf := make([]byte, 64)
	_, err := reader.Read(buf)
	require.ErrorIs(t, err, io.EOF)
}

func TestCancelableStdinReader_BuffersLargeChunks(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)
	reader.setContext(t.Context())

	go func() {
		relay.ch <- stdinChunk{data: []byte("hello world")}
	}()

	// Read with a small buffer; leftover should be buffered.
	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))

	// Second read should return the buffered remainder without hitting the relay.
	n, err = reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, " worl", string(buf[:n]))

	n, err = reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "d", string(buf[:n]))
}

func TestCancelableStdinReader_NewReaderAfterCancel(t *testing.T) {
	// Simulate watcher-mode reload: cancel the old reader, create a new one,
	// and verify that data sent after cancellation goes to the new reader.
	relay := &stdinRelay{ch: make(chan stdinChunk)}

	oldReader := newCancelableStdinReader(relay)
	ctx, cancel := context.WithCancel(t.Context())
	oldReader.setContext(ctx)

	// Cancel the old reader's context.
	cancel()

	buf := make([]byte, 64)
	_, err := oldReader.Read(buf)
	require.ErrorIs(t, err, context.Canceled)

	// Create a replacement reader connected to the same relay.
	newReader := newCancelableStdinReader(relay)
	newReader.setContext(t.Context())

	// Send data; it should be received by the new reader.
	go func() {
		relay.ch <- stdinChunk{data: []byte("after reload\n")}
	}()

	n, err := newReader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "after reload\n", string(buf[:n]))
}

func TestCancelableStdinReader_CancelUnblocksWaiting(t *testing.T) {
	relay := &stdinRelay{ch: make(chan stdinChunk)}
	reader := newCancelableStdinReader(relay)

	ctx, cancel := context.WithCancel(t.Context())
	reader.setContext(ctx)

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		done <- err
	}()

	// Give the goroutine time to block in select.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Read did not unblock after context cancellation")
	}
}
