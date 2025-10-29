// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListenWithReuseAddr(t *testing.T) {
	ctx := context.Background()

	// First listener
	listener1, err := ListenWithReuseAddr(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener1.Close()

	addr := listener1.Addr().String()

	// Close first listener
	require.NoError(t, listener1.Close())

	// Small delay to allow socket to enter TIME_WAIT
	time.Sleep(10 * time.Millisecond)

	// Second listener on the same address should succeed due to SO_REUSEADDR
	listener2, err := ListenWithReuseAddr(ctx, "tcp", addr)
	require.NoError(t, err, "Failed to bind to address after closing first listener - SO_REUSEADDR may not be working")
	defer listener2.Close()
}

func TestListenConfigWithReuseAddr_ServerReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a fixed port for this test
	addr := "127.0.0.1:19284"

	// Create first server
	lc1 := ListenConfigWithReuseAddr()
	listener1, err := lc1.Listen(ctx, "tcp", addr)
	require.NoError(t, err)

	server1 := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("server1"))
		}),
	}

	serverErr := make(chan error, 1)
	go func() {
		err := server1.Serve(listener1)
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make a request to ensure server is working
	resp, err := http.Get("http://" + addr)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Shutdown first server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	require.NoError(t, server1.Shutdown(shutdownCtx))

	// Wait for server to fully stop
	select {
	case err := <-serverErr:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}

	// Small delay to ensure port is released
	time.Sleep(100 * time.Millisecond)

	// Create second server on same address - should succeed due to SO_REUSEADDR
	lc2 := ListenConfigWithReuseAddr()
	listener2, err := lc2.Listen(ctx, "tcp", addr)
	require.NoError(t, err, "Failed to bind to port after server shutdown - SO_REUSEADDR may not be working")

	server2 := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("server2"))
		}),
	}

	go func() {
		_ = server2.Serve(listener2)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make a request to ensure new server is working
	resp, err = http.Get("http://" + addr)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Cleanup
	shutdownCtx2, shutdownCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel2()
	require.NoError(t, server2.Shutdown(shutdownCtx2))
}
