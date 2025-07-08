// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
)

func TestMetricsNil(t *testing.T) {
	var m *Metrics

	m.NewCounter("foo").Incr(1)
	m.NewGauge("bar").Set(10)
	m.NewTimer("baz").Timing(10)
}

func TestMetricsNoLabels(t *testing.T) {
	stats := metrics.NewLocal()
	nm := newReverseAirGapMetrics(stats)

	ctr := nm.NewCounter("counterone")
	ctr.Incr(10)
	ctr.Incr(11)

	gge := nm.NewGauge("gaugeone")
	gge.Set(12)

	tmr := nm.NewTimer("timerone")
	tmr.Timing(13)

	assert.Equal(t, map[string]int64{
		"counterone": 21,
		"gaugeone":   12,
	}, stats.GetCounters())

	assert.Equal(t, int64(13), stats.GetTimings()["timerone"].Max())
}

func TestMetricsWithLabels(t *testing.T) {
	stats := metrics.NewLocal()
	nm := newReverseAirGapMetrics(stats)

	ctr := nm.NewCounter("countertwo", "label1")
	ctr.Incr(10, "value1")
	ctr.Incr(11, "value2")

	gge := nm.NewGauge("gaugetwo", "label2")
	gge.Set(12, "value3")

	tmr := nm.NewTimer("timertwo", "label3", "label4")
	tmr.Timing(13, "value4", "value5")

	assert.Equal(t, map[string]int64{
		`countertwo{label1="value1"}`: 10,
		`countertwo{label1="value2"}`: 11,
		`gaugetwo{label2="value3"}`:   12,
	}, stats.GetCounters())

	assert.Equal(t, int64(13), stats.GetTimings()[`timertwo{label3="value4",label4="value5"}`].Max())
}

//------------------------------------------------------------------------------

type mockMetricsExporter struct {
	testField string
	values    map[string]int64
	lock      *sync.Mutex
}

type mockMetricsExporterType struct {
	name   string
	values map[string]int64
	lock   *sync.Mutex
}

func (m *mockMetricsExporterType) IncrFloat64(count float64) {
	m.lock.Lock()
	m.values[m.name] += int64(count)
	m.lock.Unlock()
}

func (m *mockMetricsExporterType) Incr(count int64) {
	m.lock.Lock()
	m.values[m.name] += count
	m.lock.Unlock()
}

func (m *mockMetricsExporterType) Timing(delta int64) {
	m.lock.Lock()
	m.values[m.name] = delta
	m.lock.Unlock()
}

func (m *mockMetricsExporterType) Set(value int64) {
	m.lock.Lock()
	m.values[m.name] = value
	m.lock.Unlock()
}

func (m *mockMetricsExporterType) SetFloat64(value float64) {
	m.Set(int64(value))
}

func (m *mockMetricsExporter) NewCounterCtor(name string, labelKeys ...string) MetricsExporterCounterCtor {
	return func(labelValues ...string) MetricsExporterCounter {
		return &mockMetricsExporterType{
			name:   fmt.Sprintf("counter:%v:%v:%v", name, labelKeys, labelValues),
			values: m.values,
			lock:   m.lock,
		}
	}
}

func (m *mockMetricsExporter) NewTimerCtor(name string, labelKeys ...string) MetricsExporterTimerCtor {
	return func(labelValues ...string) MetricsExporterTimer {
		return &mockMetricsExporterType{
			name:   fmt.Sprintf("timer:%v:%v:%v", name, labelKeys, labelValues),
			values: m.values,
			lock:   m.lock,
		}
	}
}

func (m *mockMetricsExporter) NewGaugeCtor(name string, labelKeys ...string) MetricsExporterGaugeCtor {
	return func(labelValues ...string) MetricsExporterGauge {
		return &mockMetricsExporterType{
			name:   fmt.Sprintf("gauge:%v:%v:%v", name, labelKeys, labelValues),
			values: m.values,
			lock:   m.lock,
		}
	}
}

func (m *mockMetricsExporter) Close(ctx context.Context) error {
	return nil
}

func TestMetricsPlugin(t *testing.T) {
	testMetrics := &mockMetricsExporter{
		values: map[string]int64{},
		lock:   &sync.Mutex{},
	}

	env := NewEnvironment()
	confSpec := NewConfigSpec().Field(NewStringField("foo"))

	require.NoError(t, env.RegisterMetricsExporter(
		"meow", confSpec,
		func(conf *ParsedConfig, log *Logger) (MetricsExporter, error) {
			testStr, err := conf.FieldString("foo")
			if err != nil {
				return nil, err
			}
			testMetrics.testField = testStr
			return testMetrics, nil
		}))

	builder := env.NewStreamBuilder()
	require.NoError(t, builder.SetYAML(`
input:
  label: fooinput
  generate:
    count: 2
    interval: 1ns
    mapping: 'root.id = uuid_v4()'

pipeline:
  processors:
    - metric:
        name: customthing
        type: gauge
        labels:
          topic: testtopic
        value: 1234

output:
  label: foooutput
  drop: {}

metrics:
  meow:
    foo: foo value from config

logger:
  level: none
`))

	strm, err := builder.Build()
	require.NoError(t, err)

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	require.NoError(t, strm.Run(ctx))

	testMetrics.lock.Lock()
	assert.Equal(t, "foo value from config", testMetrics.testField)

	assert.Greater(t, testMetrics.values["timer:input_latency_ns:[label path]:[fooinput root.input]"], int64(1))
	delete(testMetrics.values, "timer:input_latency_ns:[label path]:[fooinput root.input]")

	assert.GreaterOrEqual(t, testMetrics.values["timer:output_latency_ns:[label path]:[foooutput root.output]"], int64(1))
	delete(testMetrics.values, "timer:output_latency_ns:[label path]:[foooutput root.output]")

	assert.Equal(t, map[string]int64{
		"counter:input_connection_up:[label path]:[fooinput root.input]":               1,
		"counter:input_received:[label path]:[fooinput root.input]":                    2,
		"counter:output_batch_sent:[label path]:[foooutput root.output]":               2,
		"counter:output_connection_up:[label path]:[foooutput root.output]":            1,
		"counter:output_sent:[label path]:[foooutput root.output]":                     2,
		"gauge:customthing:[label path topic]:[ root.pipeline.processors.0 testtopic]": 1234,
	}, testMetrics.values)
	testMetrics.lock.Unlock()
}

