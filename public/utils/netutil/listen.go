// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"fmt"
	"net"
	"syscall"
)

// ListenerConfig contains TCP listener socket configuration options.
type ListenerConfig struct {
	// ReuseAddr enables SO_REUSEADDR, allowing binding to ports in TIME_WAIT state.
	// Useful for graceful restarts and config reloads where the server needs to
	// rebind to the same port immediately after shutdown.
	ReuseAddr bool

	// ReusePort enables SO_REUSEPORT, allowing multiple sockets to bind to the same
	// port for load balancing across multiple processes/threads.
	ReusePort bool
}

// DecorateListenConfig applies ListenerConfig settings to a net.ListenConfig.
// This configures socket options like SO_REUSEADDR and SO_REUSEPORT.
func DecorateListenConfig(lc *net.ListenConfig, conf ListenerConfig) error {
	// If no options are set, nothing to do
	if !conf.ReuseAddr && !conf.ReusePort {
		return nil
	}

	// Wrap any existing Control function
	existingControl := lc.Control
	lc.Control = func(network, address string, c syscall.RawConn) error {
		// Call existing control function first if it exists
		if existingControl != nil {
			if err := existingControl(network, address, c); err != nil {
				return err
			}
		}

		// Apply socket options
		var sockOptErr error
		if err := c.Control(func(fd uintptr) {
			if conf.ReuseAddr {
				sockOptErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if sockOptErr != nil {
					return
				}
			}

			if conf.ReusePort {
				// SO_REUSEPORT = 15 on Linux, not available on all platforms
				const SO_REUSEPORT = 0x0F
				sockOptErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1)
				if sockOptErr != nil {
					// Ignore error if SO_REUSEPORT is not supported on this platform
					// This allows the code to work across different OSes
					sockOptErr = nil
				}
			}
		}); err != nil {
			return fmt.Errorf("failed to access raw socket connection: %w", err)
		}

		if sockOptErr != nil {
			return fmt.Errorf("failed to set socket options: %w", sockOptErr)
		}

		return nil
	}

	return nil
}
