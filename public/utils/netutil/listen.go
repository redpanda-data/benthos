// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// ListenConfigWithReuseAddr returns a net.ListenConfig with SO_REUSEADDR enabled.
// This allows binding to ports that have connections in TIME_WAIT state, which is
// particularly useful for graceful restarts and config reloads where the server
// needs to rebind to the same port immediately after shutdown.
//
// SO_REUSEADDR enables:
// - Binding to a port with connections in TIME_WAIT state
// - Faster server restarts without waiting for TIME_WAIT to expire (typically 30-120s)
// - Multiple listeners on the same port when using wildcard and specific addresses
//
// Note: This does not bypass the need for synchronous port binding. Even with
// SO_REUSEADDR, you should use Listen() and then pass the listener to Serve()
// rather than using ListenAndServe() directly to ensure the port is bound before
// proceeding.
func ListenConfigWithReuseAddr() net.ListenConfig {
	return net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var sockOptErr error
			if err := c.Control(func(fd uintptr) {
				// Enable SO_REUSEADDR to allow binding to ports in TIME_WAIT state
				sockOptErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			}); err != nil {
				return fmt.Errorf("failed to access raw socket connection: %w", err)
			}
			if sockOptErr != nil {
				return fmt.Errorf("failed to set SO_REUSEADDR socket option: %w", sockOptErr)
			}
			return nil
		},
	}
}

// ListenWithReuseAddr is a convenience function that creates a listener with
// SO_REUSEADDR enabled on the specified network and address.
//
// Common usage:
//
//	listener, err := netutil.ListenWithReuseAddr(ctx, "tcp", "localhost:8080")
//	if err != nil {
//	    return err
//	}
//	defer listener.Close()
//	server := &http.Server{Handler: handler}
//	return server.Serve(listener)
func ListenWithReuseAddr(ctx context.Context, network, address string) (net.Listener, error) {
	lc := ListenConfigWithReuseAddr()
	return lc.Listen(ctx, network, address)
}
