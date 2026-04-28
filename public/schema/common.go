// Copyright 2025 Redpanda Data, Inc.

// Package schema implements a common standard for describing data schemas
// within the domain of benthos. The intention for these schemas is to encourage
// schema conversion between multiple common formats such as avro, parquet, and
// so on.
//
// # Schema Identification and Caching
//
// To optimize performance when converting schemas between formats, this package
// provides fingerprinting and caching mechanisms:
//
//   - SchemaCache: A thread-safe cache for storing converted schemas
//
// This allows downstream components to lazily perform conversions only once per
// unique schema identifier, avoiding redundant ToAny/FromAny serialization and
// expensive format translations.
//
// Example usage:
//
//	// Create a cache for Parquet schema conversions
//	cache := schema.NewSchemaCache(func(c schema.Common) (ParquetSchema, error) {
//	    return convertToParquet(c)
//	})
//
//	// First access converts and caches
//	parquet1, err := cache.GetOrConvert(mySchema)
//
//	// Second access uses cached result (no conversion)
//	parquet2, err := cache.GetOrConvert(mySchema)
//
// # Optimized Cache Lookups with Any Format
//
// When schemas are serialized to Any format (map[string]any), a fingerprint
// field is automatically included. This enables optimized cache lookups:
//
//	// Producer side: export schema (fingerprint included automatically)
//	schema := schema.Common{Type: schema.String, Name: "id"}
//	anySchema := schema.ToAny()
//	// ... send anySchema over network or store it ...
//
//	// Consumer side: optimized cache lookup
//	cache := schema.NewSchemaCache(convertFunc)
//	result, err := cache.GetOrConvertFromAny(anySchema)
//	// Fast path: if cached, avoids ParseFromAny and Fingerprint calculation
//
// This optimization is particularly useful in scenarios where schemas are
// transmitted over the network or stored in external systems, as it eliminates
// the need to parse and recalculate fingerprints on cache hits.
//
// # Parameterised logical types
//
// Some types carry parameters beyond what the type identifier conveys. For
// example, a Decimal requires a precision and a scale. These parameters are
// attached to the [Common] schema via the [Common.Logical] field, which holds
// a [LogicalParams] struct. Only the field within [LogicalParams] that
// corresponds to the schema's [Common.Type] should be set.
//
// Use [Common.Validate] to confirm a schema's parameters are internally
// consistent before relying on it.
package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

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
	Any       CommonType = 14
	Decimal   CommonType = 15
)

// Decimal precision bounds. The upper bound matches the widest precision that
// can be represented losslessly across Avro, Parquet and Oracle NUMBER.
const (
	DecimalMinPrecision int32 = 1
	DecimalMaxPrecision int32 = 38
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
	case Any:
		return "ANY"
	case Decimal:
		return "DECIMAL"
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
	case "ANY":
		return Any, nil
	case "DECIMAL":
		return Decimal, nil
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

	// Logical holds parameters for parameterised types (e.g. Decimal). Only
	// the field within LogicalParams that corresponds to Type should be
	// populated; setting parameters that do not apply to the type is a
	// validation error.
	Logical *LogicalParams
}

// LogicalParams groups the optional parameter blocks that parameterised
// CommonType values may carry. Each parameterised type has its own field;
// at most one is expected to be non-nil for any given Common schema.
type LogicalParams struct {
	Decimal *DecimalParams
}

// DecimalParams describes a fixed-precision decimal number.
//
// Precision is the total number of significant digits and must be in
// [DecimalMinPrecision, DecimalMaxPrecision]. Scale is the number of digits
// to the right of the decimal point and must be in [0, Precision]. These
// constraints describe the lossless intersection across Avro, Parquet and
// Oracle NUMBER.
type DecimalParams struct {
	Precision int32
	Scale     int32
}

