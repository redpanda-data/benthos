// Copyright 2026 Redpanda Data, Inc.

package studio

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/log"
)

func TestDoRateLimitedReqRedactsCredentials(t *testing.T) {
	const pass = "S3cretPass"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	var buf bytes.Buffer
	logger := &hotSwapLogger{}
	logger.swap(log.NewBenthosLogAdapter(slog.New(slog.NewTextHandler(&buf, nil))))

	tracker := &sessionTracker{logger: logger, nowFn: time.Now}

	ctx, done := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer done()

	_, err = tracker.doRateLimitedReq(ctx, func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, "http://u:"+pass+"@"+addr+"/init?secret="+pass, http.NoBody)
	})
	require.Error(t, err)
	assert.Contains(t, buf.String(), "Studio request failed")
	assert.NotContains(t, buf.String(), pass)
	assert.NotContains(t, err.Error(), pass)
}
