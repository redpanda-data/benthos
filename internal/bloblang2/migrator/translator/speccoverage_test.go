package translator_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Layer 6 — spec-coverage lint. These tests don't verify translation
// correctness; they verify that the translator code, the rules_test
// assertions, and the V1 spec anchors stay in sync. A failure means
// something drifted: a RuleID was added and never emitted, a RuleID was
// emitted and never tested, or a §14#N reference points at a quirk number
// that doesn't exist in the spec.
//
// Run the tests, read the failure messages, fix the drift. Don't add an
// exemption to silence them without understanding what slipped.

// TestRuleIDCoverage asserts the invariants:
//
//  1. Every RuleID declared in change.go is emitted somewhere in the
//     translator source (translate.go / methods.go / migrate.go).
//     RuleUnknown is the zero-value sentinel and is exempt.
//  2. Every RuleID emitted by the translator is referenced by at least
//     one case in rules_test.go.
func TestRuleIDCoverage(t *testing.T) {
	declared := declaredRuleIDs(t)
	emitted := emittedRuleIDs(t)
	tested := testedRuleIDs(t)

	var declaredMissing []string
	for name := range declared {
		if name == "RuleUnknown" {
			continue
		}
		if !emitted[name] {
			declaredMissing = append(declaredMissing, name)
		}
	}
	sort.Strings(declaredMissing)
	for _, name := range declaredMissing {
		t.Errorf("RuleID %s is declared in change.go but never emitted — either wire it up or delete the constant", name)
	}

	var testedMissing []string
	for name := range emitted {
		if !tested[name] {
			testedMissing = append(testedMissing, name)
		}
	}
	sort.Strings(testedMissing)
	for _, name := range testedMissing {
		t.Errorf("RuleID %s is emitted by the translator but no rules_test.go case asserts it — add a case in ruleCases", name)
	}
}

// TestSpec14Anchors asserts that every §14#N SpecRef cited in the
// translator source corresponds to an actual numbered quirk in
// bloblang_v1_spec.md §14. Catches typos (like §14#6 meant as §14#48)
// rather than coverage gaps.
func TestSpec14Anchors(t *testing.T) {
	highest := countSpec14Quirks(t)
	if highest < 50 {
		t.Fatalf("spec quirk count (%d) looks implausible — broken parser?", highest)
	}
	for _, n := range spec14AnchorsInTranslator(t) {
		if n < 1 || n > highest {
			t.Errorf("SpecRef §14#%d cited by translator source is out of range; the spec has quirks 1-%d", n, highest)
		}
	}
}

// declaredRuleIDs returns the set of RuleID constant names declared in
// change.go. Detection is regex-based against the const block; good
// enough for our single-file declaration convention. The type name
// RuleID itself is explicitly excluded.
func declaredRuleIDs(t *testing.T) map[string]bool {
	t.Helper()
	src := mustRead(t, "change.go")
	// Isolate the RuleID const block — everything between
	// `type RuleID int` and the closing `)` of its const declaration.
	startRE := regexp.MustCompile(`type RuleID int`)
	start := startRE.FindStringIndex(src)
	if start == nil {
		t.Fatalf("could not find `type RuleID int` in change.go")
	}
	rest := src[start[1]:]
	end := strings.Index(rest, "\n)")
	if end < 0 {
		t.Fatalf("could not find end of RuleID const block")
	}
	block := rest[:end]
	// Lines starting with a tab then "Rule<Capital>..."; the bare type
	// name RuleID has no such declaration inside this block.
	re := regexp.MustCompile(`(?m)^\t(Rule[A-Z][A-Za-z0-9]*)\b`)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		if m[1] == "RuleID" {
			continue
		}
		out[m[1]] = true
	}
	return out
}

// emittedRuleIDs returns the set of RuleID names referenced via the
// `RuleID: RuleXxx` struct-literal form in non-test translator sources.
// This matches the convention the Change constructor uses at every
// emission site.
func emittedRuleIDs(t *testing.T) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`RuleID:\s*(Rule[A-Z][A-Za-z0-9]*)\b`)
	out := map[string]bool{}
	for _, file := range []string{"translate.go", "statements.go", "expressions.go", "methods.go", "migrate.go", "change.go", "context.go"} {
		src := mustRead(t, file)
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			out[m[1]] = true
		}
	}
	return out
}

// testedRuleIDs returns the set of RuleIDs referenced in rules_test.go,
// either in wantRules (positive assertions) or notRules (negative).
func testedRuleIDs(t *testing.T) map[string]bool {
	t.Helper()
	src := mustRead(t, "rules_test.go")
	// `translator.RuleXxx` — we assert only on qualified references
	// inside the test file so we don't accidentally pick up the
	// `translator.RuleID` type name.
	re := regexp.MustCompile(`translator\.(Rule[A-Z][A-Za-z0-9]*)\b`)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		if m[1] == "RuleID" {
			continue
		}
		out[m[1]] = true
	}
	return out
}

// countSpec14Quirks returns the highest numbered quirk in §14 of the V1
// spec. Scans from the `## 14.` header down to the next top-level
// section and picks the largest `^N.` line.
func countSpec14Quirks(t *testing.T) int {
	t.Helper()
	path := filepath.Join("..", "bloblang_v1_spec.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read V1 spec: %v", err)
	}
	// Isolate the §14 section.
	startRE := regexp.MustCompile(`(?m)^##\s+14\.`)
	endRE := regexp.MustCompile(`(?m)^##\s+15\.`)
	startLoc := startRE.FindIndex(raw)
	if startLoc == nil {
		t.Fatalf("could not find §14 in V1 spec")
	}
	endLoc := endRE.FindIndex(raw[startLoc[1]:])
	if endLoc == nil {
		t.Fatalf("could not find end of §14 in V1 spec")
	}
	section := string(raw[startLoc[0] : startLoc[1]+endLoc[0]])
	// Numbered quirks are `^N. **...` lines.
	quirkRE := regexp.MustCompile(`(?m)^(\d+)\. \*\*`)
	highest := 0
	for _, m := range quirkRE.FindAllStringSubmatch(section, -1) {
		var n int
		for _, r := range m[1] {
			n = n*10 + int(r-'0')
		}
		if n > highest {
			highest = n
		}
	}
	return highest
}

// spec14AnchorsInTranslator returns every §14#N number cited in the
// non-test translator source files.
func spec14AnchorsInTranslator(t *testing.T) []int {
	t.Helper()
	re := regexp.MustCompile(`§14#(\d+)`)
	seen := map[int]bool{}
	for _, file := range []string{"translate.go", "statements.go", "expressions.go", "methods.go", "migrate.go", "change.go", "context.go"} {
		src := mustRead(t, file)
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			var n int
			for _, r := range m[1] {
				n = n*10 + int(r-'0')
			}
			seen[n] = true
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// mustRead reads a file from the translator package directory (where the
// test binary runs).
func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
