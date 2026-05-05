// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/service/migrator"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// TestEndToEndMultiConfigWithMixedImports is the full-shape integration
// test for the connect CLI's migrate-v5 use case:
//
//   - multiple YAML config files in their own directory
//   - each containing several bloblang/mapping/mutation processors
//   - bodies span all three patterns: inline mappings with no
//     imports, inline mappings with `import "path"` for named maps,
//     and from-only bodies (`from "path"`)
//   - import paths are relative to each YAML's directory (which is
//     not the test's CWD)
//   - imported `.blobl` files transitively import other `.blobl`
//     files (relative to their own directory, not the YAML's)
//
// The resolver mirrors what the connect CLI will install: anchor the
// main source's imports to the YAML's directory; anchor transitive
// imports to the parent file's directory (parentKey carries this).
func TestEndToEndMultiConfigWithMixedImports(t *testing.T) {
	root := t.TempDir()
	configsDir := filepath.Join(root, "configs")
	mappingsDir := filepath.Join(root, "mappings")
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require(os.MkdirAll(configsDir, 0o755))
	require(os.MkdirAll(mappingsDir, 0o755))

	// Mapping files. helpers.blobl declares one map; identifiers.blobl
	// imports helpers.blobl from a sibling directory and adds another
	// map; main.blobl is a whole-mapping body intended to be `from`'d
	// in to a processor body, with its own `import` of identifiers.
	writeFile := func(path, content string) {
		require(os.WriteFile(path, []byte(content), 0o644))
	}

	writeFile(filepath.Join(mappingsDir, "helpers.blobl"), `map upper {
  root = this.uppercase()
}
`)
	writeFile(filepath.Join(mappingsDir, "identifiers.blobl"), `import "./helpers.blobl"

map normalise {
  root = this.upper.apply()
}
`)
	writeFile(filepath.Join(mappingsDir, "main.blobl"), `import "./identifiers.blobl"

root.id = this.id.normalise.apply()
root.tag = this.tag
`)

	// Two YAML configs in configs/. Each references the mappings
	// directory via "../mappings/<file>", a path that is meaningless
	// from the test's CWD — only valid when resolved relative to the
	// YAML's own directory.
	app1Path := filepath.Join(configsDir, "app1.yaml")
	app1YAML := `
pipeline:
  processors:
    - mutation: 'root.flag = true'
    - mapping: |
        import "../mappings/identifiers.blobl"
        root.id = this.id.normalise.apply()
        root.kind = "user"
    - bloblang: 'from "../mappings/main.blobl"'
`
	writeFile(app1Path, app1YAML)

	app2Path := filepath.Join(configsDir, "app2.yaml")
	app2YAML := `
pipeline:
  processors:
    - mapping: |
        import "../mappings/helpers.blobl"
        root.upper_name = this.name.upper.apply()
    - mutation: 'from "../mappings/main.blobl"'
`
	writeFile(app2Path, app2YAML)

	// resolverFor returns a resolver that anchors importPath to the
	// supplied configDir for top-level imports (parentKey == "") and
	// to the parent file's directory for transitive imports. This is
	// the resolution policy the connect CLI will install.
	resolverFor := func(configDir string) func(string, string) (string, string, bool) {
		return func(parentKey, importPath string) (string, string, bool) {
			base := configDir
			if parentKey != "" {
				base = filepath.Dir(parentKey)
			}
			abs, err := filepath.Abs(filepath.Join(base, importPath))
			if err != nil {
				return "", "", false
			}
			content, err := os.ReadFile(abs)
			if err != nil {
				return "", "", false
			}
			return abs, string(content), true
		}
	}

	rewriter := func(p string) string {
		return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
	}

	// Migrate each YAML independently — the connect CLI iterates
	// targets the same way.
	type result struct {
		path string
		rep  *migrator.Report
	}
	results := make([]result, 0, 2)
	for _, yamlPath := range []string{app1Path, app2Path} {
		yamlBytes, err := os.ReadFile(yamlPath)
		require(err)
		rep, err := migrator.Migrate(yamlBytes, migrator.Options{
			BloblangFileResolver:         resolverFor(filepath.Dir(yamlPath)),
			BloblangV2ImportPathRewriter: rewriter,
			Verbose:                      true,
		})
		require(err)
		results = append(results, result{path: yamlPath, rep: rep})
	}

	app1Rep := results[0].rep
	app2Rep := results[1].rep

	// app1 expectations: 3 processors all rewritten.
	if got := strings.Count(app1Rep.OutputYAML, "bloblang_v2:"); got != 2 {
		t.Errorf("app1: expected 2 inline bloblang_v2 processors, got %d in:\n%s", got, app1Rep.OutputYAML)
	}
	if !strings.Contains(app1Rep.OutputYAML, "bloblang_v2_file:") {
		t.Errorf("app1: expected from-only body to migrate to bloblang_v2_file:\n%s", app1Rep.OutputYAML)
	}
	if !strings.Contains(app1Rep.OutputYAML, "../mappings/main.v5.blobl") {
		t.Errorf("app1: expected rewritten file path on bloblang_v2_file processor:\n%s", app1Rep.OutputYAML)
	}
	if !strings.Contains(app1Rep.OutputYAML, "../mappings/identifiers.v5.blobl") {
		t.Errorf("app1: expected rewritten import path inside V2 mapping body:\n%s", app1Rep.OutputYAML)
	}
	if app1Rep.Coverage.Rewritten != 3 || app1Rep.Coverage.Unsupported != 0 {
		t.Errorf("app1: unexpected coverage %+v", app1Rep.Coverage)
	}

	// app2 expectations: 2 processors, 1 inline + 1 file-backed.
	if got := strings.Count(app2Rep.OutputYAML, "bloblang_v2:"); got != 1 {
		t.Errorf("app2: expected 1 inline bloblang_v2 processor, got %d in:\n%s", got, app2Rep.OutputYAML)
	}
	if !strings.Contains(app2Rep.OutputYAML, "bloblang_v2_file:") {
		t.Errorf("app2: expected from-only body to migrate to bloblang_v2_file:\n%s", app2Rep.OutputYAML)
	}
	if !strings.Contains(app2Rep.OutputYAML, "../mappings/main.v5.blobl") {
		t.Errorf("app2: expected rewritten file path on bloblang_v2_file processor:\n%s", app2Rep.OutputYAML)
	}
	if !strings.Contains(app2Rep.OutputYAML, "../mappings/helpers.v5.blobl") {
		t.Errorf("app2: expected rewritten import path inside V2 mapping body:\n%s", app2Rep.OutputYAML)
	}

	// Each report's BloblangV2Files should contain canonical-keyed V2
	// translations for every .blobl file the YAML reaches transitively.
	// app1 reaches: identifiers (via import), main + identifiers + helpers (via from -> import -> import).
	// app2 reaches: helpers (via import), main + identifiers + helpers (via from -> import -> import).
	// In both cases the closure is {helpers, identifiers, main}.
	wantClosure := []string{
		filepath.Join(mappingsDir, "helpers.blobl"),
		filepath.Join(mappingsDir, "identifiers.blobl"),
		filepath.Join(mappingsDir, "main.blobl"),
	}
	for i, res := range results {
		for _, want := range wantClosure {
			absWant, err := filepath.Abs(want)
			require(err)
			if _, ok := res.rep.BloblangV2Files[absWant]; !ok {
				t.Errorf("yaml[%d] %s: expected BloblangV2Files to contain %q, got keys: %v",
					i, res.path, absWant, v2FileKeysSorted(res.rep.BloblangV2Files))
			}
		}
	}

	// The migrated identifiers.v5.blobl should contain the rewritten
	// import to helpers.v5.blobl — confirming the rewriter applies
	// inside transitively-translated files too.
	identifiersV2 := app1Rep.BloblangV2Files[filepath.Join(mappingsDir, "identifiers.blobl")]
	if !strings.Contains(identifiersV2, `import "./helpers.v5.blobl"`) {
		t.Errorf("identifiers.v5.blobl should rewrite its import to helpers.v5.blobl, got:\n%s", identifiersV2)
	}

	// main.blobl is a regular V1 mapping (import + root assignments),
	// not a from-only file — so its V2 translation preserves the
	// import structure with paths rewritten, plus assignments
	// translated to the output.* form.
	mainV2 := app1Rep.BloblangV2Files[filepath.Join(mappingsDir, "main.blobl")]
	if !strings.Contains(mainV2, `import "./identifiers.v5.blobl"`) {
		t.Errorf("main.v5.blobl should rewrite its import to identifiers.v5.blobl, got:\n%s", mainV2)
	}
	if !strings.Contains(mainV2, "output.id") || !strings.Contains(mainV2, "output.tag") {
		t.Errorf("main.v5.blobl should translate root.* assignments to output.*, got:\n%s", mainV2)
	}

	// Sanity: the test's working directory is not the configs dir.
	// All path resolution went through the resolver's anchoring on
	// each YAML's directory; CWD never entered the picture.
	cwd, err := os.Getwd()
	require(err)
	if filepath.Clean(cwd) == filepath.Clean(configsDir) {
		t.Fatalf("test setup invariant: cwd should differ from configsDir to prove resolver-based resolution")
	}
}

func v2FileKeysSorted(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
