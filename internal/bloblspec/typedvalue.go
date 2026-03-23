package bloblspec

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"time"
)

// NormalizeYAMLValue recursively converts yaml.v3 decoded values to the
// canonical Go types expected by the rest of the package:
//   - int → int64 (yaml.v3 decodes bare integers as int)
//   - map keys are asserted to string
//   - slices and maps are recursively normalized
func NormalizeYAMLValue(v any) any {
	switch val := v.(type) {
	case int:
		return int64(val)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			out[k] = NormalizeYAMLValue(v)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			out[fmt.Sprintf("%v", k)] = NormalizeYAMLValue(v)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			out[i] = NormalizeYAMLValue(v)
		}
		return out
	default:
		return v
	}
}

// DecodeTypedValues recursively walks a value tree and decodes type
// annotations of the form {_type: "typename", value: "string_value"}
// into the corresponding Go types.
//
// A map is treated as a type annotation only when it has exactly two
// keys: "_type" and "value", both with string values.
func DecodeTypedValues(v any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 2 {
			typeName, hasType := val["_type"].(string)
			valueStr, hasValue := val["value"].(string)
			if hasType && hasValue {
				return decodeTypedValue(typeName, valueStr)
			}
		}
		out := make(map[string]any, len(val))
		for k, v := range val {
			decoded, err := DecodeTypedValues(v)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			out[k] = decoded
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			decoded, err := DecodeTypedValues(v)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			out[i] = decoded
		}
		return out, nil
	default:
		return v, nil
	}
}

func decodeTypedValue(typeName, valueStr string) (any, error) {
	switch typeName {
	case "int32":
		n, err := strconv.ParseInt(valueStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("decoding int32 %q: %w", valueStr, err)
		}
		return int32(n), nil
	case "int64":
		n, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("decoding int64 %q: %w", valueStr, err)
		}
		return n, nil
	case "uint32":
		n, err := strconv.ParseUint(valueStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("decoding uint32 %q: %w", valueStr, err)
		}
		return uint32(n), nil
	case "uint64":
		n, err := strconv.ParseUint(valueStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("decoding uint64 %q: %w", valueStr, err)
		}
		return n, nil
	case "float32":
		f, err := parseFloat(valueStr)
		if err != nil {
			return nil, fmt.Errorf("decoding float32 %q: %w", valueStr, err)
		}
		return float32(f), nil
	case "float64":
		f, err := parseFloat(valueStr)
		if err != nil {
			return nil, fmt.Errorf("decoding float64 %q: %w", valueStr, err)
		}
		return f, nil
	case "bytes":
		b, err := base64.StdEncoding.DecodeString(valueStr)
		if err != nil {
			return nil, fmt.Errorf("decoding bytes (base64) %q: %w", valueStr, err)
		}
		return b, nil
	case "timestamp":
		t, err := time.Parse(time.RFC3339Nano, valueStr)
		if err != nil {
			return nil, fmt.Errorf("decoding timestamp %q: %w", valueStr, err)
		}
		return t, nil
	default:
		return nil, fmt.Errorf("unknown _type %q", typeName)
	}
}

// parseFloat handles special float string values (NaN, Infinity, -0.0).
func parseFloat(s string) (float64, error) {
	switch s {
	case "NaN":
		return math.NaN(), nil
	case "Infinity":
		return math.Inf(1), nil
	case "-Infinity":
		return math.Inf(-1), nil
	case "-0.0":
		return math.Float64frombits(1 << 63), nil // negative zero
	default:
		return strconv.ParseFloat(s, 64)
	}
}

// DecodeValue is a convenience that applies NormalizeYAMLValue then
// DecodeTypedValues in sequence.
func DecodeValue(v any) (any, error) {
	return DecodeTypedValues(NormalizeYAMLValue(v))
}
