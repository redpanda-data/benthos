# V1 → V2 Migrator — Design Plan

A Go library that accepts a Bloblang V1 mapping string and produces an equivalent Bloblang V2 mapping, along with a report of semantic divergences. **100% fidelity is not a goal** — V2 is fixing V1 ambiguities, so some translations will intentionally shift semantics. The tool surfaces every such shift so a human can audit.

## 1. Surface

```go
package translator

type Options struct {
    MinCoverage          float64 // default 0.75; below this -> *CoverageError
    PreserveComments     bool    // default true; statement-level preservation only
    Verbose              bool    // emit Info-severity notes
    TreatWarningsAsErrors bool   // for CI use
}

func Migrate(v1Source string, opts Options) (*Report, error)

type Report struct {
    V2Mapping string
    Changes   []Change
    Coverage  Coverage
}

type Change struct {
    Line, Column int
    End          Position
    Severity     Severity   // Info / Warning / Error
    Category     Category   // SemanticChange / UnsupportedConstruct / IdiomRewrite / Uncertain
    Original     string     // V1 snippet
    Translated   string     // V2 snippet (empty if dropped)
    Explanation  string     // one-line human-readable
    RuleID       RuleID     // stable enum (e.g. RuleID_BoolNumEq) — survives spec renumbering
    SpecRef      string     // e.g. "§14#48" — current spec anchor
}

type Coverage struct {
    Total       int     // weighted V1 AST nodes
    Translated  int     // translated exactly (no semantic change)
    Rewritten   int     // translated with a SemanticChange
    Unsupported int     // dropped / replaced with a "# MIGRATION: …" note
    Ratio       float64 // (Translated*1.0 + Rewritten*0.9) / Total
}
```

**Errors**: `Migrate` returns an error only when `Coverage.Ratio < MinCoverage`. All other outcomes (including mappings with behavior changes) return a `*Report` with `Changes` populated.

## 2. Pipeline

```
v1 source
  │
  ├─► [1] Scanner           → tokens + positions
  │
  ├─► [2] V1 parser         → V1 AST (Assignment, Match, Lambda, BinOp, Ident, …)
  │
  ├─► [3] Semantic tag      → annotate each node with V1 interpretation
  │                           ("bare ident foo -> this.foo", "literal-fold applies")
  │
  ├─► [4] Translator        → walk V1 AST, emit V2 AST + Change records
  │
  ├─► [5] V2 printer        → V2 AST → source text
  │
  └─► [6] Verifier          → syntax.Parse the V2 output to ensure it compiles
                              Never emit a mapping that doesn't round-trip.

                          → Report{V2Mapping, Changes, Coverage}
```

## 3. Package layout

```
migrator/
├── bloblang_v1_spec.md        # (exists)
├── v1spec/                    # (exists) spec-compliance tests
│
├── v1ast/                     # V1 AST + parser
│   ├── ast.go
│   ├── scanner.go
│   ├── parser.go
│   ├── printer.go             # V1 AST → V1 text (round-trip correctness)
│   └── parser_test.go         # conformance against v1spec/tests
│
├── translator/                # core library
│   ├── migrate.go             # public Migrate(...)
│   ├── rules/                 # one file per rule/category
│   │   ├── root_output.go
│   │   ├── bare_ident.go
│   │   ├── arithmetic.go
│   │   ├── equality.go
│   │   ├── lambdas.go
│   │   ├── match_expr.go
│   │   ├── if_expr.go
│   │   ├── meta.go
│   │   ├── sentinels.go
│   │   ├── coalesce.go
│   │   ├── methods.go
│   │   └── maps_imports.go
│   ├── context.go             # visitor context: scope, changes, coverage accumulator
│   ├── change.go              # Change / RuleID / Report types
│   └── translator_test.go
│
└── testdata/
    ├── cases/                 # per-rule YAML tables
    └── corpus/                # regression: every V1 corpus entry → translated V2
```

V2 pretty-printer lives separately in `internal/bloblang2/go/pratt/syntax/print.go` — a V2-package contribution that the migrator depends on.

## 4. V1 parser strategy

A dedicated parser under `v1ast/`:

- Matches V1's actual lenient grammar exactly. Spec §15 EBNF + quirks catalogue are the blueprint.
- Preserves line/column for every node.
- Produces structural AST nodes (not closures), so translation rules can visit and transform them.
- Reuses no code from `internal/bloblang/parser/` — the reference parser produces closures that aren't introspectable.

