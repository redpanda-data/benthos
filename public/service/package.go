// Copyright 2025 Redpanda Data, Inc.

// Package service provides a high level API for registering custom plugin
// components and executing either a standard Benthos CLI, or programmatically
// building isolated pipelines with a StreamBuilder API.
//
// For a video guide on Benthos plugins check out: https://youtu.be/uH6mKw-Ly0g
// And an example repo containing component plugins and tests can be found at:
// https://github.com/benthosdev/benthos-plugin-example
//
// In order to add custom Bloblang functions and methods use the
// ./public/bloblang package.
package service

import (
	"context"
	"errors"

	"github.com/redpanda-data/benthos/v4/internal/component"
)

var errConnectionTestNotSupported = errors.New("this component does not support testing connections")

// ConnectionTestResult represents the result of a connection test.
type ConnectionTestResult struct {
	label string
	path  []string
	Err   error
}

// ConnectionTestResults represents an aggregate of connection test results.
type ConnectionTestResults []*ConnectionTestResult

func (r ConnectionTestResults) intoInternal(o component.Observability) component.ConnectionTestResults {
	l := make(component.ConnectionTestResults, len(r))
	for i, c := range r {
		l[i] = c.intoInternal(o)
	}
	return l
}

// AsList returns an aggregated list of connection results containing only the
// target. This is convenient for components that only have one connection to
// test.
func (c *ConnectionTestResult) AsList() ConnectionTestResults {
	return ConnectionTestResults{c}
}

func (c *ConnectionTestResult) intoInternal(o component.Observability) *component.ConnectionTestResult {
	i := &component.ConnectionTestResult{Err: c.Err}
	if len(c.path) > 0 {
		i.Label = c.label
		i.Path = c.path
	} else {
		i.Label = o.Label()
		i.Path = o.Path()
	}
	return i
}

// ConnectionTestFailed returns a failed connection test result.
func ConnectionTestFailed(err error) *ConnectionTestResult {
	return &ConnectionTestResult{
		Err: err,
	}
}

// ConnectionTestSucceeded returns a successful connection test result.
func ConnectionTestSucceeded() *ConnectionTestResult {
	return &ConnectionTestResult{}
}

// ConnectionTestNotSupported returns a test result indicating that the
// component does not support testing connections.
func ConnectionTestNotSupported() *ConnectionTestResult {
	return &ConnectionTestResult{
		Err: errConnectionTestNotSupported,
	}
}

// ConnectionTestable is implemented by components that support testing the
// underlying connection separately to regular operation. This connection
// check can occur before and during normal operation.
type ConnectionTestable interface {
	// ConnectionTest attempts to establish whether the component is capable of
	// creating a connection. This will potentially require and test network
	// connectivity, but does not require the component to be initialized.
	ConnectionTest(ctx context.Context) ConnectionTestResults
}

// Closer is implemented by components that support stopping and cleaning up
// their underlying resources.
type Closer interface {
	// Close the component, blocks until either the underlying resources are
	// cleaned up or the context is cancelled. Returns an error if the context
	// is cancelled.
	Close(ctx context.Context) error
}
