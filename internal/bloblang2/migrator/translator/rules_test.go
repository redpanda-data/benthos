package translator_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
)

// TestRuleUnits — Layer 2. Each entry documents one translation rule with a
// representative V1 input, the expected V2 substring(s), and the RuleIDs the
// translator must emit. Substring matching keeps the tests insensitive to
// whitespace/layout drift; RuleID (rather than Explanation) assertions keep
// them insensitive to wording.
//
// When a single V1 construct legitimately emits more than one Change
// (e.g. the bare-expression shorthand emits both RuleRootToOutput and the
// per-expression rule), assert all of them via wantRules.
func TestRuleUnits(t *testing.T) {
	for _, c := range ruleCases {
		t.Run(c.name, func(t *testing.T) {
			// MinCoverage 0.0001 bypasses applyDefaults' 0.75 fallback
			// (which kicks in only when the value is literally 0).
			// Mappings that translate to 100% Unsupported (`from`,
			// `.apply(dynamic)`) still trip the CoverageError path; we
			// unwrap the Report from the error for those cases.
			rep, err := translator.Migrate(c.v1, translator.Options{
				Verbose:     true,
				MinCoverage: 0.0001,
			})
			var cerr *translator.CoverageError
			switch {
			case err == nil:
				// normal path; rep is populated
			case errors.As(err, &cerr):
				rep = cerr.Report
			default:
				t.Fatalf("Migrate(%q) failed: %v", c.v1, err)
			}
			for _, want := range c.wantV2 {
				if !strings.Contains(rep.V2Mapping, want) {
					t.Errorf("V2 output missing %q\nGot:\n%s", want, rep.V2Mapping)
				}
			}
			for _, rule := range c.wantRules {
				if !hasRule(rep.Changes, rule) {
					t.Errorf("expected a Change with RuleID %s; got:\n%s", rule, changeList(rep.Changes))
				}
			}
			for _, rule := range c.notRules {
				if hasRule(rep.Changes, rule) {
					t.Errorf("did not expect a Change with RuleID %s; got:\n%s", rule, changeList(rep.Changes))
				}
			}
		})
	}
}

type ruleCase struct {
	name      string
	v1        string
	wantV2    []string // substrings that must appear in the V2 output
	wantRules []translator.RuleID
	notRules  []translator.RuleID // negative assertions
}