func TestMetricsGaugeIncrDecrInt64(t *testing.T) {
	tests := []struct {
		name      string
		initial   int64
		incrValue int64
		decrValue int64
		expected  int64
	}{
		{
			name:      "increment and decrement",
			initial:   10,
			incrValue: 5,
			decrValue: 2,
			expected:  13,
		},
		{
			name:      "only increment",
			initial:   0,
			incrValue: 15,
			decrValue: 0,
			expected:  15,
		},
		{
			name:      "only decrement",
			initial:   20,
			incrValue: 0,
			decrValue: 7,
			expected:  13,
		},
		{
			name:      "large float values",
			initial:   100,
			incrValue: 999,
			decrValue: 500,
			expected:  599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := metrics.NewLocal()
			nm := newReverseAirGapMetrics(stats)

			gge := nm.NewGauge("test_gauge")
			if tt.initial != 0 {
				gge.Set(tt.initial)
			}

			if tt.incrValue != 0 {
				gge.Incr(tt.incrValue)
			}
			if tt.decrValue != 0 {
				gge.Decr(tt.decrValue)
			}

			assert.Equal(t, tt.expected, stats.GetCounters()["test_gauge"])
		})
	}
}

func TestMetricsGaugeIncrDecrInt64WithLabels(t *testing.T) {
	tests := []struct {
		name              string
		initial           int64
		incrValue         int64
		decrValue         int64
		expected          int64
		labelKeys         []string
		labelValues       []string
		expectedGaugeName string
	}{
		{
			name:              "increment and decrement",
			initial:           10,
			incrValue:         5,
			decrValue:         2,
			expected:          13,
			labelKeys:         []string{"label1"},
			labelValues:       []string{"value1"},
			expectedGaugeName: "test_gauge{label1=\"value1\"}",
		},
		{
			name:              "incorrect number of values for label",
			initial:           0,
			incrValue:         15,
			decrValue:         0,
			expected:          15,
			labelKeys:         []string{"label1"},
			labelValues:       []string{"value1", "value2"},
			expectedGaugeName: "test_gauge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := metrics.NewLocal()
			nm := newReverseAirGapMetrics(stats)

			gge := nm.NewGauge("test_gauge", tt.labelKeys...)
			if tt.initial != 0 {
				gge.Set(tt.initial, tt.labelValues...)
			}

			if tt.incrValue != 0 {
				gge.Incr(tt.incrValue, tt.labelValues...)
			}
			if tt.decrValue != 0 {
				gge.Decr(tt.decrValue, tt.labelValues...)
			}
			assert.Equal(t, tt.expected, stats.GetCounters()[tt.expectedGaugeName])
		})
	}
}

func TestMetricsGaugeIncrDecrFloat64(t *testing.T) {
	tests := []struct {
		name      string
		initial   int64
		incrValue float64
		decrValue float64
		expected  int64
	}{
		{
			name:      "increment and decrement",
			initial:   10,
			incrValue: 5.7,
			decrValue: 2.3,
			expected:  13,
		},
		{
			name:      "only increment",
			initial:   0,
			incrValue: 15.9,
			decrValue: 0,
			expected:  15,
		},
		{
			name:      "only decrement",
			initial:   20,
			incrValue: 0,
			decrValue: 7.8,
			expected:  13,
		},
		{
			name:      "large float values",
			initial:   100,
			incrValue: 999.99,
			decrValue: 500.51,
			expected:  599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := metrics.NewLocal()
			nm := newReverseAirGapMetrics(stats)

			gge := nm.NewGauge("test_gauge")
			if tt.initial != 0 {
				gge.Set(tt.initial)
			}

			if tt.incrValue != 0 {
				gge.IncrFloat64(tt.incrValue)
			}
			if tt.decrValue != 0 {
				gge.DecrFloat64(tt.decrValue)
			}

			assert.Equal(t, tt.expected, stats.GetCounters()["test_gauge"])
		})
	}
}

func TestMetricsGaugeSetFloat64(t *testing.T) {
	tests := []struct {
		name     string
		setValue float64
		expected int64
	}{
		{
			name:     "positive float",
			setValue: 42.7,
			expected: 42,
		},
		{
			name:     "negative float",
			setValue: -15.3,
			expected: -15,
		},
		{
			name:     "zero",
			setValue: 0.0,
			expected: 0,
		},
		{
			name:     "large float",
			setValue: 999999.99,
			expected: 999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := metrics.NewLocal()
			nm := newReverseAirGapMetrics(stats)

			gge := nm.NewGauge("test_gauge")
			gge.SetFloat64(tt.setValue)

			assert.Equal(t, tt.expected, stats.GetCounters()["test_gauge"])
		})
	}
}
