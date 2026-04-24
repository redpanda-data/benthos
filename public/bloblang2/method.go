// Copyright 2026 Redpanda Data, Inc.

package bloblang2

import (
	"fmt"
	"time"
)

// Method is the runtime closure implementing a plugin method. It is invoked
// once per method-call evaluation with the (already-evaluated) receiver value.
type Method func(v any) (any, error)

// MethodConstructor constructs a Method from arguments resolved against a
// PluginSpec. When all arguments at a call site are literal, the constructor
// is invoked once at parse time and its Method is reused across every Query.
// When any argument is dynamic, the constructor is invoked per call.
type MethodConstructor func(args *ParsedParams) (Method, error)

// StringMethod wraps a string-receiver function with a type check.
//
// V2 typed wrappers are strict: they do not coerce non-string inputs. Callers
// whose upstream value might be a number or bytes should use .string() in the
// mapping to coerce before invoking the method.
func StringMethod(fn func(string) (any, error)) Method {
	return func(v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string receiver, got %T", v)
		}
		return fn(s)
	}
}

// BytesMethod wraps a []byte-receiver function with a type check.
//
// Strict: does not coerce strings. Use .bytes() in the mapping if the
// upstream value is a string.
func BytesMethod(fn func([]byte) (any, error)) Method {
	return func(v any) (any, error) {
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected bytes receiver, got %T", v)
		}
		return fn(b)
	}
}

// Int64Method wraps an int64-receiver function.
//
// Widens within the integer family: int32, uint32, and in-range uint64 are
// accepted. Strings and floats are not coerced — use .int64() in the mapping
// for broader conversion.
func Int64Method(fn func(int64) (any, error)) Method {
	return func(v any) (any, error) {
		n, err := coerceInt64(v)
		if err != nil {
			return nil, err
		}
		return fn(n)
	}
}

// Float64Method wraps a float64-receiver function.
//
// Widens within the numeric family: float32, int32, int64, uint32, uint64
// are accepted. Strings are not coerced — use .float64() in the mapping if
// the upstream value is a string.
func Float64Method(fn func(float64) (any, error)) Method {
	return func(v any) (any, error) {
		f, err := coerceFloat64(v)
		if err != nil {
			return nil, err
		}
		return fn(f)
	}
}

// BoolMethod wraps a bool-receiver function with a type check.
//
// Strict: does not coerce from strings, ints, or floats. Use .bool() in the
// mapping for coercion.
func BoolMethod(fn func(bool) (any, error)) Method {
	return func(v any) (any, error) {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool receiver, got %T", v)
		}
		return fn(b)
	}
}

// TimestampMethod wraps a time.Time-receiver function with a type check.
//
// Strict: does not coerce from strings or unix timestamp numbers. Use
// .ts_parse() or .ts_from_unix() in the mapping first.
func TimestampMethod(fn func(time.Time) (any, error)) Method {
	return func(v any) (any, error) {
		t, ok := v.(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected timestamp receiver, got %T", v)
		}
		return fn(t)
	}
}

// ArrayMethod wraps an []any-receiver function with a type check.
//
// Strict: the receiver must already be an array.
func ArrayMethod(fn func([]any) (any, error)) Method {
	return func(v any) (any, error) {
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected array receiver, got %T", v)
		}
		return fn(arr)
	}
}

// ObjectMethod wraps a map[string]any-receiver function with a type check.
//
// Strict: the receiver must already be an object.
func ObjectMethod(fn func(map[string]any) (any, error)) Method {
	return func(v any) (any, error) {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object receiver, got %T", v)
		}
		return fn(obj)
	}
}
