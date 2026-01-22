// Copyright 2025 Redpanda Data, Inc.

package io_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/tracing/tracingtest"
)

func parseYAMLProcConf(t testing.TB, formatStr string, args ...any) (conf processor.Config) {
	t.Helper()
	var err error
	conf, err = testutil.ProcessorFromYAML(fmt.Sprintf(formatStr, args...))
	require.NoError(t, err)
	return
}

func TestHTTPClientRetries(t *testing.T) {
	var reqCount uint32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint32(&reqCount, 1)
		http.Error(w, "test error", http.StatusForbidden)
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  retry_period: 1ms
  retries: 3
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("test")}))
	if res != nil {
		t.Fatal(res)
	}
	if len(msgs) != 1 {
		t.Fatal("Wrong count of error messages")
	}
	if msgs[0].Len() != 1 {
		t.Fatal("Wrong count of error message parts")
	}
	if exp, act := "test", string(msgs[0].Get(0).AsBytes()); exp != act {
		t.Errorf("Wrong message contents: %v != %v", act, exp)
	}
	assert.Error(t, msgs[0].Get(0).ErrorGet())
	if exp, act := "403", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}

	if exp, act := uint32(4), atomic.LoadUint32(&reqCount); exp != act {
		t.Errorf("Wrong count of HTTP attempts: %v != %v", exp, act)
	}
}

func TestHTTPClientBasic(t *testing.T) {
	i := 0
	expPayloads := []string{"foo", "bar", "baz"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if exp, act := expPayloads[i], string(reqBytes); exp != act {
			t.Errorf("Wrong payload value: %v != %v", act, exp)
		}
		i++
		w.Header().Add("foobar", "baz")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("foobar"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  retry_period: 1ms
  retries: 3
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foobar", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "201", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	} else if exp, act := "", msgs[0].Get(0).MetaGetStr("foobar"); exp != act {
		t.Errorf("Wrong metadata value: %v != %v", act, exp)
	}

	msgs, res = h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("bar")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foobar", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "201", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	} else if exp, act := "", msgs[0].Get(0).MetaGetStr("foobar"); exp != act {
		t.Errorf("Wrong metadata value: %v != %v", act, exp)
	}

	// Check metadata persists.
	msg := message.QuickBatch([][]byte{[]byte("baz")})
	msg.Get(0).MetaSetMut("foo", "bar")
	msgs, res = h.ProcessBatch(t.Context(), msg)
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foobar", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "bar", msgs[0].Get(0).MetaGetStr("foo"); exp != act {
		t.Errorf("Metadata not preserved: %v != %v", act, exp)
	} else if exp, act := "201", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	} else if exp, act := "", msgs[0].Get(0).MetaGetStr("foobar"); exp != act {
		t.Errorf("Wrong metadata value: %v != %v", act, exp)
	}
}

func TestHTTPClientEmptyResponse(t *testing.T) {
	i := 0
	expPayloads := []string{"foo", "bar", "baz"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if exp, act := expPayloads[i], string(reqBytes); exp != act {
			t.Errorf("Wrong payload value: %v != %v", act, exp)
		}
		i++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "200", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}

	msgs, res = h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("bar")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "200", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}

	// Check metadata persists.
	msg := message.QuickBatch([][]byte{[]byte("baz")})
	msg.Get(0).MetaSetMut("foo", "bar")
	msgs, res = h.ProcessBatch(t.Context(), msg)
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "200", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}
}

func TestHTTPClientEmpty404Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foo", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "404", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	} else {
		assert.Error(t, msgs[0].Get(0).ErrorGet())
	}
}

func TestHTTPClientBasicWithMetadata(t *testing.T) {
	i := 0
	expPayloads := []string{"foo", "bar", "baz"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if exp, act := expPayloads[i], string(reqBytes); exp != act {
			t.Errorf("Wrong payload value: %v != %v", act, exp)
		}
		i++
		w.Header().Add("foobar", "baz")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("foobar"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  extract_headers:
    include_patterns: [ ".*" ]
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")}))
	if res != nil {
		t.Error(res)
	} else if expC, actC := 1, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foobar", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "201", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	} else if exp, act := "baz", msgs[0].Get(0).MetaGetStr("foobar"); exp != act {
		t.Errorf("Wrong metadata value: %v != %v", act, exp)
	}
}

func TestHTTPClientSerial(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		if string(bodyBytes) == "bar" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("foobar " + string(bodyBytes)))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  retry_period: 1ms
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	require.NoError(t, err)

	inputMsg := message.QuickBatch([][]byte{
		[]byte("foo"),
		[]byte("bar"),
		[]byte("baz"),
		[]byte("qux"),
		[]byte("quz"),
	})
	inputMsg.Get(0).MetaSetMut("foo", "bar")
	msgs, res := h.ProcessBatch(t.Context(), inputMsg)
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	require.Equal(t, 5, msgs[0].Len())

	assert.Equal(t, "foobar foo", string(msgs[0].Get(0).AsBytes()))
	assert.Equal(t, "bar", string(msgs[0].Get(1).AsBytes()))
	require.Error(t, msgs[0].Get(1).ErrorGet())
	assert.Contains(t, msgs[0].Get(1).ErrorGet().Error(), "request returned unexpected response code")
	assert.Equal(t, "foobar baz", string(msgs[0].Get(2).AsBytes()))
	assert.Equal(t, "foobar qux", string(msgs[0].Get(3).AsBytes()))
	assert.Equal(t, "foobar quz", string(msgs[0].Get(4).AsBytes()))
}

