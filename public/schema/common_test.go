// Copyright 2025 Redpanda Data, Inc.

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaStringify(t *testing.T) {
	var zeroType CommonType

	for _, test := range []struct {
		Input  CommonType
		Output string
	}{
		{Input: Boolean, Output: "BOOLEAN"},
		{Input: Int64, Output: "INT64"},
		{Input: Int32, Output: "INT32"},
		{Input: Float32, Output: "FLOAT32"},
		{Input: Float64, Output: "FLOAT64"},
		{Input: String, Output: "STRING"},
		{Input: ByteArray, Output: "BYTE_ARRAY"},
		{Input: Object, Output: "OBJECT"},
		{Input: Map, Output: "MAP"},
		{Input: Array, Output: "ARRAY"},
		{Input: Null, Output: "NULL"},
		{Input: Union, Output: "UNION"},
		{Input: Timestamp, Output: "TIMESTAMP"},
		{Input: Any, Output: "ANY"},
		{Input: Decimal, Output: "DECIMAL"},
		{Input: BigDecimal, Output: "BIG_DECIMAL"},
		{Input: Date, Output: "DATE"},
		{Input: TimeOfDay, Output: "TIME_OF_DAY"},
		{Input: UUID, Output: "UUID"},
		{Input: zeroType, Output: "UNKNOWN"},
		{Input: CommonType(-1), Output: "UNKNOWN"},
	} {
		assert.Equal(t, test.Input.String(), test.Output)
	}
}

func TestValidateRejectsChildrenOnLeafTypes(t *testing.T) {
	leafTypes := []CommonType{
		Boolean, Int32, Int64, Float32, Float64, String, ByteArray,
		Null, Timestamp, Any, BigDecimal, Date, UUID,
	}

	for _, typ := range leafTypes {
		t.Run(typ.String(), func(t *testing.T) {
			c := Common{
				Type:     typ,
				Name:     "x",
				Children: []Common{{Type: String, Name: "weird"}},
			}
			err := c.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is a leaf and must not have children")
		})
	}
}

func TestValidateAllowsChildrenOnContainerTypes(t *testing.T) {
	containers := []CommonType{Object, Map, Array, Union}

	for _, typ := range containers {
		t.Run(typ.String(), func(t *testing.T) {
			c := Common{
				Type:     typ,
				Name:     "x",
				Children: []Common{{Type: String, Name: "field"}},
			}
			assert.NoError(t, c.Validate())
		})
	}
}

func TestValidateRejectsChildrenOnDecimal(t *testing.T) {
	c := Common{
		Type:     Decimal,
		Name:     "amount",
		Logical:  &LogicalParams{Decimal: &DecimalParams{Precision: 10, Scale: 2}},
		Children: []Common{{Type: String, Name: "weird"}},
	}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a leaf and must not have children")
}

func TestValidateTimestampParams(t *testing.T) {
	t.Run("nil Logical is permitted (legacy default)", func(t *testing.T) {
		c := Common{Type: Timestamp, Name: "ts"}
		assert.NoError(t, c.Validate())
	})

	t.Run("populated Logical with valid unit", func(t *testing.T) {
		for _, u := range []TimeUnit{TimeUnitSeconds, TimeUnitMillis, TimeUnitMicros, TimeUnitNanos} {
			c := Common{
				Type:    Timestamp,
				Name:    "ts",
				Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: u, AdjustToUTC: true}},
			}
			assert.NoError(t, c.Validate(), "unit=%v", u)
		}
	})

	t.Run("invalid unit rejected", func(t *testing.T) {
		c := Common{
			Type:    Timestamp,
			Name:    "ts",
			Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnit(99), AdjustToUTC: true}},
		}
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timestamp unit")
	})

	t.Run("Timestamp params on non-Timestamp type rejected", func(t *testing.T) {
		c := Common{
			Type:    Int64,
			Name:    "x",
			Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}},
		}
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Logical.Timestamp parameters are only valid for type TIMESTAMP")
	})
}

func TestValidateTimeOfDayParams(t *testing.T) {
	t.Run("missing Logical rejected", func(t *testing.T) {
		c := Common{Type: TimeOfDay, Name: "tod"}
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type TIME_OF_DAY requires Logical.TimeOfDay parameters")
	})

	t.Run("populated Logical with valid unit", func(t *testing.T) {
		for _, u := range []TimeUnit{TimeUnitSeconds, TimeUnitMillis, TimeUnitMicros, TimeUnitNanos} {
			c := Common{
				Type:    TimeOfDay,
				Name:    "tod",
				Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: u}},
			}
			assert.NoError(t, c.Validate(), "unit=%v", u)
		}
	})

	t.Run("invalid unit rejected", func(t *testing.T) {
		c := Common{
			Type:    TimeOfDay,
			Name:    "tod",
			Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnit(0)}},
		}
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid time-of-day unit")
	})

	t.Run("TimeOfDay params on non-TimeOfDay type rejected", func(t *testing.T) {
		c := Common{
			Type:    Int64,
			Name:    "x",
			Logical: &LogicalParams{TimeOfDay: &TimeOfDayParams{Unit: TimeUnitMillis}},
		}
		err := c.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Logical.TimeOfDay parameters are only valid for type TIME_OF_DAY")
	})
}

func TestEffectiveTimestamp(t *testing.T) {
	t.Run("nil Logical defaults to legacy millis/UTC", func(t *testing.T) {
		c := Common{Type: Timestamp, Name: "ts"}
		got := c.EffectiveTimestamp()
		assert.Equal(t, TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}, got)
	})

	t.Run("populated Logical wins", func(t *testing.T) {
		c := Common{
			Type:    Timestamp,
			Name:    "ts",
			Logical: &LogicalParams{Timestamp: &TimestampParams{Unit: TimeUnitMicros, AdjustToUTC: false}},
		}
		got := c.EffectiveTimestamp()
		assert.Equal(t, TimestampParams{Unit: TimeUnitMicros, AdjustToUTC: false}, got)
	})

	t.Run("Logical present but Timestamp nil also defaults", func(t *testing.T) {
		c := Common{Type: Timestamp, Name: "ts", Logical: &LogicalParams{}}
		got := c.EffectiveTimestamp()
		assert.Equal(t, TimestampParams{Unit: TimeUnitMillis, AdjustToUTC: true}, got)
	})
}

func TestTimeUnitString(t *testing.T) {
	cases := []struct {
		u TimeUnit
		s string
	}{
		{TimeUnitSeconds, "SECONDS"},
		{TimeUnitMillis, "MILLIS"},
		{TimeUnitMicros, "MICROS"},
		{TimeUnitNanos, "NANOS"},
		{TimeUnit(0), "UNKNOWN"},
		{TimeUnit(99), "UNKNOWN"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.s, tc.u.String())
	}
}
