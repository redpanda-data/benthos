// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		contains    []string
		notContains []string
	}{
		{
			name: "create no arguments",
			args: []string{"benthos", "create"},
			contains: []string{
				"http:",
				"stdout:",
				"stdin:",
				"logger:",
			},
		},
		{
			name: "create single components",
			args: []string{"benthos", "create", "file/mapping/http_client"},
			contains: []string{
				"file:",
				"mapping:",
				"http_client:",
			},
		},
		{
			name: "create multiple components",
			args: []string{"benthos", "create", "file,http_server/mapping,http/http_client,stdout"},
			contains: []string{
				"file:",
				"http_server:",
				"mapping:",
				"http:",
				"http_client:",
				"stdout:",
			},
		},
		{
			name: "create simple",
			args: []string{"benthos", "create", "-s"},
			contains: []string{
				"stdout:",
				"stdin:",
			},
			notContains: []string{
				"http:",
				"logger:",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			opts := common.NewCLIOpts("", "")
			opts.Stdout = &stdout
			opts.Stderr = &stderr

			err := cli.App(opts).Run(test.args)
			require.NoError(t, err)

			for _, exp := range test.contains {
				assert.Contains(t, stdout.String(), exp)
			}

			for _, exp := range test.notContains {
				assert.NotContains(t, stdout.String(), exp)
			}
		})
	}
}
