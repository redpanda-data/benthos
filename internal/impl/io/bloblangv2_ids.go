// Copyright 2026 Redpanda Data, Inc.

package io

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/segmentio/ksuid"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 ID-generating functions. All non-deterministic (random or
// time-based), so they live in internal/impl/io.

func init() {
	bloblangv2.MustRegisterFunction("ksuid",
		bloblangv2.NewPluginSpec().
			Category("General").
			Description("Generates a K-Sortable Unique Identifier (KSUID) with millisecond timestamp ordering.").
			Impure(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) {
				return ksuid.New().String(), nil
			}, nil
		},
	)

	bloblangv2.MustRegisterFunction("nanoid",
		bloblangv2.NewPluginSpec().
			Category("General").
			Description("Generates a URL-safe unique identifier using Nano ID. Customise the length (default 21) and supply an alphabet to control the character set; alphabet requires length to also be supplied.").
			Param(bloblangv2.NewInt64Param("length").Description("Optional length.").Optional()).
			Param(bloblangv2.NewStringParam("alphabet").Description("Optional custom alphabet. When supplied, length must also be set.").Optional()).
			Impure(),
		nanoidV2Ctor,
	)

	bloblangv2.MustRegisterFunction("uuid_v7",
		bloblangv2.NewPluginSpec().
			Category("General").
			Description("Generates a time-ordered UUID v7. Optionally pass a timestamp to back-date the time component.").
			Param(bloblangv2.NewAnyParam("time").Description("Optional timestamp to use for the time-ordered portion of the UUID.").Optional()).
			Impure(),
		uuidV7V2Ctor,
	)
}

func nanoidV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
	lenArg, err := args.GetOptionalInt64("length")
	if err != nil {
		return nil, err
	}
	alphabetArg, err := args.GetOptionalString("alphabet")
	if err != nil {
		return nil, err
	}
	if alphabetArg != nil && lenArg == nil {
		return nil, errors.New("field length must be specified when an alphabet is specified")
	}
	return func() (any, error) {
		if alphabetArg != nil {
			return gonanoid.Generate(*alphabetArg, int(*lenArg))
		}
		if lenArg != nil {
			return gonanoid.New(int(*lenArg))
		}
		return gonanoid.New()
	}, nil
}

func uuidV7V2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
	rawTime, _ := args.Get("time")
	var ts *time.Time
	if rawTime != nil {
		t, ok := rawTime.(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected timestamp argument, got %T", rawTime)
		}
		ts = &t
	}
	return func() (any, error) {
		if ts == nil {
			u7, err := uuid.NewV7()
			if err != nil {
				return nil, fmt.Errorf("unable to generate uuid v7: %w", err)
			}
			return u7.String(), nil
		}
		u7, err := uuid.NewV7AtTime(*ts)
		if err != nil {
			return nil, fmt.Errorf("unable to generate uuid v7 at time %s: %w", *ts, err)
		}
		return u7.String(), nil
	}, nil
}
