// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

// --- Benchmarks -------------------------------------------------------------
//
// These characterise the cost of adopting try_catch on the success path (no
// failures), against two reference points:
//
//   - bare_series: the same processors run as an ordinary in-order pipeline
//     (processor.ExecuteAll) with NO per-message error isolation. The floor.
//   - try_then_catch: the real `try` + `catch` processor composition that
//     try_catch replaces. Like try_catch, these isolate errors per message, so
//     this is the apples-to-apples comparison.

func benchProc(tb testing.TB, conf string) processor.V1 {
	tb.Helper()
	c, err := testutil.ProcessorFromYAML(conf)
	require.NoError(tb, err)
	p, err := mock.NewManager().NewProcessor(c)
	require.NoError(tb, err)
	return p
}

func benchMappingProcs(tb testing.TB, n int) []processor.V1 {
	tb.Helper()
	procs := make([]processor.V1, n)
	for i := range procs {
		procs[i] = benchProc(tb, `mapping: 'root = content().uppercase()'`)
	}
	return procs
}

func benchBatch(n int) message.Batch {
	parts := make([][]byte, n)
	for i := range parts {
		parts[i] = []byte("the quick brown fox jumps over the lazy dog")
	}
	return message.QuickBatch(parts)
}

func BenchmarkTryCatchOverhead(b *testing.B) {
	const childProcs = `
  - mapping: 'root = content().uppercase()'
  - mapping: 'root = content().uppercase()'`
	const catchProcs = `
  - mapping: 'root = content().uppercase()'`

	for _, size := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("bare_series/n=%d", size), func(b *testing.B) {
			procs := benchMappingProcs(b, 2)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := processor.ExecuteAll(ctx, procs, benchBatch(size)); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("try_then_catch/n=%d", size), func(b *testing.B) {
			idiom := []processor.V1{
				benchProc(b, "try:"+childProcs),
				benchProc(b, "catch:"+catchProcs),
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := processor.ExecuteAll(ctx, idiom, benchBatch(size)); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("try_catch/n=%d", size), func(b *testing.B) {
			tc := newTryCatchProc(benchMappingProcs(b, 2), benchMappingProcs(b, 1), tcDefaultErrorMeta)
			pctx := processor.TestBatchProcContext(context.Background(), nil, nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.ProcessBatch(pctx, benchBatch(size)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
