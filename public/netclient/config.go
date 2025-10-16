// Copyright 2025 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netclient

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

// Config contains TCP socket configuration options.
// TCPUserTimeout is only supported on Linux
// since 2.6.37 (https://www.man7.org/linux/man-pages/man7/tcp.7.html).
// On other platforms it is ignored.
type Config struct {
	KeepAliveConfig net.KeepAliveConfig
	TCPUserTimeout  time.Duration
}

// Validate checks that the configuration is valid.
func (config Config) Validate() error {
	// KeepAlive MUST be greater than TCP_USER_TIMEOUT
	// per RFC 5482 (https://www.rfc-editor.org/rfc/rfc5482.html).
	if config.TCPUserTimeout > 0 && config.KeepAliveConfig.Idle <= config.TCPUserTimeout {
		return fmt.Errorf("keep_alive.idle (%s) must be greater than tcp_user_timeout (%s)", config.KeepAliveConfig.Idle, config.TCPUserTimeout)
	}
	return nil
}

// NewDialerFrom creates a new net.Dialer from the provided Config.
// It validates the Config and returns an error if validation fails.
// The returned Dialer will have TCP options applied according to the Config.
func NewDialerFrom(config Config) (*net.Dialer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config.newDialer(), nil
}

// newDialer returns a net.Dialer configured with the TCP options from the
// Config.
func (config Config) newDialer() *net.Dialer {
	dialer := &net.Dialer{
		KeepAliveConfig: config.KeepAliveConfig,
	}

	if controlFn := config.controlFunc(); controlFn != nil {
		dialer.Control = controlFn
	}
	return dialer
}

// controlFunc returns a function that configures TCP socket options.
func (config Config) controlFunc() func(network, address string, con syscall.RawConn) error {
	// Do not do anything if tcp_user_timeout is not set.
	if config.TCPUserTimeout <= 0 {
		return nil
	}
	return func(network, address string, conn syscall.RawConn) error {
		var setErr error
		// Starting connection to the specific file descriptor.
		err := conn.Control(func(fd uintptr) {
			// Set timeout.
			if err := config.setTCPUserTimeout(int(fd)); err != nil {
				setErr = err
				return
			}
		})
		if err != nil {
			// If no connection was able to be established then return error and
			// what network and address it is trying to connect to.
			return fmt.Errorf("failed to access raw connection for: %s %s: %w", network, address, err)
		}
		if setErr != nil {
			// If connection was established, but we were unable to set the timeout
			// for some reason.
			return fmt.Errorf("failed to set TCP_USER_TIMEOUT (%v) on %s %s: %w", config.TCPUserTimeout, network, address, setErr)
		}
		return nil
	}
}