// ruleCases is deliberately flat and verbose — each entry documents one rule,
// one shape. Add a new entry when a new RuleID is emitted.
var ruleCases = []ruleCase{
	// -----------------------------------------------------------------
	// Naming and shape rewrites.
	// -----------------------------------------------------------------
	{
		name:   "root -> output (identity rename, no rule fires)",
		v1:     `root = "hi"`,
		wantV2: []string{"output", `"hi"`},
	},
	{
		name:      "this -> input (read position)",
		v1:        `root = this`,
		wantV2:    []string{"output", "input"},
		wantRules: []translator.RuleID{translator.RuleThisToInput},
	},
	{
		name:      "this-target -> output (write position)",
		v1:        `this.foo = "bar"`,
		wantV2:    []string{"output.foo"},
		wantRules: []translator.RuleID{translator.RuleThisTargetToOutput},
	},
	{
		name:      "bare ident -> input.ident (null-safe)",
		v1:        `root = foo`,
		wantV2:    []string{"input?.foo"},
		wantRules: []translator.RuleID{translator.RuleBareIdentToInput},
	},
	{
		name:      "bare path target -> output.path",
		v1:        `foo.bar = 1`,
		wantV2:    []string{"output.foo.bar"},
		wantRules: []translator.RuleID{translator.RuleBarePathToOutput},
	},
	{
		name:      "bare expression mapping -> explicit output = expr",
		v1:        `"hi"`,
		wantV2:    []string{"output", `"hi"`},
		wantRules: []translator.RuleID{translator.RuleRootToOutput},
	},

	// -----------------------------------------------------------------
	// Metadata.
	// -----------------------------------------------------------------
	{
		name:      "meta target -> output@",
		v1:        `meta foo = "bar"`,
		wantV2:    []string{"output@", "foo"},
		wantRules: []translator.RuleID{translator.RuleMetaTargetToOutputMeta},
	},
	{
		name:      "meta(key) read -> input@[key]",
		v1:        `root = meta("k")`,
		wantV2:    []string{"input@"},
		wantRules: []translator.RuleID{translator.RuleMetaReadToInputMeta},
	},

	// -----------------------------------------------------------------
	// Operators.
	// -----------------------------------------------------------------
	{
		name:      "coalesce | -> .or()",
		v1:        `root = this.x | "fb"`,
		wantV2:    []string{".or(", `"fb"`},
		wantRules: []translator.RuleID{translator.RuleCoalescePrecedence},
	},
	{
		name:      "&& flags operand-typing divergence",
		v1:        `root = this.a && this.b`,
		wantV2:    []string{"&&"},
		wantRules: []translator.RuleID{translator.RuleAndOrSameLevel},
	},
	{
		name:      "== flags cross-type equality divergence",
		v1:        `root = this.a == 1`,
		wantV2:    []string{"=="},
		wantRules: []translator.RuleID{translator.RuleBoolNumberEquality},
	},
	{
		name:      "% flags float-truncation divergence",
		v1:        `root = this.x % 3`,
		wantV2:    []string{"%"},
		wantRules: []translator.RuleID{translator.RuleModuloFloatTruncation},
	},
	{
		name:      "/ flags int-division-returns-float divergence",
		v1:        `root = this.x / 2`,
		wantV2:    []string{"/"},
		wantRules: []translator.RuleID{translator.RuleIntDivReturnsFloat},
	},

	// -----------------------------------------------------------------
	// Method rewrites & flags.
	// -----------------------------------------------------------------
	{
		name:     "method rename: map_each(lambda) on array -> map",
		v1:       `root = [1,2,3].map_each(x -> x)`,
		wantV2:   []string{".map(x -> x)"},
		notRules: []translator.RuleID{translator.RuleCoalescePrecedence},
	},
	{
		name:   "method rename: map_each on object-literal receiver -> map_values",
		v1:     `root = {"a":1}.map_each(v -> v)`,
		wantV2: []string{".map_values(v -> v)"},
	},
	{
		name:      "method rename: .index(n) -> [n]",
		v1:        `root = this.items.index(0)`,
		wantV2:    []string{"[0]"},
		wantRules: []translator.RuleID{translator.RuleNoBracketIndexing},
	},
	{
		name:      "method rename: .get(k) -> [k]",
		v1:        `root = this.obj.get("k")`,
		wantV2:    []string{`["k"]`},
		wantRules: []translator.RuleID{translator.RuleNoBracketIndexing},
	},
	{
		name:      ".number() -> .float64()",
		v1:        `root = "3.14".number()`,
		wantV2:    []string{".float64()"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      ".or() flags catches-errors divergence",
		v1:        `root = this.x.or("fb")`,
		wantV2:    []string{".or(", `"fb"`},
		wantRules: []translator.RuleID{translator.RuleOrCatchesErrors},
	},
	{
		name:      ".length() flags codepoints-vs-bytes divergence",
		v1:        `root = "héllo".length()`,
		wantV2:    []string{".length()"},
		wantRules: []translator.RuleID{translator.RuleStringLengthBytes},
	},
	{
		name:      ".catch(value) wrapped as lambda",
		v1:        `root = this.x.catch("fb")`,
		wantV2:    []string{".catch(_ ->"},
		wantRules: []translator.RuleID{translator.RuleOrCatchesErrors},
	},
	{
		name:      ".exists(key) -> .has_key(key)",
		v1:        `root = this.exists("a")`,
		wantV2:    []string{".has_key(", `"a"`},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      "variadic .without(a, b) -> .without([a, b])",
		v1:        `root = this.without("a", "b")`,
		wantV2:    []string{".without(", `"a"`, `"b"`, "[", "]"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      ".find(value) rewrites to .index_of(value)",
		v1:        `root = [1,2,3].find(2)`,
		wantV2:    []string{".index_of(", "2"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      ".type() flags number-vs-int64/float64 divergence",
		v1:        `root = this.x.type()`,
		wantV2:    []string{".type()"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      "find_by(query-form) wrapped as explicit V2 lambda",
		v1:        `root = this.items.find_by(this.id == 5)`,
		wantV2:    []string{".find_by(__v ->", "__v?.id == 5"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:     "find_by(query-form bare ident) rebinds to lambda param",
		v1:       `root = this.items.find_by(name == "alice")`,
		wantV2:   []string{".find_by(__v ->", "__v?.name"},
		notRules: []translator.RuleID{translator.RuleBareIdentToInput},
	},
	{
		name:   "find_by(explicit lambda) translates 1:1",
		v1:     `root = this.items.find_by(v -> v.id == 5)`,
		wantV2: []string{".find_by(v -> v?.id == 5)"},
	},
	{
		name:      "find_all_by(query-form) wrapped as explicit V2 lambda",
		v1:        `root = this.items.find_all_by(this.active)`,
		wantV2:    []string{".find_all_by(__v ->", "__v?.active"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:   "filter(query-form) wrapped as explicit V2 lambda",
		v1:     `root = this.nums.filter(this > 10)`,
		wantV2: []string{".filter(__v ->", "__v > 10"},
	},
	{
		name:   "sort_by(query-form) wrapped as explicit V2 lambda",
		v1:     `root = this.items.sort_by(this.priority)`,
		wantV2: []string{".sort_by(__v ->", "__v?.priority"},
	},
	{
		name:   "unique(query-form) wrapped as explicit V2 lambda",
		v1:     `root = this.items.unique(this.id)`,
		wantV2: []string{".unique(__v ->", "__v?.id"},
	},
	{
		name:   "all(query-form) wrapped as explicit V2 lambda",
		v1:     `root = this.nums.all(this > 0)`,
		wantV2: []string{".all(__v ->", "__v > 0"},
	},

	// -----------------------------------------------------------------
	// Batch 3 — message-coupled stdlib (P8 migrator coverage).
	// -----------------------------------------------------------------
	{
		name:      `metadata("k") rewrites to input@["k"]`,
		v1:        `root = metadata("region")`,
		wantV2:    []string{"input@", `["region"]`},
		wantRules: []translator.RuleID{translator.RuleMetaReadToInputMeta},
	},
	{
		name:      "metadata() with no arg rewrites to input@",
		v1:        `root = metadata()`,
		wantV2:    []string{"input@"},
		wantRules: []translator.RuleID{translator.RuleMetaReadToInputMeta},
	},
	{
		name:      `meta("k") rewrites to input@["k"] with type-change Note`,
		v1:        `root = meta("region")`,
		wantV2:    []string{"input@", `["region"]`},
		wantRules: []translator.RuleID{translator.RuleMetaReadToInputMeta},
	},
	{
		name:      `root_meta("k") rewrites to output@["k"]`,
		v1:        `root.copy = root_meta("audit")`,
		wantV2:    []string{"output@", `["audit"]`},
		wantRules: []translator.RuleID{translator.RuleMetaReadToInputMeta},
	},
	{
		name:   "error() rewrites to error().what",
		v1:     `root.failed = error()`,
		wantV2: []string{"error()", ".what"},
	},
	{
		name:   "errored() passes through",
		v1:     `root.failed = errored()`,
		wantV2: []string{"errored()"},
	},
	{
		name:   "batch_index() passes through",
		v1:     `root.idx = batch_index()`,
		wantV2: []string{"batch_index()"},
	},
	{
		name:   "content() passes through",
		v1:     `root.bytes = content()`,
		wantV2: []string{"content()"},
	},

	// -----------------------------------------------------------------
	// Variadic→array rewrites for V2 (with / zip mirror without).
	// -----------------------------------------------------------------
	{
		name:      `variadic .with("a","b") -> .with(["a","b"])`,
		v1:        `root = this.with("a", "b")`,
		wantV2:    []string{".with(", `"a"`, `"b"`, "[", "]"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:   `single-array .with([...]) passes through`,
		v1:     `root = this.with(["a", "b"])`,
		wantV2: []string{".with([", `"a"`, `"b"`, "])"},
	},
	{
		name:      `variadic .zip(a, b) -> .zip([a, b])`,
		v1:        `root.foo = this.foo.zip(this.bar, this.baz)`,
		wantV2:    []string{".zip(", "[", "]"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},
	{
		name:      `variadic .format(a, b) -> .format([a, b])`,
		v1:        `root.s = "%s/%v".format(this.name, this.age)`,
		wantV2:    []string{".format(", "[", "]"},
		wantRules: []translator.RuleID{translator.RuleMethodDoesNotExist},
	},

	// -----------------------------------------------------------------
	// Timestamp idiom shifts: V1 function-form -> V2 method-form.
	// -----------------------------------------------------------------
	{
		name:   "ts.format_timestamp_strftime(fmt) -> ts.ts_format(fmt)",
		v1:     `root.iso = this.t.format_timestamp_strftime("%Y-%m-%d")`,
		wantV2: []string{".ts_format(", `"%Y-%m-%d"`},
	},
	{
		name:   "ts.format_timestamp(fmt) flagged but not auto-rewritten",
		v1:     `root.iso = this.t.format_timestamp("2006-01-02")`,
		wantV2: []string{".format_timestamp("},
	},
	{
		name:   "str.parse_timestamp_strptime(fmt) -> str.ts_parse(fmt)",
		v1:     `root.t = this.s.parse_timestamp_strptime("%Y-%m-%d")`,
		wantV2: []string{".ts_parse(", `"%Y-%m-%d"`},
	},
	{
		name:   "ts.format_timestamp_unix() -> ts.ts_unix()",
		v1:     `root.epoch = this.t.format_timestamp_unix()`,
		wantV2: []string{".ts_unix()"},
	},
	{
		name:   "ts.format_timestamp_unix_milli() -> ts.ts_unix_milli()",
		v1:     `root.epoch_ms = this.t.format_timestamp_unix_milli()`,
		wantV2: []string{".ts_unix_milli()"},
	},
	{
		name:   "ts_strftime method renamed to ts_format",
		v1:     `root.iso = this.t.ts_strftime("2006-01-02")`,
		wantV2: []string{".ts_format(", `"2006-01-02"`},
	},
	{
		name:   "ts_strptime method renamed to ts_parse",
		v1:     `root.t = this.s.ts_strptime("%Y-%m-%d")`,
		wantV2: []string{".ts_parse(", `"%Y-%m-%d"`},
	},

	// -----------------------------------------------------------------
	// Maps and imports.
	// -----------------------------------------------------------------
	{
		name:      ".apply('name') -> name(recv)",
		v1:        "map double { root = this * 2 }\nroot.v = (5).apply(\"double\")",
		wantV2:    []string{"double(", "map double(in)"},
		wantRules: []translator.RuleID{translator.RuleMapDeclTranslation},
	},
	{
		name:      ".apply(dynamic) is unsupported",
		v1:        `root = (5).apply(this.name)`,
		wantRules: []translator.RuleID{translator.RuleUnsupportedConstruct},
	},
	{
		name:      `from "file" is unsupported`,
		v1:        `from "helper.blobl"`,
		wantRules: []translator.RuleID{translator.RuleFromStatement},
	},
	{
		name:      `import "file" -> namespaced V2 import`,
		v1:        "import \"helper.blobl\"\nroot.v = 1",
		wantRules: []translator.RuleID{translator.RuleImportStatement},
	},
	{
		name:      "now() flags string-vs-timestamp divergence",
		v1:        `root = now()`,
		wantV2:    []string{"now()"},
		wantRules: []translator.RuleID{translator.RuleNowReturnsString},
	},

	// -----------------------------------------------------------------
	// Control flow.
	// -----------------------------------------------------------------
	{
		name:      "if-without-else flags nothing-sentinel divergence",
		v1:        `root = if true { 1 }`,
		wantV2:    []string{"if true"},
		wantRules: []translator.RuleID{translator.RuleIfNoElseNothing},
	},
	{
		name:      "subject-less match flags boolean-case divergence",
		v1:        `root = match { this.x > 0 => "pos", _ => "neg" }`,
		wantV2:    []string{"match"},
		wantRules: []translator.RuleID{translator.RuleMatchSubjectRebinds},
	},
	{
		name:      "let inside if-branch flags block-scope divergence",
		v1:        "if true { let x = 1 }\nroot.v = 1",
		wantRules: []translator.RuleID{translator.RuleBlockScopedLet},
	},

	// -----------------------------------------------------------------
	// Paths and indexing.
	// -----------------------------------------------------------------
	{
		name:      "numeric path segment -> index expression",
		v1:        `root = this.items.0`,
		wantV2:    []string{"input", "[0]"},
		wantRules: []translator.RuleID{translator.RuleNoBracketIndexing},
	},

	// -----------------------------------------------------------------
	// Sentinels.
	// -----------------------------------------------------------------
	{
		name:   "nothing() at statement RHS -> V2 void()",
		v1:     `root = if this.x > 0 { this.x } else { nothing() }`,
		wantV2: []string{"void()"},
	},
	{
		name:   "nothing() inside array literal -> V2 deleted()",
		v1:     `root.xs = [1, nothing(), 3]`,
		wantV2: []string{"deleted()"},
	},
	{
		name:   "nothing() inside object literal value -> V2 deleted()",
		v1:     `root.obj = {"a": 1, "b": nothing()}`,
		wantV2: []string{"deleted()"},
	},
	{
		name:      "nothing() inside let binding -> Unsupported (no V2 equivalent)",
		v1:        "let a = nothing()\nroot.v = 1",
		wantRules: []translator.RuleID{translator.RuleUnsupportedConstruct},
	},

	// -----------------------------------------------------------------
	// Error path: V2-invalid emission is a non-fatal Change.
	// -----------------------------------------------------------------
	{
		name:      "chained comparison echoes as RuleEmittedInvalidV2",
		v1:        `root = 1 < 2 < 3`,
		wantRules: []translator.RuleID{translator.RuleEmittedInvalidV2},
	},

	// -----------------------------------------------------------------
	// Variables and lambdas.
	// -----------------------------------------------------------------
	{
		name:   "let binding translates to $x declaration and reference",
		v1:     "let x = 1\nroot = $x",
		wantV2: []string{"$x = 1", "output = $x"},
	},
	{
		name:   "lambda parameter scope respected (no bare-ident rewrite on param)",
		v1:     `root = [1,2,3].map_each(n -> n + 1)`,
		wantV2: []string{".map(n -> n + 1)"},
		// The n inside the lambda body must NOT be rewritten as input.n.
		notRules: []translator.RuleID{translator.RuleBareIdentToInput},
	},
	{
		name:   "this inside lambda resolves to outer context (no rebind)",
		v1:     `root = [1,2,3].map_each(_ -> this.scale)`,
		wantV2: []string{"input?.scale"},
	},

	// -----------------------------------------------------------------
	// Object/array literals.
	// -----------------------------------------------------------------
	{
		name:   "object literal preserves string keys",
		v1:     `root = {"a": 1, "b": 2}`,
		wantV2: []string{"output", `"a"`, `"b"`, "1", "2"},
	},
	{
		name:   "array literal preserves order",
		v1:     `root = [1, 2, 3]`,
		wantV2: []string{"output", "1", "2", "3"},
	},

	// -----------------------------------------------------------------
	// Empty / whitespace inputs (property-ish edge cases at unit scope).
	// -----------------------------------------------------------------
	{
		name:   "empty mapping produces empty V2 (no changes)",
		v1:     ``,
		wantV2: []string{},
	},
}

// hasRule reports whether any change in the slice has the given RuleID.
func hasRule(changes []translator.Change, id translator.RuleID) bool {
	for _, c := range changes {
		if c.RuleID == id {
			return true
		}
	}
	return false
}

// changeList returns a human-readable summary of a Change slice for failing
// test output.
func changeList(changes []translator.Change) string {
	var out strings.Builder
	for _, c := range changes {
		out.WriteString("  - ")
		out.WriteString(c.RuleID.String())
		out.WriteString(" (")
		out.WriteString(c.Severity.String())
		out.WriteString("): ")
		out.WriteString(c.Explanation)
		out.WriteString("\n")
	}
	return out.String()
}
