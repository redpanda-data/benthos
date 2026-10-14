// Copyright 2025 Redpanda Data, Inc.

package studio_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
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

			expected := []string{
				"version", "date",
				"config",
				"buffers", "caches", "inputs", "outputs", "processors", "rate-limits", "metrics", "tracers", "scanners",
				"bloblang-functions", "bloblang-methods",
			}
			// V2 plugin fields (bloblang-v2-functions / bloblang-v2-methods)
			// only appear in the dump when the env has registered V2 plugins,
			// which happens for any binary importing public/components/{pure,io}.
			if _, ok := schema["bloblang-v2-functions"]; ok {
				expected = append(expected, "bloblang-v2-functions")
			}
			if _, ok := schema["bloblang-v2-methods"]; ok {
				expected = append(expected, "bloblang-v2-methods")
			}
			assert.ElementsMatch(t, slices.Collect(maps.Keys(schema)), expected)

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
