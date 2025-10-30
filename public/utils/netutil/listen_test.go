// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecorateListenConfig(t *testing.T) {
	ctx := context.Background()

	// Test decorating an existing ListenConfig
	lc := net.ListenConfig{}
	conf := ListenerConfig{
		ReuseAddr: true,
	}

	err := DecorateListenerConfig(&lc, conf)
	require.NoError(t, err)

	listener1, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener1.Close()

	addr := listener1.Addr().String()
	require.NoError(t, listener1.Close())

	time.Sleep(10 * time.Millisecond)

	// Should be able to rebind immediately with decorated config
	listener2, err := lc.Listen(ctx, "tcp", addr)
	require.NoError(t, err, "Failed to bind with decorated ListenConfig")
	defer listener2.Close()
}

func TestListenerConfig_ServerReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a fixed port for this test
	addr := "127.0.0.1:19284"

	conf := ListenerConfig{
		ReuseAddr: true,
	}

	// Create first server
	lc1 := net.ListenConfig{}
	err := DecorateListenerConfig(&lc1, conf)
	require.NoError(t, err)

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
	lc2 := net.ListenConfig{}
	err = DecorateListenerConfig(&lc2, conf)
	require.NoError(t, err)

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

func TestListenerConfig_Empty(t *testing.T) {
	// Test that empty config doesn't break anything
	lc := net.ListenConfig{}
	conf := ListenerConfig{}

	err := DecorateListenerConfig(&lc, conf)
	require.NoError(t, err)

	// Should still be able to listen normally
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
}
