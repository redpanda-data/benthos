// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"path"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestPublicFileResolver — end-to-end on the public surface, confirming
// FileResolver, V2ImportPathRewriter, and Report.V2Files behave the way
// the docstrings say.
func TestPublicFileResolver(t *testing.T) {
	v1Helpers := `map double { root = this * 2 }`
	v1Main := `import "./helpers.blobl"
root.x = 21.apply("double")
`
	rep, err := migrator.Migrate(v1Main, migrator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
			if importPath == "./helpers.blobl" && parentKey == "" {
				return "/abs/helpers.blobl", v1Helpers, true
			}
			return "", "", false
		},
		V2ImportPathRewriter: func(p string) string {
			return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, `import "./helpers.v5.blobl"`) {
		t.Fatalf("expected rewritten import in V2 source, got:\n%s", rep.V2Mapping)
	}
	if _, ok := rep.V2Files["/abs/helpers.blobl"]; !ok {
		t.Fatalf("expected canonical-keyed V2Files entry, got keys: %v", v2FileKeys(rep.V2Files))
	}
}

// TestPublicFileResolverTransitive — A imports B, B imports C; the
// resolver maintains parent-relative resolution via parentKey.
func TestPublicFileResolverTransitive(t *testing.T) {
	files := map[string]string{
		"/abs/a.blobl": `import "./b.blobl"
map a_helper { root = this.b_helper.apply() }
`,
		"/abs/b.blobl": `import "./c.blobl"
map b_helper { root = this.c_helper.apply() }
`,
		"/abs/c.blobl": `map c_helper { root = this * 3 }`,
	}
	rep, err := migrator.Migrate(`import "/abs/a.blobl"
root.x = 7.apply("a_helper")
`, migrator.Options{
		MinCoverage: 0,
		FileResolver: func(parentKey, importPath string) (string, string, bool) {
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
		t.Fatalf("migrate: %v", err)
	}
	for _, want := range []string{"/abs/a.blobl", "/abs/b.blobl", "/abs/c.blobl"} {
		if _, ok := rep.V2Files[want]; !ok {
			t.Fatalf("expected V2Files to contain %q, got: %v", want, v2FileKeys(rep.V2Files))
		}
	}
}

func v2FileKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
