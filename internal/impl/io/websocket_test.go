// Copyright 2026 Redpanda Data, Inc.

package io

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newHangingListener returns the address of a listener that accepts TCP
// connections but never answers the handshake, so a dial there stays blocked
// until the dialer times out. The returned channel reports each accept, which
// tells the caller that a handshake is in flight.
func newHangingListener(t *testing.T) (net.Addr, <-chan struct{}) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	accepted := make(chan struct{}, 1)
	var (
		mut   sync.Mutex
		conns []net.Conn
	)

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			mut.Lock()
			conns = append(conns, conn)
			mut.Unlock()
			select {
			case accepted <- struct{}{}:
			default:
			}
		}
	}()

	// Close the accepted connections as well as the listener, so that an
	// abandoned dial fails instead of running to the handshake timeout.
	t.Cleanup(func() {
		_ = lis.Close()
		mut.Lock()
		defer mut.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})

	return lis.Addr(), accepted
}

// newUnreachableAddr returns the address of a closed port. A dial there fails
// immediately, so a context error proves no dial was attempted.
func newUnreachableAddr(t *testing.T) net.Addr {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr()
	require.NoError(t, lis.Close())

	return addr
}

// awaitWithin runs fn in the background and returns its error, or fails the test
// if fn has not returned within d.
func awaitWithin(t *testing.T, d time.Duration, what string, fn func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("%v stayed blocked for %v", what, d)
		return nil
	}
}

// requireAccepted fails the test if the listener reported no accept within d. It
// proves that the dial reached the handshake, so a test that expects the context
// to become mid-handshake cannot pass through an earlier failure instead.
func requireAccepted(t *testing.T, d time.Duration, accepted <-chan struct{}) {
	t.Helper()

	select {
	case <-accepted:
	case <-time.After(d):
		t.Fatal("the listener accepted no connection, so the dial never reached the handshake")
	}
}
