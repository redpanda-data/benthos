// Copyright 2025 Redpanda Data, Inc.

package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
)

func getTestMetrics(t *testing.T) (metrics.Type, http.HandlerFunc) {
	t.Helper()

	conf := metrics.NewConfig()
	conf.Type = "json_api"

	ns, err := bundle.AllMetrics.Init(conf, mock.NewManager())
	require.NoError(t, err)

	prom := ns.Child()
	return prom, prom.HandlerFunc()
}

func getPage(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/foo", http.NoBody)
	w := httptest.NewRecorder()
	handler(w, req)

	body, err := io.ReadAll(w.Result().Body)
	require.NoError(t, err)

	return string(body)
}

func TestNamespacedNothing(t *testing.T) {
	prom, handler := getTestMetrics(t)

	nm := metrics.NewNamespaced(prom)

	ctr := nm.GetCounter("counterone")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.GetGauge("gaugeone")
	gge.Set(12)

	tmr := nm.GetTimer("timerone")
	tmr.Timing(13)

	ctrTwo := nm.GetCounterVec("countertwo", "label1")
	ctrTwo.With("value1").Incr(10)
	ctrTwo.With("value2").Incr(11)
	ctrTwo.With("value3").IncrFloat64(10.452)

	ggeTwo := nm.GetGaugeVec("gaugetwo", "label2")
	ggeTwo.With("value3").Set(12)

	tmrTwo := nm.GetTimerVec("timertwo", "label3", "label4")
	tmrTwo.With("value4", "value5").Timing(13)

	body := getPage(t, handler)

	assert.Contains(t, body, `"counterone":21`)
	assert.Contains(t, body, `"gaugeone":12`)
	assert.Contains(t, body, `"timerone":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"countertwo{label1=\"value1\"}":10`)
	assert.Contains(t, body, `"countertwo{label1=\"value2\"}":11`)
	assert.Contains(t, body, `"countertwo{label1=\"value3\"}":10`)
	assert.Contains(t, body, `"gaugetwo{label2=\"value3\"}":12`)
	assert.Contains(t, body, `"timertwo{label3=\"value4\",label4=\"value5\"}":{"p50":13,"p90":13,"p99":13}`)
}

func TestNamespacedPrefix(t *testing.T) {
	prom, handler := getTestMetrics(t)

	nm := metrics.NewNamespaced(prom)

	ctr := nm.GetCounter("counterone")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.GetGauge("gaugeone")
	gge.Set(12)

	tmr := nm.GetTimer("timerone")
	tmr.Timing(13)

	ctrTwo := nm.GetCounterVec("countertwo", "label1")
	ctrTwo.With("value1").Incr(10)
	ctrTwo.With("value2").Incr(11)

	ggeTwo := nm.GetGaugeVec("gaugetwo", "label2")
	ggeTwo.With("value3").Set(12)

	tmrTwo := nm.GetTimerVec("timertwo", "label3", "label4")
	tmrTwo.With("value4", "value5").Timing(13)

	ctrThree := nm.GetCounter("counterthree")
	ctrThree.Incr(22)

	body := getPage(t, handler)

	assert.Contains(t, body, `"counterone":21`)
	assert.Contains(t, body, `"gaugeone":12`)
	assert.Contains(t, body, `"timerone":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"countertwo{label1=\"value1\"}":10`)
	assert.Contains(t, body, `"countertwo{label1=\"value2\"}":11`)
	assert.Contains(t, body, `"gaugetwo{label2=\"value3\"}":12`)
	assert.Contains(t, body, `"timertwo{label3=\"value4\",label4=\"value5\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"counterthree":22`)
}

