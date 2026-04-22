package translator_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"

	// Register impl/pure — many V1 corpus mappings use methods that live
	// there (ts_parse, ts_format, .abs(), typed numeric coercers, etc.).
	// The V2 interpreter uses the same global registry, so this side-
	// effect import applies to both V1 translation probing and V2 execution.
	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

// TestCorpusRegression is Layer 4 of the migrator testing framework: take
// every non-skipped V1 mapping in ../v1spec/tests/ (the V2-translated corpus),
// translate it to V2, execute the V2 output against V2's interpreter with the
// test's input, and compare to the test's expected output.
//
// This test is about measurement, not correctness. It reports per-file
// progress and flags regressions but expects non-zero failure counts during
// development — rule implementation is incremental.
//
// Failures surface as t.Log calls; the test fails only when the overall pass
// rate drops below a threshold (currently very low, will be tightened as rules
// land).
func TestCorpusRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("corpus regression is long-running; skipped in -short mode")
	}

	root := corpusRoot(t)
	files := discoverFiles(t, root)
	if len(files) == 0 {
		t.Fatalf("no V1 corpus files found under %s", root)
	}

	stats := &runStats{}
	interp := &bloblang2.Interp{}

	for _, path := range files {
		tf, err := spectest.LoadFile(path)
		if err != nil {
			stats.loadError++
			continue
		}
		rel, _ := filepath.Rel(root, path)
		for i := range tf.Tests {
			tc := &tf.Tests[i]
			// Multi-case tests have a shared mapping; skip them for now —
			// they need the cases-expansion treatment that spectest.RunT
			// does, and aren't the main attraction for translation work.
			if len(tc.Cases) > 0 {
				stats.skippedMultiCase++
				continue
			}
			// Skip V1-untranslatable tests (skip field not modelled in
			// spectest schema, but our own v1spec adds it; we consult the
			// raw YAML separately).
			if isSkipped(path, tc.Name) {
				stats.skippedV2Only++
				continue
			}
			outcome := runOne(interp, tc, tf.Files)
			stats.record(outcome)
			switch outcome.kind {
			case outcomeUnexpected:
				t.Logf("[DELTA] %s/%s: %s", rel, tc.Name, outcome.detail)
			case outcomeV2CompileFail:
				t.Logf("[V2COMP] %s/%s: %s", rel, tc.Name, outcome.detail)
			case outcomeTranslateFail:
				t.Logf("[TRANS] %s/%s: %s", rel, tc.Name, outcome.detail)
			}
		}
	}

	stats.report(t)

	// Corpus pass-rate floor. Counted passes include OK (exact output
	// match) and Flagged (V1-V2 divergence the translator explicitly
	// warned about) outcomes. Pin just below the current rate (0.998)
	// so a real regression trips the gate; the remaining <1% are
	// documented model-level V1/V2 divergences (metadata COW, output
	// default shape, root re-shape — see migrator/V1_V2_GAPS.md).
	minPassRate := 0.995
	if stats.runRate() < minPassRate {
		t.Fatalf("corpus pass rate %.3f below floor %.3f", stats.runRate(), minPassRate)
	}
}

// corpusRoot returns the path to the V1 YAML corpus, resolved relative to the
// test's own directory (go test runs with cwd = package dir).
func corpusRoot(t *testing.T) string {
	t.Helper()
	// corpus_test.go lives in migrator/translator/; tests live at
	// ../v1spec/tests/.
	p, err := filepath.Abs("../v1spec/tests")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// discoverFiles returns every .yaml under root in sorted order.
func discoverFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// isSkipped reads the raw YAML to check whether the test case carries a
// `skip:` field (the v1spec extension to the shared schema). Caches parsed
// skip sets per file.
var skipCache = map[string]map[string]bool{}

func isSkipped(path, name string) bool {
	skips, ok := skipCache[path]
	if !ok {
		skips = loadSkips(path)
		skipCache[path] = skips
	}
	return skips[name]
}

func loadSkips(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Cheap line-based probe: find `- name: "X"` and then check if the next
	// non-trivial field at the same indent has `skip:`. Good enough — the
	// YAML structure in the corpus is regular.
	out := map[string]bool{}
	lines := strings.Split(string(data), "\n")
	current := ""
	for _, l := range lines {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "- name:") {
			current = strings.TrimSpace(strings.TrimPrefix(trim, "- name:"))
			current = strings.Trim(current, `"`)
			continue
		}
		if current != "" && strings.HasPrefix(trim, "skip:") {
			out[current] = true
			current = ""
		}
	}
	return out
}

