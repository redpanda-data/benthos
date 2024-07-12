package service_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
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
	go service.RunCLI(context.Background(),
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
