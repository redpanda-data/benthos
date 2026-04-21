// Package v1spec provides a Bloblang V1 spec-test runner. It adapts the
// shared spectest schema (originally built for V2 conformance) so that the V1
// equivalents under ./tests can be executed by the V1 interpreter and their
// outputs compared against the same expectations.
package v1spec

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/mapping"
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

// V1Interp implements spectest.Interpreter using the public V1 Bloblang API.
type V1Interp struct{}

// Compile parses a V1 mapping, wiring any in-memory import files through a
// custom importer.
func (V1Interp) Compile(src string, files map[string]string) (spectest.Mapping, error) {
	env := bloblang.NewEnvironment()
	if len(files) > 0 {
		env = env.WithCustomImporter(func(name string) ([]byte, error) {
			if content, ok := files[name]; ok {
				return []byte(content), nil
			}
			if content, ok := files[strings.TrimPrefix(name, "./")]; ok {
				return []byte(content), nil
			}
			return nil, fmt.Errorf("import %q not found in test files", name)
		})
	}
	exec, err := env.Parse(src)
	if err != nil {
		return nil, &spectest.CompileError{Message: err.Error()}
	}
	uw, ok := exec.XUnwrapper().(interface{ Unwrap() *mapping.Executor })
	if !ok {
		return nil, &spectest.CompileError{Message: "internal: executor does not expose unwrapper"}
	}
	return &v1Mapping{exec: uw.Unwrap()}, nil
}

type v1Mapping struct {
	exec *mapping.Executor
}

// Exec runs the V1 mapping against the given input + metadata. It uses
// Executor.ExecOnto directly (rather than MapPart) to preserve the raw Go type
// of the mapped root value — MapPart stringifies scalars through a message
// body, which would re-parse `"true"` as a bool.
func (m *v1Mapping) Exec(input any, meta map[string]any) (any, map[string]any, bool, error) {
	// message.Part holds output metadata (and batch-scoped meta reads).
	part := message.NewPart(nil)
	if input != nil {
		part.SetStructured(input)
	}
	for k, v := range meta {
		part.MetaSetMut(k, v)
	}
	batch := message.Batch{part}

	vars := map[string]any{}
	var newValue any = value.Nothing(nil)

	ctx := query.FunctionContext{
		Maps:     m.exec.Maps(),
		Vars:     vars,
		Index:    0,
		MsgBatch: batch,
		NewMeta:  part,
		NewValue: &newValue,
	}.WithValue(input)

	if err := m.exec.ExecOnto(ctx, mapping.AssignmentContext{
		Vars:  vars,
		Meta:  part,
		Value: &newValue,
	}); err != nil {
		return nil, nil, false, err
	}

	switch newValue.(type) {
	case value.Delete:
		return nil, nil, true, nil
	case value.Nothing:
		// Mapping made no payload assignment — preserve the input.
		newValue = input
	}

	outMeta := map[string]any{}
	_ = part.MetaIterMut(func(key string, v any) error {
		outMeta[key] = v
		return nil
	})

	return newValue, outMeta, false, nil
}

// Compile-time guard that V1Interp satisfies spectest.Interpreter.
var _ spectest.Interpreter = V1Interp{}
