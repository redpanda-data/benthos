// Copyright 2026 Redpanda Data, Inc.

package pure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"

	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

func mustProc(t *testing.T, conf string) processor.V1 {
	t.Helper()
	pConf, err := testutil.ProcessorFromYAML(conf)
	require.NoError(t, err)
	p, err := mock.NewManager().NewProcessor(pConf)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close(t.Context()) })
	return p
}

func strictExec(t *testing.T, procs []processor.V1, in string) message.Batch {
	t.Helper()
	msgs, err := processor.ExecuteAll(processor.WithStrict(t.Context()), procs, message.QuickBatch([][]byte{[]byte(in)}))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	return msgs[0]
}

// Under strict execution, a message that fails a processor skips the remaining
// processors and retains its error (which would be rejected at the output).
func TestStrictPipelineShortCircuitsUnrecovered(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `mapping: 'root = throw("boom")'`),
		mustProc(t, `mapping: 'root = "reached"'`),
	}, "x")

	require.Equal(t, 1, out.Len())
	assert.Equal(t, "x", string(out.Get(0).AsBytes()), "the second processor must be skipped")
	assert.Error(t, out.Get(0).ErrorGet(), "the unrecovered error survives to be rejected at the output")
}

// A standalone `catch` processor does NOT recover a prior failure under strict:
// the errored message short-circuits past it (catch is a downstream catcher, not
// a wrapper that contains the failure). This is a deliberate design decision —
// under strict, recovery is done with try_catch (or retry). Documented so the
// behaviour is locked and not "fixed" by accident.
func TestStrictStandaloneCatchDoesNotRecover(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `mapping: 'root = throw("boom")'`),
		mustProc(t, `
catch:
  - mapping: 'root = "RECOVERED"'
`),
	}, "x")

	require.Equal(t, 1, out.Len())
	assert.Equal(t, "x", string(out.Get(0).AsBytes()), "standalone catch must be skipped, not recover the message")
	assert.Error(t, out.Get(0).ErrorGet(), "the message remains errored and would be rejected at the output")
}

// A new error introduced inside a catch block is preserved (not swallowed by the
// strict-stripping of the catch scope): the message leaves the try_catch still
// errored and would therefore be rejected at the output under strict. Strict's
// reject applies at the output regardless of the catch scope's short-circuit
// exemption.
func TestStrictNewErrorInCatchIsPreserved(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
try_catch:
  processors:
    - mapping: 'root = throw("original")'
  catch:
    - mapping: 'root = throw("new error in catch")'
`),
	}, "x")

	require.Equal(t, 1, out.Len())
	err := out.Get(0).ErrorGet()
	require.Error(t, err, "a new error raised inside the catch block must survive to be rejected at the output")
	assert.Contains(t, err.Error(), "new error in catch")
}

// Within a try_catch catch block the failure flag is cleared before the catch
// processors run, so they execute under strict semantics: a NEW error raised by
// an early catch processor short-circuits the subsequent ones, and the surviving
// error is retained for output rejection.
func TestStrictCatchBlockShortCircuitsNewErrors(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
try_catch:
  processors:
    - mapping: 'root = throw("original")'
  catch:
    - mapping: 'root = throw("first catch step fails")'
    - mapping: 'meta ran_second = "yes"'
`),
	}, "x")

	require.Equal(t, 1, out.Len())
	assert.NotEqual(t, "yes", out.Get(0).MetaGetStr("ran_second"), "a new error in the catch block short-circuits later catch processors under strict")
	assert.Error(t, out.Get(0).ErrorGet(), "the surviving error is retained for output rejection")
}

