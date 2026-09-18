// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"time"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 pure timestamp/duration helpers. The V2 stdlib already
// covers ts_format, ts_parse, ts_unix*, ts_add, ts_from_unix*; this file
// fills in the V1 plugin-registered remainders that don't read the wall
// clock or randomness. Wall-clock helpers stay in internal/impl/io.

func init() {
	bloblangv2.MustRegisterMethod("parse_duration",
		bloblangv2.NewPluginSpec().
			Category("Time").
			Description("Parses a Go duration string (e.g. \"1h30m\") and returns the duration as int64 nanoseconds."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				d, err := time.ParseDuration(s)
				if err != nil {
					return nil, err
				}
				return d.Nanoseconds(), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("ts_round",
		bloblangv2.NewPluginSpec().
			Category("Time").
			Description("Rounds a timestamp to the nearest multiple of the supplied duration in nanoseconds. Halfway values round up.").
			Param(bloblangv2.NewInt64Param("duration").Description("Duration in nanoseconds.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			d, err := args.GetInt64("duration")
			if err != nil {
				return nil, err
			}
			dur := time.Duration(d)
			return bloblangv2.TimestampMethod(func(t time.Time) (any, error) {
				return t.Round(dur), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("ts_tz",
		bloblangv2.NewPluginSpec().
			Category("Time").
			Description("Returns the receiver timestamp expressed in a different timezone (the moment in time is preserved). Use \"UTC\", \"Local\", or an IANA Time Zone name like \"America/New_York\".").
			Param(bloblangv2.NewStringParam("tz").Description("Target timezone name.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			tzName, err := args.GetString("tz")
			if err != nil {
				return nil, err
			}
			loc, err := time.LoadLocation(tzName)
			if err != nil {
				return nil, fmt.Errorf("failed to parse timezone %q: %w", tzName, err)
			}
			return bloblangv2.TimestampMethod(func(t time.Time) (any, error) {
				return t.In(loc), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("ts_sub",
		bloblangv2.NewPluginSpec().
			Category("Time").
			Description("Returns the duration in nanoseconds between the receiver timestamp and the t2 argument (receiver - t2). Positive when the receiver is after t2; negative otherwise. Use .abs() for absolute duration.").
			Param(bloblangv2.NewAnyParam("t2").Description("Timestamp to subtract from the receiver. Accepts a time.Time, an RFC 3339 string, or a unix timestamp in seconds (int64 or float64).")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			rawT2, err := args.Get("t2")
			if err != nil {
				return nil, err
			}
			t2, err := coerceTimestamp(rawT2)
			if err != nil {
				return nil, fmt.Errorf("t2: %w", err)
			}
			return bloblangv2.TimestampMethod(func(t time.Time) (any, error) {
				return t.Sub(t2).Nanoseconds(), nil
			}), nil
		},
	)
}

// coerceTimestamp accepts the common timestamp surface forms and returns
// a time.Time. RFC 3339 strings and unix-seconds numerics are honoured;
// already-parsed time.Time passes through.
func coerceTimestamp(v any) (time.Time, error) {
	switch n := v.(type) {
	case time.Time:
		return n, nil
	case string:
		t, err := time.Parse(time.RFC3339Nano, n)
		if err != nil {
			return time.Time{}, fmt.Errorf("expected RFC 3339 timestamp, got %q", n)
		}
		return t, nil
	case int64:
		return time.Unix(n, 0).UTC(), nil
	case int:
		return time.Unix(int64(n), 0).UTC(), nil
	case float64:
		whole, frac := int64(n), n-float64(int64(n))
		return time.Unix(whole, int64(frac*1e9)).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected timestamp value, got %T", v)
}
