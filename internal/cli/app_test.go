// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"context"
	"fmt"
	"io"
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

func TestRunCLIShutdown(t *testing.T) {
	tmpDir := t.TempDir()
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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = io.Discard

	require.NoError(t, icli.App(opts).RunContext(ctx, []string{"benthos", "run", confPath}))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")
}

func TestRunCLIOldStyle(t *testing.T) {
	tmpDir := t.TempDir()
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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = io.Discard

	require.NoError(t, icli.App(opts).RunContext(ctx, []string{"benthos", "-c", confPath}))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")
}
