//go:build windows

// Copyright 2025 Redpanda Data, Inc.

package netutil

import "syscall"

// setsockoptInt wraps syscall.SetsockoptInt for Windows.
func setsockoptInt(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, value)
}
