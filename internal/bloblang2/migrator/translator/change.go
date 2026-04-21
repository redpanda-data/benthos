// Package translator converts Bloblang V1 mappings to Bloblang V2.
//
// The public entry point is Migrate. Given V1 source text and Options, it
// returns a Report containing the V2 source, a list of semantic Change
// records describing any behavioural divergences, and a Coverage summary.
// An error is returned only when Coverage.Ratio falls below Options.MinCoverage.
//
// V2 is an intentional redesign that fixes ambiguities in V1. Where V1 and V2
// differ semantically, the translator by default adopts V2 semantics and
// records a Change describing the shift. It is the caller's responsibility to
// audit Changes before relying on the translated mapping.
package translator

import "fmt"

// Severity classifies how much the user should care about a Change.
type Severity int

const (
	// SeverityInfo marks a benign rewrite — the V1 and V2 forms are
	// equivalent, but the V1 form was non-canonical (e.g. a bare identifier)
	// or idiomatic V2 differs from idiomatic V1.
	SeverityInfo Severity = iota

	// SeverityWarning means the V1 and V2 forms may diverge on some inputs,
	// and the caller should audit the translated mapping.
	SeverityWarning

	// SeverityError means the translator could not produce a V2 form that
	// preserves V1 semantics at all, and the emitted mapping almost
	// certainly behaves differently. The affected span may also have been
	// elided.
	SeverityError
)

// String satisfies fmt.Stringer for ergonomic test output.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	return fmt.Sprintf("severity(%d)", s)
}

// Category groups Changes by the kind of translation decision.
type Category int

const (
	// CategoryIdiomRewrite flags that the V1 form was rewritten to an
	// idiomatic V2 form with identical semantics. Always SeverityInfo.
	CategoryIdiomRewrite Category = iota

	// CategorySemanticChange flags that the translator deliberately adopted
	// V2 semantics where V1 and V2 diverge. The caller should audit.
	CategorySemanticChange

	// CategoryUnsupported flags a V1 construct with no V2 equivalent. The
	// emitted mapping contains a "# MIGRATION: <reason>" comment at the
	// affected site and does not translate the construct.
	CategoryUnsupported

	// CategoryUncertain flags that the translator couldn't determine the V1
	// behaviour confidently (e.g. ambiguous precedence, context-dependent
	// rebinding). The emitted form is best-effort.
	CategoryUncertain
)

// String satisfies fmt.Stringer.
func (c Category) String() string {
	switch c {
	case CategoryIdiomRewrite:
		return "idiom-rewrite"
	case CategorySemanticChange:
		return "semantic-change"
	case CategoryUnsupported:
		return "unsupported"
	case CategoryUncertain:
		return "uncertain"
	}
	return fmt.Sprintf("category(%d)", c)
}

// RuleID is a stable identifier for a translation rule. Each rule emits Changes
// tagged with its RuleID. RuleID values survive spec renumbering: a renamed
// §14 quirk still maps to the same RuleID.
//
// Add new rules by appending here. Never reuse values.
type RuleID int

