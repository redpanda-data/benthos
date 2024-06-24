package pure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestParseUrlencoded(t *testing.T) {
	testCases := []struct {
		name   string
		method string
		target any
		args   []any
		exp    any
	}{
		{
			name:   "simple parsing",
			method: "parse_form_url_encoded",
			target: "username=example",
			args:   []any{},
			exp:    map[string]any{"username": "example"},
		},
		{
			name:   "parsing multiple values under the same key",
			method: "parse_form_url_encoded",
			target: "usernames=userA&usernames=userB",
			args:   []any{},
			exp:    map[string]any{"usernames": []any{"userA", "userB"}},
		},
		{
			name:   "decodes data correctly",
			method: "parse_form_url_encoded",
			target: "email=example%40email.com",
			args:   []any{},
			exp:    map[string]any{"email": "example@email.com"},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			targetClone := value.IClone(test.target)
			argsClone := value.IClone(test.args).([]any)

			fn, err := query.InitMethodHelper(test.method, query.NewLiteralFunction("", targetClone), argsClone...)
			require.NoError(t, err)

			res, err := fn.Exec(query.FunctionContext{
				Maps:     map[string]query.Function{},
				Index:    0,
				MsgBatch: nil,
			})
			require.NoError(t, err)

			assert.Equal(t, test.exp, res)
			assert.Equal(t, test.target, targetClone)
			assert.Equal(t, test.args, argsClone)
		})
	}
}

func TestLintYAMLConfig(t *testing.T) {
	// Register a dummy deprecated processor with a deprecated field.
	err := service.RegisterBatchProcessor(
		"foobar",
		service.NewConfigSpec().Deprecated().Field(service.NewStringField("blobfish").Deprecated()),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			return nil, nil
		},
	)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		method   string
		target   any
		args     []any
		exp      any
		expError string
	}{
		{
			name:   "lints yaml configs",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: 1
    mapping: root.foo = "bar"
`,
			args: []any{},
			exp:  []any(nil),
		},
		{
			name:   "rejects invalid yaml configs with both spaces and tabs as indentation",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: 1
	mapping: root.foo = "bar"
`,
			args:     []any{},
			exp:      nil,
			expError: "failed to parse yaml: yaml: line 3: found a tab character that violates indentation",
		},
		{
			name:   "lints yaml configs with deprecated processors",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: 1
    mapping: root.foo = "bar"
  processors:
    - foobar:
        blobfish: "are cool"
`,
			args: []any{true},
			exp:  []any{"(6,1) component foobar is deprecated", "(7,1) field blobfish is deprecated"},
		},
		{
			name:   "lints yaml configs with deprecated bloblang methods",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: 1
    mapping: root.ts = 666.format_timestamp()
`,
			args: []any{true},
			exp:  []any(nil), // TODO: THIS SHOULD FAIL!
		},
		{
			name:   "lints yaml configs with missing labels",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: 1
    mapping: root.foo = "bar"
`,
			args: []any{false, true},
			exp:  []any{"(2,1) label is required for generate"},
		},
		{
			name:   "lints yaml configs with unset environment variables",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: ${BLOBFISH_COUNT}
    mapping: root.foo = "bar"
`,
			args: []any{false, false, false},
			exp:  []any{"(1,1) required environment variables were not set: [BLOBFISH_COUNT]", "(0,1) expected object value, got !!null"},
		},
		{
			name:   "lints yaml configs with unset environment variables which have a default value",
			method: "lint_yaml_config",
			target: `input:
  generate:
    count: ${BLOBFISH_COUNT:42}
    mapping: root.foo = "bar"
`,
			args: []any{false, false, false},
			exp:  []any(nil),
		},
		{
			name:   "lints yaml configs which explicitly disable linting",
			method: "lint_yaml_config",
			target: `# BENTHOS LINT DISABLE
input:
  generate:
    count: 1
`,
			args: []any{false, false, false},
			exp:  []any(nil),
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			targetClone := value.IClone(test.target)
			argsClone := value.IClone(test.args).([]any)

			fn, err := query.InitMethodHelper(test.method, query.NewLiteralFunction("", targetClone), argsClone...)
			require.NoError(t, err)

			res, err := fn.Exec(query.FunctionContext{
				Maps:     map[string]query.Function{},
				Index:    0,
				MsgBatch: nil,
			})
			if test.expError != "" {
				require.ErrorContains(t, err, test.expError)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.exp, res)
			assert.Equal(t, test.target, targetClone)
			assert.Equal(t, test.args, argsClone)
		})
	}
}
