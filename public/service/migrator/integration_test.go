// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/service"
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

// integrationEnv is a shared helper for the integration tests that
// follow. Each test gets its own tmpdir with configs/ and mappings/
// subdirectories; the resolver mirrors the connect CLI's policy.
type integrationEnv struct {
	t           *testing.T
	root        string
	configsDir  string
	mappingsDir string
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	root := t.TempDir()
	configsDir := filepath.Join(root, "configs")
	mappingsDir := filepath.Join(root, "mappings")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.MkdirAll(mappingsDir, 0o755); err != nil {
		t.Fatalf("mkdir mappings: %v", err)
	}
	return &integrationEnv{t: t, root: root, configsDir: configsDir, mappingsDir: mappingsDir}
}

func (e *integrationEnv) writeMapping(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.mappingsDir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatalf("write mapping %s: %v", name, err)
	}
	return p
}

func (e *integrationEnv) writeConfig(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.configsDir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatalf("write config %s: %v", name, err)
	}
	return p
}

// resolverFor anchors importPath to configDir for top-level imports
// (parentKey == "") and to the parent file's directory for
// transitive imports. Mirrors the resolver the connect CLI installs.
func (e *integrationEnv) resolverFor(configDir string) func(string, string) (string, string, bool) {
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

func v5SuffixRewriter(p string) string {
	return strings.TrimSuffix(p, ".blobl") + ".v5.blobl"
}

// TestEndToEndResourceFile confirms the migrator handles top-level
// resource definitions (processor_resources, cache_resources) the
// same way it handles pipeline.processors. Resource files are a
// distinct connect config shape — users factor shared processors
// into them — so this exercises the walker through the manager
// fields rather than the stream pipeline fields.
func TestEndToEndResourceFile(t *testing.T) {
	env := newIntegrationEnv(t)

	env.writeMapping("enrich.blobl", `map enricher {
  root = this.uppercase()
}
`)
	env.writeMapping("tag.blobl", `root.tagged = true
root.kind = "event"
`)

	yamlPath := env.writeConfig("resources.yaml", `
processor_resources:
  - label: enrich_user
    mapping: |
      import "../mappings/enrich.blobl"
      root.id = this.id.apply("enricher")
  - label: tag_event
    bloblang: 'from "../mappings/tag.blobl"'

cache_resources:
  - label: my_cache
    memory: {}
`)

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	rep, err := migrator.Migrate(yamlBytes, migrator.Options{
		BloblangFileResolver:         env.resolverFor(filepath.Dir(yamlPath)),
		BloblangV2ImportPathRewriter: v5SuffixRewriter,
		Verbose:                      true,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Errorf("expected mapping resource to migrate to bloblang_v2:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Errorf("expected from-only resource to migrate to bloblang_v2_file:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "../mappings/tag.v5.blobl") {
		t.Errorf("expected rewritten file path on bloblang_v2_file resource:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "../mappings/enrich.v5.blobl") {
		t.Errorf("expected rewritten import path inside V2 mapping body:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "label: enrich_user") || !strings.Contains(rep.OutputYAML, "label: tag_event") {
		t.Errorf("expected resource labels preserved:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "label: my_cache") || !strings.Contains(rep.OutputYAML, "memory:") {
		t.Errorf("expected cache_resources entry preserved untouched:\n%s", rep.OutputYAML)
	}

	for _, want := range []string{"enrich.blobl", "tag.blobl"} {
		abs := filepath.Join(env.mappingsDir, want)
		if _, ok := rep.BloblangV2Files[abs]; !ok {
			t.Errorf("expected BloblangV2Files to contain %q, got: %v", abs, v2FileKeysSorted(rep.BloblangV2Files))
		}
	}
	if rep.Coverage.Rewritten != 2 {
		t.Errorf("expected 2 rewrites, got %+v", rep.Coverage)
	}
}

// TestEndToEndMixedV1V2Processors covers the realistic mid-migration
// state: a single config containing both V1 (bloblang/mapping/
// mutation) and V2 (bloblang_v2) processors. V1 entries migrate;
// V2 entries are left strictly alone (no rule registered for the
// bloblang_v2 target) and the resulting YAML lints cleanly under
// StreamConfigLinter.
func TestEndToEndMixedV1V2Processors(t *testing.T) {
	env := newIntegrationEnv(t)

	// Self-contained bodies (no imports) so the lint check below
	// doesn't have to chase relative-path V2 import resolution. The
	// test's value is showing V1 entries migrate while V2 entries
	// stay put — import handling is exercised in the other
	// integration tests.
	yamlPath := env.writeConfig("mixed.yaml", `
pipeline:
  processors:
    - mapping: |
        root.x = this.x.uppercase()
    - bloblang_v2: |
        output = input
        output.kind = "v2-already"
    - mutation: |
        root.flag = true
`)

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	rep, err := migrator.Migrate(yamlBytes, migrator.Options{
		BloblangFileResolver:         env.resolverFor(filepath.Dir(yamlPath)),
		BloblangV2ImportPathRewriter: v5SuffixRewriter,
		Verbose:                      true,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Two V1 entries become bloblang_v2; the existing bloblang_v2
	// stays bloblang_v2. So the output should contain three
	// bloblang_v2 keys total, no V1 processor names.
	if got := strings.Count(rep.OutputYAML, "bloblang_v2:"); got != 3 {
		t.Errorf("expected 3 bloblang_v2 entries in output (2 migrated + 1 untouched), got %d:\n%s", got, rep.OutputYAML)
	}
	for _, v1Name := range []string{"mapping:", "mutation:"} {
		if strings.Contains(rep.OutputYAML, "    - "+v1Name) || strings.Contains(rep.OutputYAML, "  - "+v1Name) {
			t.Errorf("V1 processor name %q leaked into output:\n%s", v1Name, rep.OutputYAML)
		}
	}
	// Confirm the untouched V2 processor's body still says "v2-already".
	if !strings.Contains(rep.OutputYAML, `output.kind = "v2-already"`) {
		t.Errorf("expected pre-existing bloblang_v2 body untouched, got:\n%s", rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 2 {
		t.Errorf("expected 2 rewrites (V2 entry shouldn't match a rule), got %+v", rep.Coverage)
	}

	// Output should lint cleanly under StreamConfigLinter.
	schema := service.GlobalEnvironment().FullConfigSchema("", "")
	linter := schema.NewStreamConfigLinter()
	lints, err := linter.LintYAML([]byte(rep.OutputYAML))
	if err != nil {
		t.Fatalf("lint output: %v", err)
	}
	for _, l := range lints {
		t.Errorf("unexpected lint on migrated config: %+v", l)
	}
}

// TestEndToEndImportsInsideSwitchAndBranch confirms the walker
// descends into control-flow processor types (switch, branch) and
// that bloblang processors nested inside them have their imports
// resolved against the YAML's directory (parentKey stays "" for any
// depth of nesting in the same YAML body). Also confirms that
// bloblang STRING fields like branch.request_map are NOT touched.
func TestEndToEndImportsInsideSwitchAndBranch(t *testing.T) {
	env := newIntegrationEnv(t)

	env.writeMapping("users.blobl", `map normalise {
  root = this.uppercase()
}
`)
	env.writeMapping("fallback.blobl", `root.fallback = true
root.kind = "fallback"
`)

	yamlPath := env.writeConfig("nested.yaml", `
pipeline:
  processors:
    - switch:
        - check: this.kind == "user"
          processors:
            - mapping: |
                import "../mappings/users.blobl"
                root.id = this.id.apply("normalise")
        - processors:
            - branch:
                request_map: 'root = this.payload'
                processors:
                  - bloblang: 'from "../mappings/fallback.blobl"'
                result_map: 'root.enriched = this'
`)

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	rep, err := migrator.Migrate(yamlBytes, migrator.Options{
		BloblangFileResolver:         env.resolverFor(filepath.Dir(yamlPath)),
		BloblangV2ImportPathRewriter: v5SuffixRewriter,
		Verbose:                      true,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Errorf("expected nested mapping inside switch to migrate:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Errorf("expected nested from-only inside branch to migrate to bloblang_v2_file:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "../mappings/users.v5.blobl") {
		t.Errorf("expected rewritten import in switch-nested mapping body:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "../mappings/fallback.v5.blobl") {
		t.Errorf("expected rewritten path on branch-nested bloblang_v2_file:\n%s", rep.OutputYAML)
	}

	// branch.request_map / branch.result_map are bloblang STRING
	// fields, not component instances. The walker should not have
	// migrated them; the original V1 strings should survive verbatim.
	if !strings.Contains(rep.OutputYAML, "root = this.payload") {
		t.Errorf("expected branch.request_map (string field) to be untouched:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "root.enriched = this") {
		t.Errorf("expected branch.result_map (string field) to be untouched:\n%s", rep.OutputYAML)
	}

	for _, want := range []string{"users.blobl", "fallback.blobl"} {
		abs := filepath.Join(env.mappingsDir, want)
		if _, ok := rep.BloblangV2Files[abs]; !ok {
			t.Errorf("expected BloblangV2Files to contain %q, got: %v", abs, v2FileKeysSorted(rep.BloblangV2Files))
		}
	}
	if rep.Coverage.Rewritten != 2 {
		t.Errorf("expected 2 nested processors rewritten, got %+v", rep.Coverage)
	}
}

// TestEndToEndDiamondImports exercises closure-walker dedup under a
// non-tree shape: A imports B and C, both B and C import D. D should
// appear exactly once in BloblangV2Files (deduped by canonical key)
// and the resolver should fire once per unique import site, not once
// per visit-via-some-path.
func TestEndToEndDiamondImports(t *testing.T) {
	env := newIntegrationEnv(t)

	env.writeMapping("d.blobl", `map d_helper {
  root = this.uppercase()
}
`)
	env.writeMapping("b.blobl", `import "./d.blobl"

map from_b_helper {
  root = this.apply("d_helper")
}
`)
	env.writeMapping("c.blobl", `import "./d.blobl"

map from_c_helper {
  root = this.apply("d_helper")
}
`)
	env.writeMapping("a.blobl", `import "./b.blobl"
import "./c.blobl"

map from_a {
  root = {
    "from_b": this.apply("from_b_helper"),
    "from_c": this.apply("from_c_helper")
  }
}
`)

	yamlPath := env.writeConfig("diamond.yaml", `
pipeline:
  processors:
    - mapping: |
        import "../mappings/a.blobl"
        root = this.x.apply("from_a")
`)

	// Wrap the resolver with a per-site call counter.
	innerResolver := env.resolverFor(filepath.Dir(yamlPath))
	siteCalls := map[string]int{}
	resolver := func(parentKey, importPath string) (string, string, bool) {
		key := parentKey + "::" + importPath
		siteCalls[key]++
		return innerResolver(parentKey, importPath)
	}

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	rep, err := migrator.Migrate(yamlBytes, migrator.Options{
		BloblangFileResolver:         resolver,
		BloblangV2ImportPathRewriter: v5SuffixRewriter,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The closure should contain exactly four files (A, B, C, D),
	// not five (D should not be duplicated).
	if got := len(rep.BloblangV2Files); got != 4 {
		t.Errorf("expected 4 V2 files in closure (A, B, C, D), got %d: %v", got, v2FileKeysSorted(rep.BloblangV2Files))
	}
	for _, want := range []string{"a.blobl", "b.blobl", "c.blobl", "d.blobl"} {
		abs := filepath.Join(env.mappingsDir, want)
		if _, ok := rep.BloblangV2Files[abs]; !ok {
			t.Errorf("expected BloblangV2Files to contain %q, got: %v", abs, v2FileKeysSorted(rep.BloblangV2Files))
		}
	}

	// Resolver firing pattern: each unique (parentKey, importPath)
	// site fires once. Five sites total — main->a, a->b, a->c, b->d,
	// c->d — and crucially the (b->d) and (c->d) calls are both
	// counted (different parentKey) but the closure walker dedupes
	// the resulting canonical so D only translates once.
	if got := len(siteCalls); got != 5 {
		t.Errorf("expected 5 unique resolver sites (main->a, a->b, a->c, b->d, c->d), got %d: %v", got, siteCalls)
	}
	for site, n := range siteCalls {
		if n != 1 {
			t.Errorf("resolver fired %d times for site %q (should be once per unique site)", n, site)
		}
	}

	// Both b.v5.blobl and c.v5.blobl should rewrite their D import
	// to d.v5.blobl — confirming the rewriter applies in
	// transitively-translated files.
	bV2 := rep.BloblangV2Files[filepath.Join(env.mappingsDir, "b.blobl")]
	cV2 := rep.BloblangV2Files[filepath.Join(env.mappingsDir, "c.blobl")]
	for name, content := range map[string]string{"b": bV2, "c": cV2} {
		if !strings.Contains(content, `import "./d.v5.blobl"`) {
			t.Errorf("%s.v5.blobl should rewrite its d import to d.v5.blobl, got:\n%s", name, content)
		}
	}
}

// TestEndToEndPartialFailure covers the realistic case where one
// config contains a mix of resolvable and unresolvable imports.
// The migrator must rewrite the resolvable processor cleanly,
// flag the unresolvable one as Unsupported (leaving the V1 in
// place), and surface both outcomes in the report — without
// aborting the whole migration.
func TestEndToEndPartialFailure(t *testing.T) {
	env := newIntegrationEnv(t)

	env.writeMapping("exists.blobl", `map noop {
  root = this
}
`)
	// Note: missing.blobl is NOT created on disk.

	yamlPath := env.writeConfig("partial.yaml", `
pipeline:
  processors:
    - mapping: |
        import "../mappings/exists.blobl"
        root = this.apply("noop")
    - bloblang: 'from "../mappings/missing.blobl"'
`)

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	rep, err := migrator.Migrate(yamlBytes, migrator.Options{
		BloblangFileResolver:         env.resolverFor(filepath.Dir(yamlPath)),
		BloblangV2ImportPathRewriter: v5SuffixRewriter,
		Verbose:                      true,
	})
	if err != nil {
		t.Fatalf("migrate should not fail outright on a partial-failure config: %v", err)
	}

	// First processor migrated successfully.
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Errorf("expected first (resolvable) processor migrated:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "../mappings/exists.v5.blobl") {
		t.Errorf("expected rewritten import for resolved file:\n%s", rep.OutputYAML)
	}

	// Second processor left in place — original V1 should survive.
	if strings.Contains(rep.OutputYAML, "bloblang_v2_file:") {
		t.Errorf("rule must not emit bloblang_v2_file when from target is unresolved:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "from \"../mappings/missing.blobl\"") {
		t.Errorf("expected V1 from body preserved on Unsupported:\n%s", rep.OutputYAML)
	}

	// BloblangV2Files holds exactly the resolved file's translation.
	if got := len(rep.BloblangV2Files); got != 1 {
		t.Errorf("expected exactly 1 entry in BloblangV2Files (the resolved file), got %d: %v", got, v2FileKeysSorted(rep.BloblangV2Files))
	}
	existsAbs := filepath.Join(env.mappingsDir, "exists.blobl")
	if _, ok := rep.BloblangV2Files[existsAbs]; !ok {
		t.Errorf("expected BloblangV2Files to contain %q, got: %v", existsAbs, v2FileKeysSorted(rep.BloblangV2Files))
	}
	missingAbs := filepath.Join(env.mappingsDir, "missing.blobl")
	if _, ok := rep.BloblangV2Files[missingAbs]; ok {
		t.Errorf("BloblangV2Files should NOT contain unresolved %q", missingAbs)
	}

	// Coverage / changes: 1 rewritten + 1 unsupported.
	if rep.Coverage.Rewritten != 1 {
		t.Errorf("expected 1 rewrite, got %+v", rep.Coverage)
	}
	if rep.Coverage.Unsupported != 1 {
		t.Errorf("expected 1 unsupported, got %+v", rep.Coverage)
	}
}
