package syntax

import (
	"strings"
	"testing"
	"time"
)

// parseWithin runs Parse in a goroutine and fails if it does not return within
// the timeout — catching both the O(n^2) blowup and the infinite-loop /
// stack-overflow failure modes the depth cap must prevent.
func parseWithin(t *testing.T, src string, d time.Duration) []PosError {
	t.Helper()
	done := make(chan []PosError, 1)
	go func() {
		_, errs := Parse(src, "", nil)
		done <- errs
	}()
	select {
	case errs := <-done:
		return errs
	case <-time.After(d):
		t.Fatalf("Parse did not return within %s — hang / O(n^2) / stack overflow", d)
		return nil
	}
}

// TestParseDeepNestingBounded is the regression for the O(n^2) parenthesis
// lookahead AND the unbounded parseExpr recursion. Every deeply nested
// container form — parens, arrays, objects — must fail fast with an error,
// not run for minutes, overflow the stack, or (the subtler bug) spin forever
// in a loop-based literal parser whose parseExpr never advances.
func TestParseDeepNestingBounded(t *testing.T) {
	const n = 30000
	cases := map[string]string{
		"parens":  "output = " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n),
		"arrays":  "output = " + strings.Repeat("[", n) + "1" + strings.Repeat("]", n),
		"objects": "output = " + strings.Repeat(`{"a":`, n) + "1" + strings.Repeat("}", n),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			errs := parseWithin(t, src, 10*time.Second)
			if len(errs) == 0 {
				t.Fatalf("expected a parse error for %d-deep %s nesting", n, name)
			}
		})
	}
}

// TestParseUnbalancedDeepNestingTerminates is the regression for the specific
// bug the depth cap's fatal-drain fixes: an UNBALANCED deeply nested opener
// (`[[[…` with no closers, etc.) must terminate. The element/entry loops break
// only on their closing token or EOF, so once the cap trips, the parser must
// drain to EOF — otherwise the loop spins forever re-parsing the null literal
// the over-cap parseExpr returns. All balanced deep-nesting cases above happen
// to close, so this is the only test that exercises the drain path. (Was the
// live hang before the fatal flag: `[`x1001 spun; `[`x999 was clean.)
func TestParseUnbalancedDeepNestingTerminates(t *testing.T) {
	const n = 30000
	openers := map[string]string{
		"parens":  strings.Repeat("(", n),
		"arrays":  strings.Repeat("[", n),
		"objects": strings.Repeat("{", n),
		"entries": strings.Repeat(`{"a":`, n),
	}
	for name, open := range openers {
		t.Run(name, func(t *testing.T) {
			// No closing tokens: the parser must fatal-drain to EOF, not spin.
			errs := parseWithin(t, "output = "+open, 10*time.Second)
			if len(errs) == 0 {
				t.Fatalf("expected a parse error for unbalanced %d-deep %s", n, name)
			}
		})
	}
}

// TestParseNestingBoundary checks the cap boundary for each container: just
// under parses, just over fails fast (does not hang).
func TestParseNestingBoundary(t *testing.T) {
	openClose := map[string][2]string{
		"parens":  {"(", ")"},
		"arrays":  {"[", "]"},
		"objects": {`{"a":`, "}"}, // valid nested-object form: {"a": {"a": ... }}
	}
	for name, oc := range openClose {
		t.Run(name, func(t *testing.T) {
			// Exact off-by-one: maxParseDepth-1 opens is the deepest that
			// parses; maxParseDepth opens is the first that trips the cap.
			// (Measured uniform across parens/arrays/objects.) Pinning the
			// exact transition catches a `>` vs `>=` drift in the cap check.
			under := "output = " + strings.Repeat(oc[0], maxParseDepth-1) + "1" + strings.Repeat(oc[1], maxParseDepth-1)
			if errs := parseWithin(t, under, 10*time.Second); len(errs) != 0 {
				t.Errorf("exactly maxParseDepth-1 (%d) %s should parse cleanly, got: %v", maxParseDepth-1, name, errs)
			}
			over := "output = " + strings.Repeat(oc[0], maxParseDepth) + "1" + strings.Repeat(oc[1], maxParseDepth)
			if errs := parseWithin(t, over, 10*time.Second); len(errs) == 0 {
				t.Errorf("exactly maxParseDepth (%d) %s should trip the cap, got no error", maxParseDepth, name)
			}
		})
	}
}

