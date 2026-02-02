// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"
)

func TestFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		schema1  Common
		schema2  Common
		shouldMatch bool
	}{
		{
			name: "identical simple schemas",
			schema1: Common{
				Type: String,
				Name: "field1",
			},
			schema2: Common{
				Type: String,
				Name: "field1",
			},
			shouldMatch: true,
		},
		{
			name: "different types",
			schema1: Common{
				Type: String,
				Name: "field1",
			},
			schema2: Common{
				Type: Int64,
				Name: "field1",
			},
			shouldMatch: false,
		},
		{
			name: "different names",
			schema1: Common{
				Type: String,
				Name: "field1",
			},
			schema2: Common{
				Type: String,
				Name: "field2",
			},
			shouldMatch: false,
		},
		{
			name: "different optional flags",
			schema1: Common{
				Type:     String,
				Name:     "field1",
				Optional: true,
			},
			schema2: Common{
				Type:     String,
				Name:     "field1",
				Optional: false,
			},
			shouldMatch: false,
		},
		{
			name: "identical nested schemas",
			schema1: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{Type: String, Name: "field1"},
					{Type: Int64, Name: "field2", Optional: true},
				},
			},
			schema2: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{Type: String, Name: "field1"},
					{Type: Int64, Name: "field2", Optional: true},
				},
			},
			shouldMatch: true,
		},
		{
			name: "different child order",
			schema1: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{Type: String, Name: "field1"},
					{Type: Int64, Name: "field2"},
				},
			},
			schema2: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{Type: Int64, Name: "field2"},
					{Type: String, Name: "field1"},
				},
			},
			shouldMatch: false,
		},
		{
			name: "deeply nested schemas",
			schema1: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{
						Type: Array,
						Name: "items",
						Children: []Common{
							{
								Type: Object,
								Children: []Common{
									{Type: String, Name: "id"},
									{Type: String, Name: "name"},
								},
							},
						},
					},
				},
			},
			schema2: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{
						Type: Array,
						Name: "items",
						Children: []Common{
							{
								Type: Object,
								Children: []Common{
									{Type: String, Name: "id"},
									{Type: String, Name: "name"},
								},
							},
						},
					},
				},
			},
			shouldMatch: true,
		},
		{
			name: "empty vs non-empty children",
			schema1: Common{
				Type: Array,
				Name: "items",
			},
			schema2: Common{
				Type: Array,
				Name: "items",
				Children: []Common{
					{Type: String},
				},
			},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1 := tt.schema1.Fingerprint()
			fp2 := tt.schema2.Fingerprint()

			// Fingerprints should be non-empty
			if fp1 == "" {
				t.Error("fingerprint should not be empty")
			}
			if fp2 == "" {
				t.Error("fingerprint should not be empty")
			}

			// Check expected match/mismatch
			if tt.shouldMatch && fp1 != fp2 {
				t.Errorf("expected fingerprints to match but got:\n  schema1: %s\n  schema2: %s", fp1, fp2)
			}
			if !tt.shouldMatch && fp1 == fp2 {
				t.Errorf("expected fingerprints to differ but both were: %s", fp1)
			}
		})
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	schema := Common{
		Type: Object,
		Name: "root",
		Children: []Common{
			{Type: String, Name: "field1"},
			{Type: Int64, Name: "field2", Optional: true},
			{
				Type: Array,
				Name: "nested",
				Children: []Common{
					{Type: Boolean},
				},
			},
		},
	}

	// Generate fingerprint multiple times
	fp1 := schema.Fingerprint()
	fp2 := schema.Fingerprint()
	fp3 := schema.Fingerprint()

	// All should be identical
	if fp1 != fp2 || fp2 != fp3 {
		t.Errorf("fingerprint should be deterministic, got: %s, %s, %s", fp1, fp2, fp3)
	}
}

