// Copyright 2026 Redpanda Data, Inc.

package manager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	bmanager "github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/tracing/tracingtest"
)

func TestStreamSpansCarryTheStreamAttribute(t *testing.T) {
	conf, err := testutil.StreamFromYAML(`
input:
  generate:
    count: 1
    interval: 1ms
    mapping: 'root.meow = "woof"'
pipeline:
  processors:
    - mapping: 'root = this'
output:
  drop: {}
`)
	require.NoError(t, err)

	tp := tracingtest.NewInMemoryRecordingTracerProvider()

	res, err := bmanager.New(bmanager.NewResourceConfig(), bmanager.OptSetTracer(tp))
	require.NoError(t, err)

	mgr := New(res)
	require.NoError(t, mgr.Create("foo", conf))
	t.Cleanup(func() {
		_ = mgr.Stop(t.Context())
	})

	assert.Eventually(t, func() bool {
		return tp.FindSpan("output_drop") != nil
	}, time.Second*30, time.Millisecond*10)

	spans := tp.Spans()
	require.NotEmpty(t, spans)
	for _, span := range spans {
		assert.Equal(t, "foo", span.GetAttribute("stream"), "span %v", span.Name)
	}
}
