package benchmark_test

import (
	"sort"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/benchmark"
)

// TestCoverageProbe collects the corpus without running any benchmarks
// and prints a coverage breakdown. It's fast enough to run unconditionally
// so coverage regressions (e.g. a translator change that makes many
// previously-equivalent cases diverge) surface immediately.
func TestCoverageProbe(t *testing.T) {
	coll, err := benchmark.CollectDefault()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	total := len(coll.Cases) + len(coll.Skips)
	t.Logf("corpus coverage:")
	t.Logf("  total cases:    %d", total)
	t.Logf("  accepted (V1≡V2): %d  (%.1f%%)", len(coll.Cases), 100*float64(len(coll.Cases))/float64(max(total, 1)))
	t.Logf("  skipped:          %d  (%.1f%%)", len(coll.Skips), 100*float64(len(coll.Skips))/float64(max(total, 1)))
	t.Logf("")
	t.Logf("skip breakdown:")
	counts := coll.SkipCounts()
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, string(r))
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		t.Logf("  %-26s %d", r, counts[benchmark.SkipReason(r)])
	}

	// Equivalence floor (Phase 7's core promise is that the migrator
	// surfaces divergences — a translator regression that silently
	// de-equivalences corpus cases must FAIL, not log). Rates at the time
	// the floor was set: 50.7% accepted, 124 output-mismatch skips.
	// Recalibrate deliberately when the corpus or translator changes on
	// purpose — never loosen to make a red build green without
	// understanding which cases de-equivalenced.
	if rate := float64(len(coll.Cases)) / float64(max(total, 1)); rate < 0.50 {
		t.Errorf("corpus V1≡V2 acceptance rate %.1f%% fell below the 50.0%% floor — a translator change has de-equivalenced previously-matching cases", 100*rate)
	}
	if n := counts[benchmark.SkipOutputMismatch]; n > 135 {
		t.Errorf("corpus output-mismatch skips grew to %d (ceiling 135) — a translator change has introduced new V1/V2 output divergences", n)
	}
}
