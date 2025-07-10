// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"fmt"
	"sort"
)

func inferFromAny(name string, v any) (*Common, error) {
	c := Common{
		Name: name,
	}

	switch t := v.(type) {
	case bool:
		c.Type = Boolean
	case int32:
		c.Type = Int32
	case int, int64:
		c.Type = Int64
	case float32:
		c.Type = Float32
	case float64:
		c.Type = Float64
	case []byte:
		c.Type = ByteArray
	case string:
		c.Type = String
	case []any:
		c.Type = Array
		for i, e := range t {
			ec, err := inferFromAny("", e)
			if err != nil {
				return nil, fmt.Errorf(".%v%v", i, err)
			}
			if i == 0 {
				c.Children = []*Common{ec}
			} else if c.Children[0].Type != ec.Type {
				return nil, fmt.Errorf(".%v mismatched array types, found %v and %v", i, c.Children[0].Type, ec.Type)
			}
		}
	case map[string]any:
		c.Type = Object

		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := t[k]

			ec, err := inferFromAny(k, v)
			if err != nil {
				return nil, fmt.Errorf(".%v%v", k, err)
			}
			c.Children = append(c.Children, ec)
		}
	case nil:
		c.Type = Null
	default:
		return nil, fmt.Errorf(" unsupported data type: %T", v)
	}

	return &c, nil
}

// InferFromAny attempts to infer a common schema from any Go value. This
// process fails if the value, or any children of a provided map/slice, are not
// within the following subset of Go types: bool, int, int32, int64, float32,
// float64, []byte, string, map[string]any, []any.
//
// All values will be recorded as non-optional.
func InferFromAny(v any) (*Common, error) {
	return inferFromAny("", v)
}
