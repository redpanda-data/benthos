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

func v2FileKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
