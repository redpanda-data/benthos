// Copyright 2025 Redpanda Data, Inc.

// Package schema implements a common standard for describing data schemas
// within the domain of benthos. The intention for these schemas is to encourage
// schema conversion between multiple common formats such as avro, parquet, and
// so on.
package schema

import "fmt"

// CommonType represents types supported by common schemas.
type CommonType int

// Supported common types
const (
	Boolean           CommonType = 0
	Int32             CommonType = 1
	Int64             CommonType = 2
	Float32           CommonType = 3
	Float64           CommonType = 4
	String            CommonType = 5
	ByteArray         CommonType = 6
	FixedLenByteArray CommonType = 7
	Object            CommonType = 8
	Map               CommonType = 9
	Array             CommonType = 10
	Null              CommonType = 11
	Union             CommonType = 12
)

// String returns a human readable string representation of the type.
func (t CommonType) String() string {
	switch t {
	case Boolean:
		return "BOOLEAN"
	case Int32:
		return "INT32"
	case Int64:
		return "INT64"
	case Float32:
		return "FLOAT32"
	case Float64:
		return "FLOAT64"
	case String:
		return "STRING"
	case ByteArray:
		return "BYTE_ARRAY"
	case FixedLenByteArray:
		return "FIXED_LEN_BYTE_ARRAY"
	case Object:
		return "OBJECT"
	case Map:
		return "MAP"
	case Array:
		return "ARRAY"
	case Null:
		return "NULL"
	case Union:
		return "UNION"
	default:
		return "Type(?)"
	}
}

func typeFromStr(v string) (CommonType, error) {
	switch v {
	case "BOOLEAN":
		return Boolean, nil
	case "INT32":
		return Int32, nil
	case "INT64":
		return Int64, nil
	case "FLOAT32":
		return Float32, nil
	case "FLOAT64":
		return Float64, nil
	case "STRING":
		return String, nil
	case "BYTE_ARRAY":
		return ByteArray, nil
	case "FIXED_LEN_BYTE_ARRAY":
		return FixedLenByteArray, nil
	case "OBJECT":
		return Object, nil
	case "MAP":
		return Map, nil
	case "ARRAY":
		return Array, nil
	case "NULL":
		return Null, nil
	case "UNION":
		return Union, nil
	default:
		return 0, fmt.Errorf("unrecognised type string: %v", v)
	}
}

// Common schema is a neutral form that can be converted to and from other
// schemas. This is not intended to be a superset of all schema capabilites and
// instead focuses on compatibility and minimum viable translations between
// schemas.
type Common struct {
	Name     string
	Type     CommonType
	Optional bool
	Children []*Common
}

// ToAny serializes the common schema into a generic Go value, with structured
// schemas being represented as map[string]any and []any. This could be further
// manipulated using generic mapping tools such as bloblang, before either
// bringing back into a Common representation or serializing into another
// format.
//
// NOTE: Ironically, the schema for this serialization is not something that can
// actually be represented as a Common schema. This is because we do not support
// schemas that nest complex types, which would be necessary for representing
// the Children field.
func (c *Common) ToAny() any {
	m := map[string]any{
		"type": c.Type.String(),
	}

	if c.Name != "" {
		m["name"] = c.Name
	}

	if c.Optional {
		m["optional"] = true
	}

	if len(c.Children) > 0 {
		children := make([]any, len(c.Children))
		for i, child := range c.Children {
			children[i] = child.ToAny()
		}
		m["children"] = children
	}

	return m
}

// ParseFromAny deserializes a common schema from a generic Go value.
func ParseFromAny(v any) (*Common, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, received: %T", v)
	}

	c := &Common{}

	if typeStr, ok := obj["type"].(string); ok {
		var err error
		if c.Type, err = typeFromStr(typeStr); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("expected field `type` of type string, got %T", obj["type"])
	}

	if name, ok := obj["name"]; ok {
		if nameStr, ok := name.(string); ok {
			c.Name = nameStr
		} else {
			return nil, fmt.Errorf("expected field `name` of type string, got %T", obj["name"])
		}
	}

	if optional, ok := obj["optional"]; ok {
		if optionalB, ok := optional.(bool); ok {
			c.Optional = optionalB
		} else {
			return nil, fmt.Errorf("expected field `optional` of type string, got %T", obj["optional"])
		}
	}

	if cArr, ok := obj["children"].([]any); ok {
		for i, cEle := range cArr {
			cChild, err := ParseFromAny(cEle)
			if err != nil {
				return nil, fmt.Errorf("child element %v: %w", i, err)
			}

			c.Children = append(c.Children, cChild)
		}
	}

	return c, nil
}
