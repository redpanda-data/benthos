// Package bloblang2 provides a Bloblang V2 implementation that satisfies
// the spectest.Interpreter interface.
package bloblang2

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
)

// Interp implements spectest.Interpreter for Bloblang V2.
type Interp struct{}

// Compile parses and compiles a Bloblang V2 mapping.
func (i *Interp) Compile(mapping string, files map[string]string) (spectest.Mapping, error) {
	prog, errs := syntax.Parse(mapping, "", files)
	if len(errs) > 0 {
		return nil, &spectest.CompileError{Message: syntax.FormatErrors(errs)}
	}

	// Name resolution pass: semantic checks.
	methods, functions := eval.StdlibNames()
	resolveErrs := syntax.Resolve(prog, methods, functions)
	if len(resolveErrs) > 0 {
		return nil, &spectest.CompileError{Message: syntax.FormatErrors(resolveErrs)}
	}

	return &compiledMapping{prog: prog}, nil
}

type compiledMapping struct {
	prog *syntax.Program
}

// Exec runs the compiled mapping against input and metadata.
func (m *compiledMapping) Exec(input any, metadata map[string]any) (any, map[string]any, bool, error) {
	interp := eval.New(m.prog)
	interp.RegisterStdlib()
	interp.RegisterLambdaMethods()
	return interp.Run(input, metadata)
}
