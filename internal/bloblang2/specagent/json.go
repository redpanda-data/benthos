package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// encodeNaturalJSON converts a spectest Go value to natural JSON form.
// All numeric types are kept as-is for json.Marshal (which writes them as
// JSON numbers). After a JSON roundtrip, all numbers become float64.
//
// Returns the encoded value and true, or nil and false if the value tree
// contains types not representable in natural JSON (bytes, timestamps,
// NaN, Inf).
func encodeNaturalJSON(v any) (any, bool) {
	switch val := v.(type) {
	case nil:
		return nil, true
	case bool:
		return val, true
	case string:
		return val, true
	case int64:
		return val, true
	case int32:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		if val <= math.MaxInt64 {
			return int64(val), true
		}
		return float64(val), true
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, false
		}
		return val, true
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return f, true
	case []byte:
		return nil, false
	case time.Time:
		return nil, false
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			enc, ok := encodeNaturalJSON(item)
			if !ok {
				return nil, false
			}
			out[i] = enc
		}
		return out, true
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			enc, ok := encodeNaturalJSON(item)
			if !ok {
				return nil, false
			}
			out[k] = enc
		}
		return out, true
	default:
		return nil, false
	}
}

// coerceToNaturalJSON recursively converts all numeric types to float64.
// Used to normalize interpreter output before comparison.
func coerceToNaturalJSON(v any) any {
	switch val := v.(type) {
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = coerceToNaturalJSON(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = coerceToNaturalJSON(item)
		}
		return out
	default:
		return val // nil, bool, string, float64 pass through
	}
}

// naturalJSONEqual compares two values using natural JSON semantics.
// Both values are expected to have been through json.Unmarshal (so all
// numbers are float64) or through coerceToNaturalJSON.
func naturalJSONEqual(expected, actual any) (bool, string) {
	return naturalEqual(expected, actual, "root")
}

func naturalEqual(expected, actual any, path string) (bool, string) {
	if expected == nil && actual == nil {
		return true, ""
	}
	if expected == nil || actual == nil {
		return false, fmt.Sprintf("%s: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}

	switch ev := expected.(type) {
	case float64:
		av, ok := actual.(float64)
		if !ok {
			return false, fmt.Sprintf("%s: expected number (%v), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %v, got %v", path, ev, av)
		}
		return true, ""

	case string:
		av, ok := actual.(string)
		if !ok {
			return false, fmt.Sprintf("%s: expected string (%q), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %q, got %q", path, ev, av)
		}
		return true, ""

	case bool:
		av, ok := actual.(bool)
		if !ok {
			return false, fmt.Sprintf("%s: expected bool (%v), got %T (%v)", path, ev, actual, actual)
		}
		if ev != av {
			return false, fmt.Sprintf("%s: expected %v, got %v", path, ev, av)
		}
		return true, ""

	case []any:
		av, ok := actual.([]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected array, got %T", path, actual)
		}
		if len(ev) != len(av) {
			return false, fmt.Sprintf("%s: array length: expected %d, got %d", path, len(ev), len(av))
		}
		for i := range ev {
			if ok, diff := naturalEqual(ev[i], av[i], fmt.Sprintf("%s[%d]", path, i)); !ok {
				return false, diff
			}
		}
		return true, ""

	case map[string]any:
		av, ok := actual.(map[string]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected object, got %T", path, actual)
		}

		// Check for missing and extra keys.
		var diffs []string
		for k := range ev {
			if _, ok := av[k]; !ok {
				diffs = append(diffs, fmt.Sprintf("%s: missing key %q", path, k))
			}
		}
		for k := range av {
			if _, ok := ev[k]; !ok {
				diffs = append(diffs, fmt.Sprintf("%s: unexpected key %q", path, k))
			}
		}
		if len(diffs) > 0 {
			sort.Strings(diffs)
			return false, strings.Join(diffs, "\n")
		}

		keys := make([]string, 0, len(ev))
		for k := range ev {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ok, diff := naturalEqual(ev[k], av[k], path+"."+k); !ok {
				return false, diff
			}
		}
		return true, ""

	default:
		return false, fmt.Sprintf("%s: unexpected type %T", path, expected)
	}
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readJSONFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}
	return v, nil
}
