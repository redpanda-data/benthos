// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	bmanager "github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/stream"
)

func harmlessConf(t testing.TB) stream.Config {
	t.Helper()

	c, err := testutil.StreamFromYAML(`
input:
  generate:
    mapping: 'root = deleted()'
output:
  drop: {}
`)
	require.NoError(t, err)

	return c
}

func metricConf(t testing.TB, name string) stream.Config {
	t.Helper()

	c, err := testutil.StreamFromYAML(fmt.Sprintf(`
input:
  generate:
    mapping: 'root = deleted()'
pipeline:
  processors:
    - metric:
        type: counter
        name: %s
output:
  drop: {}
`, name))
	require.NoError(t, err)
	return c
}

func metricsManager(t testing.TB) (*Type, *metrics.Local) {
	t.Helper()

	stats := metrics.NewLocal()
	res, err := bmanager.New(
		bmanager.NewResourceConfig(),
		bmanager.OptSetMetrics(metrics.NewNamespaced(stats)),
	)
	require.NoError(t, err)
	return New(res), stats
}

func hasStreamMetric(stats *metrics.Local, streamID, name string) bool {
	matches := func(path string) bool {
		return strings.Contains(path, `stream="`+streamID+`"`) &&
			(name == "" || strings.Contains(path, name))
	}
	for path := range stats.GetCounters() {
		if matches(path) {
			return true
		}
	}
	for path := range stats.GetTimings() {
		if matches(path) {
			return true
		}
	}
	return false
}

func TestTypeBasicOperations(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := New(res)

	if err := mgr.Update(ctx, "foo", harmlessConf(t)); err == nil {
		t.Error("Expected error on empty update")
	}
	if _, err := mgr.Read("foo"); err == nil {
		t.Error("Expected error on empty read")
	}

	if err := mgr.Create("foo", harmlessConf(t)); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Create("foo", harmlessConf(t)); err == nil {
		t.Error("Expected error on duplicate create")
	}

	if info, err := mgr.Read("foo"); err != nil {
		t.Error(err)
	} else if !info.IsRunning() {
		t.Error("Stream not active")
	} else if act, exp := info.Config(), harmlessConf(t); !reflect.DeepEqual(act, exp) {
		t.Errorf("Unexpected config: %v != %v", act, exp)
	}

	newConf := harmlessConf(t)
	newConf.Buffer.Type = "memory"

	if err := mgr.Update(ctx, "foo", newConf); err != nil {
		t.Error(err)
	}

	if info, err := mgr.Read("foo"); err != nil {
		t.Error(err)
	} else if !info.IsRunning() {
		t.Error("Stream not active")
	} else if act, exp := info.Config(), newConf; !reflect.DeepEqual(act, exp) {
		t.Errorf("Unexpected config: %v != %v", act, exp)
	}

	if err := mgr.Delete(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Delete(ctx, "foo"); err == nil {
		t.Error("Expected error on duplicate delete")
	}

	if err := mgr.Stop(ctx); err != nil {
		t.Error(err)
	}

	if exp, act := component.ErrTypeClosed, mgr.Create("foo", harmlessConf(t)); act != exp {
		t.Errorf("Unexpected error: %v != %v", act, exp)
	}
}

func TestTypeBasicClose(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := New(res)

	conf := harmlessConf(t)
	if err := mgr.Create("foo", conf); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Stop(ctx); err != nil {
		t.Error(err)
	}

	if exp, act := component.ErrTypeClosed, mgr.Create("foo", harmlessConf(t)); act != exp {
		t.Errorf("Unexpected error: %v != %v", act, exp)
	}
}

func TestTypeDeleteRemovesStreamMetrics(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mgr, stats := metricsManager(t)
	require.NoError(t, mgr.Create("foo", metricConf(t, "stream_test_metric")))
	require.True(t, hasStreamMetric(stats, "foo", ""))

	require.NoError(t, mgr.Delete(ctx, "foo"))
	require.False(t, hasStreamMetric(stats, "foo", ""))
}

func TestTypeRecreateStreamMetrics(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mgr, stats := metricsManager(t)
	conf := metricConf(t, "stream_recreate_metric")

	require.NoError(t, mgr.Create("foo", conf))
	require.True(t, hasStreamMetric(stats, "foo", "stream_recreate_metric"))
	require.NoError(t, mgr.Delete(ctx, "foo"))
	require.False(t, hasStreamMetric(stats, "foo", ""))

	require.NoError(t, mgr.Create("foo", conf))
	require.True(t, hasStreamMetric(stats, "foo", "stream_recreate_metric"))
	require.NoError(t, mgr.Delete(ctx, "foo"))
	require.False(t, hasStreamMetric(stats, "foo", ""))
}

func TestTypeUpdateReplacesStreamMetrics(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mgr, stats := metricsManager(t)
	require.NoError(t, mgr.Create("foo", metricConf(t, "stream_old_metric")))
	require.True(t, hasStreamMetric(stats, "foo", "stream_old_metric"))

	require.NoError(t, mgr.Update(ctx, "foo", metricConf(t, "stream_new_metric")))
	require.False(t, hasStreamMetric(stats, "foo", "stream_old_metric"))
	require.True(t, hasStreamMetric(stats, "foo", "stream_new_metric"))
}

func TestTypeStopRemovesStreamMetrics(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	mgr, stats := metricsManager(t)
	require.NoError(t, mgr.Create("foo", metricConf(t, "stream_foo_metric")))
	require.NoError(t, mgr.Create("bar", metricConf(t, "stream_bar_metric")))
	require.True(t, hasStreamMetric(stats, "foo", ""))
	require.True(t, hasStreamMetric(stats, "bar", ""))

	require.NoError(t, mgr.Stop(ctx))
	require.False(t, hasStreamMetric(stats, "foo", ""))
	require.False(t, hasStreamMetric(stats, "bar", ""))
}