func TestNamespacedPrefixStaticLabels(t *testing.T) {
	prom, handler := getTestMetrics(t)

	nm := metrics.NewNamespaced(prom).WithLabels("static1", "svalue1")

	ctr := nm.GetCounter("counterone")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.GetGauge("gaugeone")
	gge.Set(12)

	tmr := nm.GetTimer("timerone")
	tmr.Timing(13)

	ctrTwo := nm.GetCounterVec("countertwo", "label1")
	ctrTwo.With("value1").Incr(10)
	ctrTwo.With("value2").Incr(11)

	ggeTwo := nm.GetGaugeVec("gaugetwo", "label2")
	ggeTwo.With("value3").Set(12)

	tmrTwo := nm.GetTimerVec("timertwo", "label3", "label4")
	tmrTwo.With("value4", "value5").Timing(13)

	nm2 := nm.WithLabels("static2", "svalue2")

	ctrThree := nm2.GetCounter("counterthree")
	ctrThree.Incr(22)

	body := getPage(t, handler)

	assert.Contains(t, body, `"counterone{static1=\"svalue1\"}":21`)
	assert.Contains(t, body, `"gaugeone{static1=\"svalue1\"}":12`)
	assert.Contains(t, body, `"timerone{static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"countertwo{label1=\"value1\",static1=\"svalue1\"}":10`)
	assert.Contains(t, body, `"countertwo{label1=\"value2\",static1=\"svalue1\"}":11`)
	assert.Contains(t, body, `"gaugetwo{label2=\"value3\",static1=\"svalue1\"}":12`)
	assert.Contains(t, body, `"timertwo{label3=\"value4\",label4=\"value5\",static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"counterthree{static1=\"svalue1\",static2=\"svalue2\"}":22`)
}

