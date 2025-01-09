// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"context"
	"fmt"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/component"
)

// RunningStream represents a resource (a Benthos stream or a streams mode
// manager) that can be stopped.
type RunningStream interface {
	ConnectionStatus() component.ConnectionStatuses
	Stop(ctx context.Context) error
}

// SwappableStopper wraps an active Stoppable resource in a mechanism that
// allows changing the resource for something else after stopping it.
type SwappableStopper struct {
	stopped bool
	current RunningStream
	mut     sync.Mutex
}

// NewSwappableStopper creates a new swappable stopper resource around an
// initial stoppable.
func NewSwappableStopper(s RunningStream) *SwappableStopper {
	return &SwappableStopper{
		current: s,
	}
}

// ConnectionStatus returns the connection status of the underlying stream.
func (s *SwappableStopper) ConnectionStatus() component.ConnectionStatuses {
	s.mut.Lock()
	defer s.mut.Unlock()

	if s.stopped {
		return nil
	}
	return s.current.ConnectionStatus()
}

// Stop the wrapped resource.
func (s *SwappableStopper) Stop(ctx context.Context) error {
	s.mut.Lock()
	defer s.mut.Unlock()

	if s.stopped {
		return nil
	}

	s.stopped = true
	return s.current.Stop(ctx)
}

// Replace the resource with something new only once the existing one is
// stopped. In order to avoid unnecessary start up of the swapping resource we
// accept a closure that constructs it and is only called when we're ready.
func (s *SwappableStopper) Replace(ctx context.Context, fn func() (RunningStream, error)) error {
	s.mut.Lock()
	defer s.mut.Unlock()

	if s.stopped {
		// If the outer stream has been stopped then do not create a new one.
		return nil
	}

	// The underlying implementation is expected to continue shutting resources
	// down in the background. An error here indicates that it hasn't managed to
	// fully clean up before reaching a context deadline.
	//
	// However, aborting the creation of the replacement would not be
	// appropriate as it would leave the service stateless, we therefore stop
	// blocking and proceed.
	_ = s.current.Stop(ctx)

	newStoppable, err := fn()
	if err != nil {
		return fmt.Errorf("failed to init updated stream: %w", err)
	}

	s.current = newStoppable
	return nil
}
