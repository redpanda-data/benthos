# Bloblang V1 → V2 Migrator

A Go library that takes a Bloblang V1 mapping and produces an equivalent
Bloblang V2 mapping plus a Report describing every semantic divergence it
had to introduce. 100% fidelity is not a goal — V2 is a deliberate redesign
that fixes V1 ambiguities, so some mappings will intentionally shift
semantics. The migrator's job is to make every shift visible so a human can
audit before cutover.

## Usage

```go
import "github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"

rep, err := translator.Migrate(v1Source, translator.Options{
    Verbose: true,                     // include Info-severity notes
    Files:   imports,                  // optional virtual filesystem for imports
})
if err != nil {
    // *CoverageError when the weighted translation ratio drops below
    // Options.MinCoverage (default 0.75). The Report is still reachable
    // via err.(*translator.CoverageError).Report.
    return err
}

fmt.Println(rep.V2Mapping)              // translated mapping
for _, c := range rep.Changes {          // per-site divergences
    fmt.Printf("%d:%d %s [%s %s] %s\n", c.Line, c.Column, c.Severity, c.Category, c.RuleID, c.Explanation)
}
fmt.Printf("coverage: %.2f (%d/%d translated exactly)\n",
    rep.Coverage.Ratio, rep.Coverage.Translated, rep.Coverage.Total)
```

A zero-valued `Options` means defaults (75% minimum coverage, terse
reporting). Imports declared in the V1 source must be supplied via
`Options.Files`; the migrator translates each imported file to V2 too and
surfaces them on `Report.V2Files`.

## Pipeline

```
V1 source
  │
  ├─► v1ast.Parse        — V1 parser (full reimplementation, handles the lenient
  │                         grammar as it is, not as the docs describe it)
  ├─► translator walk    — each V1 node → V2 node; every shift records a Change
  ├─► syntax.Print       — V2 AST → V2 source text
  └─► syntax.Parse       — non-fatal sanity check; failures become Changes
                           tagged RuleEmittedInvalidV2 rather than errors
                           (V1-invalid inputs produce V2-invalid outputs, and
                           real translator bugs still surface at the caller's
                           Compile)
```

## What is in the box

| Path | What it holds |
|---|---|
| `bloblang_v1_spec.md` | Reference spec for V1 reconciled from the parser source, the config-test corpus, and the official docs. Includes a `§14` catalogue of migration-critical quirks. |
| `v1ast/` | Self-contained V1 parser and AST. Exports `NodePos()` so translation rules can cite source positions on every Change. |
| `v1spec/` | V1 spec-compliance corpus — V2 spec tests translated to V1, run against the V1 interpreter. Acts as a pin on V1 behaviour so the translator has a stable target. |
| `translator/` | The migrator itself. `Migrate`, `Options`, `Report`, `Change`, `RuleID`. Translation rules live in `translate.go` (expression / statement walker) and `methods.go` (V2 method-shape rewrites). |

## Testing

Five layers. Run with `task test` or `go test ./internal/bloblang2/migrator/...`.

- **Layer 1 — V1 parser conformance** (`v1ast/parser_test.go`): parse every
  non-skipped YAML case in `v1spec/tests/`, then re-print → re-parse to
  check round-trip integrity.
- **Layer 2 — Per-rule unit tests** (`translator/rules_test.go`): one case
  per core RuleID. A regression here pinpoints the affected rule directly
  rather than showing up as an anonymous corpus drop.
- **Layer 3 — Contract tests** (`translator/migrate_test.go`,
  `change_test.go`): the `Migrate` surface and `Change` serialisation.
- **Layer 4 — End-to-end corpus regression**
  (`translator/corpus_test.go`): translate every V1 mapping in
  `v1spec/tests/`, compile the V2 output, run it through the V2
  interpreter, compare against the test's expected output.
  - **OK** — V2 matched V1's expected output.
  - **Flagged** — V1 and V2 diverged, but the translator warned via a
    `SemanticChange` or `Unsupported` Change, so the caller was told.
  - **Unexpected** — V2 diverged silently. Test failure territory.
  - Pass rate (OK + Flagged) is pinned via a floor in the test so real
    regressions trip the gate.
- **Layer 5 — Property tests** (`translator/property_test.go`):
  never-panic on junk input, valid-V2 output always parses, Coverage.Ratio
  always in [0,1], every Change has a non-zero position and non-empty
  explanation.

## Design choices worth knowing about

- **Adopt V2, flag the shift**. Where V1 and V2 diverge, the translator
  picks V2 semantics and records a `SemanticChange` Change pointing at the
  `§14` anchor in the spec. Faithfully preserving V1 quirks (e.g. `.or()`
  catching errors, `%` truncating floats, bare-ident shadowing) would mean
  writing V2 code that exists to ape the old language's mistakes.
- **Null-safe by default on bare paths**. V2 errors on field access over
  non-object receivers; V1 returned null. Every bare-ident rewrite and
  translated numeric path segment emits `?.` / `?[]` so V1's silent-null
  behaviour carries over, and the divergence is still flagged for audits
  where the receiver type matters.
- **Non-fatal sanity parse**. The final `syntax.Parse` on the emitted V2
  text is recorded as a Change rather than an error. Several V1 compile
  errors (chained comparisons, missing imports, duplicate namespaces) echo
  as V2 parse errors — treating them as translator bugs was noisy and
  wrong.
- **Fixpoint import translation**. Imported V1 files are translated in
  dependency order so transitively imported files finish before their
  dependants; cycles take a final pass with whatever siblings did
  translate. Nested imports that aren't statically resolvable stay
  unqualified and get a `RuleImportStatement` note — V2's namespaces don't
  re-export transitively the way V1's flat map table did.
- **`v1ast` is deliberately separate**. The official V1 parser is buried
  in Benthos's `bloblang/parser/` and isn't easy to use as an AST source.
  Reimplementing it as a standalone package here keeps the migrator
  independent of runtime concerns and lets us round-trip V1 source
  verbatim for printing.

## Contributing

- **Spec corrections**: edit `bloblang_v1_spec.md` and cite the source
  (`bloblang/parser/...` or a config-test fixture). The spec is authoritative
  for the translator — if the spec is wrong, the translator encodes a bug.
- **New rules**: add the RuleID to `translator/change.go` (append only; never
  reuse values), emit the Change from the translation site, and add a case to
  `translator/rules_test.go` asserting both the V2 substring and the RuleID.
- **New V2 output behaviours**: update `go/pratt/syntax/print.go`, not the
  translator — the translator emits V2 AST nodes, not text.

## V1 spec provenance

The V1 spec was reconciled from three sources, with the implementation
winning whenever the implementation and docs disagreed:

1. **Reference implementation** — `../../bloblang/` in this repo. Key
   files: `parser/mapping_parser.go`, `parser/query_*.go`,
   `query/arithmetic.go`, `mapping/assignment.go`, `environment.go`.
2. **Conformance-ish corpus** — `../../../config/test/bloblang/`: real
   mappings and expected outputs. Inline `*_test.go` files next to the
   parser packages were the best source for *rejected* syntax.
3. **Official documentation** — `docs.redpanda.com/redpanda-connect/`
   pages under `guides/bloblang/`. Treated as a strong prior, superseded
   by the implementation where they differed.

Documentation disagreements with the implementation are called out in
`bloblang_v1_spec.md` itself.