const (
	anyFieldType        = "type"
	anyFieldName        = "name"
	anyFieldOptional    = "optional"
	anyFieldChildren    = "children"
	anyFieldFingerprint = "fingerprint"
	anyFieldPrecision   = "precision"
	anyFieldScale       = "scale"
)

// ToAny serializes the common schema into a generic Go value, with structured
// schemas being represented as map[string]any and []any. This could be further
// manipulated using generic mapping tools such as bloblang, before either
// bringing back into a Common representation or serializing into another
// format.
//
// The serialized format includes a "fingerprint" field at the top level, which
// can be used to optimize cache lookups via SchemaCache.GetOrConvertFromAny,
// avoiding the need to parse the Any format and recalculate the fingerprint.
//
// NOTE: Ironically, the schema for this serialization is not something that can
// be precisely represented as a Common schema. The Children field requires an
// Array of structured schema objects, which cannot be described accurately
// within the Common type system.
func (c *Common) ToAny() any {
	m := map[string]any{
		anyFieldType:        c.Type.String(),
		anyFieldFingerprint: c.fingerprint(),
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

	if c.Type == Decimal && c.Logical != nil && c.Logical.Decimal != nil {
		m[anyFieldPrecision] = int64(c.Logical.Decimal.Precision)
		m[anyFieldScale] = int64(c.Logical.Decimal.Scale)
	}

	return m
}

// ParseFromAny deserializes a common schema from a generic Go value. The
// resulting schema is validated via [Common.Validate] before being returned.
func ParseFromAny(v any) (Common, error) {
	c, err := parseFromAnyNoValidate(v)
	if err != nil {
		return c, err
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// parseFromAnyNoValidate performs the structural deserialisation without
// running [Common.Validate]. It is used internally so that recursive parsing
// of nested children does not validate each subtree O(depth) times; the
// public [ParseFromAny] entry point validates exactly once at the top level.
func parseFromAnyNoValidate(v any) (Common, error) {
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
			return c, fmt.Errorf("expected field `optional` of type bool, got %T", obj[anyFieldOptional])
		}
	}

	if cArr, ok := obj[anyFieldChildren].([]any); ok {
		for i, cEle := range cArr {
			cChild, err := parseFromAnyNoValidate(cEle)
			if err != nil {
				return c, fmt.Errorf("child element %v: %w", i, err)
			}

			c.Children = append(c.Children, cChild)
		}
	}

	_, hasPrecision := obj[anyFieldPrecision]
	_, hasScale := obj[anyFieldScale]
	if hasPrecision || hasScale {
		if c.Type != Decimal {
			return c, fmt.Errorf("fields `precision` and `scale` are only valid for type DECIMAL, got %v", c.Type)
		}
		if !hasPrecision {
			return c, errors.New("type DECIMAL requires field `precision`")
		}
		if !hasScale {
			return c, errors.New("type DECIMAL requires field `scale`")
		}

		precision, err := anyIntField(obj, anyFieldPrecision)
		if err != nil {
			return c, err
		}
		scale, err := anyIntField(obj, anyFieldScale)
		if err != nil {
			return c, err
		}

		c.Logical = &LogicalParams{
			Decimal: &DecimalParams{
				Precision: precision,
				Scale:     scale,
			},
		}
	} else if c.Type == Decimal {
		return c, errors.New("type DECIMAL requires fields `precision` and `scale`")
	}

	return c, nil
}

// anyIntField extracts an integer-valued field from a map[string]any,
// accepting any of the integer or float numeric types that JSON-derived maps
// commonly produce. Float values must have no fractional part.
func anyIntField(obj map[string]any, key string) (int32, error) {
	v, ok := obj[key]
	if !ok {
		return 0, fmt.Errorf("missing field `%s`", key)
	}

	switch n := v.(type) {
	case int:
		return int32Bounded(int64(n), key)
	case int32:
		return n, nil
	case int64:
		return int32Bounded(n, key)
	case float32:
		if float32(int64(n)) != n {
			return 0, fmt.Errorf("field `%s` must be an integer, got %v", key, n)
		}
		return int32Bounded(int64(n), key)
	case float64:
		if float64(int64(n)) != n {
			return 0, fmt.Errorf("field `%s` must be an integer, got %v", key, n)
		}
		return int32Bounded(int64(n), key)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("field `%s` must be an integer, got %v", key, n)
		}
		return int32Bounded(i, key)
	default:
		return 0, fmt.Errorf("expected field `%s` of integer type, got %T", key, v)
	}
}

