// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"strings"
	"testing"

	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
	"github.com/redpanda-data/benthos/v4/public/service/migrator"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// TestBloblangFileResolverForwarded — a top-level BloblangFileResolver
// is consulted when a migrated bloblang processor body contains an
// import. The resolved file appears in Report.BloblangV2Files keyed by
// canonical key.
func TestBloblangFileResolverForwarded(t *testing.T) {
	helpers := `map double { root = this * 2 }`
	in := `
pipeline:
  processors:
    - mapping: |
        import "./helpers.blobl"
        root.x = 21.apply("double")
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			if importPath == "./helpers.blobl" && parentKey == "" {
				return "/abs/helpers.blobl", helpers, true
			}
			return "", "", false
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected processor migrated to bloblang_v2:\n%s", rep.OutputYAML)
	}
	if rep.BloblangV2Files == nil {
		t.Fatalf("expected BloblangV2Files to be populated")
	}
	if _, ok := rep.BloblangV2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected canonical-keyed import in BloblangV2Files, got: %v", v2FileKeys(rep.BloblangV2Files))
	}
}

// TestBloblangV2ImportPathRewriterForwarded — a top-level rewriter is
// applied to the import statements emitted into the V2 mapping body,
// while BloblangV2Files keys remain canonical.
func TestBloblangV2ImportPathRewriterForwarded(t *testing.T) {
	helpers := `map double { root = this * 2 }`
	in := `
pipeline:
  processors:
    - mapping: |
        import "./helpers.blobl"
        root.x = 21.apply("double")
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", helpers, true
		},
		BloblangV2ImportPathRewriter: func(p string) string {
			return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, `import "./helpers.v5.blobl"`) {
		t.Fatalf("expected rewritten import in migrated body, got:\n%s", rep.OutputYAML)
	}
	if _, ok := rep.BloblangV2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected canonical-keyed BloblangV2Files (rewriter affects emitted source only), got: %v", v2FileKeys(rep.BloblangV2Files))
	}
}

// TestBloblangV2FilesAggregatedAcrossComponents — two processors with
// the same import contribute one entry to BloblangV2Files (deduped by
// canonical key).
func TestBloblangV2FilesAggregatedAcrossComponents(t *testing.T) {
	helpers := `map double { root = this * 2 }`
	in := `
pipeline:
  processors:
    - mapping: |
        import "./helpers.blobl"
        root.x = 21.apply("double")
    - mutation: |
        import "./helpers.blobl"
        root.y = 42.apply("double")
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", helpers, true
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := len(rep.BloblangV2Files); got != 1 {
		t.Fatalf("expected exactly 1 import file (deduped across components), got %d: %v", got, v2FileKeys(rep.BloblangV2Files))
	}
}

// TestBloblangFileResolverHonoursExplicitBloblangOptions — when the
// caller pre-populates BloblangOptions.FileResolver directly (instead
// of using the top-level hook), it still works. Useful for callers
// who construct a fully custom *bloblmig.Options.
func TestBloblangFileResolverHonoursExplicitBloblangOptions(t *testing.T) {
	helpers := `map double { root = this * 2 }`
	in := `
pipeline:
  processors:
    - bloblang: |
        import "./helpers.blobl"
        root.x = 21.apply("double")
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangOptions: bloblmig.Options{
			FileResolver: func(parentKey, importPath string) (string, string, bool) {
				return "/abs/helpers.blobl", helpers, true
			},
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, ok := rep.BloblangV2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected import resolved via BloblangOptions.FileResolver, got: %v", v2FileKeys(rep.BloblangV2Files))
	}
}

// TestTopLevelResolverShadowsBloblangOptions — if both the top-level
// hook and BloblangOptions.FileResolver are set, the top-level hook
// wins. Documents the precedence so callers know which to set.
func TestTopLevelResolverShadowsBloblangOptions(t *testing.T) {
	in := `
pipeline:
  processors:
    - mapping: |
        import "./helpers.blobl"
        root.x = "ok"
`
	var topLevelCalled, embeddedCalled bool
	_, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			topLevelCalled = true
			return "/abs/helpers.blobl", `map noop { root = this }`, true
		},
		BloblangOptions: bloblmig.Options{
			FileResolver: func(parentKey, importPath string) (string, string, bool) {
				embeddedCalled = true
				return "/should-not-happen", "", true
			},
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !topLevelCalled {
		t.Fatalf("expected top-level BloblangFileResolver to be called")
	}
	if embeddedCalled {
		t.Fatalf("BloblangOptions.FileResolver should be shadowed by the top-level hook")
	}
}

