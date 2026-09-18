// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeConfigBody(t *testing.T) {
	tests := map[string]struct {
		body string
		// passthrough asserts the body is returned as the very same slice, which
		// is what guarantees a non-envelope request reaches ReplaceEnvVariables
		// byte-for-byte as it did before the envelope form existed.
		passthrough bool
		overrides   map[string]string
		template    string
		errContains string
	}{
		"a raw JSON config passes through": {
			body:        `{"input":{"generate":{"mapping":"root.id = uuid_v4()"}}}`,
			passthrough: true,
		},
		"a raw YAML config passes through": {
			body: `
input:
  generate:
    mapping: root.id = uuid_v4()
`,
			passthrough: true,
		},
		"a body that only parses after substitution passes through": {
			// The documented ${VAR: default} form puts a ": " inside an
			// otherwise plain scalar, which is a YAML syntax error in block
			// context. This is the case the body-field design could not serve.
			body: `
output:
  file:
    path: ${OUT_PATH: ./fallback.jsonl}
`,
			passthrough: true,
		},
		"an empty body passes through": {
			body:        "",
			passthrough: true,
		},
		"a bare scalar body passes through": {
			body:        "just a string\n",
			passthrough: true,
		},
		"a template key holding a mapping passes through": {
			body:        `{"template":{"input":{"generate":{}}}}`,
			passthrough: true,
		},
		"a template key holding a number passes through": {
			body:        `{"template":5}`,
			passthrough: true,
		},
		"a template key alongside a config field passes through": {
			body:        `{"template":"input: {}\n","input":{"generate":{}}}`,
			passthrough: true,
		},
		"an env key with no template passes through": {
			body:        `{"env":{"FOO":"bar"}}`,
			passthrough: true,
		},
		"a duplicated template key passes through": {
			body:        "template: 'input: {}'\ntemplate: 'output: {}'\n",
			passthrough: true,
		},
		"a JSON envelope yields overrides and the template": {
			body:      `{"env":{"FOO":"bar"},"template":"input:\n  generate: {}\n"}`,
			overrides: map[string]string{"FOO": "bar"},
			template:  "input:\n  generate: {}\n",
		},
		"multiple overrides are read": {
			body:      `{"env":{"FOO":"bar","BAZ":"buz"},"template":"input: {}"}`,
			overrides: map[string]string{"FOO": "bar", "BAZ": "buz"},
			template:  "input: {}",
		},
		"a template with no env yields no overrides": {
			body:     `{"template":"input: {}"}`,
			template: "input: {}",
		},
		"an explicit null env yields no overrides": {
			body:     "env:\ntemplate: 'input: {}'\n",
			template: "input: {}",
		},
		"an empty env object yields an empty override set": {
			body:      `{"env":{},"template":"input: {}"}`,
			overrides: map[string]string{},
			template:  "input: {}",
		},
		"a YAML block scalar template is taken literally": {
			// A block scalar swallows the ${VAR: default} form, so the envelope
			// stays parseable no matter what the template contains.
			body: `
env:
  MAPPING: root.id = "x"
template: |
  input:
    generate:
      mapping: ${MAPPING}
  output:
    file:
      path: ${OUT_PATH: ./fallback.jsonl}
`,
			overrides: map[string]string{"MAPPING": `root.id = "x"`},
			template: `input:
  generate:
    mapping: ${MAPPING}
output:
  file:
    path: ${OUT_PATH: ./fallback.jsonl}
`,
		},
		"a quoted numeric override stays a string": {
			body:      `{"env":{"FOO":"5"},"template":"input: {}"}`,
			overrides: map[string]string{"FOO": "5"},
			template:  "input: {}",
		},
		"an empty string override is preserved": {
			body:      `{"env":{"FOO":""},"template":"input: {}"}`,
			overrides: map[string]string{"FOO": ""},
			template:  "input: {}",
		},
		"an env array is rejected": {
			body:        `{"env":["a","b"],"template":"input: {}"}`,
			errContains: "field env: must be an object of string values",
		},
		"an env string is rejected": {
			body:        `{"env":"FOO=bar","template":"input: {}"}`,
			errContains: "field env: must be an object of string values",
		},
		"a numeric override is rejected rather than coerced": {
			body:        `{"env":{"FOO":3},"template":"input: {}"}`,
			errContains: "field env: value of 'FOO' must be a string",
		},
		"a boolean override is rejected rather than coerced": {
			body:        `{"env":{"FOO":true},"template":"input: {}"}`,
			errContains: "field env: value of 'FOO' must be a string",
		},
		"a nested object override is rejected": {
			body:        `{"env":{"FOO":{"BAR":"baz"}},"template":"input: {}"}`,
			errContains: "field env: value of 'FOO' must be a string",
		},
		"a null override is rejected rather than coerced": {
			body:        `{"env":{"FOO":null},"template":"input: {}"}`,
			errContains: "field env: value of 'FOO' must be a string",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			raw := []byte(test.body)

			overrides, template, err := decodeConfigBody(raw)
			if test.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
				return
			}
			require.NoError(t, err)

			if test.passthrough {
				assert.Nil(t, overrides)
				require.Equal(t, raw, template)
				if len(raw) > 0 {
					assert.Same(t, &raw[0], &template[0],
						"expected the body to be returned as the same slice, unmodified")
				}
				return
			}

			assert.Equal(t, test.overrides, overrides)
			assert.Equal(t, test.template, string(template))
		})
	}
}
