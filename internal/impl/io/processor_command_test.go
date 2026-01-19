// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"testing"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand(t *testing.T) {
	tCtx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	tests := []struct {
		name           string
		config         string
		input          string
		outputContains string
		errContains    string
	}{
		{
			name: "static with args",
			config: `
name: go
args_mapping: '[ "help" ]'
`,
			outputContains: `Go is a tool for managing Go source code.`,
			input:          "",
		},
		{
			name: "static no args",
			config: `
name: cat
`,
			outputContains: `foo`,
			input:          "foo",
		},
		{
			name: "error command",
			config: `
name: go
`,
			input:       "",
			errContains: "exit status 2",
		},
		{
			name: "dynamic command",
			config: `
name: ${! this.name }
args_mapping: '[ "help" ]'
`,
			input:          `{"name":"go"}`,
			outputContains: `Go is a tool for managing Go source code.`,
		},
		{
			name: "dynamic args",
			config: `
name: ${! this.name }
args_mapping: 'this.args'
`,
			input:          `{"name":"go","args":["help"]}`,
			outputContains: `Go is a tool for managing Go source code.`,
		},
		{
			name: "static capture stdout",
			config: `
name: cat
args_mapping: '[ "-n" ]'
`,
			input:          "hello world",
			outputContains: "1\thello world",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pConf, err := commandProcSpec().ParseYAML(test.config, nil)
			require.NoError(t, err)

			cmdProc, err := newCommandProcFromParsed(pConf, service.MockResources())
			require.NoError(t, err)

			res, err := cmdProc.Process(tCtx, service.NewMessage([]byte(test.input)))

			exitCode, ok := res[0].MetaGetMut("exit_code")
			assert.True(t, ok)

			if test.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)

				assert.Equal(t, 2, exitCode)
			} else {
				require.NoError(t, err)
				require.Len(t, res, 1)

				assert.Equal(t, 0, exitCode)

				resBytes, err := res[0].AsBytes()
				require.NoError(t, err)
				assert.Contains(t, string(resBytes), test.outputContains)
			}
		})
	}
}