// nestedParenTailSrc builds a VALID mapping of the shape
//
//	output = ( ( ... (  [a+ a+ ... a]  ) ... ) )
//	         \__ parens _/\___ tail ___/\_ parens _/
//
// i.e. `parens` opening '(' , then a large array literal `[` followed by
// `tail` copies of `a+` and a trailing `a`, then `]`, then `parens` closing
// ')'. The array's long `a + a + ... + a` body is the "tail" every '(' 's
// lambda-ahead scan used to re-traverse: the old O(parens * tail) lookahead
// made this parse take seconds, while the memoized scan is linear.
func nestedParenTailSrc(parens, tail int) string {
	var b strings.Builder
	b.WriteString("output = ")
	b.WriteString(strings.Repeat("(", parens))
	b.WriteByte('[')
	b.WriteString(strings.Repeat("a+", tail))
	b.WriteByte('a')
	b.WriteByte(']')
	b.WriteString(strings.Repeat(")", parens))
	return b.String()
}

// TestParseNestedParenLargeTailIsLinear is the regression for the O(n^2)
// lambda-ahead lookahead. Before memoization, each of the `parens` open-'('
// re-scanned the entire array tail looking for a '->', so a ~600KB valid
// mapping (500 parens over a 300k-term array) took ~6.8s. With the per-'('
// verdict cached, the whole parse is linear and completes well under the
// generous bound below. The bound is far above the fixed-code time (tens of
// ms) yet far below the old O(n^2) time, so it fails loudly on regression.
func TestParseNestedParenLargeTailIsLinear(t *testing.T) {
	src := nestedParenTailSrc(500, 300000) // ~600KB of valid Bloblang
	errs := parseWithin(t, src, 2*time.Second)
	if len(errs) != 0 {
		t.Fatalf("valid nested-paren-with-large-tail mapping should parse cleanly, got: %v", errs)
	}
}

// TestParseNestedParenScaling is a sanity check that parse time grows
// linearly, not quadratically, in the input size. We scale BOTH dimensions
// (paren count and tail length) together by a factor N: the old cost was
// O(parens * tail) = O(N^2), so doubling N would ~quadruple the time (~4x),
// whereas the memoized parse is O(parens + tail) = O(N), so doubling N should
// only ~double it (~2x). The 3.0 threshold sits cleanly between those regimes,
// catching a return to the quadratic lookahead while tolerating measurement
// noise on a linear parse.
func TestParseNestedParenScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling check skipped in -short mode")
	}
	time1 := timeParse(t, nestedParenTailSrc(300, 30000))
	time2 := timeParse(t, nestedParenTailSrc(600, 60000)) // both dimensions x2
	// Guard against divide-by-zero / clock granularity on very fast machines.
	if time1 < time.Millisecond {
		t.Skipf("parse too fast to measure a stable ratio (%.3fms)", float64(time1)/float64(time.Millisecond))
	}
	ratio := float64(time2) / float64(time1)
	t.Logf("N=1 -> %v, N=2 -> %v, ratio=%.2f (expect ~2x linear, ~4x if O(n^2))", time1, time2, ratio)
	if ratio > 3.0 {
		t.Fatalf("doubling the input size increased parse time %.2fx (expected ~2x for linear); "+
			"looks like O(n^2) lambda-ahead lookahead regressed", ratio)
	}
}

// timeParse parses src once and returns how long it took, failing on error.
func timeParse(t *testing.T, src string) time.Duration {
	t.Helper()
	start := time.Now()
	_, errs := Parse(src, "", nil)
	d := time.Since(start)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	return d
}

// TestParseModestNestingStillWorks guards against the depth cap being so tight
// it rejects legitimately (modestly) nested mappings.
func TestParseModestNestingStillWorks(t *testing.T) {
	const n = 50 // far below maxParseDepth, comfortably realistic
	src := "output = " + strings.Repeat("(", n) + "1 + 2" + strings.Repeat(")", n)
	if _, errs := Parse(src, "", nil); len(errs) != 0 {
		t.Fatalf("modest %d-deep nesting should parse cleanly, got errors: %v", n, errs)
	}
}
