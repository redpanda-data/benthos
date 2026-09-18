package eval

// registerMessageFunctions registers stdlib functions that read from the
// bound MessageContext (batch_index, batch_size, content, error,
// errored, tracing_id, tracing_span). They are dispatched only when the
// interpreter is running with a MessageContext bound; otherwise the
// caller sees a runtime error of the form
// "function NAME requires a message context, but Run was called without
// one".
//
// Each function uses MessageFunctionFunc, which causes
// RegisterFunction to set RequiresMessageContext on the spec
// automatically. Folding bypass is implicit: MessageFn functions leave
// Fn nil, so the resolver has nothing to fold against.
func (interp *Interpreter) registerMessageFunctions() {
	interp.RegisterFunction("batch_index", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return int64(msg.BatchIndex())
		},
	})
	interp.RegisterFunction("batch_size", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return int64(msg.BatchSize())
		},
	})
	interp.RegisterFunction("content", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return msg.Bytes()
		},
	})
	interp.RegisterFunction("error", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			err := msg.Error()
			if err == nil {
				return nil
			}
			// V2 error() returns a structured object. The minimal
			// shape is {what: string}; future iterations may add
			// source.* fields once the underlying MessageContext.Error
			// surfaces them.
			return map[string]any{"what": err.Error()}
		},
	})
	interp.RegisterFunction("errored", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return msg.Error() != nil
		},
	})
	interp.RegisterFunction("tracing_id", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return msg.TraceID()
		},
	})
	interp.RegisterFunction("tracing_span", FunctionSpec{
		MessageFn: func(msg MessageContext, _ []any) any {
			return msg.Span()
		},
	})
}
