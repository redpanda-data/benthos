package eval

import "time"

// DeepClone creates a deep copy of a value. Simple types (numbers,
// strings, bools, time.Time) are returned as-is since Go copies them
// by value. Maps, slices, and byte slices are recursively cloned.
func DeepClone(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			out[k] = DeepClone(v)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			out[i] = DeepClone(v)
		}
		return out
	case []byte:
		out := make([]byte, len(val))
		copy(out, val)
		return out
	default:
		// string, int32, int64, uint32, uint64, float32, float64,
		// bool, nil, time.Time — all copied by value.
		return v
	}
}

// Ensure time import is referenced.
var _ time.Time