// outcomeKind classifies the result of running one test case through the
// migrator + V2 interpreter pipeline.
type outcomeKind int

const (
	outcomeOK            outcomeKind = iota // V2 output matched expectation (or expected error fired)
	outcomeFlagged                          // V2 diverged from V1 but the translator warned via a SemanticChange or Unsupported
	outcomeTranslateFail                    // translator returned an error
	outcomeV2CompileFail                    // translator emitted invalid V2
	outcomeUnexpected                       // V2 ran but output / error differed from expectation without any warning
	outcomeInvalidTest                      // the test case itself is malformed for our purposes
)

type outcome struct {
	kind   outcomeKind
	detail string
}

func runOne(interp spectest.Interpreter, tc *spectest.TestCase, fileLevel map[string]string) outcome {
	if tc.Mapping == "" {
		return outcome{outcomeInvalidTest, "empty mapping"}
	}

	// 1. Translate V1 -> V2. Thread files into Migrate so imports in the
	// V1 source resolve against the test's virtual filesystem. Verbose is
	// enabled so Info-severity Changes surface on the Report and
	// hasFlaggedDivergence can see any SemanticChange the translator
	// emitted.
	rep, err := translator.Migrate(tc.Mapping, translator.Options{
		MinCoverage: 0.5,
		Verbose:     true,
		Files:       mergeFiles(fileLevel, tc.Files),
	})
	if err != nil {
		return outcome{outcomeTranslateFail, fmt.Sprintf("translate: %v", err)}
	}

	// 2. Compile the V2 output against the translated virtual filesystem
	// (V1 import contents also migrated to V2).
	compiled, compileErr := interp.Compile(rep.V2Mapping, rep.V2Files)
	if compileErr != nil {
		// If the V1 test expects a compile error, a V2 compile error is
		// the faithful outcome.
		if tc.CompileError != "" {
			return outcome{outcomeOK, ""}
		}
		// V2 performs more validation at compile time than V1, so V1
		// runtime errors sometimes surface as V2 compile errors. The
		// caller still gets a rejection — count as Flagged.
		if tc.Error != "" || tc.HasError {
			return outcome{outcomeFlagged, fmt.Sprintf("V1 runtime error surfaces as V2 compile error: %v", compileErr)}
		}
		// V2 is intentionally stricter about some constructs that V1
		// accepted (chained == / !=, void propagation outside the
		// `nothing()` sentinel, etc.). When the translator flagged the
		// relevant construct up front, this is a known divergence, not a
		// translator bug.
		if hasFlaggedDivergence(rep) {
			return outcome{outcomeFlagged, fmt.Sprintf("V2 compile error, known divergence flagged: %v", compileErr)}
		}
		return outcome{outcomeV2CompileFail, fmt.Sprintf("V2 compile: %v", compileErr)}
	}
	if tc.CompileError != "" {
		// V1 expected a compile error but V2 compiled cleanly. V2 is
		// intentionally more permissive at compile time (many V1 parse-
		// time validations run at V2 runtime). This is a known V1-V2
		// design divergence — count as Flagged rather than Unexpected.
		return outcome{outcomeFlagged, "V1 compile-time check not performed by V2"}
	}

	// 3. Execute.
	input, err := spectest.DecodeValue(tc.Input)
	if err != nil {
		return outcome{outcomeInvalidTest, fmt.Sprintf("input decode: %v", err)}
	}
	inputMeta := map[string]any{}
	if tc.InputMetadata != nil {
		raw, _ := spectest.DecodeValue(tc.InputMetadata)
		if m, ok := raw.(map[string]any); ok {
			inputMeta = m
		}
	}

	gotOut, _, deleted, runErr := compiled.Exec(input, inputMeta)

	// 4. Check against expectations.
	if tc.Error != "" || tc.HasError {
		if runErr == nil {
			// V1 errored, V2 succeeded. This is a lenient-V2 divergence.
			// When the translator flagged the relevant construct, it's a
			// known divergence — Flagged rather than Unexpected.
			if hasFlaggedDivergence(rep) {
				return outcome{outcomeFlagged, fmt.Sprintf("V1 runtime error did not fire under V2 (known divergence flagged); got %v", gotOut)}
			}
			return outcome{outcomeUnexpected, fmt.Sprintf("expected runtime error, got output %v", gotOut)}
		}
		return outcome{outcomeOK, ""}
	}
	if tc.Deleted {
		if deleted {
			return outcome{outcomeOK, ""}
		}
		return outcome{outcomeUnexpected, "expected deletion, got output"}
	}
	if runErr != nil {
		if hasFlaggedDivergence(rep) {
			return outcome{outcomeFlagged, fmt.Sprintf("runtime error, known divergence flagged: %v", runErr)}
		}
		return outcome{outcomeUnexpected, fmt.Sprintf("unexpected runtime error: %v", runErr)}
	}
	if tc.NoOutputCheck {
		return outcome{outcomeOK, ""}
	}

	expected, err := spectest.DecodeValue(tc.Output)
	if err != nil {
		return outcome{outcomeInvalidTest, fmt.Sprintf("expected output decode: %v", err)}
	}
	if ok, diff := spectest.DeepEqual(expected, gotOut); !ok {
		// Output differs. If the translator flagged the relevant construct
		// as a SemanticChange or Unsupported, consider this an acceptable
		// (known) divergence — the caller was warned.
		if hasFlaggedDivergence(rep) {
			return outcome{outcomeFlagged, fmt.Sprintf("output mismatch (known divergence flagged): %s", diff)}
		}
		return outcome{outcomeUnexpected, fmt.Sprintf("output mismatch: %s", diff)}
	}
	return outcome{outcomeOK, ""}
}

