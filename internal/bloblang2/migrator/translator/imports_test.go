package translator_test

import (
	"path"
	"strings"
	"testing"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestFileResolverSimple — a single import resolved via FileResolver.
// Confirms the resolver is consulted, canonical key is honoured, and
// the imported file appears in Report.V2Files under the canonical key.
func TestFileResolverSimple(t *testing.T) {
	v1Helpers := `map double { root = this * 2 }`
	v1Main := `import "./helpers.blobl"
root.x = 21.apply("double")
`
	rep, err := translator.Migrate(v1Main, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			if parentKey == "" && importPath == "./helpers.blobl" {
				return "/abs/helpers.blobl", v1Helpers, true
			}
			return "", "", false
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if rep.V2Files == nil {
		t.Fatalf("expected Report.V2Files to be populated")
	}
	// Canonical-key emission: V2Files should be keyed by the canonical
	// key the resolver returned.
	if _, ok := rep.V2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected V2Files to contain canonical key /abs/helpers.blobl, got keys: %v", keys(rep.V2Files))
	}
}

// TestFileResolverTransitive — A imports B, B imports C, all resolved
// via FileResolver. parentKey for B's import should be A's canonical
// key, and the entire closure should land in V2Files.
func TestFileResolverTransitive(t *testing.T) {
	files := map[string]string{
		"/abs/a.blobl": `import "./b.blobl"
map a_helper { root = this.b_helper.apply() }
`,
		"/abs/b.blobl": `import "./c.blobl"
map b_helper { root = this.c_helper.apply() }
`,
		"/abs/c.blobl": `map c_helper { root = this * 3 }`,
	}
	v1Main := `import "/abs/a.blobl"
root.x = 7.apply("a_helper")
`
	var resolverCalls int
	rep, err := translator.Migrate(v1Main, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			resolverCalls++
			var canonical string
			if strings.HasPrefix(importPath, "/") {
				canonical = importPath
			} else {
				canonical = path.Join(path.Dir(parentKey), importPath)
			}
			content, ok := files[canonical]
			return canonical, content, ok
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, want := range []string{"/abs/a.blobl", "/abs/b.blobl", "/abs/c.blobl"} {
		if _, ok := rep.V2Files[want]; !ok {
			t.Fatalf("expected V2Files to contain %q, got keys: %v", want, keys(rep.V2Files))
		}
	}
	if resolverCalls < 3 {
		t.Fatalf("expected resolver to fire at least 3 times, got %d", resolverCalls)
	}
}

// TestFileResolverDedupesByCanonicalKey — two imports resolving to the
// same canonical key should result in a single V2Files entry, not two.
func TestFileResolverDedupesByCanonicalKey(t *testing.T) {
	v1Helpers := `map double { root = this * 2 }`
	v1Main := `import "./helpers.blobl"
import "helpers.blobl"
root.x = 21.apply("double")
`
	var resolverCalls int
	rep, err := translator.Migrate(v1Main, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			resolverCalls++
			return "/canonical/helpers.blobl", v1Helpers, true
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := len(rep.V2Files); got != 1 {
		t.Fatalf("expected 1 V2 file (deduped), got %d: %v", got, keys(rep.V2Files))
	}
	if resolverCalls != 2 {
		t.Fatalf("expected resolver to fire once per import site, got %d", resolverCalls)
	}
}

// TestFileResolverUnresolvedFlagsUnsupported — a resolver that returns
// ok=false should produce an Unsupported RuleImportStatement change
// at the import site, and the V2 source should NOT contain the import.
func TestFileResolverUnresolvedFlagsUnsupported(t *testing.T) {
	rep, err := translator.Migrate(`import "./missing.blobl"
root.x = "hi"
`, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "", "", false
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var sawUnsupportedImport bool
	for _, c := range rep.Changes {
		if c.RuleID == translator.RuleImportStatement && c.Severity == translator.SeverityError {
			sawUnsupportedImport = true
		}
	}
	if !sawUnsupportedImport {
		t.Fatalf("expected an Unsupported RuleImportStatement change, got: %v", rep.Changes)
	}
	if strings.Contains(rep.V2Mapping, "import") {
		t.Fatalf("V2 source should not contain a dropped import statement, got:\n%s", rep.V2Mapping)
	}
}

// TestNoResolverNoFilesPreservesLegacyBehaviour — when neither Files
// nor FileResolver is set, the V2 import statement is emitted verbatim
// (legacy behaviour). The V2 sanity-check parse will still complain
// about the unresolved import via RuleEmittedInvalidV2.
func TestNoResolverNoFilesPreservesLegacyBehaviour(t *testing.T) {
	rep, err := translator.Migrate(`import "./helpers.blobl"
root.x = "hi"
`, translator.Options{MinCoverage: 0})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, `import "./helpers.blobl"`) {
		t.Fatalf("expected legacy V2 source to contain verbatim import, got:\n%s", rep.V2Mapping)
	}
}

// TestV2ImportPathRewriter — V1 path strings are rewritten in the
// emitted V2 source, but canonical keys in V2Files are unaffected.
func TestV2ImportPathRewriter(t *testing.T) {
	v1Helpers := `map double { root = this * 2 }`
	rep, err := translator.Migrate(`import "./helpers.blobl"
root.x = 21.apply("double")
`, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", v1Helpers, true
		},
		V2ImportPathRewriter: func(p string) string {
			return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, `import "./helpers.v5.blobl"`) {
		t.Fatalf("expected rewritten V2 import path, got:\n%s", rep.V2Mapping)
	}
	// Canonical-key emission is unaffected by the rewriter.
	if _, ok := rep.V2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected V2Files to use canonical key (rewriter affects emitted source only), got keys: %v", keys(rep.V2Files))
	}
}

// TestFilesTakesPrecedenceOverResolver — pre-populated Files entries
// shadow the resolver. Useful for in-memory test fixtures and for
// callers who want to override specific imports.
func TestFilesTakesPrecedenceOverResolver(t *testing.T) {
	var resolverCalled bool
	rep, err := translator.Migrate(`import "helpers.blobl"
root.x = 21.apply("double")
`, translator.Options{
		MinCoverage: 0,
		Files: map[string]string{
			"helpers.blobl": `map double { root = this * 2 }`,
		},
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			resolverCalled = true
			return "should-not-happen", "should-not-happen", true
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if resolverCalled {
		t.Fatalf("resolver should not be called when Files satisfies the import")
	}
	if _, ok := rep.V2Files["helpers.blobl"]; !ok {
		t.Fatalf("expected V2Files keyed by Files entry's path, got: %v", keys(rep.V2Files))
	}
}

// TestFromOnlyInlinesResolvedContent — `from "path"` as the entire
// V1 mapping body is replaced by the migrated V2 content of the
// referenced file. The Report records a Rewritten RuleFromStatement
// change.
func TestFromOnlyInlinesResolvedContent(t *testing.T) {
	helpers := `root.id = this.id
root.upper_name = this.name.uppercase()
`
	rep, err := translator.Migrate(`from "./helpers.blobl"`, translator.Options{
		MinCoverage: 0,
		Verbose:     true,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			if importPath == "./helpers.blobl" {
				return "/abs/helpers.blobl", helpers, true
			}
			return "", "", false
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "output.id") {
		t.Fatalf("expected helpers.blobl to be migrated and inlined, got:\n%s", rep.V2Mapping)
	}
	if strings.Contains(rep.V2Mapping, "from ") {
		t.Fatalf(`V2 output should not contain a "from" statement, got:\n%s`, rep.V2Mapping)
	}
	var sawFromRewrite bool
	for _, c := range rep.Changes {
		if c.RuleID == translator.RuleFromStatement && c.Severity == translator.SeverityInfo {
			sawFromRewrite = true
		}
	}
	if !sawFromRewrite {
		t.Fatalf("expected a Rewritten RuleFromStatement change, got: %v", rep.Changes)
	}
}

// TestFromTransitiveInlining — A from-references B, B from-references C.
// The closure walker resolves all three and the migrator inlines C's
// content into B and B's (which is C's) into A.
func TestFromTransitiveInlining(t *testing.T) {
	files := map[string]string{
		"/abs/a.blobl": `from "./b.blobl"`,
		"/abs/b.blobl": `from "./c.blobl"`,
		"/abs/c.blobl": `root.kind = "leaf"`,
	}
	rep, err := translator.Migrate(`from "/abs/a.blobl"`, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			var canonical string
			if strings.HasPrefix(importPath, "/") {
				canonical = importPath
			} else {
				// Resolve relative paths against parent's directory.
				canonical = "/abs/" + strings.TrimPrefix(importPath, "./")
			}
			content, ok := files[canonical]
			return canonical, content, ok
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, `output.kind = "leaf"`) {
		t.Fatalf("expected transitive inlining to surface c.blobl content, got:\n%s", rep.V2Mapping)
	}
}

// TestFromUnresolvedFallsThroughToUnsupported — when the closure
// walker can't resolve the from path, the existing Unsupported
// behaviour is preserved (no inlining, RuleFromStatement Error).
func TestFromUnresolvedFallsThroughToUnsupported(t *testing.T) {
	_, err := translator.Migrate(`from "./missing.blobl"`, translator.Options{
		MinCoverage: 0.5,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "", "", false
		},
	})
	cerr, ok := err.(*translator.CoverageError)
	if !ok {
		t.Fatalf("expected *CoverageError, got %T: %v", err, err)
	}
	var sawFromUnsupported bool
	for _, c := range cerr.Report.Changes {
		if c.RuleID == translator.RuleFromStatement {
			sawFromUnsupported = true
		}
	}
	if !sawFromUnsupported {
		t.Fatalf("expected a RuleFromStatement change, got: %v", cerr.Report.Changes)
	}
}

// TestFromMixedWithOtherStmtsFallsThroughToUnsupported — `from`
// alongside other statements is not the simple whole-mapping replace
// case. Falls through to current Unsupported behaviour.
func TestFromMixedWithOtherStmtsFallsThroughToUnsupported(t *testing.T) {
	helpers := `root.id = this.id`
	rep, err := translator.Migrate(`from "./helpers.blobl"
root.extra = "foo"
`, translator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", helpers, true
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// The `from` should remain Unsupported because it's mixed with
	// other statements; we don't try to inline in that case.
	var sawUnsupported bool
	for _, c := range rep.Changes {
		if c.RuleID == translator.RuleFromStatement && c.Severity == translator.SeverityError {
			sawUnsupported = true
		}
	}
	if !sawUnsupported {
		t.Fatalf("expected mixed-statement from to fall through to Unsupported, got: %v", rep.Changes)
	}
}

// TestFromCycleDoesNotInfiniteLoop — A from B, B from A. The closure
// walker dedups by canonical key so it terminates; the fixpoint can
// make no progress (each side waits for the other) and the
// post-fixpoint cleanup translates them anyway. This test asserts
// the migrator returns rather than hanging, regardless of the
// (broken) input semantics.
func TestFromCycleDoesNotInfiniteLoop(t *testing.T) {
	files := map[string]string{
		"/abs/a.blobl": `from "/abs/b.blobl"`,
		"/abs/b.blobl": `from "/abs/a.blobl"`,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = translator.Migrate(`from "/abs/a.blobl"`, translator.Options{
			MinCoverage: 0,
			Verbose:     true,
			FileResolver: func(parentKey, importPath string) (string, string, bool) {
				content, ok := files[importPath]
				return importPath, content, ok
			},
		})
	}()
	select {
	case <-done:
		// Returned cleanly — exact V2 content for cyclic input is not
		// well-defined (it's broken V1) so we don't assert on it.
	case <-time.After(2 * time.Second):
		t.Fatalf("Migrate hung on a from-cycle input — fixpoint or closure walker is non-terminating")
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
