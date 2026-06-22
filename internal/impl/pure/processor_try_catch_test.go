// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"

	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

// runTryCatchMsg runs a try_catch config against a pre-built batch (allowing the
// caller to seed error flags or metadata) and returns all resulting batches.
func runTryCatchMsg(t *testing.T, conf string, msg message.Batch) []message.Batch {
	t.Helper()
	pConf, err := testutil.ProcessorFromYAML(conf)
	require.NoError(t, err)
	proc, err := mock.NewManager().NewProcessor(pConf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msgs, res := proc.ProcessBatch(t.Context(), msg)
	require.NoError(t, res)
	return msgs
}

func runTryCatch(t *testing.T, conf string, in [][]byte) message.Batch {
	t.Helper()
	pConf, err := testutil.ProcessorFromYAML(conf)
	require.NoError(t, err)
	proc, err := mock.NewManager().NewProcessor(pConf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msgs, res := proc.ProcessBatch(t.Context(), message.QuickBatch(in))
	require.NoError(t, res)
	require.Len(t, msgs, 1)
	return msgs[0]
}

func assertNoErrors(t *testing.T, b message.Batch) {
	t.Helper()
	_ = b.Iter(func(i int, p *message.Part) error {
		assert.NoErrorf(t, p.ErrorGet(), "unexpected failure flag on part %v", i)
		return nil
	})
}

// When no processor fails the catch block is not applied and messages pass
// through transformed and error-free.
func TestTryCatchNoFailure(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = "ok: " + content().string()'
  catch:
    - mapping: 'root = "should not run"'
`, [][]byte{[]byte("hello"), []byte("world")})

	assert.Equal(t, [][]byte{[]byte("ok: hello"), []byte("ok: world")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// A failure in the processors routes the message to the catch block, which can
// recover it, and the error flag is cleared afterwards.
func TestTryCatchRecovers(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = "recovered"'
`, [][]byte{[]byte("hello")})

	assert.Equal(t, [][]byte{[]byte("recovered")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// The catch block can inspect the failure via the error metadata field rather
// than the error() function (the failure flag is cleared before catch runs).
func TestTryCatchErrorAccessible(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("specific failure")'
  catch:
    - mapping: 'root = "got: " + @error.what'
`, [][]byte{[]byte("hello")})

	require.Len(t, out, 1)
	assert.Contains(t, string(out.Get(0).AsBytes()), "specific failure")
	assertNoErrors(t, out)
}

// The caught error is a structured object: `what` holds the message and the
// source `name` (and `label`/`path` when available) mirror the error_source_*
// functions.
func TestTryCatchErrorObjectFields(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = {"what": @error.what, "name": @error.name}'
`, [][]byte{[]byte("hello")})

	require.Len(t, out, 1)
	got := string(out.Get(0).AsBytes())
	assert.Contains(t, got, `"what":"failed assignment`, "the object carries the error message under `what`")
	assert.Contains(t, got, `"name":"mapping"`, "the object carries the failing component name")
	assertNoErrors(t, out)
}

// The error metadata key is configurable.
func TestTryCatchErrorMetadataKeyConfigurable(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  error_metadata: why_failed
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = "reason: " + @why_failed.what'
`, [][]byte{[]byte("hello")})

	require.Len(t, out, 1)
	assert.Contains(t, string(out.Get(0).AsBytes()), "reason: ")
	assert.Contains(t, string(out.Get(0).AsBytes()), "boom")
	assertNoErrors(t, out)
}

// Only the messages that failed are routed through the catch block; the others
// pass through untouched.
func TestTryCatchMixedBatch(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: |
        if content().string() == "fail" {
          root = throw("boom")
        }
  catch:
    - mapping: 'root = "recovered"'
`, [][]byte{[]byte("good"), []byte("fail"), []byte("good2")})

	assert.Equal(t, [][]byte{
		[]byte("good"),
		[]byte("recovered"),
		[]byte("good2"),
	}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// "try" semantics: once a processor fails for a message the remaining
// processors in the list are skipped for that message.
func TestTryCatchShortCircuits(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("first failed")'
    - mapping: 'root = "second ran"'
  catch: []
`, [][]byte{[]byte("original")})

	// The second processor must have been skipped, so content is unchanged, and
	// the empty catch clears the failure flag.
	assert.Equal(t, [][]byte{[]byte("original")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// An omitted catch block swallows the error: the failure flag is cleared even
// though no recovery processors run.
func TestTryCatchEmptyCatchSwallows(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
`, [][]byte{[]byte("original")})

	assert.Equal(t, [][]byte{[]byte("original")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// A catch block may filter messages away.
func TestTryCatchCatchFilters(t *testing.T) {
	pConf, err := testutil.ProcessorFromYAML(`
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = deleted()'
`)
	require.NoError(t, err)
	proc, err := mock.NewManager().NewProcessor(pConf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msgs, res := proc.ProcessBatch(t.Context(), message.QuickBatch([][]byte{[]byte("hello")}))
	require.NoError(t, res)
	assert.Empty(t, msgs)
}

// When only a later processor in the list fails, the earlier successful
// transforms are retained and the catch block still fires.
func TestTryCatchLaterProcessorFails(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = "step1: " + content().string()'
    - mapping: 'root = throw("step2 failed")'
    - mapping: 'root = "step3 should not run"'
  catch:
    - mapping: 'root = "caught after: " + content().string()'
`, [][]byte{[]byte("in")})

	// catch sees the content as left by step1 (step2's throw didn't mutate it,
	// step3 was skipped).
	assert.Equal(t, [][]byte{[]byte("caught after: step1: in")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// Metadata set before a failure is visible to the catch block.
func TestTryCatchMetadataVisibleInCatch(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'meta foo = "bar"'
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = "meta was: " + meta("foo")'
`, [][]byte{[]byte("in")})

	assert.Equal(t, [][]byte{[]byte("meta was: bar")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// After try_catch handles a failure the error flag is cleared, so a downstream
// catch processor does NOT fire on it.
func TestTryCatchClearsErrorForDownstream(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = "handled"'
`, [][]byte{[]byte("in")})
	require.Len(t, out, 1)
	assert.Equal(t, [][]byte{[]byte("handled")}, message.GetAllBytes(out))
	assertNoErrors(t, out)
}

// FOUNDATION PROBE: a message that arrives already in an errored state has the
// `processors` skipped entirely (inherited "try" semantics — ExecuteTryAll skips
// pre-errored messages) and is routed straight to the catch block. Documents
// that try_catch does NOT define a fresh error scope today.
func TestTryCatchPreExistingErrorSkipsProcessors(t *testing.T) {
	msg := message.QuickBatch([][]byte{[]byte("original")})
	msg.Get(0).ErrorSet(errors.New("prior failure"))

	out := runTryCatchMsg(t, `
try_catch:
  processors:
    - mapping: 'root = "processors ran"'
  catch:
    - mapping: 'root = "caught: " + @error.what'
`, msg)

	require.Len(t, out, 1)
	// The `processors` were skipped (output is not "processors ran"); the catch
	// fired against the PRE-EXISTING error, read from metadata.
	assert.Equal(t, "caught: prior failure", string(out[0].Get(0).AsBytes()))
	assertNoErrors(t, out[0])
}

// If a catch processor itself fails, the new error is NOT cleared: a failed
// recovery propagates downstream rather than being reported as a success.
func TestTryCatchFailingCatchPropagates(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("original")'
  catch:
    - mapping: 'root = throw("catch itself failed")'
`, [][]byte{[]byte("in")})

	require.Equal(t, 1, out.Len())
	err := out.Get(0).ErrorGet()
	require.Error(t, err, "a failure within the catch block must propagate")
	assert.Contains(t, err.Error(), "catch itself failed")
}

// In a mixed batch, a catch failure on one message propagates while a successful
// recovery on another is cleared — proving the propagation is per-message.
func TestTryCatchPartialCatchFailure(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: |
        if content().string() == "explode" {
          root = throw("catch failed")
        } else {
          root = "recovered"
        }
`, [][]byte{[]byte("ok"), []byte("explode")})

	require.Equal(t, 2, out.Len())

	// First message: catch recovered it, error cleared.
	assert.Equal(t, "recovered", string(out.Get(0).AsBytes()))
	assert.NoError(t, out.Get(0).ErrorGet())

	// Second message: catch failed, error propagates.
	require.Error(t, out.Get(1).ErrorGet())
	assert.Contains(t, out.Get(1).ErrorGet().Error(), "catch failed")
}

// Inside the catch block the failure flag has been cleared, so the error()
// function reports no error; the failure is available via metadata instead.
func TestTryCatchErrorFunctionClearedInCatch(t *testing.T) {
	out := runTryCatch(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = {"errored": errored(), "from_func": error().or("none"), "from_meta": @error.what}'
`, [][]byte{[]byte("in")})

	require.Equal(t, 1, out.Len())
	got := string(out.Get(0).AsBytes())
	assert.Contains(t, got, `"errored":false`, "errored() is false inside catch (flag cleared)")
	assert.Contains(t, got, `"from_func":"none"`, "error() reports no error inside catch")
	assert.Contains(t, got, `"from_meta":"failed assignment`, "the failure is available via metadata")
	assertNoErrors(t, out)
}