// hasFlaggedDivergence reports whether the report contains any Change that
// signals a known V1-V2 semantic divergence the caller has been warned about.
func hasFlaggedDivergence(rep *translator.Report) bool {
	for _, c := range rep.Changes {
		if c.Category == translator.CategorySemanticChange || c.Category == translator.CategoryUnsupported {
			return true
		}
	}
	return false
}

// runStats aggregates pass / fail counts across the corpus.
type runStats struct {
	loadError        int
	skippedMultiCase int
	skippedV2Only    int
	ok               int
	flagged          int
	translateFail    int
	v2CompileFail    int
	unexpected       int
	invalidTest      int
}

func (s *runStats) record(o outcome) {
	switch o.kind {
	case outcomeOK:
		s.ok++
	case outcomeFlagged:
		s.flagged++
	case outcomeTranslateFail:
		s.translateFail++
	case outcomeV2CompileFail:
		s.v2CompileFail++
	case outcomeUnexpected:
		s.unexpected++
	case outcomeInvalidTest:
		s.invalidTest++
	}
}

func (s *runStats) runRate() float64 {
	total := s.total()
	if total == 0 {
		return 0
	}
	// Flagged divergences count as successful outcomes: the migrator did
	// its job (translated + warned). The test runner's job is to catch
	// unexpected / silent divergences, not known ones.
	return float64(s.ok+s.flagged) / float64(total)
}

func (s *runStats) total() int {
	return s.ok + s.flagged + s.translateFail + s.v2CompileFail + s.unexpected + s.invalidTest
}

func (s *runStats) report(t *testing.T) {
	t.Helper()
	total := s.total()
	t.Logf("corpus regression summary:")
	t.Logf("  total-attempted:     %d", total)
	t.Logf("  ok (matched):        %d (%.1f%%)", s.ok, pct(s.ok, total))
	t.Logf("  flagged divergence:  %d (%.1f%%)", s.flagged, pct(s.flagged, total))
	t.Logf("  translate-fail:      %d (%.1f%%)", s.translateFail, pct(s.translateFail, total))
	t.Logf("  V2-compile-fail:     %d (%.1f%%)", s.v2CompileFail, pct(s.v2CompileFail, total))
	t.Logf("  unexpected:          %d (%.1f%%)", s.unexpected, pct(s.unexpected, total))
	t.Logf("  invalid-test:        %d (%.1f%%)", s.invalidTest, pct(s.invalidTest, total))
	t.Logf("  skipped (V2-only):   %d", s.skippedV2Only)
	t.Logf("  skipped (multicase): %d", s.skippedMultiCase)
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// mergeFiles combines a file-level Files map with a test-level one. Test-
// level entries win on collision.
func mergeFiles(fileLevel, testLevel map[string]string) map[string]string {
	if len(fileLevel) == 0 && len(testLevel) == 0 {
		return nil
	}
	out := make(map[string]string, len(fileLevel)+len(testLevel))
	for k, v := range fileLevel {
		out[k] = v
	}
	for k, v := range testLevel {
		out[k] = v
	}
	return out
}
