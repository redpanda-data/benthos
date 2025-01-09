// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	icli "github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func executeLintSubcmd(args []string) (string, error) {
	var buf bytes.Buffer

	opts := common.NewCLIOpts("1.2.3", "now")
	opts.Stderr = &buf

	err := icli.App(opts).Run(args)
	return buf.String(), err
}

func TestLints(t *testing.T) {
	tmpDir := t.TempDir()
	tFile := func(name string) string {
		return filepath.Join(tmpDir, name)
	}

	tests := []struct {
		name          string
		files         map[string]string
		args          []string
		expectedErr   bool
		expectedLints []string
	}{
		{
			name: "one file no errors",
			args: []string{"benthos", "lint", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    mapping: 'root.id = uuid_v4()'
output:
  drop: {}
`,
			},
		},
		{
			name: "one file unexpected fields",
			args: []string{"benthos", "lint", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    huh: what
    mapping: 'root.id = uuid_v4()'
output:
  nah: nope
  drop: {}
`,
			},
			expectedErr: true,
			expectedLints: []string{
				"field huh not recognised",
				"field nah is invalid",
			},
		},
		{
			name: "one file with c flag",
			args: []string{"benthos", "-c", tFile("foo.yaml"), "lint"},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    huh: what
    mapping: 'root.id = uuid_v4()'
output:
  nah: nope
  drop: {}
`,
			},
			expectedErr: true,
			expectedLints: []string{
				"field huh not recognised",
				"field nah is invalid",
			},
		},
		{
			name: "one file with r flag",
			args: []string{"benthos", "-r", tFile("foo.yaml"), "lint"},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    huh: what
    mapping: 'root.id = uuid_v4()'
output:
  nah: nope
  drop: {}
`,
			},
			expectedErr: true,
			expectedLints: []string{
				"field huh not recognised",
				"field nah is invalid",
			},
		},
		{
			name: "one file with r flag tailed",
			args: []string{"benthos", "lint", "-r", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    huh: what
    mapping: 'root.id = uuid_v4()'
output:
  nah: nope
  drop: {}
`,
			},
			expectedErr: true,
			expectedLints: []string{
				"field huh not recognised",
				"field nah is invalid",
			},
		},
		{
			name: "env var missing",
			args: []string{"benthos", "lint", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    mapping: 'root.id = "${BENTHOS_ENV_VAR_HOPEFULLY_MISSING}"'
output:
  drop: {}
`,
			},
			expectedErr: true,
			expectedLints: []string{
				"required environment variables were not set: [BENTHOS_ENV_VAR_HOPEFULLY_MISSING]",
			},
		},
		{
			name: "env var missing but we dont care",
			args: []string{"benthos", "lint", "--skip-env-var-check", tFile("foo.yaml")},
			files: map[string]string{
				"foo.yaml": `
input:
  generate:
    mapping: 'root.id = "${BENTHOS_ENV_VAR_HOPEFULLY_MISSING}"'
output:
  drop: {}
`,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for name, c := range test.files {
				require.NoError(t, os.WriteFile(tFile(name), []byte(c), 0o644))
			}

			outStr, err := executeLintSubcmd(test.args)
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if len(test.expectedLints) == 0 {
				assert.Empty(t, outStr)
			} else {
				for _, l := range test.expectedLints {
					assert.Contains(t, outStr, l)
				}
			}
		})
	}
}