// RuleID values. Trailing-line comments describe the rule. See the
// bloblang_v1_spec.md §14 quirk anchors in parentheses.
const (
	// RuleUnknown is the zero value; only appears when a Change was built
	// without setting a rule.
	RuleUnknown RuleID = iota

	// Naming & shape.
	RuleRootToOutput       // root -> output
	RuleThisToInput        // this -> input (read position)
	RuleThisTargetToOutput // this as write target -> output (§14#72)
	RuleBareIdentToInput   // bare ident `foo` -> `input.foo` (§14#1)
	RuleBarePathToOutput   // bare-path target `foo.bar = v` -> `output.foo.bar = v` (§14#2)

	// Metadata rules.
	RuleMetaTargetToOutputMeta // `meta foo = v` -> `output@.foo = v`
	RuleMetaReadToInputMeta    // `meta("k")` or `@k` -> `input@.k` or `input@[k]`

	// Operator rules.
	RuleCoalescePrecedence    // `a + b | c` parens preserved (§14#4)
	RuleAndOrSameLevel        // `a || b && c` V1=(a||b)&&c vs V2=a||(b&&c) (§14#3)
	RuleBoolNumberEquality    // `true == 1` / `1 == true` asymmetry (§14#38)
	RuleModuloFloatTruncation // `%` silent float->int64 truncation (§14#39)
	RuleIntDivReturnsFloat    // `/` on ints returns float64 (§14#5)
	RuleLiteralConstantFold   // arithmetic/comparison literal folds at parse (§14#37)

	// Sentinel and error-model rules.
	RuleOrCatchesErrors   // V1 `.or()` catches errors; V2 `.or()` doesn't (§12.2)
	RuleDeletedInMapEach  // deleted()/nothing() propagation in map_each (§14#34)
	RuleSentinelInLiteral // sentinels in array/object literals elide (§9.4)

	// Lambda and method rules.
	RuleLambdaContextPop  // inside `x -> body`, `this` is outer (§14#35)
	RuleIteratorRebinding // `.map_each(this.foo)` non-lambda rebinds `this` (§6.5)
	RuleSortComparator    // `.sort(left > right)` implicit-param form
	RuleFoldObjectParam   // `.fold(init, item -> ...)` with item={tally,value}

	// Control-flow rules.
	RuleIfNoElseNothing     // `if cond { x }` no-else produces nothing sentinel (§14#44)
	RuleMatchNoMatchNothing // match with no matching arm produces nothing (§8.4)
	RuleMatchLiteralFold    // match pattern constant folding (§14#75)
	RuleMatchSubjectRebinds // match arms rebind `this` to subject (§8.4)

	// Path and indexing rules.
	RuleNumericPathWrite  // path.0 = v creates object key (§14#46)
	RuleNoBracketIndexing // `this[0]` not valid; use `.index(0)` (§14#10)

	// String rules.
	RuleStringLengthBytes // `.length()` on string returns byte count (§14#40)

	// Method and function existence/rename rules.
	RuleMethodDoesNotExist // e.g. map_values, collect, chunk, char — no V2/V1 equivalent
	RuleNowReturnsString   // `now()` returns a string in V1 (§14#57)

	// Map and import rules.
	RuleMapDeclTranslation // `map foo { body }` -> V2 `map foo { body }`
	RuleImportStatement    // `import "path"` -> V2 equivalent
	RuleFromStatement      // `from "path"` whole-mapping include (§10.5)

	// Object and array literal rules.
	RuleBareIdentObjectKey // `{a: 1}` -> `{(input.a): 1}` (§14#8)
	RuleComputedKey        // `{(expr): v}` -> V2 equivalent

	// RuleUnsupportedConstruct is the catch-all when no more specific rule
	// applies.
	RuleUnsupportedConstruct

	// RuleEmittedInvalidV2 flags that the translator's emitted V2 text did
	// not parse under syntax.Parse. This is either a genuine translator bug
	// (when V1 input was valid) or an echo of a V1 compile error the V2
	// parser also rejects (e.g. chained `<`, missing imports, duplicate
	// namespaces). Callers that want to detect real bugs can filter on this
	// rule; the report is still returned with the best-effort V2 text.
	RuleEmittedInvalidV2

	// RuleBlockScopedLet flags a `let` declaration inside an if/else branch
	// body. V1 scopes variables at the mapping level so declarations leak
	// out; V2 scopes them per block. If the variable is referenced outside
	// the branch, the V2 output will fail to compile.
	RuleBlockScopedLet
)

// String satisfies fmt.Stringer.
func (r RuleID) String() string {
	switch r {
	case RuleUnknown:
		return "unknown"
	case RuleRootToOutput:
		return "root-to-output"
	case RuleThisToInput:
		return "this-to-input"
	case RuleThisTargetToOutput:
		return "this-target-to-output"
	case RuleBareIdentToInput:
		return "bare-ident-to-input"
	case RuleBarePathToOutput:
		return "bare-path-to-output"
	case RuleMetaTargetToOutputMeta:
		return "meta-target-to-output-meta"
	case RuleMetaReadToInputMeta:
		return "meta-read-to-input-meta"
	case RuleCoalescePrecedence:
		return "coalesce-precedence"
	case RuleAndOrSameLevel:
		return "and-or-same-level"
	case RuleBoolNumberEquality:
		return "bool-number-equality"
	case RuleModuloFloatTruncation:
		return "modulo-float-truncation"
	case RuleIntDivReturnsFloat:
		return "int-div-returns-float"
	case RuleLiteralConstantFold:
		return "literal-constant-fold"
	case RuleOrCatchesErrors:
		return "or-catches-errors"
	case RuleDeletedInMapEach:
		return "deleted-in-map-each"
	case RuleSentinelInLiteral:
		return "sentinel-in-literal"
	case RuleLambdaContextPop:
		return "lambda-context-pop"
	case RuleIteratorRebinding:
		return "iterator-rebinding"
	case RuleSortComparator:
		return "sort-comparator"
	case RuleFoldObjectParam:
		return "fold-object-param"
	case RuleIfNoElseNothing:
		return "if-no-else-nothing"
	case RuleMatchNoMatchNothing:
		return "match-no-match-nothing"
	case RuleMatchLiteralFold:
		return "match-literal-fold"
	case RuleMatchSubjectRebinds:
		return "match-subject-rebinds"
	case RuleNumericPathWrite:
		return "numeric-path-write"
	case RuleNoBracketIndexing:
		return "no-bracket-indexing"
	case RuleStringLengthBytes:
		return "string-length-bytes"
	case RuleMethodDoesNotExist:
		return "method-does-not-exist"
	case RuleNowReturnsString:
		return "now-returns-string"
	case RuleMapDeclTranslation:
		return "map-decl-translation"
	case RuleImportStatement:
		return "import-statement"
	case RuleFromStatement:
		return "from-statement"
	case RuleBareIdentObjectKey:
		return "bare-ident-object-key"
	case RuleComputedKey:
		return "computed-key"
	case RuleUnsupportedConstruct:
		return "unsupported-construct"
	case RuleEmittedInvalidV2:
		return "emitted-invalid-v2"
	case RuleBlockScopedLet:
		return "block-scoped-let"
	}
	return fmt.Sprintf("rule(%d)", r)
}

