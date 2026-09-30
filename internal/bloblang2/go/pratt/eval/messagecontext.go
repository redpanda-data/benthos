package eval

// MessageContext exposes per-message read access used by message-coupled
// stdlib functions (batch_index, batch_size, content, error, errored,
// tracing_id, tracing_span). The interface is intentionally small: it
// describes only the read surface batch-3 needs, so the eval package
// stays decoupled from public/service.Message and the executor remains
// reusable outside Benthos pipelines.
//
// Callers obtain the bound context inside a function implementation by
// receiving it as the first argument of MessageFunctionFunc. It is only
// non-nil while Interpreter.RunWithMessage is in flight; calls to a
// message-coupled function from a plain Run / Exec path produce a
// runtime error before the function body is invoked.
type MessageContext interface {
	// Input returns the structured form of the message body that should
	// be bound to the mapping's `input` keyword. May be []byte, a
	// scalar, an array, an object, or nil.
	Input() any

	// Metadata returns a snapshot of the message metadata to be bound
	// to `input@`. Returning nil is equivalent to an empty map.
	Metadata() map[string]any

	// Bytes returns the raw byte form of the message body, used by
	// content().
	Bytes() []byte

	// Error returns the error currently set on the message, or nil. Used
	// by error() and errored().
	Error() error

	// BatchIndex is the 0-based position of the current message within
	// its batch. Used by batch_index().
	BatchIndex() int

	// BatchSize is the total number of messages in the current batch.
	// Used by batch_size().
	BatchSize() int

	// TraceID returns the OpenTelemetry trace ID associated with the
	// message, or the empty string if none is set. Used by tracing_id().
	TraceID() string

	// Span returns the active tracing span for the message, or nil.
	// Used by tracing_span().
	Span() any
}

// MessageFunctionFunc is the implementation shape for message-coupled
// stdlib functions. Functions of this shape read from the bound
// MessageContext and ignore the interpreter's input value.
//
// Functions registered with MessageFunctionFunc bypass parse-time
// argument folding implicitly (folding requires Fn, which they leave
// nil). They are dispatched only when the interpreter is running with
// a MessageContext bound (i.e. via Interpreter.RunWithMessage).
type MessageFunctionFunc func(msg MessageContext, args []any) any
