// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"strings"
	"testing"

	bloblmig "github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
	"github.com/redpanda-data/benthos/v4/public/service/migrator"

	// Register the bundled processors so the schema resolves
	// `bloblang`, `mapping`, `mutation` and `bloblang_v2` during
	// walking.
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestMigrateBloblangProcessor(t *testing.T) {
	in := `
pipeline:
  processors:
    - bloblang: |
        root.id = this.id
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected bloblang_v2 in output:\n%s", rep.OutputYAML)
	}
	if strings.Contains(rep.OutputYAML, "bloblang:") {
		t.Fatalf("V1 bloblang key leaked into output:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "output.id") {
		t.Fatalf("expected V2 output.id rewrite, got:\n%s", rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 1 || rep.Coverage.Matched != 1 {
		t.Fatalf("unexpected coverage: %+v", rep.Coverage)
	}
	if len(rep.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(rep.Changes), rep.Changes)
	}
	if rep.Changes[0].Outcome != migrator.OutcomeRewritten {
		t.Fatalf("expected rewritten outcome, got %v", rep.Changes[0].Outcome)
	}
	if rep.Changes[0].NewName != "bloblang_v2" {
		t.Fatalf("expected NewName bloblang_v2, got %q", rep.Changes[0].NewName)
	}
	if rep.Changes[0].BloblangReport == nil {
		t.Fatalf("expected attached BloblangReport on change")
	}
}

func TestMigrateMappingProcessor(t *testing.T) {
	in := `
pipeline:
  processors:
    - mapping: |
        root.id = this.id
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected bloblang_v2 in output:\n%s", rep.OutputYAML)
	}
	if strings.Contains(rep.OutputYAML, "mapping:") {
		t.Fatalf("V1 mapping key leaked into output:\n%s", rep.OutputYAML)
	}
}

func TestMigrateMutationProcessor(t *testing.T) {
	in := `
pipeline:
  processors:
    - mutation: |
        root.id = this.id
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected bloblang_v2 in output:\n%s", rep.OutputYAML)
	}
	if strings.Contains(rep.OutputYAML, "mutation:") {
		t.Fatalf("V1 mutation key leaked into output:\n%s", rep.OutputYAML)
	}
}

func TestMigratePreservesLabel(t *testing.T) {
	in := `
pipeline:
  processors:
    - label: my_proc
      bloblang: |
        root = this
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "label: my_proc") {
		t.Fatalf("expected label preserved:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected bloblang_v2 alongside label:\n%s", rep.OutputYAML)
	}
	if rep.Changes[0].Label != "my_proc" {
		t.Fatalf("expected label captured on change, got %q", rep.Changes[0].Label)
	}
}

func TestMigrateMultipleProcessors(t *testing.T) {
	in := `
pipeline:
  processors:
    - bloblang: 'root.a = this.a'
    - mutation: 'root.b = this.b'
    - mapping: 'root.c = this.c'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := strings.Count(rep.OutputYAML, "bloblang_v2:"); got != 3 {
		t.Fatalf("expected 3 bloblang_v2 keys, got %d in:\n%s", got, rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 3 {
		t.Fatalf("expected 3 rewritten, got %+v", rep.Coverage)
	}
}

func TestMigrateNestedInsideSwitch(t *testing.T) {
	in := `
pipeline:
  processors:
    - switch:
        - check: this.kind == "user"
          processors:
            - mapping: |
                root.id = this.user_id
        - processors:
            - mutation: |
                root.fallback = true
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := strings.Count(rep.OutputYAML, "bloblang_v2:"); got != 2 {
		t.Fatalf("expected 2 bloblang_v2 keys, got %d in:\n%s", got, rep.OutputYAML)
	}
}

