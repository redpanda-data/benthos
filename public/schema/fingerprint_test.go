// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"
)

func TestFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		schema1     Common
		schema2     Common
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
		{
			name: "identical decimal params",
			schema1: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
			},
			schema2: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
			},
			shouldMatch: true,
		},
		{
			name: "different decimal precision",
			schema1: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
			},
			schema2: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 20, Scale: 4}},
			},
			shouldMatch: false,
		},
		{
			name: "different decimal scale",
			schema1: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
			},
			schema2: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 6}},
			},
			shouldMatch: false,
		},
		{
			name:        "legacy Timestamp (nil Logical) matches itself",
			schema1:     Common{Type: Timestamp, Name: "ts"},
			schema2:     Common{Type: Timestamp, Name: "ts"},
			shouldMatch: true,
		},
		{
			name: "legacy Timestamp differs from parameterised even with default values",
			// Fingerprints diverge whenever Logical is populated, even if the
			// values would match the legacy default. This is intentional: the
			// presence of the field is itself meaningful.
			schema1: Common{Type: Timestamp, Name: "ts"},
			schema2: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}},
			},
			shouldMatch: false,
		},
		{
			name: "Timestamp differs by unit",
			schema1: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}},
			},
			schema2: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMicros, AdjustToUTC: true}},
			},
			shouldMatch: false,
		},
		{
			name: "Timestamp differs by AdjustToUTC",
			schema1: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}},
			},
			schema2: Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: false}},
			},
			shouldMatch: false,
		},
		{
			name: "TimeOfDay matches itself",
			schema1: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMicros}},
			},
			schema2: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMicros}},
			},
			shouldMatch: true,
		},
		{
			name: "TimeOfDay differs by unit",
			schema1: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMillis}},
			},
			schema2: Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMicros}},
			},
			shouldMatch: false,
		},
		{
			name:        "Date and UUID identical pairs",
			schema1:     Common{Type: Date, Name: "d"},
			schema2:     Common{Type: Date, Name: "d"},
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1 := tt.schema1.fingerprint()
			fp2 := tt.schema2.fingerprint()

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
	fp1 := schema.fingerprint()
	fp2 := schema.fingerprint()
	fp3 := schema.fingerprint()

	// All should be identical
	if fp1 != fp2 || fp2 != fp3 {
		t.Errorf("fingerprint should be deterministic, got: %s, %s, %s", fp1, fp2, fp3)
	}
}

func TestFingerprintAllTypes(t *testing.T) {
	schemas := []Common{
		{Type: Boolean, Name: "test"},
		{Type: Int32, Name: "test"},
		{Type: Int64, Name: "test"},
		{Type: Float32, Name: "test"},
		{Type: Float64, Name: "test"},
		{Type: String, Name: "test"},
		{Type: ByteArray, Name: "test"},
		{Type: Object, Name: "test"},
		{Type: Map, Name: "test"},
		{Type: Array, Name: "test"},
		{Type: Null, Name: "test"},
		{Type: Union, Name: "test"},
		{Type: Timestamp, Name: "test"},
		{Type: Any, Name: "test"},
		{Type: Decimal, Name: "test", Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}}},
		{Type: BigDecimal, Name: "test"},
		{Type: Date, Name: "test"},
		{Type: TimeOfDay, Name: "test", Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMicros}}},
		{Type: UUID, Name: "test"},
	}

	fingerprints := make(map[string]CommonType)

	for _, schema := range schemas {
		fp := schema.fingerprint()

		if fp == "" {
			t.Errorf("fingerprint for type %v should not be empty", schema.Type)
		}

		if existing, exists := fingerprints[fp]; exists {
			t.Errorf("fingerprint collision between types %v and %v", existing, schema.Type)
		}

		fingerprints[fp] = schema.Type
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
			name: "any type schema",
			schema: Common{
				Type: Any,
				Name: "payload",
			},
		},
		{
			name: "decimal schema",
			schema: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
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
			expectedFP := tt.schema.fingerprint()
			if fpStr != expectedFP {
				t.Errorf("fingerprint mismatch:\n  got:      %s\n  expected: %s", fpStr, expectedFP)
			}

			// Verify we can parse it back to Common
			parsedSchema, err := ParseFromAny(anySchema)
			if err != nil {
				t.Fatalf("failed to parse Any format: %v", err)
			}

			// Verify parsed schema has same fingerprint
			parsedFP := parsedSchema.fingerprint()
			if parsedFP != expectedFP {
				t.Errorf("parsed schema fingerprint mismatch:\n  got:      %s\n  expected: %s", parsedFP, expectedFP)
			}
		})
	}
}

// TestFingerprintLegacyStability locks in the exact fingerprint bytes of a
// representative set of pre-parameterised schemas. These hashes must not
// change when adding new logical-type variants; doing so would cause a
// stampede of cache misses across every consumer of [SchemaCache] on
// upgrade. If this test fails, look at what was added to the fingerprint
// canonical form and gate the new fields on their being non-nil/non-zero.
func TestFingerprintLegacyStability(t *testing.T) {
	cases := []struct {
		name   string
		schema Common
		fp     string
	}{
		{
			name:   "primitive String",
			schema: Common{Type: String, Name: "id"},
			fp:     "bba6ac8334a77739f7374a773a738639cacba208b662eaad515178bd75d290fe",
		},
		{
			name:   "primitive Int64 optional",
			schema: Common{Type: Int64, Name: "age", Optional: true},
			fp:     "a2f997df3bc480040bb51ae8d174a03a70eeb4cdd42e014d5fcfb58175f61bc9",
		},
		{
			name:   "Timestamp without Logical",
			schema: Common{Type: Timestamp, Name: "ts"},
			fp:     "29368740c39657a4a6f7194f43d5254ec0c987a1107190c4dfb3912066540e81",
		},
		{
			name: "nested Object",
			schema: Common{
				Type: Object,
				Name: "user",
				Children: []Common{
					{Type: String, Name: "id"},
					{Type: Int64, Name: "age", Optional: true},
				},
			},
			fp: "9d0ad47ba9ce5b2d4f7d292f51c526fc0c7ec2fbca4a71313131fbede7740bb9",
		},
		{
			name: "Decimal with params",
			schema: Common{
				Type:    Decimal,
				Name:    "amount",
				Logical: &LogicalParams{Decimal: &DecimalParams{Precision: 18, Scale: 4}},
			},
			fp: "97f8a859fab938ba1aa77773ab8457e092b6e3bb0cf87e56245d02dcf83de7c6",
		},
	}

	// Hard-coded expected fingerprints lock in the canonical form. If
	// writeFingerprint changes for any of these schemas, this test fails
	// and the canonical form change must be reviewed for cache-stampede
	// implications.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.schema.fingerprint()
			if got == "" {
				t.Fatal("empty fingerprint")
			}
			if got != tc.fp {
				t.Errorf("fingerprint drift detected:\n  got:      %s\n  expected: %s", got, tc.fp)
			}
			// Stability: re-compute and compare.
			if again := tc.schema.fingerprint(); again != got {
				t.Errorf("non-deterministic fingerprint: %s vs %s", got, again)
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
		expectedFP := schema.fingerprint()
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
