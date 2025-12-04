// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"time"

	"github.com/itchyny/timefmt-go"
	"github.com/rickb777/period"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func asDeprecated(s *bloblang.PluginSpec) *bloblang.PluginSpec {
	tmpSpec := *s
	newSpec := &tmpSpec
	newSpec = newSpec.Deprecated()
	return newSpec
}

func init() {
	// Note: The examples are run and tested from within
	// ./internal/bloblang/query/parsed_test.go

	tsRoundSpec := bloblang.NewPluginSpec().
		Beta().
		Static().
		Category(query.MethodCategoryTime).
		Description(`Rounds a timestamp to the nearest multiple of the specified duration. Halfway values round up. Accepts unix timestamps (seconds with optional decimal precision) or RFC 3339 formatted strings.`).
		Param(bloblang.NewInt64Param("duration").Description("A duration measured in nanoseconds to round by.")).
		Version("4.2.0").
		Example("Round timestamp to the nearest hour.",
			`root.created_at_hour = this.created_at.ts_round("1h".parse_duration())`,
			[2]string{
				`{"created_at":"2020-08-14T05:54:23Z"}`,
				`{"created_at_hour":"2020-08-14T06:00:00Z"}`,
			}).
		Example("Round timestamp to the nearest minute.",
			`root.created_at_minute = this.created_at.ts_round("1m".parse_duration())`,
			[2]string{
				`{"created_at":"2020-08-14T05:54:23Z"}`,
				`{"created_at_minute":"2020-08-14T05:54:00Z"}`,
			})

	tsRoundCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		iDur, err := args.GetInt64("duration")
		if err != nil {
			return nil, err
		}
		dur := time.Duration(iDur)
		return bloblang.TimestampMethod(func(t time.Time) (any, error) {
			return t.Round(dur), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_round", tsRoundSpec, tsRoundCtor)

	tsTZSpec := bloblang.NewPluginSpec().
		Beta().
		Static().
		Category(query.MethodCategoryTime).
		Description(`Converts a timestamp to a different timezone while preserving the moment in time. Accepts unix timestamps (seconds with optional decimal precision) or RFC 3339 formatted strings.`).
		Param(bloblang.NewStringParam("tz").Description(`The timezone to change to. Use "UTC" for UTC, "Local" for local timezone, or an IANA Time Zone database location name like "America/New_York".`)).
		Version("4.3.0").
		Example("Convert timestamp to UTC timezone.",
			`root.created_at_utc = this.created_at.ts_tz("UTC")`,
			[2]string{
				`{"created_at":"2021-02-03T17:05:06+01:00"}`,
				`{"created_at_utc":"2021-02-03T16:05:06Z"}`,
			}).
		Example("Convert timestamp to a specific timezone.",
			`root.created_at_ny = this.created_at.ts_tz("America/New_York")`,
			[2]string{
				`{"created_at":"2021-02-03T16:05:06Z"}`,
				`{"created_at_ny":"2021-02-03T11:05:06-05:00"}`,
			})

	tsTZCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		timezoneStr, err := args.GetString("tz")
		if err != nil {
			return nil, err
		}
		timezone, err := time.LoadLocation(timezoneStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timezone location name: %w", err)
		}
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			return target.In(timezone), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_tz", tsTZSpec, tsTZCtor)

	tsAddISOSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Adds an ISO 8601 duration to a timestamp with calendar-aware precision for years, months, and days. Useful when you need to add durations that account for variable month lengths or leap years.").
		Param(bloblang.NewStringParam("duration").Description(`Duration in ISO 8601 format (e.g., "P1Y2M3D" for 1 year, 2 months, 3 days)`)).
		Example("Add one year to a timestamp.",
			`root.next_year = this.created_at.ts_add_iso8601("P1Y")`,
			[2]string{
				`{"created_at":"2020-08-14T05:54:23Z"}`,
				`{"next_year":"2021-08-14T05:54:23Z"}`,
			}).
		Example("Add a complex duration with multiple units.",
			`root.future_date = this.created_at.ts_add_iso8601("P1Y2M3DT4H5M6S")`,
			[2]string{
				`{"created_at":"2020-01-01T00:00:00Z"}`,
				`{"future_date":"2021-03-04T04:05:06Z"}`,
			})

	tsSubISOSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Subtracts an ISO 8601 duration from a timestamp with calendar-aware precision for years, months, and days. Useful when you need to subtract durations that account for variable month lengths or leap years.").
		Param(bloblang.NewStringParam("duration").Description(`Duration in ISO 8601 format (e.g., "P1Y2M3D" for 1 year, 2 months, 3 days)`)).
		Example("Subtract one year from a timestamp.",
			`root.last_year = this.created_at.ts_sub_iso8601("P1Y")`,
			[2]string{
				`{"created_at":"2020-08-14T05:54:23Z"}`,
				`{"last_year":"2019-08-14T05:54:23Z"}`,
			}).
		Example("Subtract a complex duration with multiple units.",
			`root.past_date = this.created_at.ts_sub_iso8601("P1Y2M3DT4H5M6S")`,
			[2]string{
				`{"created_at":"2021-03-04T04:05:06Z"}`,
				`{"past_date":"2020-01-01T00:00:00Z"}`,
			})

	tsModifyISOCtor := func(callback func(d period.Period, t time.Time) time.Time) func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			s, err := args.GetString("duration")
			if err != nil {
				return nil, err
			}
			dur, err := period.Parse(s)
			if err != nil {
				return nil, err
			}
			return bloblang.TimestampMethod(func(t time.Time) (any, error) {
				return callback(dur, t), nil
			}), nil
		}
	}

	bloblang.MustRegisterMethodV2("ts_add_iso8601", tsAddISOSpec,
		tsModifyISOCtor(func(d period.Period, t time.Time) time.Time {
			r, _ := d.AddTo(t)
			return r
		}))

	bloblang.MustRegisterMethodV2("ts_sub_iso8601", tsSubISOSpec,
		tsModifyISOCtor(func(d period.Period, t time.Time) time.Time {
			r, _ := d.Negate().AddTo(t)
			return r
		}))

	//--------------------------------------------------------------------------

	parseDurSpec := bloblang.NewPluginSpec().
		Static().
		Category(query.MethodCategoryTime).
		Description(`Parses a Go-style duration string into nanoseconds. A duration string is a signed sequence of decimal numbers with unit suffixes like "300ms", "-1.5h", or "2h45m". Valid units: "ns", "us" (or "µs"), "ms", "s", "m", "h".`).
		Example("Parse microseconds to nanoseconds.",
			`root.delay_for_ns = this.delay_for.parse_duration()`,
			[2]string{
				`{"delay_for":"50us"}`,
				`{"delay_for_ns":50000}`,
			},
		).
		Example("Parse hours to seconds.",
			`root.delay_for_s = this.delay_for.parse_duration() / 1000000000`,
			[2]string{
				`{"delay_for":"2h"}`,
				`{"delay_for_s":7200}`,
			},
		)

	parseDurCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.StringMethod(func(s string) (any, error) {
			d, err := time.ParseDuration(s)
			if err != nil {
				return nil, err
			}
			return d.Nanoseconds(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("parse_duration", parseDurSpec, parseDurCtor)

	parseDurISOSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description(`Parses an ISO 8601 duration string into nanoseconds. Format: "P[n]Y[n]M[n]DT[n]H[n]M[n]S" or "P[n]W". Example: "P3Y6M4DT12H30M5S" means 3 years, 6 months, 4 days, 12 hours, 30 minutes, 5 seconds. Supports fractional seconds with full precision (not just one decimal place).`).
		Example("Parse complex ISO 8601 duration to nanoseconds.",
			`root.delay_for_ns = this.delay_for.parse_duration_iso8601()`,
			[2]string{
				`{"delay_for":"P3Y6M4DT12H30M5S"}`,
				`{"delay_for_ns":110839937000000000}`,
			},
		).
		Example("Parse hours to seconds.",
			`root.delay_for_s = this.delay_for.parse_duration_iso8601() / 1000000000`,
			[2]string{
				`{"delay_for":"PT2H"}`,
				`{"delay_for_s":7200}`,
			},
		)

	parseDurISOCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.StringMethod(func(s string) (any, error) {
			d, err := period.Parse(s)
			if err != nil {
				return nil, err
			}
			// The conversion is likely imprecise when the period specifies years, months and days.
			// See method documentation for details on precision.
			return d.DurationApprox().Nanoseconds(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("parse_duration_iso8601", parseDurISOSpec, parseDurISOCtor)

	//--------------------------------------------------------------------------

	parseTSSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description(`Parses a timestamp string using Go's reference time format and outputs a timestamp object. The format uses "Mon Jan 2 15:04:05 -0700 MST 2006" as a reference - show how this reference time would appear in your format. Use ts_strptime for strftime-style formats instead.`).
		Param(bloblang.NewStringParam("format").Description("The format of the input string using Go's reference time."))

	parseTSSpecDep := asDeprecated(parseTSSpec)

	parseTSSpec = parseTSSpec.
		Example("Parse a date with abbreviated month name.",
			`root.doc.timestamp = this.doc.timestamp.ts_parse("2006-Jan-02")`,
			[2]string{
				`{"doc":{"timestamp":"2020-Aug-14"}}`,
				`{"doc":{"timestamp":"2020-08-14T00:00:00Z"}}`,
			},
		).
		Example("Parse a custom datetime format.",
			`root.parsed = this.timestamp.ts_parse("Jan 2, 2006 at 3:04pm (MST)")`,
			[2]string{
				`{"timestamp":"Aug 14, 2020 at 5:54am (UTC)"}`,
				`{"parsed":"2020-08-14T05:54:00Z"}`,
			},
		)

	parseTSCtor := func(deprecated bool) bloblang.MethodConstructorV2 {
		return func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			layout, err := args.GetString("format")
			if err != nil {
				return nil, err
			}
			return bloblang.StringMethod(func(s string) (any, error) {
				ut, err := time.Parse(layout, s)
				if err != nil {
					return nil, err
				}
				if deprecated {
					return ut.Format(time.RFC3339Nano), nil
				}
				return ut, nil
			}), nil
		}
	}

	bloblang.MustRegisterMethodV2("ts_parse", parseTSSpec, parseTSCtor(false))

	bloblang.MustRegisterMethodV2("parse_timestamp", parseTSSpecDep, parseTSCtor(true))

	parseTSStrptimeSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Parses a timestamp string using strptime format specifiers (like %Y, %m, %d) and outputs a timestamp object. Use ts_parse for Go-style reference time formats instead.").
		Param(bloblang.NewStringParam("format").Description("The format string using strptime specifiers (e.g., %Y-%m-%d)."))

	parseTSStrptimeSpecDep := asDeprecated(parseTSStrptimeSpec)

	parseTSStrptimeSpec = parseTSStrptimeSpec.
		Example(
			"Parse date with abbreviated month using strptime format.",
			`root.doc.timestamp = this.doc.timestamp.ts_strptime("%Y-%b-%d")`,
			[2]string{
				`{"doc":{"timestamp":"2020-Aug-14"}}`,
				`{"doc":{"timestamp":"2020-08-14T00:00:00Z"}}`,
			},
		).
		Example(
			"Parse datetime with microseconds using %f directive.",
			`root.doc.timestamp = this.doc.timestamp.ts_strptime("%Y-%b-%d %H:%M:%S.%f")`,
			[2]string{
				`{"doc":{"timestamp":"2020-Aug-14 11:50:26.371000"}}`,
				`{"doc":{"timestamp":"2020-08-14T11:50:26.371Z"}}`,
			},
		)

	parseTSStrptimeCtor := func(deprecated bool) bloblang.MethodConstructorV2 {
		return func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			layout, err := args.GetString("format")
			if err != nil {
				return nil, err
			}
			return bloblang.StringMethod(func(s string) (any, error) {
				ut, err := timefmt.Parse(s, layout)
				if err != nil {
					return nil, err
				}
				if deprecated {
					return ut.Format(time.RFC3339Nano), nil
				}
				return ut, nil
			}), nil
		}
	}

	bloblang.MustRegisterMethodV2("ts_strptime", parseTSStrptimeSpec, parseTSStrptimeCtor(false))

	bloblang.MustRegisterMethodV2("parse_timestamp_strptime", parseTSStrptimeSpecDep, parseTSStrptimeCtor(true))

	//--------------------------------------------------------------------------

	formatTSSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description(`Formats a timestamp as a string using Go's reference time format. Defaults to RFC 3339 if no format specified. The format uses "Mon Jan 2 15:04:05 -0700 MST 2006" as a reference. Accepts unix timestamps (with decimal precision) or RFC 3339 strings. Use ts_strftime for strftime-style formats.`).
		Param(bloblang.NewStringParam("format").Description("The output format using Go's reference time.").Default(time.RFC3339Nano)).
		Param(bloblang.NewStringParam("tz").Description("Optional timezone (e.g., 'UTC', 'America/New_York'). Defaults to input timezone or local time for unix timestamps.").Optional())

	formatTSSpecDep := asDeprecated(formatTSSpec)

	formatTSSpec = formatTSSpec.
		Example("Format timestamp with custom format.",
			`root.something_at = this.created_at.ts_format("2006-Jan-02 15:04:05")`,
			[2]string{
				`{"created_at":"2020-08-14T11:50:26.371Z"}`,
				`{"something_at":"2020-Aug-14 11:50:26"}`,
			},
		).
		Example("Format unix timestamp with timezone specification.",
			`root.something_at = this.created_at.ts_format(format: "2006-Jan-02 15:04:05", tz: "UTC")`,
			[2]string{
				`{"created_at":1597405526}`,
				`{"something_at":"2020-Aug-14 11:45:26"}`,
			},
		)

	formatTSCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		layout, err := args.GetString("format")
		if err != nil {
			return nil, err
		}
		var timezone *time.Location
		tzOpt, err := args.GetOptionalString("tz")
		if err != nil {
			return nil, err
		}
		if tzOpt != nil {
			if timezone, err = time.LoadLocation(*tzOpt); err != nil {
				return nil, fmt.Errorf("failed to parse timezone location name: %w", err)
			}
		}
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			if timezone != nil {
				target = target.In(timezone)
			}
			return target.Format(layout), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_format", formatTSSpec, formatTSCtor)

	bloblang.MustRegisterMethodV2("format_timestamp", formatTSSpecDep, formatTSCtor)

	formatTSStrftimeSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Formats a timestamp as a string using strptime format specifiers (like %Y, %m, %d). Accepts unix timestamps (with decimal precision) or RFC 3339 strings. Supports %f for microseconds. Use ts_format for Go-style reference time formats.").
		Param(bloblang.NewStringParam("format").Description("The output format using strptime specifiers.")).
		Param(bloblang.NewStringParam("tz").Description("Optional timezone. Defaults to input timezone or local time for unix timestamps.").Optional())

	formatTSStrftimeSpecDep := asDeprecated(formatTSStrftimeSpec)

	formatTSStrftimeSpec = formatTSStrftimeSpec.
		Example(
			"Format timestamp with strftime specifiers.",
			`root.something_at = this.created_at.ts_strftime("%Y-%b-%d %H:%M:%S")`,
			[2]string{
				`{"created_at":"2020-08-14T11:50:26.371Z"}`,
				`{"something_at":"2020-Aug-14 11:50:26"}`,
			},
		).
		Example(
			"Format with microseconds using %f directive.",
			`root.something_at = this.created_at.ts_strftime("%Y-%b-%d %H:%M:%S.%f", "UTC")`,
			[2]string{
				`{"created_at":"2020-08-14T11:50:26.371Z"}`,
				`{"something_at":"2020-Aug-14 11:50:26.371000"}`,
			},
		)

	formatTSStrftimeCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		layout, err := args.GetString("format")
		if err != nil {
			return nil, err
		}
		var timezone *time.Location
		tzOpt, err := args.GetOptionalString("tz")
		if err != nil {
			return nil, err
		}
		if tzOpt != nil {
			if timezone, err = time.LoadLocation(*tzOpt); err != nil {
				return nil, fmt.Errorf("failed to parse timezone location name: %w", err)
			}
		}
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			if timezone != nil {
				target = target.In(timezone)
			}
			return timefmt.Format(target, layout), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_strftime", formatTSStrftimeSpec, formatTSStrftimeCtor)

	bloblang.MustRegisterMethodV2("format_timestamp_strftime", formatTSStrftimeSpecDep, formatTSStrftimeCtor)

	formatTSUnixSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Converts a timestamp to a unix timestamp (seconds since epoch). Accepts unix timestamps or RFC 3339 strings. Returns an integer representing seconds.")

	formatTSUnixSpecDep := asDeprecated(formatTSUnixSpec)

	formatTSUnixSpec = formatTSUnixSpec.
		Example("Convert RFC 3339 timestamp to unix seconds.",
			`root.created_at_unix = this.created_at.ts_unix()`,
			[2]string{
				`{"created_at":"2009-11-10T23:00:00Z"}`,
				`{"created_at_unix":1257894000}`,
			},
		).
		Example("Unix timestamp passthrough returns same value.",
			`root.timestamp = this.ts.ts_unix()`,
			[2]string{
				`{"ts":1257894000}`,
				`{"timestamp":1257894000}`,
			},
		)

	formatTSUnixCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			return target.Unix(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_unix", formatTSUnixSpec, formatTSUnixCtor)

	bloblang.MustRegisterMethodV2("format_timestamp_unix", formatTSUnixSpecDep, formatTSUnixCtor)

	formatTSUnixMilliSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Converts a timestamp to a unix timestamp with millisecond precision (milliseconds since epoch). Accepts unix timestamps or RFC 3339 strings. Returns an integer representing milliseconds.")

	formatTSUnixMilliSpecDep := asDeprecated(formatTSUnixMilliSpec)

	formatTSUnixMilliSpec = formatTSUnixMilliSpec.
		Example("Convert timestamp to milliseconds since epoch.",
			`root.created_at_unix = this.created_at.ts_unix_milli()`,
			[2]string{
				`{"created_at":"2009-11-10T23:00:00Z"}`,
				`{"created_at_unix":1257894000000}`,
			},
		).
		Example("Useful for JavaScript timestamp compatibility.",
			`root.js_timestamp = this.event_time.ts_unix_milli()`,
			[2]string{
				`{"event_time":"2020-08-14T11:45:26.123Z"}`,
				`{"js_timestamp":1597405526123}`,
			},
		)

	formatTSUnixMilliCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			return target.UnixMilli(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_unix_milli", formatTSUnixMilliSpec, formatTSUnixMilliCtor)

	bloblang.MustRegisterMethodV2("format_timestamp_unix_milli", formatTSUnixMilliSpecDep, formatTSUnixMilliCtor)

	formatTSUnixMicroSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Converts a timestamp to a unix timestamp with microsecond precision (microseconds since epoch). Accepts unix timestamps or RFC 3339 strings. Returns an integer representing microseconds.")

	formatTSUnixMicroSpecDep := asDeprecated(formatTSUnixMicroSpec)

	formatTSUnixMicroSpec = formatTSUnixMicroSpec.
		Example("Convert timestamp to microseconds since epoch.",
			`root.created_at_unix = this.created_at.ts_unix_micro()`,
			[2]string{
				`{"created_at":"2009-11-10T23:00:00Z"}`,
				`{"created_at_unix":1257894000000000}`,
			},
		).
		Example("Preserve microsecond precision from timestamp.",
			`root.precise_time = this.timestamp.ts_unix_micro()`,
			[2]string{
				`{"timestamp":"2020-08-14T11:45:26.123456Z"}`,
				`{"precise_time":1597405526123456}`,
			},
		)

	formatTSUnixMicroCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			return target.UnixMicro(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_unix_micro", formatTSUnixMicroSpec, formatTSUnixMicroCtor)

	bloblang.MustRegisterMethodV2("format_timestamp_unix_micro", formatTSUnixMicroSpecDep, formatTSUnixMicroCtor)

	formatTSUnixNanoSpec := bloblang.NewPluginSpec().
		Category(query.MethodCategoryTime).
		Beta().
		Static().
		Description("Converts a timestamp to a unix timestamp with nanosecond precision (nanoseconds since epoch). Accepts unix timestamps or RFC 3339 strings. Returns an integer representing nanoseconds.")

	formatTSUnixNanoSpecDep := asDeprecated(formatTSUnixNanoSpec)

	formatTSUnixNanoSpec = formatTSUnixNanoSpec.
		Example("Convert timestamp to nanoseconds since epoch.",
			`root.created_at_unix = this.created_at.ts_unix_nano()`,
			[2]string{
				`{"created_at":"2009-11-10T23:00:00Z"}`,
				`{"created_at_unix":1257894000000000000}`,
			},
		).
		Example("Preserve full nanosecond precision.",
			`root.precise_time = this.timestamp.ts_unix_nano()`,
			[2]string{
				`{"timestamp":"2020-08-14T11:45:26.123456789Z"}`,
				`{"precise_time":1597405526123456789}`,
			},
		)

	formatTSUnixNanoCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		return bloblang.TimestampMethod(func(target time.Time) (any, error) {
			return target.UnixNano(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_unix_nano", formatTSUnixNanoSpec, formatTSUnixNanoCtor)

	bloblang.MustRegisterMethodV2("format_timestamp_unix_nano", formatTSUnixNanoSpecDep, formatTSUnixNanoCtor)

	tsSubSpec := bloblang.NewPluginSpec().
		Beta().
		Static().
		Category(query.MethodCategoryTime).
		Description(`Calculates the duration in nanoseconds between two timestamps (t1 - t2). Returns a signed integer: positive if t1 is after t2, negative if t1 is before t2. Use .abs() for absolute duration.`).
		Param(bloblang.NewTimestampParam("t2").Description("The timestamp to subtract from the target timestamp.")).
		Version("4.23.0").
		Example("Calculate absolute duration between two timestamps.",
			`root.between = this.started_at.ts_sub("2020-08-14T05:54:23Z").abs()`,
			[2]string{
				`{"started_at":"2020-08-13T05:54:23Z"}`,
				`{"between":86400000000000}`,
			}).
		Example("Calculate signed duration (can be negative).",
			`root.duration_ns = this.end_time.ts_sub(this.start_time)`,
			[2]string{
				`{"start_time":"2020-08-14T10:00:00Z","end_time":"2020-08-14T11:30:00Z"}`,
				`{"duration_ns":5400000000000}`,
			})

	tsSubCtor := func(args *bloblang.ParsedParams) (bloblang.Method, error) {
		t2, err := args.GetTimestamp("t2")
		if err != nil {
			return nil, err
		}
		return bloblang.TimestampMethod(func(t1 time.Time) (any, error) {
			return t1.Sub(t2).Nanoseconds(), nil
		}), nil
	}

	bloblang.MustRegisterMethodV2("ts_sub", tsSubSpec, tsSubCtor)
}
