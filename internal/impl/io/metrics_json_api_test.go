// Copyright 2026 Redpanda Data, Inc.

package io

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func jsonAPIBody(t *testing.T, h http.HandlerFunc) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/stats", http.NoBody)
	w := httptest.NewRecorder()
	h(w, req)

	body, err := io.ReadAll(w.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func TestJSONAPIMetricsDeleteSeriesPartialMatch(t *testing.T) {
	m, err := newJSONAPI(nil)
	require.NoError(t, err)

	m.NewCounterCtor("input_received", "stream")("foo").Incr(3)
	m.NewCounterCtor("input_received", "stream")("bar").Incr(4)
	m.NewGaugeCtor("connection_up", "stream")("foo").Set(1)
	m.NewTimerCtor("latency", "stream")("foo").Timing(100)
	m.NewTimerCtor("latency", "stream")("bar").Timing(200)
	m.NewCounterCtor("uptime")().Incr(9)

	body := jsonAPIBody(t, m.HandlerFunc())
	require.Contains(t, body, `stream=\"foo\"`)

	deleter, ok := any(m).(service.MetricsExporterSeriesDeleter)
	require.True(t, ok, "json_api should support series deletion")
	deleter.DeleteSeriesPartialMatch(map[string]string{"stream": "foo"})

	body = jsonAPIBody(t, m.HandlerFunc())
	assert.NotContains(t, body, `stream=\"foo\"`)
	assert.Contains(t, body, `input_received{stream=\"bar\"}`)
	assert.Contains(t, body, `latency{stream=\"bar\"}`)
	assert.Contains(t, body, "uptime")
}
