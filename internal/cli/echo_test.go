// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestEcho(t *testing.T) {
	tmpDir := t.TempDir()
	tFile := func(name string) string {
		return filepath.Join(tmpDir, name)
	}

	tests := []struct {
		name     string
		files    map[string]string
		args     []string
		contains []string
	}{
		{
			name: "echo single file",
			args: []string{"benthos", "echo", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    mapping: 'root.id = uuid_v4()'
output:
  drop: {}
`,
			},
			contains: []string{
				"generate:",
				"root.id = uuid_v4",
				"drop: {}",
			},
		},
		{
			name: "echo single file old style",
			args: []string{"benthos", "-c", tFile("foo.yaml"), "echo"},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    mapping: 'root.id = uuid_v4()'
output:
  drop: {}
`,
			},
			contains: []string{
				"generate:",
				"root.id = uuid_v4",
				"drop: {}",
			},
		},
		{
			name: "echo with set flag",
			args: []string{"benthos", "echo", "--set", `input.generate.mapping=root.id = uuid_v4()`},
			contains: []string{
				"generate:",
				"root.id = uuid_v4",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for name, c := range test.files {
				require.NoError(t, os.WriteFile(tFile(name), []byte(c), 0o644))
			}

			var stdout, stderr bytes.Buffer

			opts := common.NewCLIOpts("", "")
			opts.Stdout = &stdout
			opts.Stderr = &stderr

			err := cli.App(opts).Run(test.args)
			require.NoError(t, err)

			for _, exp := range test.contains {
				assert.Contains(t, stdout.String(), exp)
			}
		})
	}
}
