package spectest

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

// DeepEqual compares expected and actual values using spec-aware semantics.
// Returns true if they match, or false with a human-readable diff message.
//
// Differences from reflect.DeepEqual:
//   - NaN == NaN is true (for test assertions)
//   - float32 and float64 are compared within their own type (no cross-type promotion)
//   - time.Time uses .Equal() for timezone-aware comparison
//   - Produces a path-annotated diff message on mismatch
func DeepEqual(expected, actual any) (bool, string) {
	return deepEqual(expected, actual, "")
}

func deepEqual(expected, actual any, path string) (bool, string) {
	if path == "" {
		path = "root"
	}

	// Both nil.
	if expected == nil && actual == nil {
		return true, ""
	}
	if expected == nil || actual == nil {
		return false, fmt.Sprintf("%s: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}

	// Type must match exactly for typed values.
	et := reflect.TypeOf(expected)
	at := reflect.TypeOf(actual)
	if et != at {
		return false, fmt.Sprintf("%s: type mismatch: expected %T, got %T (expected value: %v, actual value: %v)", path, expected, actual, expected, actual)
	}

	switch ev := expected.(type) {
	case map[string]any:
		av := actual.(map[string]any)
		return compareMaps(ev, av, path)
	case []any:
		av := actual.([]any)
		return compareSlices(ev, av, path)
	case float64:
		av := actual.(float64)
		return compareFloat64(ev, av, path)
	case float32:
		av := actual.(float32)
		return compareFloat32(ev, av, path)
	case time.Time:
		av := actual.(time.Time)
		if ev.Equal(av) {
			return true, ""
		}
		return false, fmt.Sprintf("%s: timestamp mismatch: expected %s, got %s", path, ev.Format(time.RFC3339Nano), av.Format(time.RFC3339Nano))
	case []byte:
		av := actual.([]byte)
		if bytes.Equal(ev, av) {
			return true, ""
		}
		return false, fmt.Sprintf("%s: bytes mismatch: expected %v, got %v", path, ev, av)
	default:
		if expected == actual {
			return true, ""
		}
		return false, fmt.Sprintf("%s: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}
}

func compareMaps(expected, actual map[string]any, path string) (bool, string) {
	// Check for missing and extra keys.
	var diffs []string
	for k := range expected {
		if _, ok := actual[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s: missing key %q", path, k))
		}
	}
	for k := range actual {
		if _, ok := expected[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s: unexpected key %q", path, k))
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		return false, strings.Join(diffs, "\n")
	}

	// Compare values for each key.
	keys := make([]string, 0, len(expected))
	for k := range expected {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		ok, diff := deepEqual(expected[k], actual[k], path+"."+k)
		if !ok {
			return false, diff
		}
	}
	return true, ""
}

func compareSlices(expected, actual []any, path string) (bool, string) {
	if len(expected) != len(actual) {
		return false, fmt.Sprintf("%s: array length mismatch: expected %d, got %d", path, len(expected), len(actual))
	}
	for i := range expected {
		ok, diff := deepEqual(expected[i], actual[i], fmt.Sprintf("%s[%d]", path, i))
		if !ok {
			return false, diff
		}
	}
	return true, ""
}

func compareFloat64(expected, actual float64, path string) (bool, string) {
	// NaN == NaN for test assertion purposes.
	if math.IsNaN(expected) && math.IsNaN(actual) {
		return true, ""
	}
	// -0.0 == 0.0 per spec (they are equal per IEEE 754).
	if expected == 0 && actual == 0 {
		return true, ""
	}
	// Bitwise comparison for exact values (handles ±Inf).
	if math.Float64bits(expected) == math.Float64bits(actual) {
		return true, ""
	}
	return false, fmt.Sprintf("%s: float64 mismatch: expected %v, got %v", path, expected, actual)
}

func compareFloat32(expected, actual float32, path string) (bool, string) {
	// NaN == NaN for test assertion purposes.
	if math.IsNaN(float64(expected)) && math.IsNaN(float64(actual)) {
		return true, ""
	}
	// -0.0 == 0.0 per spec.
	if expected == 0 && actual == 0 {
		return true, ""
	}
	if math.Float32bits(expected) == math.Float32bits(actual) {
		return true, ""
	}
	return false, fmt.Sprintf("%s: float32 mismatch: expected %v, got %v", path, expected, actual)
}

// CheckOutputType verifies that actual has the expected Bloblang type name.
func CheckOutputType(expectedType string, actual any) (bool, string) {
	actualType := goTypeToBloblangType(actual)
	if actualType == expectedType {
		return true, ""
	}
	return false, fmt.Sprintf("output type: expected %q, got %q (%T)", expectedType, actualType, actual)
}

func goTypeToBloblangType(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case int32:
		return "int32"
	case int64:
		return "int64"
	case uint32:
		return "uint32"
	case uint64:
		return "uint64"
	case float32:
		return "float32"
	case float64:
		return "float64"
	case bool:
		return "bool"
	case []byte:
		return "bytes"
	case time.Time:
		return "timestamp"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("unknown(%T)", v)
	}
}
