// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"fmt"
	"sync"
	"testing"
)

// mockConvertedSchema represents a target schema format (e.g., Parquet, Avro)
type mockConvertedSchema struct {
	originalType CommonType
	convertCount int // tracks how many times conversion was called
}

func TestSchemaCacheBasicOperations(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)

	schema := Common{Type: String, Name: "test"}

	// First access should trigger conversion
	result1, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result1.originalType != String {
		t.Errorf("expected type %v, got %v", String, result1.originalType)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}

	// Second access should use cache
	result2, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.originalType != String {
		t.Errorf("expected type %v, got %v", String, result2.originalType)
	}
	if convertCount != 1 {
		t.Errorf("expected conversion count to remain 1, got %d", convertCount)
	}

	// Results should be identical (came from cache)
	if result1.convertCount != result2.convertCount {
		t.Error("expected cached result to be identical to first result")
	}
}

func TestSchemaCacheGet(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Int64, Name: "test"}
	fingerprint := schema.Fingerprint()

	// Get should return false before conversion
	_, ok := cache.Get(fingerprint)
	if ok {
		t.Error("expected Get to return false for non-existent entry")
	}

	// Convert and cache
	_, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get should now return true
	result, ok := cache.Get(fingerprint)
	if !ok {
		t.Error("expected Get to return true after conversion")
	}
	if result.originalType != Int64 {
		t.Errorf("expected type %v, got %v", Int64, result.originalType)
	}
}

func TestSchemaCachePut(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Boolean, Name: "test"}
	fingerprint := schema.Fingerprint()

	// Manually put a value
	manual := mockConvertedSchema{originalType: Float64, convertCount: 999}
	cache.Put(fingerprint, manual)

	// GetOrConvert should return the manually set value
	result, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.convertCount != 999 {
		t.Errorf("expected manually set value, got convertCount=%d", result.convertCount)
	}
}

func TestSchemaCacheSize(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)

	if cache.Size() != 0 {
		t.Errorf("expected size 0, got %d", cache.Size())
	}

	schemas := []Common{
		{Type: String, Name: "field1"},
		{Type: Int64, Name: "field2"},
		{Type: Boolean, Name: "field3"},
	}

	for i, schema := range schemas {
		_, err := cache.GetOrConvert(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedSize := i + 1
		if cache.Size() != expectedSize {
			t.Errorf("expected size %d, got %d", expectedSize, cache.Size())
		}
	}
}

func TestSchemaCacheClear(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: String, Name: "test"}

	// Add entry
	_, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	// Clear
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}

	// Get should fail after clear
	fingerprint := schema.Fingerprint()
	_, ok := cache.Get(fingerprint)
	if ok {
		t.Error("expected Get to return false after clear")
	}
}

func TestSchemaCacheConversionError(t *testing.T) {
	expectedErr := fmt.Errorf("conversion failed")
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{}, expectedErr
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: String, Name: "test"}

	_, err := cache.GetOrConvert(schema)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Failed conversions should not be cached
	if cache.Size() != 0 {
		t.Errorf("expected size 0 after failed conversion, got %d", cache.Size())
	}
}

func TestSchemaCacheConcurrency(t *testing.T) {
	convertCount := 0
	var mu sync.Mutex

	converter := func(c Common) (mockConvertedSchema, error) {
		mu.Lock()
		convertCount++
		count := convertCount
		mu.Unlock()
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: count,
		}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Object, Name: "concurrent"}

	// Run multiple goroutines trying to convert the same schema
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([]mockConvertedSchema, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = cache.GetOrConvert(schema)
		}(i)
	}

	wg.Wait()

	// Check all succeeded
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d got error: %v", i, err)
		}
	}

	// Check that conversion only happened once
	mu.Lock()
	finalCount := convertCount
	mu.Unlock()

	if finalCount != 1 {
		t.Errorf("expected exactly 1 conversion, got %d", finalCount)
	}

	// Check that all results are identical (came from same conversion)
	firstResult := results[0]
	for i, result := range results[1:] {
		if result.convertCount != firstResult.convertCount {
			t.Errorf("goroutine %d got different result: %v vs %v", i+1, result, firstResult)
		}
	}
}

