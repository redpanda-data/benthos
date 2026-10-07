// Copyright 2026 Redpanda Data, Inc.

package studio_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	icli "github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
)

func TestSyncSchema(t *testing.T) {
	dummyVersion := "1.2.3"
	dummyDate := "justnow"

	testHandler := func(apiPathPrefix string) func(w http.ResponseWriter, r *http.Request) {
		if apiPathPrefix == "" {
			apiPathPrefix = "api"
		}
		return func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, fmt.Sprintf("/%s/v1/token/footoken/session/foosession/schema", apiPathPrefix), r.URL.Path)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var schema map[string]json.RawMessage
			err = json.Unmarshal(body, &schema)
			require.NoError(t, err)

			assert.ElementsMatch(t, slices.Collect(maps.Keys(schema)), []string{
				"version", "date",
				"config",
				"buffers", "caches", "inputs", "outputs", "processors", "rate-limits", "metrics", "tracers", "scanners",
				"bloblang-functions", "bloblang-methods",
			})

			var version string
			err = json.Unmarshal(schema["version"], &version)
			require.NoError(t, err)
			assert.Equal(t, dummyVersion, version)

			var date string
			err = json.Unmarshal(schema["date"], &date)
			require.NoError(t, err)
			assert.Equal(t, dummyDate, date)
		}
	}

	tests := []struct {
		name          string
		apiPathPrefix string
	}{
		{
			name: "default api path",
		},
		{
			name:          "custom api path",
			apiPathPrefix: "foobar",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(testHandler(test.apiPathPrefix)))

			cliApp := icli.App(common.NewCLIOpts(dummyVersion, dummyDate))
			require.NoError(t, cliApp.Run([]string{"benthos", "studio", "--endpoint", testServer.URL, "sync-schema", "--session", "foosession", "--token", "footoken", "--api-path-prefix", test.apiPathPrefix}))
		})
	}
}

func TestSyncSchemaErrorRedactsToken(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	cliApp := icli.App(common.NewCLIOpts("1.2.3", "justnow"))
	err = cliApp.Run([]string{"benthos", "studio", "--endpoint", "http://" + addr + "?secret=S3cretPass", "sync-schema", "--session", "foosession", "--token", "S3cretTok"})
	require.ErrorContains(t, err, "sync request failed")
	assert.NotContains(t, err.Error(), "S3cretTok")
	assert.NotContains(t, err.Error(), "S3cretPass")
}
