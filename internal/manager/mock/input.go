// Copyright 2025 Redpanda Data, Inc.

package mock

import (
	"context"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// Input provides a mocked input implementation.
type Input struct {
	batches []message.Batch

	startOnce sync.Once
	TChan     chan message.Transaction
	closed    bool
	closeOnce sync.Once
}

// NewInput creates a new mock input that will return transactions containing a
// list of batches, then exit.
func NewInput(batches []message.Batch) *Input {
	ts := make(chan message.Transaction, len(batches))
	return &Input{batches: batches, TChan: ts}
}

// ConnectionTest always returns active (for now).
func (f *Input) ConnectionTest(ctx context.Context) component.ConnectionTestResults {
	return component.ConnectionTestSucceeded(component.NoopObservability()).AsList()
}

// ConnectionStatus returns the current connection activity.
func (f *Input) ConnectionStatus() component.ConnectionStatuses {
	if f.closed {
		return component.ConnectionStatuses{
			component.ConnectionClosed(component.NoopObservability()),
		}
	}
	return component.ConnectionStatuses{
		component.ConnectionActive(component.NoopObservability()),
	}
}

// TransactionChan returns a transaction channel.
func (f *Input) TransactionChan() <-chan message.Transaction {
	return f.TChan
}

// TriggerStartConsuming kicks of data consumption.
func (f *Input) TriggerStartConsuming() {
	if len(f.batches) == 0 {
		return
	}

	f.startOnce.Do(func() {
		resChan := make(chan error, len(f.batches))
		go func() {
			defer close(f.TChan)
			for _, b := range f.batches {
				f.TChan <- message.NewTransaction(b, resChan)
			}
		}()
	})
}

// TriggerStopConsuming closes the input transaction channel.
func (f *Input) TriggerStopConsuming() {
	f.closeOnce.Do(func() {
		close(f.TChan)
		f.closed = true
	})
}

// TriggerCloseNow closes the input transaction channel.
func (f *Input) TriggerCloseNow() {
	f.closeOnce.Do(func() {
		close(f.TChan)
		f.closed = true
	})
}

// WaitForClose does nothing.
func (f *Input) WaitForClose(ctx context.Context) error {
	return nil
}