**Validation**: `v1ast/parser_test.go` iterates every non-skipped YAML in `v1spec/tests/`. Each mapping must parse. Round-trip the AST through the printer and re-parse; ASTs must match.

## 5. Coverage metric

- Weighted: `Ratio = (Exact*1.0 + Rewritten*0.9) / Total`
- Unit: one V1 AST node = one unit, counted uniformly.
- Classification done by the translation rule that handles the node:
  - **Exact** — direct 1:1 translation, no semantics lost.
  - **Rewritten** — emits at least one `SemanticChange` Change. Contributes 0.9 to coverage; still "translated".
  - **Unsupported** — the rule couldn't produce V2 equivalent. Emits a `# MIGRATION: <reason>` comment in V2 output. Contributes 0 to coverage.
- Default threshold: `MinCoverage = 0.75` — roughly, "more than 25% of the mapping is unsupported" triggers an error.

## 6. Design decisions

These were resolved upfront:

- **Default translation behaviour**: adopt V2 semantics where V1 and V2 diverge. Emit a `SemanticChange` Change record so the user sees the shift. No `--preserve-v1-semantics` flag for now.
- **Comment preservation**: statement-level only. Best-effort — comments above a V1 statement migrate to the equivalent V2 statement.
- **RuleID + SpecRef**: every Change carries both. `RuleID` is a stable Go enum (survives spec renumbering); `SpecRef` is the current `§14#N` anchor (for docs navigation). Keep both in sync.
- **Library only**: no CLI in this plan. The translator is a pure library; CLI/tooling can be built on top later.
- **V2 pretty-printer location**: `go/pratt/syntax/print.go` — contributed to the V2 package, reusable beyond the migrator.

## 7. Translation rules — anchor table

High-leverage rules, each implemented in its own `translator/rules/*.go` file. Each rule:

- Receives a V1 AST node.
- Returns a V2 AST node (or a skip marker).
- Emits zero or more `Change` records tagged with a stable `RuleID` and the current `§…` anchor.

| V1 construct | V2 target | Quirk | Classification |
|---|---|---|---|
| `root = X` | `output = X` | — | Exact |
| `this` (read) | `input` | — | Exact |
| `this.foo = v` (legacy root alias) | `output.foo = v` | §6.4, #72 | Rewritten (Info) |
| Bare ident `foo` in expression | `input.foo` | §14#1 | Rewritten (Info) |
| `items.map_each(this.value)` | `items.map(x -> x.value)` | §6.5 | Rewritten (Info) — V1's implicit rebind |
| `items.map_each(x -> this.foo)` | `items.map(x -> input.foo)` | §14#35 | Rewritten (Warning) — lambda context pop |
| `.or(fb)` (catches errors in V1) | `.catch(fb).or(fb)` | §12.2 | Rewritten (SemanticChange) |
| `true == 1` | V2: `false` | §5.3, #38 | Rewritten (SemanticChange) — constant fold flips result |
| `a \|\| b && c` | `(a \|\| b) && c` not preserved; V2 adopts standard | §14#3 | Rewritten (SemanticChange) |
| `a + b \| c` | `a + (b \| c)` parens preserved | §14#4 | Rewritten (Info) |
| `%` on float operand | Insert `.floor()` on both operands | §14#39 | Rewritten (SemanticChange) |
| `.length()` on string | `.length_bytes()` or codepoint-equivalent | §14#40 | Rewritten (SemanticChange) — user audit |
| `meta foo = v` | `output@.foo = v` | — | Exact |
| `meta("key")` read | `input@.key` | — | Exact |
| `if cond { x }` (no else) | V2 equivalent with explicit skip / `null` | §14#44 | Rewritten (Info) |
| `{a: 1}` (dynamic-key quirk) | `{(input.a): 1}` explicit | §14#8 | Rewritten (SemanticChange) |
| `.int32()`, `.abs()`, etc. | 1:1 | §9.0 | Exact |
| `$var = value` (impossible in V1) | — | — | Parser rejects |
| `from "file"` | V2 `import` + apply pattern | §10.5 | Rewritten (Warning) |
| V1-unknown construct | `# MIGRATION: <reason>` comment | — | Unsupported |

More rules land as each V1 quirk is implemented. The §14 quirks catalogue is the canonical source for things the migrator must handle.