func TestMigrateNoOpWhenNoMatch(t *testing.T) {
	in := `
pipeline:
  processors:
    - log:
        message: hello
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(rep.Changes) != 0 {
		t.Fatalf("expected no changes, got %+v", rep.Changes)
	}
	if rep.Coverage.Matched != 0 {
		t.Fatalf("expected no matches, got %+v", rep.Coverage)
	}
}

func TestRegisterCustomRuleOverridesBuiltin(t *testing.T) {
	mig := migrator.New()
	mig.RegisterRule(migrator.Target{ComponentType: "processor", Name: "bloblang"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			return ctx.Unsupported("custom rule says no")
		})

	in := `
pipeline:
  processors:
    - bloblang: 'root = this'
`
	rep, err := mig.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang:") {
		t.Fatalf("expected V1 bloblang preserved (rule was unsupported):\n%s", rep.OutputYAML)
	}
	if strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("custom rule should have blocked the rewrite:\n%s", rep.OutputYAML)
	}
	if len(rep.Changes) != 1 || rep.Changes[0].Outcome != migrator.OutcomeUnsupported {
		t.Fatalf("expected one unsupported change, got %+v", rep.Changes)
	}
	if rep.Coverage.Unsupported != 1 || rep.Coverage.Ratio != 0 {
		t.Fatalf("unexpected coverage: %+v", rep.Coverage)
	}
}

func TestRegisterCustomRuleNewTarget(t *testing.T) {
	mig := migrator.New()
	mig.RegisterRule(migrator.Target{ComponentType: "processor", Name: "log"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			return ctx.ReplaceStructured("log", map[string]any{"message": "rewritten"})
		})

	in := `
pipeline:
  processors:
    - log:
        message: hello
`
	rep, err := mig.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "rewritten") {
		t.Fatalf("expected rewritten message, got:\n%s", rep.OutputYAML)
	}
}

func TestMinCoverageGate(t *testing.T) {
	mig := migrator.New()
	mig.RegisterRule(migrator.Target{ComponentType: "processor", Name: "bloblang"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			return ctx.Unsupported("nope")
		})

	in := `
pipeline:
  processors:
    - bloblang: 'root = this'
`
	_, err := mig.Migrate([]byte(in), migrator.Options{MinCoverage: 0.5})
	if err == nil {
		t.Fatalf("expected coverage error")
	}
	cerr, ok := err.(*migrator.CoverageError)
	if !ok {
		t.Fatalf("expected *CoverageError, got %T: %v", err, err)
	}
	if cerr.Report == nil {
		t.Fatalf("CoverageError should expose the Report")
	}
}

// TestModeMappingPrependsOutputInput verifies that the `mapping`
// processor is migrated using ModeMapping — the bloblang translator
// prepends `output = input` so unwritten fields pass through, matching
// V1 mapping semantics.
func TestModeMappingPrependsOutputInput(t *testing.T) {
	in := `
pipeline:
  processors:
    - mapping: 'root.id = this.id'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "output = input") {
		t.Fatalf("ModeMapping should prepend `output = input`, got:\n%s", rep.OutputYAML)
	}
}

// TestModeMappingProcessorBloblangAlsoPrepends — the `bloblang`
// processor shares semantics with `mapping`, so it must also use
// ModeMapping.
func TestModeMappingProcessorBloblangAlsoPrepends(t *testing.T) {
	in := `
pipeline:
  processors:
    - bloblang: 'root.id = this.id'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "output = input") {
		t.Fatalf("bloblang processor should use ModeMapping (prepend `output = input`), got:\n%s", rep.OutputYAML)
	}
}

// TestModeMutationDoesNotPrepend verifies that the `mutation`
// processor uses ModeMutation — V2's empty `output` aligns with V1's
// `mutation` semantics, so no prelude should be inserted.
func TestModeMutationDoesNotPrepend(t *testing.T) {
	in := `
pipeline:
  processors:
    - mutation: 'root.id = this.id'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if strings.Contains(rep.OutputYAML, "output = input") {
		t.Fatalf("ModeMutation should NOT prepend `output = input`, got:\n%s", rep.OutputYAML)
	}
}

// TestMigrateInsideInputProcessors verifies the walker descends into
// input.processors. It also asserts that the `generate` input's
// `mapping` field — a Bloblang STRING field, not a plugin instance —
// is left untouched (string-field migration is out of scope for this
// component-level migrator).
func TestMigrateInsideInputProcessors(t *testing.T) {
	in := `
input:
  generate:
    mapping: 'root = "hello"'
    interval: 1s
  processors:
    - mapping: 'root.upper = this.uppercase()'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected input.processors mapping to be migrated:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, `mapping: 'root = "hello"'`) &&
		!strings.Contains(rep.OutputYAML, `mapping: root = "hello"`) {
		t.Fatalf("generate.mapping (string field) should be untouched, got:\n%s", rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 1 {
		t.Fatalf("expected exactly 1 rewrite (only the processor, not the string field), got %+v", rep.Coverage)
	}
}

// TestMigrateInsideOutputProcessors verifies the walker descends into
// output.processors.
func TestMigrateInsideOutputProcessors(t *testing.T) {
	in := `
output:
  drop: {}
  processors:
    - mutation: 'root.flag = true'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected output.processors mutation to be migrated:\n%s", rep.OutputYAML)
	}
}

