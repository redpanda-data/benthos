// Copyright 2026 Redpanda Data, Inc.

package bloblang2

// Function is the runtime closure implementing a plugin function. It takes
// no receiver — plugins that operate on a value should be registered as a
// Method instead.
type Function func() (any, error)

// FunctionConstructor constructs a Function from arguments resolved against a
// PluginSpec. When all arguments at a call site are literal, the constructor
// is invoked once at parse time and its Function is reused across every
// Query. When any argument is dynamic, the constructor is invoked per call.
type FunctionConstructor func(args *ParsedParams) (Function, error)