## 8. Testing framework — six layers

Layered to catch different failure modes. High confidence comes from *all* layers green.

### Layer 1 — V1 parser conformance

`v1ast/parser_test.go`. Parse every non-skipped YAML test in `v1spec/tests/`. Must succeed. Re-print → re-parse; ASTs must match (round-trip integrity).

### Layer 2 — Per-rule unit tests (table-driven)

`translator/rules/*_test.go`. Each rule has its own table. Example:

```go
func TestCoalesceRule(t *testing.T) {
    cases := []ruleCase{
        {
            name: "basic .or fallback on null",
            v1:   `root = this.x.or("default")`,
            v2:   `output = input.x.catch("default").or("default")`,
            change: expectChange(SemanticChange, RuleID_OrCatchError, "§12.2"),
        },
    }
}
```

Target: ~15 rules × ~10 cases each = 150 unit tests.

### Layer 3 — YAML translation corpus

`testdata/cases/`: hand-curated combinations. Tests translate, then diff V2 text (whitespace-normalised) + Change set (subset match on `{Category, RuleID}` so tests don't over-specify wording).

```yaml
description: "match with predicate pattern and outer this capture"
v1: |
  root = match this.user {
    this.age > 18 => "adult"
    _ => "minor"
  }
v2: |
  output = match input.user as u {
    u.age > 18 => "adult"
    _ => "minor"
  }
changes:
  - category: SemanticChange
    rule_id: RuleID_MatchPredicateBinding
    contains: "match pattern rebinding"
    line: 2
coverage: { ratio: 1.0 }
```

### Layer 4 — End-to-end corpus regression

The big one. Iterate every non-skipped V1 mapping in `v1spec/tests/`:

1. Take V1 mapping + input.
2. Translate to V2. Record Change list.
3. Parse V2 with `syntax.Parse` — must be valid.
4. Run V2 against same input through V2 interpreter.
5. Compare outputs:
   - Exact match → PASS.
   - Differs, but a `SemanticChange` Change record covers the difference → PASS.
   - Differs without a matching Change record → **FAIL** — translator silently lost fidelity.

### Layer 5 — Property tests

`testdata/property_test.go`:

- **Never-panic**: quickcheck-generated junk must succeed or error cleanly.
- **Valid-V2**: any successful output must `syntax.Parse` clean.
- **Coverage monotonicity**: appending a `.catch(null)` tail to a mapping never decreases coverage.
- **Comment preservation** (when `PreserveComments`): every V1 comment appears in V2 output.

### Layer 6 — Change-record coverage lint

A test that greps the spec for `^<N>\. ` (quirk numbers) and the test corpus for `rule_id:` / `spec_ref:`. Every quirk in §14 must have ≥1 test case referencing it. Every `RuleID` used in translator code must be covered by ≥1 test.

Catches drift between spec, translator, and tests.

## 9. Implementation phases

| Phase | Work |
|---|---|
| P1 | `v1ast/` — scanner, parser, AST, printer. Layer 1 tests against the V1 corpus. |
| P2 | V2 pretty-printer in `go/pratt/syntax/`. Separate commit. |
| P3 | Translator scaffolding: Report/Change/Coverage types, visitor, Layer 2 harness. Three foundational rules (`root→output`, `this→input`, bare-ident). |
| P4 | Rule implementation — arithmetic, comparison, lambda, match, if. "Core 80%". Each rule lands with Layer 2 cases. |
| P5 | Meta, sentinels, maps, imports. |
| P6 | Layer 4 corpus regression. Iterate on failures until stable. |
| P7 | Layer 5 fuzzing + Layer 6 spec-coverage linter. |

Phases P4–P6 dominate. Layer 4 will keep surfacing missing rules. Expect several iterations.

## 10. Risks

- **V1 grammar corners**: even V1's own parser has accepted edge cases we've documented. Our parser must match them, not "clean them up".
- **V2 target surface evolution**: V2 is still in active development. Some rule targets may need revision as V2 stabilises.
- **Comment preservation edge cases**: V1 accepts comments in unusual positions (mid-method-chain, inside arg lists). Statement-level preservation won't catch those; acceptable trade-off for now.
- **Coverage gaming**: a mapping dominated by simple literals could artificially inflate coverage. Per-node weighting mitigates but doesn't eliminate this. Layer 4 regression is the real correctness check; coverage is only the threshold for "abort or proceed".