func TestHTTPClientParallel(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(5)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
		wg.Wait()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("foobar"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  parallel: true
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	inputMsg := message.QuickBatch([][]byte{
		[]byte("foo"),
		[]byte("bar"),
		[]byte("baz"),
		[]byte("qux"),
		[]byte("quz"),
	})
	inputMsg.Get(0).MetaSetMut("foo", "bar")
	msgs, res := h.ProcessBatch(t.Context(), inputMsg)
	if res != nil {
		t.Error(res)
	} else if expC, actC := 5, msgs[0].Len(); actC != expC {
		t.Errorf("Wrong result count: %v != %v", actC, expC)
	} else if exp, act := "foobar", string(message.GetAllBytes(msgs[0])[0]); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	} else if exp, act := "bar", msgs[0].Get(0).MetaGetStr("foo"); exp != act {
		t.Errorf("Metadata not preserved: %v != %v", act, exp)
	} else if exp, act := "201", msgs[0].Get(0).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}
}

func TestHTTPClientParallelError(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(5)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
		wg.Wait()
		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(reqBytes) == "baz" {
			http.Error(w, "test error", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("foobar"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/testpost
  parallel: true
  retries: 0
`, ts.URL)

	h, err := mock.NewManager().NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	msgs, res := h.ProcessBatch(t.Context(), message.QuickBatch([][]byte{
		[]byte("foo"),
		[]byte("bar"),
		[]byte("baz"),
		[]byte("qux"),
		[]byte("quz"),
	}))
	if res != nil {
		t.Error(res)
	}
	if expC, actC := 5, msgs[0].Len(); actC != expC {
		t.Fatalf("Wrong result count: %v != %v", actC, expC)
	}
	if exp, act := "baz", string(msgs[0].Get(2).AsBytes()); act != exp {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}
	assert.Error(t, msgs[0].Get(2).ErrorGet())
	if exp, act := "403", msgs[0].Get(2).MetaGetStr("http_status_code"); exp != act {
		t.Errorf("Wrong response code metadata: %v != %v", act, exp)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if exp, act := "foobar", string(msgs[0].Get(i).AsBytes()); act != exp {
			t.Errorf("Wrong result: %v != %v", act, exp)
		}
		assert.NoError(t, msgs[0].Get(i).ErrorGet())
		if exp, act := "200", msgs[0].Get(i).MetaGetStr("http_status_code"); exp != act {
			t.Errorf("Wrong response code metadata: %v != %v", act, exp)
		}
	}
}

func TestHTTPProcessorTracing(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("response body"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/test
`, ts.URL)

	mgr := mock.NewManager()
	mgr.T = tp

	h, err := mgr.NewProcessor(conf)
	require.NoError(t, err)

	inMsg := message.NewPart([]byte("test message"))
	inBatch := message.Batch{inMsg}

	outBatches, res := h.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, outBatches, 1)
	require.Len(t, outBatches[0], 1)

	// Verify the HTTP request worked
	assert.Equal(t, "response body", string(outBatches[0][0].AsBytes()))
	assert.Equal(t, "200", outBatches[0][0].MetaGetStr("http_status_code"))

	// Verify spans were created with correct names and finished
	// There should be two spans: one for the processor and one for the HTTP request
	spans := tp.Spans()
	require.Len(t, spans, 2)

	// Find the spans by name
	httpSpan := tp.FindSpan("http")
	httpRequestSpan := tp.FindSpan("http_request")
	require.NotNil(t, httpSpan, "http span should exist")
	require.NotNil(t, httpRequestSpan, "http_request span should exist")

	// Verify spans are ended
	assert.True(t, httpSpan.Ended, "http span should be ended")
	assert.True(t, httpRequestSpan.Ended, "http_request span should be ended")

	// Verify proper nesting: http_request should be a child of http
	assert.True(t, httpRequestSpan.IsChildOf(httpSpan),
		"http_request span should be a child of http span")
	assert.True(t, httpSpan.IsRoot(),
		"http span should be a root span")

	// Verify OpenTelemetry semantic convention attributes
	assert.Equal(t, "POST", httpRequestSpan.GetStringAttribute("http.request.method"))
	assert.Contains(t, httpRequestSpan.GetStringAttribute("url.full"), ts.URL)
	assert.Contains(t, httpRequestSpan.GetStringAttribute("url.full"), "/test")
	assert.NotEmpty(t, httpRequestSpan.GetStringAttribute("server.address"))
	assert.Equal(t, 200, httpRequestSpan.GetIntAttribute("http.response.status_code"))
	assert.NotEmpty(t, httpRequestSpan.GetStringAttribute("network.protocol.version"))
}

func TestHTTPProcessorTracingBatch(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		_, _ = fmt.Fprintf(w, "response %d", count)
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/test
`, ts.URL)

	mgr := mock.NewManager()
	mgr.T = tp

	h, err := mgr.NewProcessor(conf)
	require.NoError(t, err)

	// Create batch with multiple messages
	inBatch := message.Batch{
		message.NewPart([]byte("message 1")),
		message.NewPart([]byte("message 2")),
		message.NewPart([]byte("message 3")),
	}

	outBatches, res := h.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, outBatches, 1)
	require.Len(t, outBatches[0], 3)

	// Verify each message was processed
	for i := range 3 {
		assert.Contains(t, string(outBatches[0][i].AsBytes()), "response")
		assert.Equal(t, "200", outBatches[0][i].MetaGetStr("http_status_code"))
	}

	// Verify spans were created for each message in the batch
	// There should be 6 spans total: 3 processor spans (one per message) and 3 HTTP request spans (one per message)
	spans := tp.Spans()
	require.Len(t, spans, 6)

	// Find all http and http_request spans
	httpSpans := tp.FindSpansByName("http")
	httpRequestSpans := tp.FindSpansByName("http_request")
	require.Len(t, httpSpans, 3, "should have 3 http processor spans")
	require.Len(t, httpRequestSpans, 3, "should have 3 http_request spans")

	// Verify all spans are ended
	for i, span := range httpSpans {
		assert.True(t, span.Ended, "http span %d should be ended", i)
	}
	for i, span := range httpRequestSpans {
		assert.True(t, span.Ended, "http_request span %d should be ended", i)
	}

	// Verify proper nesting: each http_request span should be a child of an http span
	// Since spans are processed serially (not in parallel mode), we can match them by order
	for i := range 3 {
		httpSpan := httpSpans[i]
		httpRequestSpan := httpRequestSpans[i]

		assert.True(t, httpRequestSpan.IsChildOf(httpSpan),
			"http_request span %d should be a child of http span %d", i, i)

		// Verify each http span has exactly one child
		children := tp.GetChildren(httpSpan)
		assert.Len(t, children, 1, "http span %d should have exactly one child", i)
		assert.Equal(t, "http_request", children[0].Name)
	}

	// Verify each http_request span has the correct attributes
	for i, httpRequestSpan := range httpRequestSpans {
		assert.Equal(t, "POST", httpRequestSpan.GetStringAttribute("http.request.method"),
			"span %d should have http.request.method", i)
		assert.Equal(t, 200, httpRequestSpan.GetIntAttribute("http.response.status_code"),
			"span %d should have http.response.status_code", i)
		assert.NotEmpty(t, httpRequestSpan.GetStringAttribute("url.full"),
			"span %d should have url.full", i)
	}
}

func TestHTTPProcessorTracingWithErrors(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/test
  retries: 0
`, ts.URL)

	mgr := mock.NewManager()
	mgr.T = tp

	h, err := mgr.NewProcessor(conf)
	require.NoError(t, err)

	inMsg := message.NewPart([]byte("test message"))
	inBatch := message.Batch{inMsg}

	outBatches, res := h.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, outBatches, 1)

	// Verify error handling
	httpRequestSpan := tp.FindSpan("http_request")
	require.NotNil(t, httpRequestSpan)

	// Verify error attributes
	assert.Equal(t, 404, httpRequestSpan.GetIntAttribute("http.response.status_code"))
	assert.NotEmpty(t, httpRequestSpan.GetStringAttribute("error.type"))
	// Check that error event was logged (the event name is in the Events slice)
	require.NotEmpty(t, httpRequestSpan.Events, "should have at least one event")
	assert.Contains(t, httpRequestSpan.Events[0], "event")
}

func TestHTTPProcessorTracingWithRetries(t *testing.T) {
	tp := tracingtest.NewInMemoryRecordingTracerProvider()
	var attemptCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attemptCount.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer ts.Close()

	conf := parseYAMLProcConf(t, `
http:
  url: %v/test
  retries: 3
  retry_period: 1ms
  backoff_on: [503]
`, ts.URL)

	mgr := mock.NewManager()
	mgr.T = tp

	h, err := mgr.NewProcessor(conf)
	require.NoError(t, err)

	inMsg := message.NewPart([]byte("test message"))
	inBatch := message.Batch{inMsg}

	outBatches, res := h.ProcessBatch(t.Context(), inBatch)
	require.NoError(t, res)
	require.Len(t, outBatches, 1)

	// Verify retry count attribute
	httpRequestSpan := tp.FindSpan("http_request")
	require.NotNil(t, httpRequestSpan)

	// Should have retried twice before succeeding
	assert.Equal(t, 2, httpRequestSpan.GetIntAttribute("http.request.resend_count"))
	assert.Equal(t, 200, httpRequestSpan.GetIntAttribute("http.response.status_code"))
}
