// Copyright 2026 Redpanda Data, Inc.

package bloblangv2

import (
	"errors"
	"fmt"
	"math"
)

// ParsedParams holds the resolved argument values passed to a plugin
// constructor. Values have been positionally matched to the plugin's
// ParamDefinitions and, where applicable, coerced into the declared kind.
// Plugin authors query arguments by name via GetString, GetInt64, and friends.
type ParsedParams struct {
	spec   *PluginSpec
	byName map[string]any
	raw    []any
}

// newParsedParams resolves raw positional arguments against a PluginSpec. It
// applies defaults for missing optional parameters, validates types, and
// rejects surplus arguments unless the spec is variadic.
func newParsedParams(spec *PluginSpec, rawArgs []any) (*ParsedParams, error) {
	pp := &ParsedParams{spec: spec, raw: rawArgs}

	if spec.variadic {
		// Variadic plugins bypass named-parameter resolution entirely.
		// Authors consume args through AsSlice.
		return pp, nil
	}

	pp.byName = make(map[string]any, len(spec.params))
	for i, p := range spec.params {
		if i < len(rawArgs) {
			v, err := coerceArg(rawArgs[i], p)
			if err != nil {
				return nil, fmt.Errorf("argument %q: %w", p.name, err)
			}
			pp.byName[p.name] = v
			continue
		}
		// Missing argument.
		if p.hasDefault {
			pp.byName[p.name] = p.defaultVal
			continue
		}
		if p.optional {
			continue
		}
		return nil, fmt.Errorf("missing required argument %q", p.name)
	}

	if len(rawArgs) > len(spec.params) {
		return nil, fmt.Errorf("too many arguments: got %d, expected at most %d", len(rawArgs), len(spec.params))
	}

	return pp, nil
}

// coerceArg validates and, where safe, coerces a raw argument into the kind
// declared on the ParamDefinition. Bloblang V2 admits multiple integer and
// float widths, so Int64Param accepts any integer that fits in int64 and
// Float64Param accepts any numeric value losslessly convertible to float64.
func coerceArg(v any, p ParamDefinition) (any, error) {
	switch p.kind {
	case paramKindAny:
		return v, nil
	case paramKindString:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", v)
		}
		return s, nil
	case paramKindBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool, got %T", v)
		}
		return b, nil
	case paramKindInt64:
		return coerceInt64(v)
	case paramKindFloat64:
		return coerceFloat64(v)
	}
	return nil, fmt.Errorf("unsupported param kind %d", p.kind)
}

func coerceInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	case uint64:
		if n > math.MaxInt64 {
			return 0, errors.New("uint64 value exceeds int64 range")
		}
		return int64(n), nil
	}
	return 0, fmt.Errorf("expected integer, got %T", v)
}

func coerceFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	}
	return 0, fmt.Errorf("expected number, got %T", v)
}

// AsSlice returns the raw positional argument values. This is the primary
// accessor for variadic plugins.
func (p *ParsedParams) AsSlice() []any {
	return p.raw
}

// Get returns the value associated with a named parameter. An error is
// returned if the parameter was optional and not provided.
func (p *ParsedParams) Get(name string) (any, error) {
	v, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("parameter %q was not provided", name)
	}
	return v, nil
}

// GetString returns a string argument by name.
func (p *ParsedParams) GetString(name string) (string, error) {
	v, err := p.Get(name)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %q is not a string", name)
	}
	return s, nil
}

// GetOptionalString returns the argument's value if provided, or nil
// otherwise. The param must be declared as a string parameter.
func (p *ParsedParams) GetOptionalString(name string) (*string, error) {
	v, ok := p.byName[name]
	if !ok {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("parameter %q is not a string", name)
	}
	return &s, nil
}

// GetInt64 returns an int64 argument by name.
func (p *ParsedParams) GetInt64(name string) (int64, error) {
	v, err := p.Get(name)
	if err != nil {
		return 0, err
	}
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("parameter %q is not an int64", name)
	}
	return n, nil
}

// GetOptionalInt64 returns the argument's value if provided, or nil otherwise.
func (p *ParsedParams) GetOptionalInt64(name string) (*int64, error) {
	v, ok := p.byName[name]
	if !ok {
		return nil, nil
	}
	n, ok := v.(int64)
	if !ok {
		return nil, fmt.Errorf("parameter %q is not an int64", name)
	}
	return &n, nil
}

// GetFloat64 returns a float64 argument by name.
func (p *ParsedParams) GetFloat64(name string) (float64, error) {
	v, err := p.Get(name)
	if err != nil {
		return 0, err
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("parameter %q is not a float64", name)
	}
	return f, nil
}

// GetOptionalFloat64 returns the argument's value if provided, or nil.
func (p *ParsedParams) GetOptionalFloat64(name string) (*float64, error) {
	v, ok := p.byName[name]
	if !ok {
		return nil, nil
	}
	f, ok := v.(float64)
	if !ok {
		return nil, fmt.Errorf("parameter %q is not a float64", name)
	}
	return &f, nil
}

// GetBool returns a bool argument by name.
func (p *ParsedParams) GetBool(name string) (bool, error) {
	v, err := p.Get(name)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("parameter %q is not a bool", name)
	}
	return b, nil
}

// GetOptionalBool returns the argument's value if provided, or nil.
func (p *ParsedParams) GetOptionalBool(name string) (*bool, error) {
	v, ok := p.byName[name]
	if !ok {
		return nil, nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("parameter %q is not a bool", name)
	}
	return &b, nil
}
