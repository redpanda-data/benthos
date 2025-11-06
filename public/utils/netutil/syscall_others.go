//go:build !windows

// Copyright 2025 Redpanda Data, Inc.

package netutil

import "syscall"

// setsockoptInt wraps syscall.SetsockoptInt for Unix-like systems.
func setsockoptInt(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(int(fd), level, opt, value)
}