func TestFingerprintAllTypes(t *testing.T) {
	types := []CommonType{
		Boolean, Int32, Int64, Float32, Float64,
		String, ByteArray, Object, Map, Array,
		Null, Union, Timestamp,
	}

	fingerprints := make(map[string]CommonType)

	for _, typ := range types {
		schema := Common{Type: typ, Name: "test"}
		fp := schema.Fingerprint()

		if fp == "" {
			t.Errorf("fingerprint for type %v should not be empty", typ)
		}

		if existing, exists := fingerprints[fp]; exists {
			t.Errorf("fingerprint collision between types %v and %v", existing, typ)
		}

		fingerprints[fp] = typ
	}
}

func TestToAnyIncludesFingerprint(t *testing.T) {
	tests := []struct {
		name   string
		schema Common
	}{
		{
			name: "simple schema",
			schema: Common{
				Type: String,
				Name: "test",
			},
		},
		{
			name: "schema with optional",
			schema: Common{
				Type:     Int64,
				Name:     "age",
				Optional: true,
			},
		},
		{
			name: "nested schema",
			schema: Common{
				Type: Object,
				Name: "user",
				Children: []Common{
					{Type: String, Name: "id"},
					{Type: String, Name: "email"},
				},
			},
		},
		{
			name: "deeply nested schema",
			schema: Common{
				Type: Object,
				Name: "root",
				Children: []Common{
					{
						Type: Array,
						Name: "items",
						Children: []Common{
							{
								Type: Object,
								Children: []Common{
									{Type: String, Name: "id"},
									{Type: Boolean, Name: "active"},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get Any format (fingerprint included automatically)
			anySchema := tt.schema.ToAny()

			// Verify it's a map
			m, ok := anySchema.(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any, got %T", anySchema)
			}

			// Verify fingerprint field exists
			fpVal, exists := m[anyFieldFingerprint]
			if !exists {
				t.Fatal("expected fingerprint field to exist")
			}

			// Verify fingerprint is a string
			fpStr, ok := fpVal.(string)
			if !ok {
				t.Fatalf("expected fingerprint to be string, got %T", fpVal)
			}

			// Verify fingerprint is not empty
			if fpStr == "" {
				t.Error("fingerprint should not be empty")
			}

			// Verify fingerprint matches the schema's actual fingerprint
			expectedFP := tt.schema.Fingerprint()
			if fpStr != expectedFP {
				t.Errorf("fingerprint mismatch:\n  got:      %s\n  expected: %s", fpStr, expectedFP)
			}

			// Verify we can parse it back to Common
			parsedSchema, err := ParseFromAny(anySchema)
			if err != nil {
				t.Fatalf("failed to parse Any format: %v", err)
			}

			// Verify parsed schema has same fingerprint
			parsedFP := parsedSchema.Fingerprint()
			if parsedFP != expectedFP {
				t.Errorf("parsed schema fingerprint mismatch:\n  got:      %s\n  expected: %s", parsedFP, expectedFP)
			}
		})
	}
}

func TestToAnyAlwaysIncludesFingerprint(t *testing.T) {
	schema := Common{
		Type: Object,
		Name: "user",
		Children: []Common{
			{Type: String, Name: "id"},
			{Type: Int64, Name: "age", Optional: true},
		},
	}

	// Get Any format
	anySchema := schema.ToAny()

	// Should be a map
	m, ok := anySchema.(map[string]any)
	if !ok {
		t.Fatal("expected map[string]any")
	}

	// ToAny should always have fingerprint field
	fpVal, hasFP := m[anyFieldFingerprint]
	if !hasFP {
		t.Error("ToAny should include fingerprint field")
	}

	// Verify fingerprint is correct
	if fpStr, ok := fpVal.(string); ok {
		expectedFP := schema.Fingerprint()
		if fpStr != expectedFP {
			t.Errorf("fingerprint mismatch: got %s, expected %s", fpStr, expectedFP)
		}
	} else {
		t.Errorf("fingerprint should be string, got %T", fpVal)
	}

	// All standard fields should be present
	for _, field := range []string{anyFieldType, anyFieldName, anyFieldChildren, anyFieldFingerprint} {
		if _, ok := m[field]; !ok && (field != anyFieldOptional) {
			t.Errorf("ToAny missing field: %s", field)
		}
	}
}
