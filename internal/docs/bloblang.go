// Copyright 2025 Redpanda Data, Inc.

package docs

import (
	"errors"

	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// LintBloblangMapping is function for linting a config field expected to be a
// bloblang mapping.
func LintBloblangMapping(ctx LintContext, line, col int, v any) []Lint {
	str, ok := v.(string)
	if !ok {
		return nil
	}
	if str == "" {
		return nil
	}
	_, err := ctx.conf.BloblangEnv.Parse(str)
	if err == nil {
		return nil
	}
	if mErr, ok := err.(*bloblang.ParseError); ok {
		lint := NewLintError(line+mErr.Line-1, LintBadBloblang, mErr)
		lint.Column = col + mErr.Column
		return []Lint{lint}
	}
	return []Lint{NewLintError(line, LintBadBloblang, err)}
}

// LintBloblangField is function for linting a config field expected to be an
// interpolation string.
func LintBloblangField(ctx LintContext, line, col int, v any) []Lint {
	str, ok := v.(string)
	if !ok {
		return nil
	}
	if str == "" {
		return nil
	}
	err := ctx.conf.BloblangEnv.CheckInterpolatedString(str)
	if err == nil {
		return nil
	}
	if mErr, ok := err.(*bloblang.ParseError); ok {
		lint := NewLintError(line+mErr.Line-1, LintBadBloblang, mErr)
		lint.Column = col + mErr.Column
		return []Lint{lint}
	}
	return []Lint{NewLintError(line, LintBadBloblang, err)}
}

// LintBloblangV2Mapping is the linter for a config field expected to be a
// Bloblang V2 mapping. V2 parsing is side-effect free so the configured
// environment is used directly, no deactivated mode required.
func LintBloblangV2Mapping(ctx LintContext, line, col int, v any) []Lint {
	str, ok := v.(string)
	if !ok {
		return nil
	}
	if str == "" {
		return nil
	}
	env := ctx.conf.BloblangV2Env
	if env == nil {
		env = bloblangv2.GlobalEnvironment()
	}
	_, err := env.Parse(str)
	if err == nil {
		return nil
	}
	var pErr *bloblangv2.ParseError
	if errors.As(err, &pErr) {
		lint := NewLintError(line+pErr.Line-1, LintBadBloblang, pErr)
		lint.Column = col + pErr.Column
		return []Lint{lint}
	}
	return []Lint{NewLintError(line, LintBadBloblang, err)}
}