// TestMigrateInsideBranchProcessors verifies nested processors inside
// a `branch` processor get migrated, and that branch.request_map /
// result_map (Bloblang STRING fields) are left untouched.
func TestMigrateInsideBranchProcessors(t *testing.T) {
	in := `
pipeline:
  processors:
    - branch:
        request_map: 'root = this.payload'
        processors:
          - mapping: 'root.id = this.id'
        result_map: 'root.enriched = this'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected branch.processors mapping to be migrated:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "request_map") || !strings.Contains(rep.OutputYAML, "result_map") {
		t.Fatalf("branch string fields should be preserved:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "root = this.payload") {
		t.Fatalf("branch.request_map (string field) should be untouched, got:\n%s", rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 1 {
		t.Fatalf("expected exactly 1 rewrite (only the nested processor), got %+v", rep.Coverage)
	}
}

// TestMigrateResourcesFile verifies the walker handles top-level
// resource definitions (cache_resources, processor_resources, etc.) —
// not just the stream pipeline.
func TestMigrateResourcesFile(t *testing.T) {
	in := `
processor_resources:
  - label: my_resource
    bloblang: 'root.x = this.x'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("expected processor_resources entry to be migrated:\n%s", rep.OutputYAML)
	}
	if !strings.Contains(rep.OutputYAML, "label: my_resource") {
		t.Fatalf("expected resource label preserved:\n%s", rep.OutputYAML)
	}
	if rep.Coverage.Rewritten != 1 {
		t.Fatalf("expected 1 rewrite, got %+v", rep.Coverage)
	}
}

// TestMigratePreservesComments verifies that YAML comments adjacent
// to a migrated component survive the rewrite. This is a load-bearing
// guarantee for users running the migrator on hand-curated configs.
func TestMigratePreservesComments(t *testing.T) {
	in := `
pipeline:
  processors:
    # head comment on the processor
    - bloblang: 'root.id = this.id' # inline comment
    # comment between processors
    - mutation: 'root.flag = true'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, want := range []string{
		"head comment on the processor",
		"inline comment",
		"comment between processors",
	} {
		if !strings.Contains(rep.OutputYAML, want) {
			t.Fatalf("comment %q lost in migration:\n%s", want, rep.OutputYAML)
		}
	}
}

// TestInvalidBloblangBodyMarkedUnsupported verifies that a syntax
// error in the V1 mapping body produces an Unsupported Change rather
// than failing the whole Migrate call. The original component is left
// untouched so the rewritten YAML remains valid for the user to fix.
func TestInvalidBloblangBodyMarkedUnsupported(t *testing.T) {
	in := `
pipeline:
  processors:
    - bloblang: '@@@ not valid bloblang @@@'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate should not fail outright on bad body: %v", err)
	}
	if len(rep.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(rep.Changes), rep.Changes)
	}
	if rep.Changes[0].Outcome != migrator.OutcomeUnsupported {
		t.Fatalf("expected OutcomeUnsupported, got %v", rep.Changes[0].Outcome)
	}
	if rep.Changes[0].Severity != migrator.SeverityError {
		t.Fatalf("expected SeverityError, got %v", rep.Changes[0].Severity)
	}
	if !strings.Contains(rep.OutputYAML, "bloblang:") {
		t.Fatalf("V1 plugin should be left in place on Unsupported:\n%s", rep.OutputYAML)
	}
	if strings.Contains(rep.OutputYAML, "bloblang_v2:") {
		t.Fatalf("V1 plugin should NOT be rewritten on Unsupported:\n%s", rep.OutputYAML)
	}
}

// TestVerboseEmitsSkipChange verifies that a Skip(reason) result is
// silent in the default report and emitted as an info Change in
// verbose mode.
func TestVerboseEmitsSkipChange(t *testing.T) {
	rule := func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
		return ctx.Skip("just because")
	}

	build := func() *migrator.Migrator {
		mig := migrator.New()
		mig.RegisterRule(migrator.Target{ComponentType: "processor", Name: "bloblang"}, rule)
		return mig
	}

	in := `
pipeline:
  processors:
    - bloblang: 'root = this'
`

	quiet, err := build().Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("quiet migrate: %v", err)
	}
	if len(quiet.Changes) != 0 {
		t.Fatalf("non-verbose Skip should not emit Changes, got %+v", quiet.Changes)
	}
	if quiet.Coverage.Skipped != 0 {
		t.Fatalf("non-verbose Skip should not count toward Coverage.Skipped, got %+v", quiet.Coverage)
	}

	loud, err := build().Migrate([]byte(in), migrator.Options{Verbose: true})
	if err != nil {
		t.Fatalf("verbose migrate: %v", err)
	}
	if len(loud.Changes) != 1 {
		t.Fatalf("verbose Skip should emit one Change, got %+v", loud.Changes)
	}
	if loud.Changes[0].Outcome != migrator.OutcomeSkipped {
		t.Fatalf("expected OutcomeSkipped, got %v", loud.Changes[0].Outcome)
	}
	if loud.Changes[0].Reason != "just because" {
		t.Fatalf("Skip reason lost, got %q", loud.Changes[0].Reason)
	}
	if loud.Changes[0].Severity != migrator.SeverityInfo {
		t.Fatalf("Skip should be SeverityInfo, got %v", loud.Changes[0].Severity)
	}
}

