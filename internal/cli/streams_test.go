// Copyright 2025 Redpanda Data, Inc.

package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second))
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

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second))
	defer cancel()

	var stdout bytes.Buffer
	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = &stdout

	require.NoError(t, icli.App(opts).RunContext(ctx, []string{"benthos", "-c", obsPath, "streams", confPath}))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")

	assert.Contains(t, stdout.String(), "level=trace")
}

// startStreamsForTest launches streams mode with the service-wide HTTP server
// bound to a free loopback port and returns that address once the server is
// reachable. The stream CRUD API is enabled only when bindHTTP is true.
func startStreamsForTest(t *testing.T, bindHTTP bool) string {
	t.Helper()

	// Grab a free loopback port for the service-wide HTTP server to bind to.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	tmpDir := t.TempDir()
	obsPath := filepath.Join(tmpDir, "o11y.yaml")
	confPath := filepath.Join(tmpDir, "foo.yaml")

	require.NoError(t, os.WriteFile(confPath, []byte(`
input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"
output:
  drop: {}
`), 0o644))

	require.NoError(t, os.WriteFile(obsPath, fmt.Appendf(nil, `
http:
  address: %v
`, addr), 0o644))

	args := []string{"benthos", "streams", "-o", obsPath, confPath}
	if bindHTTP {
		args = []string{"benthos", "streams", "--bind-http", "-o", obsPath, confPath}
	}

	opts := common.NewCLIOpts("1.2.3", "aaa")
	opts.Stdout = io.Discard

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- icli.App(opts).RunContext(ctx, args)
	}()

	// Ensure the run goroutine has terminated before the test finishes, and
	// surface any startup/shutdown error with its own message rather than as
	// a bare "server should bind" timeout.
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(time.Second * 5):
			t.Fatal("timed out waiting for streams mode to shut down")
		}
	})

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/ping")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, time.Second*5, time.Millisecond*50, "service-wide HTTP server should bind")

	return addr
}

// TestStreamsModeBindHTTP confirms the stream CRUD API is registered when
// --bind-http is passed.
func TestStreamsModeBindHTTP(t *testing.T) {
	addr := startStreamsForTest(t, true)

	resp, err := http.Get("http://" + addr + "/streams")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "stream CRUD endpoints should be registered with --bind-http")
}

// TestStreamsModeNoBindHTTP confirms the service-wide HTTP server still binds
// but the stream CRUD API is not registered without --bind-http.
func TestStreamsModeNoBindHTTP(t *testing.T) {
	addr := startStreamsForTest(t, false)

	resp, err := http.Get("http://" + addr + "/streams")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "stream CRUD endpoints should not be registered without --bind-http")
}
