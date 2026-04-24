// Copyright 2026 Redpanda Data, Inc.

package bloblang2

import (
	"errors"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// ErrRootDeleted is returned from Executor.Query when a mapping deletes the
// root of the output document.
var ErrRootDeleted = errors.New("root was deleted")

// Executor is the compiled form of a Bloblang V2 mapping. It is safe for
// concurrent use: Query and QueryMetadata allocate independent interpreter
// state per call (pooled via sync.Pool).
type Executor struct {
	program *syntax.Program

	pluginMethods   map[string]eval.MethodSpec
	pluginFunctions map[string]eval.FunctionSpec

	pool sync.Pool
}

func newExecutor(
	program *syntax.Program,
	pluginMethods map[string]eval.MethodSpec,
	pluginFunctions map[string]eval.FunctionSpec,
) *Executor {
	e := &Executor{
		program:         program,
		pluginMethods:   pluginMethods,
		pluginFunctions: pluginFunctions,
	}
	e.pool.New = func() any { return e.newInterp() }
	return e
}

func (e *Executor) newInterp() *eval.Interpreter {
	interp := eval.NewWithStdlib(e.program)
	for name, spec := range e.pluginMethods {
		interp.RegisterMethod(name, spec)
	}
	for name, spec := range e.pluginFunctions {
		interp.RegisterFunction(name, spec)
	}
	return interp
}

// Query executes the mapping against an input value and returns the output.
// If the mapping deletes the root, ErrRootDeleted is returned.
func (e *Executor) Query(input any) (any, error) {
	output, _, err := e.query(input, nil)
	return output, err
}

// QueryMetadata is Query plus access to the resulting metadata map. The
// returned metadata may be nil if the mapping didn't assign any.
func (e *Executor) QueryMetadata(input any, inputMeta map[string]any) (any, map[string]any, error) {
	return e.query(input, inputMeta)
}

func (e *Executor) query(input any, inputMeta map[string]any) (any, map[string]any, error) {
	interp := e.pool.Get().(*eval.Interpreter)
	defer e.pool.Put(interp)

	output, outputMeta, deleted, err := interp.Run(input, inputMeta)
	if err != nil {
		return nil, nil, err
	}
	if deleted {
		return nil, nil, ErrRootDeleted
	}
	return output, outputMeta, nil
}
