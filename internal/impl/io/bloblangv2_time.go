// Copyright 2026 Redpanda Data, Inc.

package io

import (
	"time"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 timestamp_unix* functions. These read the wall clock and
// are therefore impure; they live in internal/impl/io.

func init() {
	bloblangv2.MustRegisterFunction("timestamp_unix",
		bloblangv2.NewPluginSpec().
			Category("Environment").
			Description("Returns the current Unix timestamp in seconds since epoch.").
			Impure(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return time.Now().Unix(), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterFunction("timestamp_unix_milli",
		bloblangv2.NewPluginSpec().
			Category("Environment").
			Description("Returns the current Unix timestamp in milliseconds since epoch.").
			Impure(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return time.Now().UnixMilli(), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterFunction("timestamp_unix_micro",
		bloblangv2.NewPluginSpec().
			Category("Environment").
			Description("Returns the current Unix timestamp in microseconds since epoch.").
			Impure(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return time.Now().UnixMicro(), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterFunction("timestamp_unix_nano",
		bloblangv2.NewPluginSpec().
			Category("Environment").
			Description("Returns the current Unix timestamp in nanoseconds since epoch.").
			Impure(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return time.Now().UnixNano(), nil
			}, nil
		},
	)
}