// TestFromOnlyBodyRewritesToBloblangV2File — a `mapping` processor
// whose body is a single `from "path"` statement is rewritten to the
// new `bloblang_v2_file` processor. The referenced file is migrated
// V1->V2 and surfaces in Report.BloblangV2Files.
func TestFromOnlyBodyRewritesToBloblangV2File(t *testing.T) {
	helpers := `root.id = this.id
root.upper_name = this.name.uppercase()
`
	in := `
pipeline:
  processors:
    - mapping: 'from "./helpers.blobl"'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			if importPath == "./helpers.blobl" {
				return "/abs/helpers.blobl", helpers, true
			}
			return "", "", false
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Fatalf("expected from-only mapping to migrate to bloblang_v2_file:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "./helpers.blobl") {
		t.Fatalf("expected file path preserved (no rewriter set), got:\n%s", rep.OutputYAML)
	}
	if rep.BloblangV2Files == nil {
		t.Fatalf("expected BloblangV2Files to be populated")
	}
	v2 := rep.BloblangV2Files["/abs/helpers.blobl"]
	if !strings.Contains(v2, "output.id") {
		t.Fatalf("expected helpers.blobl translated to V2, got:\n%s", v2)
	}
}

// TestFromOnlyBodyAppliesPathRewriter — when a rewriter is set, the
// path emitted into the bloblang_v2_file processor reflects the V2
// path (e.g. helpers.blobl -> helpers.v5.blobl).
func TestFromOnlyBodyAppliesPathRewriter(t *testing.T) {
	helpers := `root.id = this.id`
	in := `
pipeline:
  processors:
    - bloblang: 'from "./helpers.blobl"'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", helpers, true
		},
		BloblangV2ImportPathRewriter: func(p string) string {
			return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "./helpers.v5.blobl") {
		t.Fatalf("expected rewritten path in bloblang_v2_file processor:\n%s", rep.OutputYAML)
	}
}

// TestFromOnlyBodyMutationProcessor — same handling for the mutation
// processor (uses ModeMutation but the from-only rewrite is identical).
func TestFromOnlyBodyMutationProcessor(t *testing.T) {
	helpers := `root.id = this.id`
	in := `
pipeline:
  processors:
    - mutation: 'from "./helpers.blobl"'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "/abs/helpers.blobl", helpers, true
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Fatalf("expected mutation/from-only to rewrite to bloblang_v2_file:\n%s", rep.OutputYAML)
	}
}

// TestFromOnlyBodyWithUnresolvedTargetEmitsUnsupported — if the
// resolver can't satisfy the from path, the rule MUST NOT rewrite to
// bloblang_v2_file (the resulting config would point at a file that
// won't be in Report.BloblangV2Files). Instead the rule emits
// Unsupported and leaves the V1 processor untouched.
//
// This test exercises the foundation-bug case where MigrateBloblang
// returns success because BloblangOptions.MinCoverage was set to 0,
// which would otherwise let the rule blindly emit a broken rewrite.
func TestFromOnlyBodyWithUnresolvedTargetEmitsUnsupported(t *testing.T) {
	in := `
pipeline:
  processors:
    - mapping: 'from "./missing.blobl"'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{
		BloblangFileResolver: func(parentKey, importPath string) (string, string, bool) {
			return "", "", false
		},
		// Effectively disable the bloblang coverage gate (the
		// migrator's applyDefaults clobbers 0 to 0.75, so we set a
		// positive-but-tiny floor). This makes MigrateBloblang
		// return success-with-an-unsupported-change instead of an
		// error — the bug-shaped case the rule must guard.
		BloblangOptions: bloblmig.Options{MinCoverage: 0.01},
	})
	if err != nil {
		t.Fatalf("migrate should not fail outright: %v", err)
	}
	if strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Fatalf("rule must not emit bloblang_v2_file when from target is unresolved:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "mapping:") {
		t.Fatalf("expected V1 mapping processor preserved on Unsupported, got:\n%s", rep.OutputYAML)
	}
	if len(rep.Changes) != 1 || rep.Changes[0].Outcome != migrator.OutcomeUnsupported {
		t.Fatalf("expected exactly one Unsupported change, got %+v", rep.Changes)
	}
	if !strings.Contains(rep.Changes[0].Reason, "missing.blobl") {
		t.Fatalf("expected change reason to name the unresolved path, got %q", rep.Changes[0].Reason)
	}
}

func v2FileKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
