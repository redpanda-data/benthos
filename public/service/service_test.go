// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
	"github.com/redpanda-data/benthos/v4/public/service"
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

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second))
	defer cancel()

	service.RunCLI(ctx, service.CLIOptSetArgs("benthos", "-c", confPath))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "foobar")
}

func TestRunCLIConnectivityStatus(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")

	require.NoError(t, os.WriteFile(confPath, []byte(`
input:
  label: inputa
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"

output:
  label: outputa
  drop: {}
`), 0o644))

	var summary atomic.Pointer[service.RunningStreamSummary]
	go service.RunCLI(t.Context(),
		service.CLIOptSetArgs("meow", "-c", confPath),
		service.CLIOptOnStreamStart(func(s *service.RunningStreamSummary) error {
			summary.Store(s)
			return nil
		}),
	)

	statusMap := map[string]bool{}
	require.Eventually(t, func() bool {
		tmp := summary.Load()
		if tmp == nil {
			return false
		}

		statuses := tmp.ConnectionStatuses()
		if len(statuses) != 2 {
			return false
		}

		for _, s := range statuses {
			statusMap[s.Label()] = s.Active()
		}
		return true
	}, time.Second, time.Millisecond*50)

	assert.Equal(t, map[string]bool{
		"inputa":  true,
		"outputa": true,
	}, statusMap)
}

func TestRunCLICustomFlags(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")

	// The test subcommand fails when the config has no tests, and a test needs the pipeline key and an output
	// condition. So the config has a pipeline with no processors and one passthrough test.
	require.NoError(t, os.WriteFile(confPath, []byte(`
input:
  label: inputa
  generate:
    count: 1
    mapping: 'root.id = "foobar"'
    interval: "1ms"

pipeline:
  processors: []

output:
  label: outputa
  drop: {}

tests:
  - name: passthrough
    input_batch:
      - content: foo
    output_batches:
      - - content_equals: foo
`), 0o644))

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "run",
			args: []string{"meow", "run", "--meow", "foobar", confPath},
			want: []string{"foobar"},
		},
		{
			name: "lint",
			args: []string{"meow", "lint", "--meow", "foobar", confPath},
			want: []string{"foobar"},
		},
		{
			name: "test",
			args: []string{"meow", "test", "--meow", "foobar", confPath},
			want: []string{"foobar"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var flagExtracted []string
			code, err := service.RunCLIToCode(t.Context(),
				service.CLIOptSetArgs(test.args...),
				service.CLIOptCustomRunFlags([]cli.Flag{
					&cli.StringSliceFlag{
						Name: "meow",
					},
				}, func(c *cli.Context) error {
					flagExtracted = c.StringSlice("meow")
					return nil
				}))
			require.NoError(t, err)
			require.Equal(t, 0, code)
			assert.Equal(t, test.want, flagExtracted)
		})
	}
}

func TestRunCLITemplate(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")
	tmplPath := filepath.Join(tmpDir, "bar.yaml")
	outPath := filepath.Join(tmpDir, "out.txt")

	require.NoError(t, os.WriteFile(confPath, fmt.Appendf(nil, `
input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"
  processors:
    - bar: {}
output:
  file:
    codec: lines
    path: %v
`, outPath), 0o644))

	require.NoError(t, os.WriteFile(tmplPath, []byte(`
name: bar
type: processor

mapping: 'root.mapping = "content().uppercase()"'
`), 0o644))

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second))
	defer cancel()

	service.RunCLI(ctx, service.CLIOptSetArgs("benthos", "run", "-t", tmplPath, confPath))

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "FOOBAR")
}

func TestRunCLITemplateCustomEnv(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")
	tmplPath := filepath.Join(tmpDir, "bar.yaml")
	outPath := filepath.Join(tmpDir, "out.txt")

	env := service.NewEnvironment().Clone()

	require.NoError(t, os.WriteFile(confPath, fmt.Appendf(nil, `
input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"
  processors:
    - aaabbbccc: {}
output:
  file:
    codec: lines
    path: %v
`, outPath), 0o644))

	require.NoError(t, os.WriteFile(tmplPath, []byte(`
name: aaabbbccc
type: processor

mapping: 'root.mapping = "content().uppercase()"'
`), 0o644))

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second))
	defer cancel()

	service.RunCLI(ctx,
		service.CLIOptSetMainSchemaFrom(func() *service.ConfigSchema {
			return env.FullConfigSchema("", "")
		}),
		service.CLIOptSetEnvironment(env),
		service.CLIOptSetArgs("benthos", "run", "-t", tmplPath, confPath),
	)

	data, _ := os.ReadFile(outPath)
	assert.Contains(t, string(data), "FOOBAR")
}

func TestRunCLIResourcesRegisterEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")

	// Reserve a free port for the HTTP server, since the API doesn't report
	// the address it bound to.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	require.NoError(t, os.WriteFile(confPath, fmt.Appendf(nil, `
http:
  address: %v
  root_path: /meow

input:
  generate:
    mapping: 'root.id = "foobar"'
    interval: "100ms"

output:
  drop: {}
`, addr), 0o644))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := service.RunCLIToCode(ctx,
			service.CLIOptSetArgs("meow", "-c", confPath),
			service.CLIOptOnConfigParse(func(pConf *service.ParsedConfig) error {
				pConf.Resources().RegisterEndpoint("/custom", "A custom endpoint.", func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("custom body"))
				})
				return nil
			}),
		)
		done <- result{code: code, err: err}
	}()

	get := func(path string) (int, string, error) {
		res, err := http.Get("http://" + addr + path)
		if err != nil {
			return 0, "", err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		return res.StatusCode, string(body), err
	}

	require.Eventually(t, func() bool {
		code, body, err := get("/custom")
		return err == nil && code == http.StatusOK && body == "custom body"
	}, time.Second*5, time.Millisecond*50)

	// Also served under the configured root_path.
	code, body, err := get("/meow/custom")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "custom body", body)

	_, body, err = get("/endpoints")
	require.NoError(t, err)

	var endpoints map[string]string
	require.NoError(t, json.Unmarshal([]byte(body), &endpoints))
	assert.Equal(t, "A custom endpoint.", endpoints["/custom"])

	cancel()
	select {
	case res := <-done:
		require.NoError(t, res.err)
		assert.Equal(t, 0, res.code)
	case <-time.After(time.Second * 10):
		t.Fatal("timed out waiting for the CLI to exit")
	}
}

func TestRunCLIResourcesRegisterEndpointServerDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "foo.yaml")

	require.NoError(t, os.WriteFile(confPath, []byte(`
http:
  enabled: false

input:
  generate:
    count: 1
    mapping: 'root.id = "foobar"'
    interval: "1ms"

output:
  drop: {}
`), 0o644))

	var registered bool
	code, err := service.RunCLIToCode(t.Context(),
		service.CLIOptSetArgs("meow", "-c", confPath),
		service.CLIOptOnConfigParse(func(pConf *service.ParsedConfig) error {
			pConf.Resources().RegisterEndpoint("/custom", "A custom endpoint.", func(http.ResponseWriter, *http.Request) {})
			registered = true
			return nil
		}),
	)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.True(t, registered)
}