// TestInvalidYAMLReturnsError verifies that malformed YAML is
// rejected with an error rather than panicking or silently producing
// empty output.
func TestInvalidYAMLReturnsError(t *testing.T) {
	_, err := migrator.Migrate([]byte("not: valid: yaml: ::\n  - oops"), migrator.Options{})
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

// TestZeroResultLeavesComponentUntouched verifies the defensive path
// in buildChange: if a custom rule returns the zero Result (no kind
// set), the component is left untouched and no Change is recorded.
func TestZeroResultLeavesComponentUntouched(t *testing.T) {
	mig := migrator.New()
	mig.RegisterRule(
		migrator.Target{ComponentType: "processor", Name: "log"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			return migrator.Result{}
		},
	)

	in := `
pipeline:
  processors:
    - log:
        message: hello
`
	rep, err := mig.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(rep.Changes) != 0 {
		t.Fatalf("zero Result should not emit Changes, got %+v", rep.Changes)
	}
	if !strings.Contains(rep.OutputYAML, "message: hello") {
		t.Fatalf("config should be unchanged:\n%s", rep.OutputYAML)
	}
}

// TestUnchangedConfigWhenNoRulesMatch verifies the byte stability of
// configs that have nothing to migrate. A round-trip through the
// migrator should yield a YAML document equivalent to the input.
func TestUnchangedConfigWhenNoRulesMatch(t *testing.T) {
	in := `
input:
  generate:
    mapping: 'root = "hello"'
    interval: 1s
output:
  drop: {}
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(rep.Changes) != 0 {
		t.Fatalf("expected zero Changes, got %+v", rep.Changes)
	}
	for _, want := range []string{
		`mapping: 'root = "hello"'`,
		"interval: 1s",
		"drop: {}",
	} {
		if !strings.Contains(rep.OutputYAML, want) {
			t.Fatalf("expected %q in unchanged output, got:\n%s", want, rep.OutputYAML)
		}
	}
}

// TestBloblangMigratorOptionThreadedThrough verifies that a custom
// *bloblmig.Migrator supplied via Options.BloblangMigrator is the one
// that translates embedded mapping bodies — confirming the built-in
// rules consult the per-call sub-migrator rather than constructing
// their own.
func TestBloblangMigratorOptionThreadedThrough(t *testing.T) {
	bloblMig := bloblmig.New()
	var fired int
	bloblMig.RegisterMethodRule("widget_encode", func(ctx *bloblmig.Context, m *bloblmig.V1MethodCall) bloblmig.Result {
		fired++
		return ctx.Replace(&bloblmig.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_encode_v2",
		})
	})

	in := `
pipeline:
  processors:
    - bloblang: 'root.encoded = this.payload.widget_encode()'
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{BloblangMigrator: bloblMig})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected custom bloblang rule to fire exactly once, got %d", fired)
	}
	if !strings.Contains(rep.OutputYAML, "widget_encode_v2") {
		t.Fatalf("expected V2 rewrite from custom rule, got:\n%s", rep.OutputYAML)
	}
}

// TestRegisterRuleNoOpAccessors covers Component accessors used by
// custom rules: BodyString for scalar bodies, BodyAny for structured.
func TestComponentAccessors(t *testing.T) {
	mig := migrator.New()
	var sawBodyString, sawBodyAny bool
	mig.RegisterRule(
		migrator.Target{ComponentType: "processor", Name: "log"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			if _, ok := c.BodyString(); ok {
				sawBodyString = true
			}
			if v, err := c.BodyAny(); err == nil && v != nil {
				sawBodyAny = true
			}
			return ctx.Skip("introspect only")
		},
	)
	in := `
pipeline:
  processors:
    - log:
        message: hello
`
	if _, err := mig.Migrate([]byte(in), migrator.Options{Verbose: true}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sawBodyString {
		t.Fatalf("log body is structured; BodyString should not have reported ok")
	}
	if !sawBodyAny {
		t.Fatalf("BodyAny should have decoded the structured body")
	}
}
