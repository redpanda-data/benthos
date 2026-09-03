// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"context"
	"reflect"
	"sync"
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

type purgeTrackingMetrics struct {
	metrics.DudType

	onPurge func()

	mut    sync.Mutex
	purged []map[string]string
}

func (p *purgeTrackingMetrics) DeleteSeriesPartialMatch(labels map[string]string) {
	if p.onPurge != nil {
		p.onPurge()
	}
	p.mut.Lock()
	defer p.mut.Unlock()
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	p.purged = append(p.purged, cp)
}

func (p *purgeTrackingMetrics) getPurged() []map[string]string {
	p.mut.Lock()
	defer p.mut.Unlock()
	return append([]map[string]string(nil), p.purged...)
}

func TestTypeDeletePurgesMetricSeries(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	stats := &purgeTrackingMetrics{}
	res, err := bmanager.New(bmanager.NewResourceConfig(), bmanager.OptSetMetrics(metrics.NewNamespaced(stats)))
	require.NoError(t, err)

	mgr := New(res)

	require.NoError(t, mgr.Create("foo", harmlessConf(t)))
	require.Empty(t, stats.getPurged())

	require.NoError(t, mgr.Delete(ctx, "foo"))
	require.Equal(t, []map[string]string{{"stream": "foo"}}, stats.getPurged())

	require.ErrorIs(t, mgr.Delete(ctx, "bar"), ErrStreamDoesNotExist)
	require.Equal(t, []map[string]string{{"stream": "foo"}}, stats.getPurged())
}

// A Create that reuses a just-deleted id must not begin emitting series until
// Delete's purge has completed, otherwise the purge wipes the new stream's
// series: components cache their metric children, so for an exporter like
// prometheus the live stream would stay invisible for its whole lifetime.
func TestTypeDeletePurgeBlocksSameIDCreate(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	purgeEntered := make(chan struct{})
	purgeRelease := make(chan struct{})
	stats := &purgeTrackingMetrics{onPurge: func() {
		close(purgeEntered)
		<-purgeRelease
	}}
	res, err := bmanager.New(bmanager.NewResourceConfig(), bmanager.OptSetMetrics(metrics.NewNamespaced(stats)))
	require.NoError(t, err)

	mgr := New(res)
	require.NoError(t, mgr.Create("foo", harmlessConf(t)))

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- mgr.Delete(ctx, "foo")
	}()

	select {
	case <-purgeEntered:
	case <-ctx.Done():
		t.Fatal("delete never reached the purge")
	}

	createDone := make(chan error, 1)
	go func() {
		createDone <- mgr.Create("foo", harmlessConf(t))
	}()

	select {
	case err := <-createDone:
		t.Fatalf("create for the same id completed while the purge was still in flight (err: %v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(purgeRelease)
	require.NoError(t, <-deleteDone)
	require.NoError(t, <-createDone)
	require.NoError(t, mgr.Stop(ctx))
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
