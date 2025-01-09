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