func TestNamespacedPrefixStaticLabelsWithMappings(t *testing.T) {
	prom, handler := getTestMetrics(t)

	mappingFooToBar, err := metrics.NewMapping(`root = this.replace_all("foo","bar")`, log.Noop())
	require.NoError(t, err)

	mappingBarToBaz, err := metrics.NewMapping(`root = this.replace_all("bar","baz")`, log.Noop())
	require.NoError(t, err)

	nm := metrics.NewNamespaced(prom).WithLabels("static1", "svalue1")
	nm = nm.WithMapping(mappingBarToBaz)
	nm = nm.WithMapping(mappingFooToBar)

	ctr := nm.GetCounter("counter")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.GetGauge("gauge")
	gge.Set(12)

	tmr := nm.GetTimer("timer")
	tmr.Timing(13)

	ctrTwo := nm.GetCounterVec("countertwo", "label1")
	ctrTwo.With("value1").Incr(10)
	ctrTwo.With("value2").Incr(11)

	ggeTwo := nm.GetGaugeVec("gaugetwo", "label2")
	ggeTwo.With("value3").Set(12)

	tmrTwo := nm.GetTimerVec("timertwo", "label3", "label4")
	tmrTwo.With("value4", "value5").Timing(13)

	body := getPage(t, handler)

	assert.Contains(t, body, `"counter{static1=\"svalue1\"}":21`)
	assert.Contains(t, body, `"gauge{static1=\"svalue1\"}":12`)
	assert.Contains(t, body, `"timer{static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"countertwo{label1=\"value1\",static1=\"svalue1\"}":10`)
	assert.Contains(t, body, `"countertwo{label1=\"value2\",static1=\"svalue1\"}":11`)
	assert.Contains(t, body, `"gaugetwo{label2=\"value3\",static1=\"svalue1\"}":12`)
	assert.Contains(t, body, `"timertwo{label3=\"value4\",label4=\"value5\",static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
}

func TestNamespacedPrefixStaticLabelsWithMappingLabels(t *testing.T) {
	prom, handler := getTestMetrics(t)

	mappingFooToBar, err := metrics.NewMapping(`meta = meta().map_each(kv -> kv.value.replace_all("value","bar"))
meta extra1 = "extravalue1"
root = this.replace_all("foo","bar")`, log.Noop())
	require.NoError(t, err)

	mappingBarToBaz, err := metrics.NewMapping(`meta = meta().map_each(kv -> kv.value.replace_all("bar","baz"))
meta extra2 = "extravalue2"
root = this.replace_all("bar","baz")`, log.Noop())
	require.NoError(t, err)

	nm := metrics.NewNamespaced(prom).WithLabels("static1", "svalue1")
	nm = nm.WithMapping(mappingBarToBaz)
	nm = nm.WithMapping(mappingFooToBar)

	ctr := nm.GetCounter("counter")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.GetGauge("gauge")
	gge.Set(12)

	tmr := nm.GetTimer("timer")
	tmr.Timing(13)

	ctrTwo := nm.GetCounterVec("countertwo", "label1")
	ctrTwo.With("value1").Incr(10)
	ctrTwo.With("value2").Incr(11)

	ggeTwo := nm.GetGaugeVec("gaugetwo", "label2")
	ggeTwo.With("value3").Set(12)

	tmrTwo := nm.GetTimerVec("timertwo", "label3", "label4")
	tmrTwo.With("value4", "value5").Timing(13)

	body := getPage(t, handler)

	assert.Contains(t, body, `"counter{extra1=\"extravalue1\",extra2=\"extravalue2\",static1=\"sbaz1\"}":21`)
	assert.Contains(t, body, `"gauge{extra1=\"extravalue1\",extra2=\"extravalue2\",static1=\"sbaz1\"}":12`)
	assert.Contains(t, body, `"timer{extra1=\"extravalue1\",extra2=\"extravalue2\",static1=\"sbaz1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"countertwo{extra1=\"extravalue1\",extra2=\"extravalue2\",label1=\"value1\",static1=\"sbaz1\"}":10`)
	assert.Contains(t, body, `"countertwo{extra1=\"extravalue1\",extra2=\"extravalue2\",label1=\"value2\",static1=\"sbaz1\"}":11`)
	assert.Contains(t, body, `"gaugetwo{extra1=\"extravalue1\",extra2=\"extravalue2\",label2=\"value3\",static1=\"sbaz1\"}":12`)
	assert.Contains(t, body, `"timertwo{extra1=\"extravalue1\",extra2=\"extravalue2\",label3=\"value4\",label4=\"value5\",static1=\"sbaz1\"}":{"p50":13,"p90":13,"p99":13}`)
}

type purgeTrackingDud struct {
	metrics.DudType

	purged []map[string]string
}

func (p *purgeTrackingDud) DeleteSeriesPartialMatch(labels map[string]string) {
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	p.purged = append(p.purged, cp)
}

func TestNamespacedDeleteSeriesPartialMatch(t *testing.T) {
	child := &purgeTrackingDud{}
	nm := metrics.NewNamespaced(child).WithLabels("stream", "foo")

	purger, ok := any(nm).(interface {
		DeleteSeriesPartialMatch(labels map[string]string)
	})
	require.True(t, ok, "Namespaced should expose series deletion")

	purger.DeleteSeriesPartialMatch(map[string]string{"stream": "foo"})
	assert.Equal(t, []map[string]string{{"stream": "foo"}}, child.purged)
}

func TestNamespacedDeleteSeriesPartialMatchNoopChild(t *testing.T) {
	nm := metrics.Noop()

	purger, ok := any(nm).(interface {
		DeleteSeriesPartialMatch(labels map[string]string)
	})
	require.True(t, ok, "Namespaced should expose series deletion")

	assert.NotPanics(t, func() {
		purger.DeleteSeriesPartialMatch(map[string]string{"stream": "foo"})
	})
}

func TestNamespacedRawTee(t *testing.T) {
	prom, handler := getTestMetrics(t)
	tee, teeHandler := getTestMetrics(t)

	mapping, err := metrics.NewMapping(`root = if this == "dropped" { deleted() } else { this.replace_all("foo", "bar") }`, log.Noop())
	require.NoError(t, err)

	nm := metrics.NewNamespaced(prom).WithRawTee(tee).WithMapping(mapping).WithLabels("static1", "svalue1")

	nm.GetCounter("foocounter").Incr(10)
	nm.GetCounter("dropped").Incr(11)
	nm.GetGauge("foogauge").Set(12)
	nm.GetTimer("footimer").Timing(13)
	nm.GetCounterVec("foocountertwo", "label1").With("value1").Incr(14)
	nm.GetGaugeVec("foogaugetwo", "label2").With("value2").Set(15)
	nm.GetTimerVec("footimertwo", "label3").With("value3").Timing(16)

	// The configured exporter sees mapped names and loses the deleted metric.
	body := getPage(t, handler)
	assert.Contains(t, body, `"barcounter{static1=\"svalue1\"}":10`)
	assert.NotContains(t, body, "dropped")
	assert.Contains(t, body, `"bargauge{static1=\"svalue1\"}":12`)
	assert.Contains(t, body, `"bartimer{static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, body, `"barcountertwo{label1=\"value1\",static1=\"svalue1\"}":14`)
	assert.Contains(t, body, `"bargaugetwo{label2=\"value2\",static1=\"svalue1\"}":15`)
	assert.Contains(t, body, `"bartimertwo{label3=\"value3\",static1=\"svalue1\"}":{"p50":16,"p90":16,"p99":16}`)

	// The tee sees original names with the static labels, including the
	// metric the mapping deleted.
	teeBody := getPage(t, teeHandler)
	assert.Contains(t, teeBody, `"foocounter{static1=\"svalue1\"}":10`)
	assert.Contains(t, teeBody, `"dropped{static1=\"svalue1\"}":11`)
	assert.Contains(t, teeBody, `"foogauge{static1=\"svalue1\"}":12`)
	assert.Contains(t, teeBody, `"footimer{static1=\"svalue1\"}":{"p50":13,"p90":13,"p99":13}`)
	assert.Contains(t, teeBody, `"foocountertwo{label1=\"value1\",static1=\"svalue1\"}":14`)
	assert.Contains(t, teeBody, `"foogaugetwo{label2=\"value2\",static1=\"svalue1\"}":15`)
	assert.Contains(t, teeBody, `"footimertwo{label3=\"value3\",static1=\"svalue1\"}":{"p50":16,"p90":16,"p99":16}`)
	assert.NotContains(t, teeBody, "bar")

	// Close does not reach the tee.
	require.NoError(t, nm.Close())
	nm.GetCounter("afterclose").Incr(1)
	assert.Contains(t, getPage(t, teeHandler), `"afterclose{static1=\"svalue1\"}":1`)
}

func TestNamespacedRawTeeRemovedByNil(t *testing.T) {
	prom, handler := getTestMetrics(t)
	tee, teeHandler := getTestMetrics(t)

	nm := metrics.NewNamespaced(prom).WithRawTee(tee).WithRawTee(nil)
	nm.GetCounter("counter").Incr(10)

	assert.Contains(t, getPage(t, handler), `"counter":10`)
	assert.NotContains(t, getPage(t, teeHandler), "counter")
}

func TestNamespacedRawTeeSurvivesWithStats(t *testing.T) {
	first, firstHandler := getTestMetrics(t)
	second, secondHandler := getTestMetrics(t)
	tee, teeHandler := getTestMetrics(t)

	nm := metrics.NewNamespaced(first).WithRawTee(tee).WithStats(second)
	nm.GetCounter("counter").Incr(10)

	assert.NotContains(t, getPage(t, firstHandler), "counter")
	assert.Contains(t, getPage(t, secondHandler), `"counter":10`)
	assert.Contains(t, getPage(t, teeHandler), `"counter":10`)
}

func TestNamespacedRawTeeDeleteSeriesPartialMatch(t *testing.T) {
	child := &purgeTrackingDud{}
	tee := &purgeTrackingDud{}
	nm := metrics.NewNamespaced(child).WithRawTee(tee).WithLabels("stream", "foo")

	purger, ok := any(nm).(interface {
		DeleteSeriesPartialMatch(labels map[string]string)
	})
	require.True(t, ok, "Namespaced should expose series deletion")

	purger.DeleteSeriesPartialMatch(map[string]string{"stream": "foo"})
	assert.Equal(t, []map[string]string{{"stream": "foo"}}, child.purged)
	assert.Equal(t, []map[string]string{{"stream": "foo"}}, tee.purged)
}
