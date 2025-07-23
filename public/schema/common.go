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
	Boolean   CommonType = 1
	Int32     CommonType = 2
	Int64     CommonType = 3
	Float32   CommonType = 4
	Float64   CommonType = 5
	String    CommonType = 6
	ByteArray CommonType = 7
	Object    CommonType = 8
	Map       CommonType = 9
	Array     CommonType = 10
	Null      CommonType = 11
	Union     CommonType = 12
	Timestamp CommonType = 13
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
	case Timestamp:
		return "TIMESTAMP"
	default:
		return "UNKNOWN"
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
	case "TIMESTAMP":
		return Timestamp, nil
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
	Children []Common
}

const (
	anyFieldType     = "type"
	anyFieldName     = "name"
	anyFieldOptional = "optional"
	anyFieldChildren = "children"
)

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
		anyFieldType: c.Type.String(),
	}

	if c.Name != "" {
		m[anyFieldName] = c.Name
	}

	if c.Optional {
		m[anyFieldOptional] = true
	}

	if len(c.Children) > 0 {
		children := make([]any, len(c.Children))
		for i, child := range c.Children {
			children[i] = child.ToAny()
		}
		m[anyFieldChildren] = children
	}

	return m
}

// ParseFromAny deserializes a common schema from a generic Go value.
func ParseFromAny(v any) (Common, error) {
	var c Common

	obj, ok := v.(map[string]any)
	if !ok {
		return c, fmt.Errorf("expected map, received: %T", v)
	}

	if typeStr, ok := obj[anyFieldType].(string); ok {
		var err error
		if c.Type, err = typeFromStr(typeStr); err != nil {
			return c, err
		}
	} else {
		return c, fmt.Errorf("expected field `type` of type string, got %T", obj[anyFieldType])
	}

	if name, ok := obj[anyFieldName]; ok {
		if nameStr, ok := name.(string); ok {
			c.Name = nameStr
		} else {
			return c, fmt.Errorf("expected field `name` of type string, got %T", obj[anyFieldName])
		}
	}

	if optional, ok := obj[anyFieldOptional]; ok {
		if optionalB, ok := optional.(bool); ok {
			c.Optional = optionalB
		} else {
			return c, fmt.Errorf("expected field `optional` of type string, got %T", obj[anyFieldOptional])
		}
	}

	if cArr, ok := obj[anyFieldChildren].([]any); ok {
		for i, cEle := range cArr {
			cChild, err := ParseFromAny(cEle)
			if err != nil {
				return c, fmt.Errorf("child element %v: %w", i, err)
			}

			c.Children = append(c.Children, cChild)
		}
	}

	return c, nil
}
