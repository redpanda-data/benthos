package benchmark_test

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/benchmark"
)

// BenchmarkCorpus emits one sub-benchmark per equivalent V1↔V2 case,
// timing each engine separately. Run with:
//
//	go test ./internal/bloblang2/migrator/benchmark -bench BenchmarkCorpus -benchmem
//
// Each sub-benchmark name is "<rel-path>/<test-name>/(v1|v2)", so
// `benchstat` / `go test -bench` filters can isolate a specific case
// or engine.
func BenchmarkCorpus(b *testing.B) {
	coll, err := benchmark.CollectDefault()
	if err != nil {
		b.Fatalf("collect corpus: %v", err)
	}
	if len(coll.Cases) == 0 {
		b.Fatal("no equivalent cases found — corpus probably unreachable")
	}
	for _, c := range coll.Cases {
		cpy := c
		b.Run(cpy.Name+"/v1", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := cpy.V1.Exec(cpy.Input, cpy.InputMetadata); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(cpy.Name+"/v2", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := cpy.V2.Exec(cpy.Input, cpy.InputMetadata); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestCorpusAnalysis measures V1 and V2 performance for every equivalent
// case in the corpus and prints per-case + aggregate statistics
// (geometric-mean speed-up, median, best, worst, allocation savings).
// The test does NOT enforce a performance floor — it passes as long as
// the corpus collection succeeds.
//
// Skipped in -short mode. The per-case time budget is controlled by the
// MIGRATOR_BENCH_BUDGET environment variable (default "100ms"); shorter
// values trade statistical stability for faster runs.
func TestCorpusAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("corpus benchmark analysis is slow; skipped in -short mode")
	}
	budget := 100 * time.Millisecond
	if s := os.Getenv("MIGRATOR_BENCH_BUDGET"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			budget = d
		}
	}

	coll, err := benchmark.CollectDefault()
	if err != nil {
		t.Fatalf("collect corpus: %v", err)
	}
	t.Logf("accepted cases: %d", len(coll.Cases))
	t.Logf("skipped:        %d", len(coll.Skips))
	t.Logf("per-case budget: %s per engine (set MIGRATOR_BENCH_BUDGET to override)", budget)
	t.Logf("")
	t.Logf("skips by reason:")
	counts := coll.SkipCounts()
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, string(r))
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		t.Logf("  %-26s %d", r, counts[benchmark.SkipReason(r)])
	}
	t.Logf("")

	rows := make([]benchRow, 0, len(coll.Cases))
	for _, c := range coll.Cases {
		cpy := c
		v1 := timeExec(func() error {
			_, err := cpy.V1.Exec(cpy.Input, cpy.InputMetadata)
			return err
		}, budget)
		v2 := timeExec(func() error {
			_, err := cpy.V2.Exec(cpy.Input, cpy.InputMetadata)
			return err
		}, budget)
		if v1.ns == 0 || v2.ns == 0 {
			continue
		}
		rows = append(rows, benchRow{
			name:    cpy.Name,
			v1ns:    v1.ns,
			v2ns:    v2.ns,
			v1alloc: v1.allocs,
			v2alloc: v2.allocs,
			v1bytes: v1.bytes,
			v2bytes: v2.bytes,
		})
	}
	if len(rows) == 0 {
		t.Skip("no equivalent cases to benchmark")
	}

	// Per-case table, sorted by V2/V1 ns/op descending so slowdowns
	// land at the top of the log (easiest to spot).
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].v2OverV1() > rows[j].v2OverV1()
	})
	t.Logf("per-case breakdown (sorted by V2/V1 ns/op, worst first):")
	t.Logf("%-80s %12s %12s %8s %8s %8s %10s", "case", "v1 ns/op", "v2 ns/op", "v1/v2", "v1 allocs", "v2 allocs", "alloc Δ")
	for _, r := range rows {
		t.Logf("%-80s %12.0f %12.0f %7.2fx %8.0f %8.0f %9s",
			truncateName(r.name, 80),
			r.v1ns, r.v2ns,
			r.v1OverV2(),
			r.v1alloc, r.v2alloc,
			formatPct(r.v2alloc-r.v1alloc, r.v1alloc))
	}
	t.Logf("")

	// Aggregate.
	t.Logf("aggregate (N=%d cases where V1 and V2 agree on output):", len(rows))
	speedups := extract(rows, func(r benchRow) float64 { return r.v1OverV2() })
	slowdowns := extract(rows, func(r benchRow) float64 { return r.v2OverV1() })
	sumV1Ns := sum(extract(rows, func(r benchRow) float64 { return r.v1ns }))
	sumV2Ns := sum(extract(rows, func(r benchRow) float64 { return r.v2ns }))
	sumV1Alloc := sum(extract(rows, func(r benchRow) float64 { return r.v1alloc }))
	sumV2Alloc := sum(extract(rows, func(r benchRow) float64 { return r.v2alloc }))
	sumV1Bytes := sum(extract(rows, func(r benchRow) float64 { return r.v1bytes }))
	sumV2Bytes := sum(extract(rows, func(r benchRow) float64 { return r.v2bytes }))

	t.Logf("  V2 speed-up (v1/v2): geomean=%.2fx  median=%.2fx  min=%.2fx  max=%.2fx  p95(slowest)=%.2fx",
		geomean(speedups),
		median(speedups),
		minf(speedups),
		maxf(speedups),
		percentile(slowdowns, 95),
	)
	if sumV2Ns > 0 {
		t.Logf("  V2 ns/op (summed): v1=%.0f  v2=%.0f  overall=%.2fx faster",
			sumV1Ns, sumV2Ns, sumV1Ns/sumV2Ns)
	}
	t.Logf("  V2 allocs/op (summed): v1=%.0f  v2=%.0f  delta=%s",
		sumV1Alloc, sumV2Alloc, formatPct(sumV2Alloc-sumV1Alloc, sumV1Alloc))
	t.Logf("  V2 B/op (summed): v1=%.0f  v2=%.0f  delta=%s",
		sumV1Bytes, sumV2Bytes, formatPct(sumV2Bytes-sumV1Bytes, sumV1Bytes))

	// Win/loss/tie split.
	faster, slower, tied := 0, 0, 0
	for _, r := range rows {
		switch {
		case r.v2ns < r.v1ns*0.98:
			faster++
		case r.v2ns > r.v1ns*1.02:
			slower++
		default:
			tied++
		}
	}
	t.Logf("  case split: V2 faster=%d  V2 slower=%d  tied=%d (±2%%)", faster, slower, tied)
}

