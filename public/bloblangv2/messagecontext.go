// Copyright 2026 Redpanda Data, Inc.

package bloblangv2

// MessageContext is the read surface used by Bloblang V2's
// message-coupled stdlib (batch_index, batch_size, content, error,
// errored, tracing_id, tracing_span). Callers running a mapping with
// access to a pipeline message build a MessageContext and pass it to
// Executor.QueryMessage.
//
// The interface is intentionally small: it covers only the read paths
// the bundled stdlib needs, so the V2 executor stays decoupled from
// public/service.Message and remains usable in non-pipeline contexts
// (for example, tests stub the interface directly).
//
// Mappings parsed by an Environment without a bound message — i.e.
// invoked through Executor.Query or Executor.QueryMetadata — that
// reference a message-coupled function will produce a runtime error
// rather than a silent null fallback.
type MessageContext interface {
	// Input returns the structured form of the message body to bind to
	// the mapping's `input` keyword. May be []byte, a scalar, an array,
	// an object, or nil.
	Input() any

	// Metadata returns a snapshot of the message metadata to bind to
	// `input@`. Returning nil is equivalent to an empty map.
	Metadata() map[string]any

	// Bytes returns the raw byte form of the message body, used by
	// `content()`.
	Bytes() []byte

	// Error returns the error currently set on the message, or nil. Used
	// by `error()` and `errored()`.
	Error() error

	// BatchIndex is the 0-based position of the current message within
	// its batch. Used by `batch_index()`.
	BatchIndex() int

	// BatchSize is the total number of messages in the current batch.
	// Used by `batch_size()`.
	BatchSize() int

	// TraceID returns the OpenTelemetry trace ID associated with the
	// message, or the empty string if none. Used by `tracing_id()`.
	TraceID() string

	// Span returns the active tracing span for the message, or nil.
	// Used by `tracing_span()`.
	Span() any
}
