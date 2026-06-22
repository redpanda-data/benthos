// Copyright 2026 Redpanda Data, Inc.

package service_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// pushOneAndStop runs the built stream, pushes a single message, then stops the
// stream, returning the acknowledgement error (non-nil = the message was
// nacked). Bounded by timeouts so a poison/backpressure loop cannot hang it.
func pushOneAndStop(t *testing.T, b *service.StreamBuilder, payload string) error {
	t.Helper()
	pushFn, err := b.AddBatchProducerFunc()
	require.NoError(t, err)
	strm, err := b.Build()
	require.NoError(t, err)

	var pushErr error
	wg := sync.WaitGroup{}
	wg.Go(func() {
		ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		pushErr = pushFn(ctx, service.MessageBatch{service.NewMessage([]byte(payload))})
		time.Sleep(200 * time.Millisecond)
		_ = strm.StopWithin(5 * time.Second)
	})
	require.NoError(t, strm.Run(context.Background()))
	wg.Wait()
	return pushErr
}

func readOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// THE HEADLINE CON-461 PATTERN: under strict, when a primary output's own
// processors fail (e.g. parquet_encode), the failed tier rejects and `fallback`
// forwards the ORIGINAL (pre-processor) message to the secondary tier, which
// writes it. The corrupt/unprocessed data is never written to the primary.
func TestStrictFallbackRecoversFailedOutputProcessor(t *testing.T) {
	dir := t.TempDir()
	a, bf := filepath.Join(dir, "A.txt"), filepath.Join(dir, "B.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	sb.SetStrict(true)
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`
fallback:
  - file:
      codec: lines
      path: %s
    processors:
      - mapping: 'root = throw("encode failed")'
  - file:
      codec: lines
      path: %s
`, a, bf)))

	ackErr := pushOneAndStop(t, sb, "raw-payload")

	assert.NoError(t, ackErr, "the message is recovered by the secondary tier, so the overall delivery is acked")
	assert.Empty(t, readOrEmpty(a), "the primary must NOT write the message whose processing failed")
	assert.Equal(t, "raw-payload\n", readOrEmpty(bf), "the secondary tier receives and writes the original, pre-processor message")
}

// Sanity: when the primary output's processors SUCCEED under strict, the primary
// writes (transformed) and the fallback secondary is never used.
func TestStrictFallbackPrimarySuccess(t *testing.T) {
	dir := t.TempDir()
	a, bf := filepath.Join(dir, "A.txt"), filepath.Join(dir, "B.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	sb.SetStrict(true)
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`
fallback:
  - file:
      codec: lines
      path: %s
    processors:
      - mapping: 'root = content().uppercase()'
  - file:
      codec: lines
      path: %s
`, a, bf)))

	ackErr := pushOneAndStop(t, sb, "ok")

	assert.NoError(t, ackErr)
	assert.Equal(t, "OK\n", readOrEmpty(a), "the primary writes the successfully processed message")
	assert.Empty(t, readOrEmpty(bf), "the secondary tier is not used when the primary succeeds")
}

// When BOTH fallback tiers fail their own processing under strict, nothing is
// written anywhere and the message is nacked (backpressure).
func TestStrictFallbackBothTiersFailProcessors(t *testing.T) {
	dir := t.TempDir()
	a, bf := filepath.Join(dir, "A.txt"), filepath.Join(dir, "B.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	sb.SetStrict(true)
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`
fallback:
  - file:
      codec: lines
      path: %s
    processors:
      - mapping: 'root = throw("A failed")'
  - file:
      codec: lines
      path: %s
    processors:
      - mapping: 'root = throw("B failed")'
`, a, bf)))

	ackErr := pushOneAndStop(t, sb, "hello")

	assert.Error(t, ackErr, "with every tier failing, the message must be nacked")
	assert.Empty(t, readOrEmpty(a), "primary writes nothing")
	assert.Empty(t, readOrEmpty(bf), "secondary writes nothing")
}

// A message already flagged BEFORE reaching the output (a pipeline error) is
// rejected by EVERY fallback tier — it arrives flagged at each — so nothing is
// written and it is nacked. (Contrast with the output-processor case above.)
func TestStrictFallbackFlaggedOnEntryRejectedByAllTiers(t *testing.T) {
	dir := t.TempDir()
	a, bf := filepath.Join(dir, "A.txt"), filepath.Join(dir, "B.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	sb.SetStrict(true)
	require.NoError(t, sb.AddProcessorYAML(`mapping: 'root = throw("pipeline failed")'`))
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`
fallback:
  - file: {codec: lines, path: %s}
  - file: {codec: lines, path: %s}
`, a, bf)))

	ackErr := pushOneAndStop(t, sb, "hello")

	assert.Error(t, ackErr, "a message flagged on entry is rejected by every tier and nacked")
	assert.Empty(t, readOrEmpty(a), "primary writes nothing")
	assert.Empty(t, readOrEmpty(bf), "secondary writes nothing")
}

// An unguarded pipeline error under strict, with a single output: the message is
// nacked and never written to the output.
func TestStrictUnguardedPipelineErrorNackedNotWritten(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	sb.SetStrict(true)
	require.NoError(t, sb.AddProcessorYAML(`mapping: 'root = throw("boom")'`))
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`file: {codec: lines, path: %s}`, out)))

	ackErr := pushOneAndStop(t, sb, "hello")

	assert.Error(t, ackErr, "the errored message must be nacked")
	assert.Empty(t, readOrEmpty(out), "the errored message must not be written to the output")
}

// CONTRAST (documents why strict matters): WITHOUT strict, the same
// failed-output-processor fallback writes the raw, unprocessed data to the
// primary (the CON-461 footgun) and never reaches the secondary.
func TestNonStrictFallbackWritesRawToPrimary(t *testing.T) {
	dir := t.TempDir()
	a, bf := filepath.Join(dir, "A.txt"), filepath.Join(dir, "B.txt")

	sb := service.NewStreamBuilder()
	require.NoError(t, sb.SetLoggerYAML("level: NONE"))
	// strict NOT set (default false).
	require.NoError(t, sb.AddOutputYAML(fmt.Sprintf(`
fallback:
  - file:
      codec: lines
      path: %s
    processors:
      - mapping: 'root = throw("encode failed")'
  - file:
      codec: lines
      path: %s
`, a, bf)))

	ackErr := pushOneAndStop(t, sb, "raw-payload")

	assert.NoError(t, ackErr)
	assert.Equal(t, "raw-payload\n", readOrEmpty(a), "without strict the unprocessed data is written to the primary (the footgun)")
	assert.Empty(t, readOrEmpty(bf), "the secondary is never reached because the primary 'succeeds'")
}
