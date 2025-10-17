//go:build linux

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
	"fmt"
	"syscall"
)

// tcpUserTimeout is a linux specific constant that is used to reference
// the tcp_user_timeout option.
const tcpUserTimeout = 18

// SetTCPUserTimeout sets the "TCP_USER_TIMEOUT" socket option on Linux.
func (c *Config) setTCPUserTimeout(fd int) error {
	timeoutMs := int(c.TCPUserTimeout.Milliseconds())

	err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, tcpUserTimeout, timeoutMs)
	if err != nil {
		return fmt.Errorf("failed to set tcp_user_timeout: %w", err)
	}

	return nil
}