func int32Bounded(n int64, key string) (int32, error) {
	const maxInt32 = int64(^uint32(0) >> 1)
	const minInt32 = -maxInt32 - 1
	if n < minInt32 || n > maxInt32 {
		return 0, fmt.Errorf("field `%s` value %d does not fit in int32", key, n)
	}
	return int32(n), nil
}

// Validate enforces the parameter invariants of parameterised types
// (currently only [Decimal]) and that no parameter blocks are attached to
// types that do not accept them. It recurses into [Common.Children].
//
// Structural invariants — for example that an [Object] has children, or that
// a [Union] has more than one child — are not currently enforced; the
// validation surface may grow as new logical types arrive.
//
// Schemas constructed via [ParseFromAny] are validated automatically. Schemas
// constructed by struct literal should call Validate before being passed to
// converters or caches.
func (c *Common) Validate() error {
	if c.Type == Decimal {
		if c.Logical == nil || c.Logical.Decimal == nil {
			return errors.New("type DECIMAL requires Logical.Decimal parameters")
		}
		d := c.Logical.Decimal
		if d.Precision < DecimalMinPrecision || d.Precision > DecimalMaxPrecision {
			return fmt.Errorf("decimal precision %d out of range [%d, %d]", d.Precision, DecimalMinPrecision, DecimalMaxPrecision)
		}
		if d.Scale < 0 || d.Scale > d.Precision {
			return fmt.Errorf("decimal scale %d out of range [0, precision=%d]", d.Scale, d.Precision)
		}
	} else if c.Logical != nil && c.Logical.Decimal != nil {
		return fmt.Errorf("Logical.Decimal parameters are only valid for type DECIMAL, got %v", c.Type)
	}

	for i, child := range c.Children {
		if err := child.Validate(); err != nil {
			return fmt.Errorf("child %d (%q): %w", i, child.Name, err)
		}
	}

	return nil
}

// Fingerprint returns a deterministic hash identifier for the schema structure.
// Two schemas with the same structure will produce the same fingerprint,
// regardless of memory location. This is useful for caching schema conversions
// to avoid redundant serialization and translation operations.
//
// The fingerprint is computed using SHA-256 and returned as a hex-encoded string.
func (c *Common) fingerprint() string {
	h := sha256.New()
	c.writeFingerprint(h)
	return hex.EncodeToString(h.Sum(nil))
}

// writeFingerprint writes a canonical representation of the schema to the hash
func (c *Common) writeFingerprint(w io.Writer) {
	// Write type as its integer value for stability
	fmt.Fprintf(w, "T:%d|", c.Type)

	// Write name
	fmt.Fprintf(w, "N:%s|", c.Name)

	// Write optional flag
	if c.Optional {
		fmt.Fprint(w, "O:1|")
	} else {
		fmt.Fprint(w, "O:0|")
	}

	// Write parameters for parameterised types. Only emitted when present so
	// that schemas of unparameterised types retain their existing fingerprint.
	if c.Type == Decimal && c.Logical != nil && c.Logical.Decimal != nil {
		fmt.Fprintf(w, "D:%d:%d|", c.Logical.Decimal.Precision, c.Logical.Decimal.Scale)
	}

	// Write children count and recursively fingerprint each child
	fmt.Fprintf(w, "C:%d|", len(c.Children))
	for i, child := range c.Children {
		fmt.Fprintf(w, "[%d:", i)
		child.writeFingerprint(w)
		fmt.Fprint(w, "]")
	}
}