// A try_catch that recovers a failure clears the error, so subsequent processors
// run normally and the message is not rejected.
func TestStrictPipelineTryCatchRecovers(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = "recovered"'
`),
		mustProc(t, `mapping: 'root = content().string() + "-reached"'`),
	}, "x")

	require.Equal(t, 1, out.Len())
	assert.Equal(t, "recovered-reached", string(out.Get(0).AsBytes()), "recovery clears the error so later processors run")
	assert.NoError(t, out.Get(0).ErrorGet())
}

// A non-recovering catch leaves the message failed, so it short-circuits and is
// rejected — confirming only genuinely-handled errors are cleared.
func TestStrictPipelineFailingCatchStillShortCircuits(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
try_catch:
  processors:
    - mapping: 'root = throw("boom")'
  catch:
    - mapping: 'root = throw("catch failed too")'
`),
		mustProc(t, `mapping: 'root = "reached"'`),
	}, "x")

	require.Equal(t, 1, out.Len())
	assert.NotEqual(t, "reached", string(out.Get(0).AsBytes()), "a failed recovery still short-circuits")
	assert.Error(t, out.Get(0).ErrorGet())
}

// branch manipulates the error flag itself; under strict its children
// short-circuit, but it must still behave correctly (D7 verification).
func TestStrictBranchDoesNotBreak(t *testing.T) {
	// A branch whose child fails maps the error back onto the message.
	out := strictExec(t, []processor.V1{
		mustProc(t, `
branch:
  processors:
    - mapping: 'root = throw("branch child boom")'
`),
	}, "x")
	require.Equal(t, 1, out.Len())
	assert.Error(t, out.Get(0).ErrorGet(), "branch maps the child failure back onto the message")

	// A branch whose child succeeds works normally under strict.
	out = strictExec(t, []processor.V1{
		mustProc(t, `
branch:
  request_map: 'root = this'
  processors:
    - mapping: 'root.added = "yes"'
  result_map: 'root.added = this.added'
`),
	}, `{"id":1}`)
	require.Equal(t, 1, out.Len())
	assert.NoError(t, out.Get(0).ErrorGet())
	assert.JSONEq(t, `{"id":1,"added":"yes"}`, string(out.Get(0).AsBytes()))
}

// while runs its children in a loop; under strict a child failure short-circuits
// without breaking the processor (D7 verification).
func TestStrictWhileDoesNotBreak(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
while:
  at_least_once: true
  check: 'false'
  processors:
    - mapping: 'root = throw("while child boom")'
`),
	}, "x")
	require.Equal(t, 1, out.Len())
	assert.Error(t, out.Get(0).ErrorGet())
}

// Short-circuit must fire INSIDE composition processors (not just at the top
// level) — verifying the strict signal propagates through their child chains.
func TestStrictShortCircuitsInsideForEach(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
for_each:
  - mapping: 'root = throw("boom")'
  - mapping: 'root = "reached"'
`),
	}, "x")
	require.Equal(t, 1, out.Len())
	assert.Equal(t, "x", string(out.Get(0).AsBytes()), "for_each child after the failure must be skipped")
	assert.Error(t, out.Get(0).ErrorGet())
}

func TestStrictShortCircuitsInsideSwitchCase(t *testing.T) {
	out := strictExec(t, []processor.V1{
		mustProc(t, `
switch:
  - processors:
      - mapping: 'root = throw("boom")'
      - mapping: 'root = "reached"'
`),
	}, "x")
	require.Equal(t, 1, out.Len())
	assert.Equal(t, "x", string(out.Get(0).AsBytes()), "switch-case child after the failure must be skipped")
	assert.Error(t, out.Get(0).ErrorGet())
}

// Filtering (deleting) a message is NOT an error: under strict the survivors
// continue through the pipeline and nothing is rejected.
func TestStrictDoesNotRejectFilteredMessages(t *testing.T) {
	msgs, err := processor.ExecuteAll(processor.WithStrict(t.Context()), []processor.V1{
		mustProc(t, `mapping: 'root = if content().string() == "drop" { deleted() }'`),
		mustProc(t, `mapping: 'root = content().string() + "-kept"'`),
	}, message.QuickBatch([][]byte{[]byte("drop"), []byte("keep")}))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, 1, msgs[0].Len(), "the filtered message is gone, the survivor remains")
	assert.Equal(t, "keep-kept", string(msgs[0].Get(0).AsBytes()))
	assert.NoError(t, msgs[0].Get(0).ErrorGet())
}
