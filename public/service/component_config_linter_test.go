// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func TestComponentLinter(t *testing.T) {
	blobl := bloblang.NewEmptyEnvironment()

	require.NoError(t, blobl.RegisterFunction("cow", func(args ...any) (bloblang.Function, error) {
		return nil, errors.New("nope")
	}))

	env := service.NewEmptyEnvironment()
	env.UseBloblangEnvironment(blobl)

	require.NoError(t, env.RegisterInput("dog", service.NewConfigSpec().Fields(
		service.NewStringField("woof").Example("WOOF"),
	),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Input, error) {
			return nil, errors.New("nope")
		}))

	require.NoError(t, env.RegisterBatchBuffer("none", service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchBuffer, error) {
			return nil, errors.New("nope")
		}))

	require.NoError(t, env.RegisterProcessor("testprocessor", service.NewConfigSpec().Field(service.NewBloblangField("mapfield")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			return nil, errors.New("nope")
		}))

	require.NoError(t, env.RegisterOutput("stdout", service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (out service.Output, maxInFlight int, err error) {
			err = errors.New("nope")
			return
		}))

	tests := []struct {
		name         string
		typeStr      string
		config       string
		lintContains []string
		errContains  string
		linter       *service.ComponentConfigLinter
	}{
		{
			name:    "basic config no lints",
			typeStr: "input",
			config: `
dog:
  woof: wooooowooof
`,
		},
		{
			name:    "good bloblang",
			typeStr: "processor",
			config: `
testprocessor:
  mapfield: 'root = cow("test")'
`,
		},
		{
			name:    "bad bloblang",
			typeStr: "processor",
			config: `
testprocessor:
  mapfield: 'root = meow("test")'
`,
			lintContains: []string{
				"unrecognised function",
			},
		},
		{
			name:    "unknown field lint",
			typeStr: "input",
			config: `
dog:
  woof: wooooowooof
  huh: whats this?
`,
			lintContains: []string{
				"field huh not recognised",
			},
		},
		{
			name:        "invalid yaml",
			typeStr:     "input",
			config:      `	this			 !!!! isn't valid: yaml dog`,
			errContains: "found character",
		},
		{
			name:    "env var defined",
			typeStr: "input",
			config: `
dog:
  woof: ${WOOF}`,
			linter: env.NewComponentConfigLinter().
				SetEnvVarLookupFunc(func(ctx context.Context, s string) (string, bool) {
					return "meow", true
				}),
		},
		{
			name:    "env var missing with default",
			typeStr: "input",
			config: `
dog:
  woof: ${WOOF:defaultvalue}`,
			linter: env.NewComponentConfigLinter().
				SetEnvVarLookupFunc(func(ctx context.Context, s string) (string, bool) {
					return "", false
				}),
		},
		{
			name:    "env var missing with lint disabled",
			typeStr: "input",
			config: `
dog:
  woof: ${WOOF}`,
			linter: env.NewComponentConfigLinter().
				SetSkipEnvVarCheck(true).
				SetEnvVarLookupFunc(func(ctx context.Context, s string) (string, bool) {
					return "", false
				}),
		},
		{
			name:    "env var missing and linted",
			typeStr: "input",
			config: `
dog:
  woof: ${WOOF}`,
			linter: env.NewComponentConfigLinter().
				SetEnvVarLookupFunc(func(ctx context.Context, s string) (string, bool) {
					return "", false
				}),
			lintContains: []string{
				"required environment variables were not set",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if test.linter == nil {
				test.linter = env.NewComponentConfigLinter()
			}

			lints, err := test.linter.LintYAML(test.typeStr, []byte(test.config))
			if test.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
				return
			}

			require.NoError(t, err)
			require.Len(t, lints, len(test.lintContains))
			for i, lc := range test.lintContains {
				assert.Contains(t, lints[i].Error(), lc)
			}
		})
	}
}
