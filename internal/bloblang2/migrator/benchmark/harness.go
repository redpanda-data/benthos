// Package benchmark runs a comparative performance suite between the V1
// Bloblang interpreter and the V2 Pratt interpreter over the full V1
// corpus (../v1spec/tests). For every corpus case it translates the V1
// mapping to V2, verifies the two runtimes agree on the output, and
// then benchmarks both sides. Only equivalent pairs contribute to the
// summary — divergent cases are counted but not timed.
//
// Run per-case numbers with:
//
//	task test:go -- -bench BenchmarkCorpus ./internal/bloblang2/migrator/benchmark
//
// Run the aggregate analysis with:
//
//	go test ./internal/bloblang2/migrator/benchmark -run TestCorpusAnalysis -v
//
// The analysis test prints a summary table (ok / skipped / per-case
// V2/V1 ratios) to t.Log; it does not gate the build.
package benchmark

import (
	"errors"
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/mapping"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/bloblang"

	// Side-effect imports: registers the impl/pure stdlib extensions that
	// real V1 mappings depend on (ts_*, typed numeric coercers, etc.).
	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

// v1Runner compiles a V1 mapping once and exposes a fast per-iteration
// Exec closure. It uses the internal *mapping.Executor directly (via
// XUnwrapper) so the AssignmentContext has a real message.Part and
// `meta x = y` writes don't error — matching the production pipeline.
type v1Runner struct {
	exec *mapping.Executor
}

// newV1Runner compiles a V1 mapping. The import map is threaded through
// the default bloblang environment's custom importer so corpus cases
// with `import "foo"` work.
func newV1Runner(src string, files map[string]string) (*v1Runner, error) {
	env := bloblang.NewEnvironment()
	if len(files) > 0 {
		env = env.WithCustomImporter(func(name string) ([]byte, error) {
			if content, ok := files[name]; ok {
				return []byte(content), nil
			}
			return nil, fmt.Errorf("import %q not in test files", name)
		})
	}
	exe, err := env.Parse(src)
	if err != nil {
		return nil, err
	}
	uw, ok := exe.XUnwrapper().(interface{ Unwrap() *mapping.Executor })
	if !ok {
		return nil, errors.New("v1 executor does not expose Unwrap()")
	}
	return &v1Runner{exec: uw.Unwrap()}, nil
}

// Exec runs the V1 mapping against input + input metadata and returns
// the mapped value (never nil on success — V1's Nothing sentinel is
// mapped to the input to match V1's mapping-processor default).
func (r *v1Runner) Exec(input any, meta map[string]any) (any, error) {
	part := message.NewPart(nil)
	if input != nil {
		part.SetStructured(input)
	}
	for k, v := range meta {
		part.MetaSetMut(k, v)
	}
	vars := map[string]any{}
	var newValue any = value.Nothing(nil)
	ctx := query.FunctionContext{
		Maps:     r.exec.Maps(),
		Vars:     vars,
		Index:    0,
		MsgBatch: message.Batch{part},
		NewMeta:  part,
		NewValue: &newValue,
	}.WithValue(input)
	if err := r.exec.ExecOnto(ctx, mapping.AssignmentContext{
		Vars:  vars,
		Meta:  part,
		Value: &newValue,
	}); err != nil {
		return nil, err
	}
	switch newValue.(type) {
	case value.Delete:
		return deletedSentinel{}, nil
	case value.Nothing:
		return input, nil
	}
	return newValue, nil
}

// v2Runner compiles a V2 mapping once and exposes a fast per-iteration
// Run closure. The eval.Interpreter keeps its variable stack allocated
// across iterations so the benchmark exercises the steady-state cost of
// executing a compiled V2 program.
type v2Runner struct {
	interp *eval.Interpreter
}

// newV2Runner parses, optimizes, resolves, and instantiates an
// interpreter for a V2 source produced by the translator.
func newV2Runner(src string) (*v2Runner, error) {
	prog, errs := syntax.Parse(src, "", nil)
	if len(errs) > 0 {
		return nil, fmt.Errorf("v2 parse: %v", errs)
	}
	syntax.Optimize(prog)

	methods, functions := eval.StdlibNames()
	methodOpcodes, functionOpcodes := eval.StdlibOpcodes()
	if rerrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methods,
		Functions:       functions,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	}); len(rerrs) > 0 {
		return nil, fmt.Errorf("v2 resolve: %v", rerrs)
	}
	return &v2Runner{interp: eval.NewWithStdlib(prog)}, nil
}

// Exec runs the V2 mapping against input + input metadata and returns
// the mapped value. The deletedSentinel value is returned when the V2
// program sets `output = deleted()` so the equivalence check can match
// V1's Delete sentinel symmetrically.
func (r *v2Runner) Exec(input any, meta map[string]any) (any, error) {
	out, _, deleted, err := r.interp.Run(input, meta)
	if err != nil {
		return nil, err
	}
	if deleted {
		return deletedSentinel{}, nil
	}
	return out, nil
}

// deletedSentinel stands in for V1's value.Delete and V2's deleted=true
// return path so Case equivalence can compare the two outcomes without
// pulling value.Delete into the equivalence predicate (V2 never produces
// that internal Go type).
type deletedSentinel struct{}
