// Copyright 2025 Redpanda Data, Inc.

package schema_test

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/public/schema"
)

// ParquetSchema represents a target schema format (simplified example)
type ParquetSchema struct {
	Name   string
	Fields []ParquetField
}

type ParquetField struct {
	Name string
	Type string
}

// convertToParquet simulates converting a Common schema to Parquet format
func convertToParquet(c schema.Common) (ParquetSchema, error) {
	ps := ParquetSchema{Name: c.Name}

	for _, child := range c.Children {
		field := ParquetField{
			Name: child.Name,
			Type: child.Type.String(),
		}
		ps.Fields = append(ps.Fields, field)
	}

	return ps, nil
}

// ExampleSchemaCache demonstrates using SchemaCache to avoid redundant conversions
func ExampleSchemaCache() {
	// Create a cache for Parquet schema conversions
	cache := schema.NewSchemaCache(convertToParquet)

	// Define a common schema
	userSchema := schema.Common{
		Type: schema.Object,
		Name: "user",
		Children: []schema.Common{
			{Type: schema.String, Name: "id"},
			{Type: schema.String, Name: "email"},
			{Type: schema.Int64, Name: "age", Optional: true},
		},
	}

	// First conversion - will call convertToParquet
	parquet1, err := cache.GetOrConvert(userSchema)
	if err != nil {
		panic(err)
	}
	fmt.Printf("First conversion: %s with %d fields\n", parquet1.Name, len(parquet1.Fields))

	// Second conversion - will use cached result (no conversion)
	parquet2, err := cache.GetOrConvert(userSchema)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Cached result: %s with %d fields\n", parquet2.Name, len(parquet2.Fields))

	// Check cache size
	fmt.Printf("Cache size: %d\n", cache.Size())

	// Output:
	// First conversion: user with 3 fields
	// Cached result: user with 3 fields
	// Cache size: 1
}

// ExampleCommon_Fingerprint demonstrates generating schema fingerprints
func ExampleCommon_Fingerprint() {
	schema1 := schema.Common{
		Type: schema.Object,
		Name: "person",
		Children: []schema.Common{
			{Type: schema.String, Name: "name"},
			{Type: schema.Int64, Name: "age"},
		},
	}

	schema2 := schema.Common{
		Type: schema.Object,
		Name: "person",
		Children: []schema.Common{
			{Type: schema.String, Name: "name"},
			{Type: schema.Int64, Name: "age"},
		},
	}

	schema3 := schema.Common{
		Type: schema.Object,
		Name: "person",
		Children: []schema.Common{
			{Type: schema.String, Name: "name"},
			{Type: schema.String, Name: "email"}, // Different field
		},
	}

	fp1 := schema1.Fingerprint()
	fp2 := schema2.Fingerprint()
	fp3 := schema3.Fingerprint()

	fmt.Printf("Schema1 == Schema2: %t\n", fp1 == fp2)
	fmt.Printf("Schema1 == Schema3: %t\n", fp1 == fp3)

	// Output:
	// Schema1 == Schema2: true
	// Schema1 == Schema3: false
}

// ExampleSchemaCache_manualPut demonstrates manually populating the cache
func ExampleSchemaCache_manualPut() {
	cache := schema.NewSchemaCache(convertToParquet)

	userSchema := schema.Common{
		Type: schema.Object,
		Name: "user",
		Children: []schema.Common{
			{Type: schema.String, Name: "id"},
		},
	}

	// Manually create and cache a Parquet schema
	manualParquet := ParquetSchema{
		Name: "user_custom",
		Fields: []ParquetField{
			{Name: "id", Type: "STRING"},
			{Name: "extra_field", Type: "INT64"},
		},
	}

	fingerprint := userSchema.Fingerprint()
	cache.Put(fingerprint, manualParquet)

	// GetOrConvert will now return the manually set value
	result, _ := cache.GetOrConvert(userSchema)
	fmt.Printf("Result name: %s, fields: %d\n", result.Name, len(result.Fields))

	// Output:
	// Result name: user_custom, fields: 2
}

// ExampleSchemaCache_GetOrConvertFromAny demonstrates optimized cache usage with Any format
func ExampleSchemaCache_GetOrConvertFromAny() {
	cache := schema.NewSchemaCache(convertToParquet)

	// Define a schema
	userSchema := schema.Common{
		Type: schema.Object,
		Name: "user",
		Children: []schema.Common{
			{Type: schema.String, Name: "id"},
			{Type: schema.String, Name: "email"},
			{Type: schema.Int64, Name: "age"},
		},
	}

	// Convert to Any format (fingerprint included automatically)
	// This is typically done by the producer/sender
	anySchema := userSchema.ToAny()

	// First call: converts and caches (requires parsing Any -> Common)
	result1, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		panic(err)
	}
	fmt.Printf("First call: %s with %d fields\n", result1.Name, len(result1.Fields))

	// Second call: uses cached result (optimized - skips parsing and fingerprint calculation)
	result2, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Second call (cached): %s with %d fields\n", result2.Name, len(result2.Fields))

	fmt.Printf("Cache size: %d\n", cache.Size())

	// Output:
	// First call: user with 3 fields
	// Second call (cached): user with 3 fields
	// Cache size: 1
}

// ExampleCommon_ToAny demonstrates that ToAny includes fingerprints
func ExampleCommon_ToAny() {
	userSchema := schema.Common{
		Type: schema.Object,
		Name: "user",
		Children: []schema.Common{
			{Type: schema.String, Name: "id"},
			{Type: schema.String, Name: "email"},
		},
	}

	// Export to Any format (fingerprint included automatically)
	anySchema := userSchema.ToAny()

	// The result includes a "fingerprint" field at the top level
	if m, ok := anySchema.(map[string]any); ok {
		if fp, ok := m["fingerprint"].(string); ok {
			fmt.Printf("Has fingerprint: %t\n", len(fp) > 0)
			fmt.Printf("Fingerprint length: %d\n", len(fp))
		}
	}

	// This format can be sent over the network, stored, etc.
	// When received, GetOrConvertFromAny can use the fingerprint
	// to optimize cache lookups

	// Output:
	// Has fingerprint: true
	// Fingerprint length: 64
}
