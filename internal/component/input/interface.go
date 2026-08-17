// Copyright 2025 Redpanda Data, Inc.

package input

import (
	"context"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// Streamed is a common interface implemented by inputs and provides channel
// based streaming APIs.
type Streamed interface {
	// TransactionChan returns a channel used for consuming transactions from
	// this type. Every transaction received must be resolved before another
	// transaction will be sent.
	TransactionChan() <-chan message.Transaction

	// ConnectionTest attempts to establish whether the component is capable of
	// creating a connection. This will potentially require and test network
	// connectivity, but does not require the component to be initialized.
	ConnectionTest(ctx context.Context) component.ConnectionTestResults

	// ConnectionStatus returns the current status of the given component
	// connection. The result is a slice in order to accommodate higher order
	// components that wrap several others.
	ConnectionStatus() component.ConnectionStatuses

	// TriggerStartConsuming instructs the input to start consuming data, and attempting
	// to write it to the transaction channel.
	TriggerStartConsuming()

	// TriggerStopConsuming instructs the input to start shutting down resources
	// once all pending messages are delivered and acknowledged. This call does
	// not block.
	TriggerStopConsuming()

	// TriggerCloseNow triggers the shut down of this component but should not
	// block the calling goroutine.
	TriggerCloseNow()

	// WaitForClose is a blocking call to wait until the component has finished
	// shutting down and cleaning up resources.
	WaitForClose(ctx context.Context) error
}

// AsyncAckFn is a function used to acknowledge receipt of a message batch. The
// provided response indicates whether the message batch was successfully
// delivered. Returns an error if the acknowledge was not propagated.
type AsyncAckFn func(context.Context, error) error

// Async is a type that reads Benthos messages from an external source and
// allows acknowledgements for a message batch to be propagated asynchronously.
type Async interface {
	// ConnectionTest attempts to establish whether the component is capable of
	// creating a connection. This will potentially require and test network
	// connectivity, but does not require the component to be initialized.
	ConnectionTest(ctx context.Context) component.ConnectionTestResults

	// Connect attempts to establish a connection to the source, if
	// unsuccessful returns an error. If the attempt is successful (or not
	// necessary) returns nil.
	Connect(ctx context.Context) error

	// ReadBatch attempts to read a new message from the source. If
	// successful a message is returned along with a function used to
	// acknowledge receipt of the returned message. It's safe to process the
	// returned message and read the next message asynchronously.
	ReadBatch(ctx context.Context) (message.Batch, AsyncAckFn, error)

	// Close triggers the shut down of this component and blocks until
	// completion or context cancellation.
	Close(ctx context.Context) error
}

// BackfillAsync is an optional extension of Async for readers that have a
// distinct initial backfill (backfill) phase before switching to continuous
// streaming, such as CDC-style connectors performing a table backfill ahead
// of log-based replication.
//
// When a reader implements this interface, AsyncReader calls
// BackfillReadBatch repeatedly - exactly like ReadBatch - until it returns
// component.ErrBackfillComplete. AsyncReader tracks every batch dispatched
// this way against a barrier scoped to the backfill phase, and blocks until
// each has been acknowledged (or nacked) downstream before moving on to
// steady-state ReadBatch calls. This removes the need for the reader to
// build its own ack-counting barrier in order to safely persist a
// post-backfill resume position.
type BackfillAsync interface {
	Async

	// BackfillReadBatch attempts to read the next batch belonging to the
	// backfill phase. Once the backfill has been fully read, including any
	// trailing partial batch, this must return component.ErrBackfillComplete.
	BackfillReadBatch(ctx context.Context) (message.Batch, AsyncAckFn, error)
}

// BackfillCompleter is an optional extension for BackfillAsync readers that
// want to be notified once every batch read via BackfillReadBatch has been
// fully acknowledged or nacked downstream, and before AsyncReader begins
// calling ReadBatch. This is the safe point at which to persist a
// post-backfill resume position, since it's guaranteed no backfill batch is
// still in flight.
type BackfillCompleter interface {
	BackfillComplete(ctx context.Context) error
}