// benchRow is one per-case benchmark sample.
type benchRow struct {
	name    string
	v1ns    float64
	v2ns    float64
	v1alloc float64
	v2alloc float64
	v1bytes float64
	v2bytes float64
}

func (r benchRow) v1OverV2() float64 {
	if r.v2ns == 0 {
		return math.Inf(1)
	}
	return r.v1ns / r.v2ns
}

func (r benchRow) v2OverV1() float64 {
	if r.v1ns == 0 {
		return math.Inf(1)
	}
	return r.v2ns / r.v1ns
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// timing bundles per-iteration ns/op, allocs/op, and B/op measured by
// timeExec. Zeroed result means "no measurement" and is skipped by the
// caller.
type timing struct {
	ns     float64
	allocs float64
	bytes  float64
}

// timeExec measures exec under a fixed wall-clock budget. It runs a
// short warm-up / calibration phase to estimate iteration cost, then
// a measurement phase sized to fill `budget`. Memory stats come from
// runtime.MemStats (same source testing.B uses under the hood), with a
// forced GC before the measurement window so only per-iteration
// allocations count.
func timeExec(exec func() error, budget time.Duration) timing {
	// Calibration: 3 iterations to estimate cost.
	const calIter = 3
	for range calIter {
		if err := exec(); err != nil {
			return timing{}
		}
	}
	start := time.Now()
	for range calIter {
		if err := exec(); err != nil {
			return timing{}
		}
	}
	per := time.Since(start) / calIter
	if per <= 0 {
		per = time.Microsecond
	}
	n := int(budget / per)
	if n < 1 {
		n = 1
	}

	// Measurement.
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	start = time.Now()
	for range n {
		if err := exec(); err != nil {
			return timing{}
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&m1)

	return timing{
		ns:     float64(elapsed.Nanoseconds()) / float64(n),
		allocs: float64(m1.Mallocs-m0.Mallocs) / float64(n),
		bytes:  float64(m1.TotalAlloc-m0.TotalAlloc) / float64(n),
	}
}

func formatPct(delta, base float64) string {
	if base == 0 {
		if delta == 0 {
			return "    0%"
		}
		return "   n/a"
	}
	p := delta / base * 100
	sign := "+"
	if p < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%5.1f%%", sign, p)
}

func truncateName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n+1:]
}

func extract(rows []benchRow, fn func(benchRow) float64) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = fn(r)
	}
	return out
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func minf(xs []float64) float64 {
	m := math.Inf(1)
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}

func maxf(xs []float64) float64 {
	m := math.Inf(-1)
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func geomean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sumLn := 0.0
	count := 0
	for _, x := range xs {
		if x > 0 {
			sumLn += math.Log(x)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Exp(sumLn / float64(count))
}

// percentile returns the p-th percentile (0–100) using nearest-rank.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	idx := int(math.Ceil(p/100*float64(len(s)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}
