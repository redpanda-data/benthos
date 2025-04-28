// Copyright 2025 Redpanda Data, Inc.

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvSwapping(t *testing.T) {
	envFn := func(s string) (string, bool) {
		switch s {
		case "BENTHOS_TEST_FOO":
			return "", true
		case "BENTHOS.TEST.FOO":
			return "testfoo", true
		case "BENTHOS.TEST.BAR":
			return "test\nbar", true
		case "BENTHOS.TEST.B64":
			return "Zm9vYmFy", true
		}
		return "", false
	}

	tests := map[string]struct {
		result      string
		errContains string
	}{
		"foo ${DOES_NOT_EXIST:} baz":                         {result: "foo  baz"},
		"${DOES_NOT_EXIST:}":                                 {result: ""},
		"${BENTHOS_TEST_FOO:}":                               {result: ""},
		"${BENTHOS.TEST.FOO:}":                               {result: "testfoo"},
		"foo ${BENTHOS_TEST_FOO:bar} baz":                    {result: "foo bar baz"},
		"foo ${BENTHOS.TEST.FOO:bar} baz":                    {result: "foo testfoo baz"},
		"foo ${BENTHOS.TEST.FOO} baz":                        {result: "foo testfoo baz"},
		"foo ${BENTHOS_TEST_FOO:http://bar.com} baz":         {result: "foo http://bar.com baz"},
		"foo ${BENTHOS_TEST_FOO:http://bar.com?wat=nuh} baz": {result: "foo http://bar.com?wat=nuh baz"},
		"foo ${BENTHOS_TEST_FOO:http://bar.com#wat} baz":     {result: "foo http://bar.com#wat baz"},
		"foo ${BENTHOS_TEST_FOO:tcp://*:2020} baz":           {result: "foo tcp://*:2020 baz"},
		"foo ${BENTHOS_TEST_FOO:bar} http://bar.com baz":     {result: "foo bar http://bar.com baz"},
		"foo ${BENTHOS_TEST_FOO} http://bar.com baz":         {result: "foo  http://bar.com baz"},
		"foo ${BENTHOS_TEST_FOO:wat@nuh.com} baz":            {result: "foo wat@nuh.com baz"},
		"foo ${} baz":                                                              {result: "foo ${} baz"},
		"foo ${BENTHOS_TEST_FOO:foo,bar} baz":                                      {result: "foo foo,bar baz"},
		"foo ${BENTHOS_TEST_FOO} baz":                                              {result: "foo  baz"},
		"foo ${BENTHOS_TEST_FOO:${!metadata:foo}} baz":                             {result: "foo ${!metadata:foo} baz"},
		"foo ${BENTHOS_TEST_FOO:${!metadata:foo}${!metadata:bar}} baz":             {result: "foo ${!metadata:foo}${!metadata:bar} baz"},
		"foo ${BENTHOS_TEST_FOO:${!count:foo}-${!timestamp_unix_nano}.tar.gz} baz": {result: "foo ${!count:foo}-${!timestamp_unix_nano}.tar.gz baz"},
		"foo ${{BENTHOS_TEST_FOO:bar}} baz":                                        {result: "foo ${BENTHOS_TEST_FOO:bar} baz"},
		"foo ${{BENTHOS_TEST_FOO}} baz":                                            {result: "foo ${BENTHOS_TEST_FOO} baz"},
		"foo ${BENTHOS.TEST.BAR} baz":                                              {result: "foo test\\nbar baz"},
		"foo ${BENTHOS_TEST_THIS_DOESNT_EXIST_LOL} baz":                            {errContains: "required environment variables were not set: [BENTHOS_TEST_THIS_DOESNT_EXIST_LOL]"},
		"foo ${BENTHOS_TEST_NOPE_A} baz ${BENTHOS_TEST_NOPE_B} buz":                {errContains: "required environment variables were not set: [BENTHOS_TEST_NOPE_A BENTHOS_TEST_NOPE_B]"},
		"foo ${DOES_NOT_EXIST::} baz":                                              {result: "foo : baz"},
		`${DOES_NOT_EXIST:${BENTHOS.TEST}}`:                                        {result: "${BENTHOS.TEST}"},
		`${BENTHOS.TEST.B64|base64decode}`:                                         {result: "foobar"},
		`${BENTHOS.TEST.B64:foo|base64decode}`:                                     {result: "foobar"},
		`${BENTHOS.TEST.B64:|base64decode}`:                                        {result: "foobar"},
		`${BENTHOS.TEST.B64:foo|bar|base64decode}`:                                 {result: "foobar"},
		`${BENTHOS.TEST.B64|lolwut}`:                                               {errContains: "unknown env var decode function: lolwut"},
		`${DOES_NOT_EXIST:|kaboom|base64decode}`:                                   {errContains: "failed to decode base64-encoded env var: illegal base64 data at input byte 0"},
		`${BENTHOS.TEST.B64:ignoreme|base64decode}`:                                {result: "foobar"},
		`${DOES_NOT_EXIST:Zm9vYmFy|base64decode}`:                                  {result: "foobar"},
	}

	for in, exp := range tests {
		r := NewReader("", nil, OptUseEnvLookupFunc(func(ctx context.Context, s string) (string, bool) {
			return envFn(s)
		}))
		out, err := r.ReplaceEnvVariables(t.Context(), []byte(in))
		if exp.errContains != "" {
			require.Error(t, err)
			assert.Contains(t, err.Error(), exp.errContains)
		} else {
			require.NoError(t, err)
			assert.Equal(t, exp.result, string(out))
		}
	}
}
