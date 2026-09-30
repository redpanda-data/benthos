// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestBloblangV2ParseDuration(t *testing.T) {
	got := runBloblangV2(t, `output = input.parse_duration()`, "1h30m")
	assert.Equal(t, (1*time.Hour + 30*time.Minute).Nanoseconds(), got)
}

func TestBloblangV2ParseDurationInvalid(t *testing.T) {
	exec, err := bloblangv2.GlobalEnvironment().Parse(`output = input.parse_duration()`)
	require.NoError(t, err)
	_, qerr := exec.Query("not a duration")
	assert.Error(t, qerr)
}

func TestBloblangV2TsRoundToNearestHour(t *testing.T) {
	ts := time.Date(2020, 8, 14, 5, 54, 23, 0, time.UTC)
	got := runBloblangV2(t, `output = input.ts_round("1h".parse_duration())`, ts)
	want := time.Date(2020, 8, 14, 6, 0, 0, 0, time.UTC)
	assert.Equal(t, want, got)
}

func TestBloblangV2TsTZConvertsTimezone(t *testing.T) {
	ts := time.Date(2021, 2, 3, 16, 5, 6, 0, time.UTC)
	got := runBloblangV2(t, `output = input.ts_tz("America/New_York")`, ts)
	gotTime, ok := got.(time.Time)
	require.True(t, ok, "expected time.Time, got %T", got)
	// The instant in time is preserved.
	assert.True(t, gotTime.Equal(ts))
	// And the wall-clock representation moves to the new zone.
	assert.Equal(t, "America/New_York", gotTime.Location().String())
}

func TestBloblangV2TsTZUnknownZoneErrors(t *testing.T) {
	// V2 folds plugin constructors at parse time when the args are
	// literals, so an unknown zone surfaces as a parse error rather
	// than a runtime error.
	_, err := bloblangv2.GlobalEnvironment().Parse(`output = input.ts_tz("Not/A_Zone")`)
	assert.Error(t, err)
}

func TestBloblangV2TsSubReturnsNanoseconds(t *testing.T) {
	end := time.Date(2020, 8, 14, 11, 30, 0, 0, time.UTC)
	got := runBloblangV2(t,
		`output = input.ts_sub("2020-08-14T10:00:00Z")`,
		end,
	)
	assert.Equal(t, (90 * time.Minute).Nanoseconds(), got)
}

func TestBloblangV2TsSubAcceptsTimestamp(t *testing.T) {
	end := time.Date(2020, 8, 14, 11, 30, 0, 0, time.UTC)
	start := time.Date(2020, 8, 14, 10, 0, 0, 0, time.UTC)
	got := runBloblangV2(t,
		`output = input.end_time.ts_sub(input.start_time)`,
		map[string]any{"end_time": end, "start_time": start},
	)
	assert.Equal(t, (90 * time.Minute).Nanoseconds(), got)
}

func TestBloblangV2TsSubNegativeWhenReceiverEarlier(t *testing.T) {
	earlier := time.Date(2020, 8, 13, 5, 54, 23, 0, time.UTC)
	got := runBloblangV2(t,
		`output = input.ts_sub("2020-08-14T05:54:23Z")`,
		earlier,
	)
	assert.Equal(t, -(24 * time.Hour).Nanoseconds(), got)
}
