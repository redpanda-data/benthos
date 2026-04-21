# Bloblang V1 → V2 Migrator

Tooling for mechanically migrating existing Bloblang V1 mappings to Bloblang V2. V2 is a redesign of the language with stricter semantics, explicit context management, and a formal specification (see `../spec/`). Existing V1 mappings in the wild — some of them large, some of them load-bearing — need a clear path forward, and the point of this directory is to build that path.

## Goal

Produce, over time, a set of tools that can:

1. **Parse** V1 mappings (including their legacy/lenient forms) into a faithful AST.
2. **Recognise** every V1 idiom that has a direct V2 counterpart, every idiom that needs rewriting, and every idiom that has no V2 equivalent yet.
3. **Rewrite** mechanically where rewrites are safe, and **annotate** where human review is required.
4. **Report** on the migration surface of a V1 mapping — what would change, what would not, what is ambiguous.

This directory starts with the **specification** of V1 as its own deliverable. A migration tool is only as correct as its understanding of the source language; writing the spec first forces us to catalogue every quirk up front rather than discover them as regressions later.

## Current contents

- **`bloblang_v1_spec.md`** — the complete V1 reference specification. A single document describing lexical structure, types, literals, operators (with verified precedence), paths, statements, expressions, the two dialects (full mapping vs. `${!...}` interpolation), error model, extensibility, and a catalogue of 25 migration-critical quirks. Includes an informal EBNF and a file map back to the reference implementation.
- **`tests/`** — V1 equivalents of every V2 conformance test under `../spec/tests/`. Same YAML schema (shared with the V2 spec runner), with one addition: a `skip: "<reason>"` field on tests that have no direct V1 equivalent. Same per-subdirectory layout as the V2 suite (`access/`, `case_studies/`, `control_flow/`, …).
- **`interp.go`** — V1 adapter implementing `spectest.Interpreter` from `../go/spectest`. Uses `mapping.Executor.ExecOnto` directly so the raw mapped value is preserved (avoiding V1's bytes-round-trip quirk that would re-parse scalars like `"true"` as booleans).
- **`runner.go`** — thin wrapper around `spectest.RunT` that pre-scans each YAML file for the `skip:` field and surfaces those tests through `t.Skip` instead of compiling them.
- **`migrator_test.go`** — the Go test entrypoint: `TestBloblangV1Spec` runs every file under `tests/` against the V1 adapter.

Further tooling (parser, AST diff, rewriter, report generator) will be added alongside the spec.

## Running the test suite

From the `bloblang2/` directory:

```sh
task test:migrator               # run everything
task test:migrator -- -v         # verbose output
task test:migrator -- -run 'TestBloblangV1Spec/types/bool_null'   # single file
```

Or from the benthos repo root:

```sh
go test ./internal/bloblang2/migrator/... -run TestBloblangV1Spec
```

A test is PASS when the V1 interpreter produces the `output` / `deleted` / `error` the YAML expects, FAIL when it differs, and SKIP when the YAML entry has a `skip:` field. Failures are an expected outcome of this suite — they flag either (a) a V1-vs-V2 semantic divergence that the migrator tool will need to handle, (b) an over-eager translation from the V2 test, or (c) a `# FIXME-v1: verify` substring that was left for a real V1 run to confirm. The value of the suite is in the delta, not in a green bar.

## Sources used to produce the V1 spec

The spec was reconciled from three primary sources, preferring the implementation wherever the documentation and implementation disagreed.

### 1. Reference implementation

`../../bloblang/` in this repository — the canonical V1 parser, evaluator, and built-in catalogue. Key files consulted:

- `parser/mapping_parser.go` — statement dispatch, assignment targets, `let`, `map`, `import`, `from`.
- `parser/query_parser.go` and `parser/query_function_parser.go` — expression primaries, path access, method/function dispatch, variable/metadata references.
- `parser/query_arithmetic_parser.go` — flat arithmetic parse and unary minus handling.
- `parser/query_expression_parser.go` — `if`, `match`, lambdas, bracketed map expressions.
- `parser/query_literal_parser.go`, `parser/combinators.go` — literal grammars (number, string, triple-quoted, array, object).
- `parser/field_parser.go` and `field/` — the `${!...}` interpolation dialect.
- `parser/root_expression_parser.go` — root-level `if` / `else if` / `else`.
- `query/arithmetic.go` — build-time precedence resolution (four-pass reduction), operator semantics, coalesce behaviour.
- `query/functions*.go`, `query/methods*.go`, `query/function_set.go`, `query/method_set.go` — function/method registration.
- `query/docs.go`, `query/params.go` — per-builtin metadata (status, category, named-args support).
- `mapping/assignment.go`, `mapping/statement.go` — assignment target evaluation, delete/nothing sentinel handling.
- `environment.go` — plugin API, import policies, purity restrictions.

### 2. Conformance-ish corpus

`../../../config/test/bloblang/` — real mappings and expected outputs used as integration tests. These cover idioms that the implementation accepts but that the docs do not always advertise:

- `cities.blobl`, `csv_formatter.blobl`, `github_releases.blobl` — full mappings exercising `map`/`apply`, match patterns, `fold`, `map_each`, and string manipulation.
- `*_test.yaml` files — input/output pairs for each `.blobl` mapping.
- `boolean_operands.yaml`, `literals.yaml`, `csv.yaml`, `fans.yaml`, `env.yaml`, `message_expansion.yaml`, `walk_json.yaml`, `windowed.yaml` — focused tests for operator precedence, literal forms, environment access, batch handling, and meta manipulation.

Inline `*_test.go` files next to the parser and query packages were consulted for parse edge cases — these were the most valuable source for rejected syntax (what the parser *doesn't* accept):

- `parser/combinators_test.go` — pinpointed the number-literal grammar (digits required on both sides of the decimal), triple-quoted string edge cases, and the lenient comma rules for arrays/objects.
- `parser/mapping_parser_test.go` — confirmed that two statements on one line are rejected, that `from` cannot be mixed with other statements, and the exact error message for the bare-expression-shorthand single-statement restriction.
- `parser/query_literal_parser_test.go` — confirmed computed-key object syntax `{(expr): val}` and the rejection of non-string literal keys.
- `parser/query_arithmetic_parser_test.go` — provided concrete inputs exercising `|` coalesce and `&&`/`||` chains that informed the precedence description.
- `parser/query_expression_parser_test.go` — match separator flexibility (commas, newlines, both) and the literal-vs-expression classification of match patterns.
- `parser/field_parser_test.go` — confirmed the `${{!...}}` literal-output escape form.
- `query/arithmetic.go` + `query/arithmetic_test.go` — the four-pass precedence reduction and the `numberDegradationFunc` coercion rules.
- `query/methods.go` — `.catch` / `.or` exact semantics; `.apply` variable-isolation reset; `iterator.go` for how `deleted()` and `nothing()` propagate through `.map_each`.
- `query/type_helpers.go` — `IIsNull` treating `deleted()` / `nothing()` as null for coalesce purposes; `ICompare` for structural equality.

### 3. Official documentation

From `docs.redpanda.com/redpanda-connect/`:

- `guides/bloblang/about/` — overview, assignment syntax, literals, control flow, coalesce, `.or()` vs `.catch()`, `deleted()` semantics.
- `guides/bloblang/arithmetic/` — operator categories and operand typing (noted for its **absence** of a precedence table — the spec's precedence section is derived from the implementation).
- `guides/bloblang/walkthrough/` — a tutorial that introduces idioms not mentioned on the main page, including the `.(name -> body)` bracketed named-capture form, the `without()` / `not_empty()` validators, and recursive `map`-definition patterns.
- `guides/bloblang/advanced/` — map-parameter passing via object literals, stateful `count()`, `sort_by()`, `key_values()`, and recursive tree-walking idioms.
- `configuration/interpolation/` — the `${!...}` dialect: what expression forms are permitted, the `${{!...}}` literal-output escape, multi-interpolation in a single field.
- `guides/bloblang/functions/` and `guides/bloblang/methods/` — linked as the authoritative per-builtin reference rather than inlined.

Documentation was treated as a strong prior, but superseded by the implementation where they differed. Disagreements are called out in the spec.

## Contributing to the spec

If you find a V1 construct that the spec does not cover, or a case where the spec contradicts the reference implementation, update `bloblang_v1_spec.md` and cite the source file and line. The spec is meant to be self-correcting over the life of the migration.
