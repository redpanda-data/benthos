// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	icli "github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestStreamsMode(t *testing.T) {
	tmpDir := t.TempDir()
	obsPath := filepath.Join(tmpDir, "o11y.yaml")
	confPath := filepath.Join(tmpDir, "foo.yaml")
	outPath := filepath.Join(tmpDir, "out.txt")

	require.NoError(t, os.WriteFile(confPath, fmt.Appendf(nil, `
input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"
output:
  file:
    codec: lines
    path: %v
`, outPath), 0o644))

	require.NoError(t, os.WriteFile(obsPath, []byte(`
logger:
  level: TRACE
`), 0o644))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	var stdout bytes.Buffer
	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = &stdout

	require.NoError(t, icli.App(opts).RunContext(ctx, []string{"benthos", "streams", "-o", obsPath, confPath}))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")

	assert.Contains(t, stdout.String(), "level=trace")
}

func TestStreamsModeOldStyle(t *testing.T) {
	tmpDir := t.TempDir()
	obsPath := filepath.Join(tmpDir, "o11y.yaml")
	confPath := filepath.Join(tmpDir, "foo.yaml")
	outPath := filepath.Join(tmpDir, "out.txt")

	require.NoError(t, os.WriteFile(confPath, fmt.Appendf(nil, `
input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"
output:
  file:
    codec: lines
    path: %v
`, outPath), 0o644))

	require.NoError(t, os.WriteFile(obsPath, []byte(`
logger:
  level: TRACE
`), 0o644))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	var stdout bytes.Buffer
	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = &stdout

	require.NoError(t, icli.App(opts).RunContext(ctx, []string{"benthos", "-c", obsPath, "streams", confPath}))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")

	assert.Contains(t, stdout.String(), "level=trace")
}
