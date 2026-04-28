// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func inferFromAny(name string, v any) (Common, error) {
	c := Common{Name: name}

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
	case json.Number:
		// json.Number is produced by json.Decoder.UseNumber(); it has no
		// int-vs-float discriminator, so try integer parsing first and fall
		// back to float.
		if _, err := t.Int64(); err == nil {
			c.Type = Int64
		} else if _, err := t.Float64(); err == nil {
			c.Type = Float64
		} else {
			return c, fmt.Errorf(" json.Number value %q is not parseable as int64 or float64", string(t))
		}
	case []byte:
		c.Type = ByteArray
	case string:
		c.Type = String
	case time.Time:
		c.Type = Timestamp
	case []any:
		c.Type = Array
		for i, e := range t {
			ec, err := inferFromAny("", e)
			if err != nil {
				return c, fmt.Errorf(".%v%v", i, err)
			}
			if i == 0 {
				c.Children = []Common{ec}
			} else if c.Children[0].Type != ec.Type {
				return c, fmt.Errorf(".%v mismatched array types, found %v and %v", i, c.Children[0].Type, ec.Type)
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
				return c, fmt.Errorf(".%v%v", k, err)
			}
			c.Children = append(c.Children, ec)
		}
	case nil:
		c.Type = Null
	default:
		return c, fmt.Errorf(" unsupported data type: %T", v)
	}

	return c, nil
}

// InferFromAny attempts to infer a common schema from any Go value. This
// process fails if the value, or any children of a provided map/slice, are not
// within the following subset of Go types: bool, int, int32, int64, float32,
// float64, [encoding/json.Number], []byte, string, map[string]any, []any.
//
// [encoding/json.Number] values are inferred as Int64 when they parse as an
// integer and as Float64 otherwise.
//
// Decimal types (both [Decimal] and [BigDecimal]) cannot be inferred from
// generic Go values and must be constructed explicitly via [NewDecimal] or
// [NewBigDecimal].
//
// All values will be recorded as non-optional.
func InferFromAny(v any) (Common, error) {
	return inferFromAny("", v)
}