func TestSchemaCacheMultipleSchemas(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)

	schemas := []Common{
		{Type: String, Name: "field1"},
		{Type: Int64, Name: "field2"},
		{Type: Boolean, Name: "field3"},
		{Type: Float64, Name: "field4"},
	}

	// Convert all schemas
	for _, schema := range schemas {
		_, err := cache.GetOrConvert(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Verify all are cached
	if cache.Size() != len(schemas) {
		t.Errorf("expected size %d, got %d", len(schemas), cache.Size())
	}

	// Verify each can be retrieved
	for _, schema := range schemas {
		fingerprint := schema.Fingerprint()
		result, ok := cache.Get(fingerprint)
		if !ok {
			t.Errorf("expected to find cached entry for schema %v", schema)
		}
		if result.originalType != schema.Type {
			t.Errorf("expected type %v, got %v", schema.Type, result.originalType)
		}
	}
}

func TestSchemaCacheGetOrConvertFromAny_WithFingerprint(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: String, Name: "test"}

	// First, populate cache using normal GetOrConvert
	_, err := cache.GetOrConvert(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}

	// Create Any format (fingerprint included automatically)
	anySchema := schema.ToAny()

	// GetOrConvertFromAny should hit cache and not convert again
	result, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected conversion count to remain 1, got %d", convertCount)
	}
	if result.originalType != String {
		t.Errorf("expected type %v, got %v", String, result.originalType)
	}
}

func TestSchemaCacheGetOrConvertFromAny_WithoutFingerprint(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Int64, Name: "test"}

	// Create Any format without fingerprint
	anyWithoutFP := schema.ToAny()

	// First access should convert and cache
	result1, err := cache.GetOrConvertFromAny(anyWithoutFP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}
	if result1.originalType != Int64 {
		t.Errorf("expected type %v, got %v", Int64, result1.originalType)
	}

	// Second access should use cache even without fingerprint
	result2, err := cache.GetOrConvertFromAny(anyWithoutFP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected conversion count to remain 1, got %d", convertCount)
	}
	if result2.originalType != Int64 {
		t.Errorf("expected type %v, got %v", Int64, result2.originalType)
	}
}

func TestSchemaCacheGetOrConvertFromAny_InvalidFingerprint(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Boolean, Name: "test"}

	// Create Any format and modify to have invalid fingerprint
	anySchema := schema.ToAny().(map[string]any)
	anySchema[anyFieldFingerprint] = "invalid_fingerprint_12345"

	// Should fall back to parsing and converting
	result, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}
	if result.originalType != Boolean {
		t.Errorf("expected type %v, got %v", Boolean, result.originalType)
	}

	// Verify it was cached with the correct fingerprint
	correctFingerprint := schema.Fingerprint()
	result2, ok := cache.Get(correctFingerprint)
	if !ok {
		t.Error("expected schema to be cached with correct fingerprint")
	}
	if result2.originalType != Boolean {
		t.Errorf("expected type %v, got %v", Boolean, result2.originalType)
	}
}

func TestSchemaCacheGetOrConvertFromAny_ParseError(t *testing.T) {
	converter := func(c Common) (mockConvertedSchema, error) {
		return mockConvertedSchema{originalType: c.Type}, nil
	}

	cache := NewSchemaCache(converter)

	// Invalid Any format (not a map)
	invalidAny := "not a map"

	_, err := cache.GetOrConvertFromAny(invalidAny)
	if err == nil {
		t.Fatal("expected error for invalid Any format, got nil")
	}
}

func TestSchemaCacheGetOrConvertFromAny_EmptyCache(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)
	schema := Common{Type: Float64, Name: "test"}

	// Create Any format (fingerprint included automatically)
	anySchema := schema.ToAny()

	// Cache is empty, so should convert even with fingerprint
	result, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}
	if result.originalType != Float64 {
		t.Errorf("expected type %v, got %v", Float64, result.originalType)
	}

	// Second call should hit cache
	result2, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected conversion count to remain 1, got %d", convertCount)
	}
	if result2.convertCount != result.convertCount {
		t.Error("expected second result to be from cache")
	}
}

func TestSchemaCacheGetOrConvertFromAny_ComplexSchema(t *testing.T) {
	convertCount := 0
	converter := func(c Common) (mockConvertedSchema, error) {
		convertCount++
		return mockConvertedSchema{
			originalType: c.Type,
			convertCount: convertCount,
		}, nil
	}

	cache := NewSchemaCache(converter)

	// Complex schema with nested children
	schema := Common{
		Type: Object,
		Name: "user",
		Children: []Common{
			{Type: String, Name: "id"},
			{Type: String, Name: "email", Optional: true},
			{Type: Int64, Name: "age"},
		},
	}

	// Convert to Any format (fingerprint included automatically)
	anySchema := schema.ToAny()

	// First access
	result1, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected 1 conversion, got %d", convertCount)
	}

	// Second access should hit cache
	result2, err := cache.GetOrConvertFromAny(anySchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if convertCount != 1 {
		t.Errorf("expected conversion count to remain 1, got %d", convertCount)
	}

	// Results should be identical
	if result1.convertCount != result2.convertCount {
		t.Error("expected results to be identical (from cache)")
	}
}
