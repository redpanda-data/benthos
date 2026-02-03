// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"fmt"
	"sync"
)

// ConvertFunc is a function that converts a Common schema to a target format T.
// This function is called lazily when a schema is first accessed in the cache.
type ConvertFunc[T any] func(Common) (T, error)

// Cache provides a thread-safe cache for storing converted schemas.
// It uses schema fingerprints as keys to ensure that conversions only happen
// once per unique schema structure, avoiding redundant ToAny/FromAny
// serialization and expensive format translations (e.g., to Parquet).
//
// Example usage:
//
//	type ParquetSchema struct { /* ... */ }
//
//	cache := schema.NewCache(func(c schema.Common) (ParquetSchema, error) {
//	    // Convert Common schema to Parquet format
//	    return convertToParquet(c)
//	})
//
//	schema := schema.Common{ /* ... */ }
//	parquetSchema, err := cache.GetOrConvert(schema)
type Cache[T any] struct {
	mu      sync.RWMutex
	cache   map[string]T
	convert ConvertFunc[T]
}

// NewCache creates a new Cache with the provided conversion function.
// The conversion function will be called lazily when a schema is first accessed.
func NewCache[T any](convert ConvertFunc[T]) *Cache[T] {
	return &Cache[T]{
		cache:   make(map[string]T),
		convert: convert,
	}
}

// Get retrieves a converted schema from the cache by fingerprint.
// Returns the cached value and true if found, or the zero value and false if not found.
func (sc *Cache[T]) Get(fingerprint string) (T, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	val, ok := sc.cache[fingerprint]
	return val, ok
}

// GetOrConvert retrieves a converted schema from the cache, or converts and caches it
// if not already present. This method is thread-safe and ensures that the conversion
// function is only called once per unique schema structure.
//
// The fingerprint is computed automatically from the provided schema.
func (sc *Cache[T]) GetOrConvert(schema Common) (T, error) {
	fingerprint := schema.Fingerprint()

	// Fast path: check if already cached (read lock)
	sc.mu.RLock()
	if val, ok := sc.cache[fingerprint]; ok {
		sc.mu.RUnlock()
		return val, nil
	}
	sc.mu.RUnlock()

	// Slow path: convert and cache (write lock)
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Double-check in case another goroutine converted it while we waited for the lock
	if val, ok := sc.cache[fingerprint]; ok {
		return val, nil
	}

	// Convert the schema
	converted, err := sc.convert(schema)
	if err != nil {
		var zero T
		return zero, err
	}

	// Cache the result
	sc.cache[fingerprint] = converted
	return converted, nil
}

// GetOrConvertFromAny retrieves a converted schema from the cache, or converts and caches it
// if not already present. This method optimizes cache lookups when working with schemas in
// Any format (map[string]any).
//
// Since ToAny() automatically includes a "fingerprint" field at the top level, this method:
//  1. First attempts to retrieve the cached value using that fingerprint (fast path)
//  2. Only parses the Any format and recalculates the fingerprint on cache miss
//
// This optimization is particularly useful when schemas are frequently received in Any format,
// as it avoids the expensive ParseFromAny conversion and Fingerprint calculation on cache hits.
//
// Usage example:
//
//	// Producer side: export schema (fingerprint included automatically)
//	schema := schema.Common{Type: schema.String, Name: "id"}
//	anySchema := schema.ToAny()
//	// ... send anySchema over network or store it ...
//
//	// Consumer side: optimized cache lookup
//	cache := schema.NewCache(convertFunc)
//	result, err := cache.GetOrConvertFromAny(anySchema)
//	// Fast path: if cached, avoids ParseFromAny and Fingerprint calculation
//
// If the Any format does not include a fingerprint (e.g., from an older version),
// this method falls back to parsing the schema and calling GetOrConvert normally.
func (sc *Cache[T]) GetOrConvertFromAny(anySchema any) (T, error) {
	var zero T

	// Extract fingerprint if present
	var providedFingerprint string
	if m, ok := anySchema.(map[string]any); ok {
		if fp, ok := m[anyFieldFingerprint].(string); ok {
			providedFingerprint = fp
		}
	}

	// Fast path: if fingerprint provided, try cache lookup first
	if providedFingerprint != "" {
		sc.mu.RLock()
		if val, ok := sc.cache[providedFingerprint]; ok {
			sc.mu.RUnlock()
			return val, nil
		}
		sc.mu.RUnlock()
	}

	// Slow path: parse Any format to Common schema
	common, err := ParseFromAny(anySchema)
	if err != nil {
		return zero, fmt.Errorf("failed to parse Any schema: %w", err)
	}

	// Use standard GetOrConvert for conversion and caching
	// Note: This will recalculate the fingerprint and check cache again,
	// but that's acceptable since we only hit this path on cache miss or when
	// no fingerprint was provided
	return sc.GetOrConvert(common)
}

// Put manually stores a converted schema in the cache with the given fingerprint.
// This is useful when you want to pre-populate the cache or store a conversion
// result obtained through other means.
func (sc *Cache[T]) Put(fingerprint string, value T) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache[fingerprint] = value
}

// Size returns the number of cached schemas.
func (sc *Cache[T]) Size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.cache)
}

// Clear removes all entries from the cache.
func (sc *Cache[T]) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache = make(map[string]T)
}