// Change records one translation decision worth surfacing to the caller.
type Change struct {
	Line, Column int // start of the affected V1 span
	EndLine      int // end line (may equal Line)
	EndColumn    int // end column
	Severity     Severity
	Category     Category
	RuleID       RuleID
	SpecRef      string // e.g. "§14#48"; current spec anchor for docs
	Original     string // V1 snippet (for citation)
	Translated   string // V2 snippet emitted; empty if dropped
	Explanation  string // one-line human-readable
}

// Report is the result of a successful Migrate call.
type Report struct {
	V2Mapping string
	// V2Files is the set of imported files translated from V1 to V2. Keys
	// are the paths used by the V1 source's import statements. Empty when
	// Options.Files was empty.
	V2Files  map[string]string
	Changes  []Change
	Coverage Coverage
}

// Coverage summarises the translator's progress over the V1 input.
type Coverage struct {
	Total       int     // total V1 AST nodes weighed
	Translated  int     // translated exactly (Exact)
	Rewritten   int     // translated with a SemanticChange
	Unsupported int     // dropped / replaced with a MIGRATION comment
	Ratio       float64 // (Translated*1.0 + Rewritten*0.9) / Total
}

// Options controls Migrate.
type Options struct {
	// MinCoverage is the minimum Coverage.Ratio required before Migrate
	// returns successfully. If the computed ratio is below this value,
	// Migrate returns (nil, *CoverageError). Default 0.75.
	MinCoverage float64

	// Verbose emits Info-severity Changes. Without this, only Warning and
	// Error Changes are recorded, keeping the report focused on items that
	// need human attention.
	Verbose bool

	// TreatWarningsAsErrors causes Warning-severity Changes to be promoted
	// to Error; useful for CI.
	TreatWarningsAsErrors bool

	// Files is a virtual filesystem for `import` resolution, keyed by the
	// path used in the V1 source. Both the V1 parser's import resolution
	// and the final V2 parse check consult this map. nil means "use the
	// host filesystem" — but note the V1 parser currently does not accept
	// a files argument either (it resolves via its own configured
	// importer), so this field is presently consumed only by the final V2
	// parse verifier. Future work may thread it through more deeply.
	Files map[string]string
}

// DefaultOptions returns reasonable defaults.
func DefaultOptions() Options {
	return Options{
		MinCoverage: 0.75,
	}
}

// CoverageError is returned by Migrate when Coverage.Ratio < Options.MinCoverage.
type CoverageError struct {
	Coverage Coverage
	Min      float64
	Report   *Report // the would-be report; inspect for context even on error
}

// Error satisfies the error interface.
func (e *CoverageError) Error() string {
	return fmt.Sprintf(
		"migrator: translation coverage %.2f is below threshold %.2f (translated=%d rewritten=%d unsupported=%d total=%d)",
		e.Coverage.Ratio, e.Min,
		e.Coverage.Translated, e.Coverage.Rewritten, e.Coverage.Unsupported, e.Coverage.Total,
	)
}

// computeRatio applies the weighted formula:
//
//	(Translated*1.0 + Rewritten*0.9) / Total
//
// Returns 1.0 when Total is zero (nothing to translate is 100% successful).
func computeRatio(c Coverage) float64 {
	if c.Total == 0 {
		return 1.0
	}
	return (float64(c.Translated)*1.0 + float64(c.Rewritten)*0.9) / float64(c.Total)
}
