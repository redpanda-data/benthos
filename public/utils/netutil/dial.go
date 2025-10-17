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

package netutil

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"syscall"
	"time"
)

// DialerConfig contains TCP socket configuration options used to configure
// a net.Dialer.
type DialerConfig struct {
	KeepAliveConfig net.KeepAliveConfig
	// TCPUserTimeout is only supported on Linux since 2.6.37, on other
	// platforms it's ignored.
	// See: https://www.man7.org/linux/man-pages/man7/tcp.7.html.
	TCPUserTimeout time.Duration
}

// Validate checks that the configuration is valid.
func (c DialerConfig) Validate() error {
	// KeepAlive MUST be greater than TCP_USER_TIMEOUT per RFC 5482.
	// See: https://www.rfc-editor.org/rfc/rfc5482.html
	if c.TCPUserTimeout > 0 && c.KeepAliveConfig.Idle <= c.TCPUserTimeout {
		return fmt.Errorf("keep_alive.idle (%s) must be greater than tcp_user_timeout (%s)", c.KeepAliveConfig.Idle, c.TCPUserTimeout)
	}
	return nil
}

type controlContextFunc func(ctx context.Context, network, address string, conn syscall.RawConn) error

// DecorateDialer applies DialerConfig to a net.Dialer, configuring keep-alive
// and TCP socket options.
func DecorateDialer(d *net.Dialer, conf DialerConfig) error {
	if err := conf.Validate(); err != nil {
		return err
	}

	d.KeepAliveConfig = conf.KeepAliveConfig

	fn := d.ControlContext
	if fn == nil && d.Control != nil {
		fn = func(ctx context.Context, network, address string, conn syscall.RawConn) error {
			return d.Control(network, address, conn)
		}
	}
	d.Control = nil
	d.ControlContext = wrapControlContext(fn, conf)

	return nil
}

// controlFunc returns a function that configures TCP socket options.
func wrapControlContext(inner controlContextFunc, conf DialerConfig) controlContextFunc {
	// We only need to wrap the control function if we have a TCPUserTimeout.
	if !isLinux() || conf.TCPUserTimeout <= 0 {
		return inner
	}
	return func(ctx context.Context, network, address string, conn syscall.RawConn) error {
		if inner != nil {
			if err := inner(ctx, network, address, conn); err != nil {
				return err
			}
		}

		// tcpUserTimeout is a linux specific constant that is used to reference
		// the tcp_user_timeout option.
		const tcpUserTimeout = 18

		var syscallErr error
		if err := conn.Control(func(fd uintptr) {
			syscallErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP,
				tcpUserTimeout, int(conf.TCPUserTimeout.Milliseconds()))
		}); err != nil {
			return fmt.Errorf("failed to set tcp_user_timeout: %w", err)
		}
		if syscallErr != nil {
			return fmt.Errorf("failed to set tcp_user_timeout: %w", syscallErr)
		}

		return nil
	}
}

func isLinux() bool {
	return runtime.GOOS == "linux"
}
