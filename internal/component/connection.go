// Copyright 2025 Redpanda Data, Inc.

package component

import "errors"

// ConnectionStatus represents the current connection status of a given
// component.
type ConnectionStatus struct {
	Label     string
	Path      []string
	Connected bool
	Err       error
}

// ConnectionStatuses represents an aggregate of connection statuses.
type ConnectionStatuses []*ConnectionStatus

// AllActive returns true if there is one or more connections and they are all
// active.
func (s ConnectionStatuses) AllActive() bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !c.Connected {
			return false
		}
	}
	return true
}

// ConnectionFailing returns a ConnectionStatus representing a component
// connection where we are attempting to connect to the service but are
// currently unable due to the provided error.
func ConnectionFailing(o Observability, err error) *ConnectionStatus {
	return &ConnectionStatus{
		Label:     o.Label(),
		Path:      o.Path(),
		Connected: false,
		Err:       err,
	}
}

// ConnectionActive returns a ConnectionStatus representing a component
// connection where we have an active connection.
func ConnectionActive(o Observability) *ConnectionStatus {
	return &ConnectionStatus{
		Label:     o.Label(),
		Path:      o.Path(),
		Connected: true,
	}
}

// ConnectionPending returns a ConnectionStatus representing a component that
// has not yet attempted to establish its connection.
func ConnectionPending(o Observability) *ConnectionStatus {
	return &ConnectionStatus{
		Label:     o.Label(),
		Path:      o.Path(),
		Connected: false,
	}
}

// ConnectionClosed returns a ConnectionStatus representing a component that has
// intentionally closed its connection.
func ConnectionClosed(o Observability) *ConnectionStatus {
	return &ConnectionStatus{
		Label:     o.Label(),
		Path:      o.Path(),
		Connected: false,
	}
}

//------------------------------------------------------------------------------

// ErrConnectionTestNotSupported is returned by components that cannot yet test
// their connections.
var ErrConnectionTestNotSupported = errors.New("this component does not support testing connections")

// ConnectionTestResult represents the result of a connection test.
type ConnectionTestResult struct {
	Label string
	Path  []string
	Err   error
}

// ConnectionTestResults represents an aggregate of connection test results.
type ConnectionTestResults []*ConnectionTestResult

// AsList returns an aggregated list of connection results containing only the
// target. This is convenient for components that only have one connection to
// test.
func (c *ConnectionTestResult) AsList() ConnectionTestResults {
	return ConnectionTestResults{c}
}

// ConnectionTestFailed returns a failed connection test result.
func ConnectionTestFailed(o Observability, err error) *ConnectionTestResult {
	return &ConnectionTestResult{
		Label: o.Label(),
		Path:  o.Path(),
		Err:   err,
	}
}

// ConnectionTestSucceeded returns a successful connection test result.
func ConnectionTestSucceeded(o Observability) *ConnectionTestResult {
	return &ConnectionTestResult{
		Label: o.Label(),
		Path:  o.Path(),
	}
}

// ConnectionTestNotSupported returns a test result indicating that the
// component does not support testing connections.
func ConnectionTestNotSupported(o Observability) *ConnectionTestResult {
	return &ConnectionTestResult{
		Label: o.Label(),
		Path:  o.Path(),
		Err:   ErrConnectionTestNotSupported,
	}
}
