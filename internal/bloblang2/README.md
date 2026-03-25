# Bloblang V2

A redesign of the Bloblang mapping language for Redpanda Connect V5. Bloblang V2 is backed by a formal specification, designed for explicit context management, predictable behavior, and first-class tooling support.

See [`spec/PROPOSAL.md`](spec/PROPOSAL.md) for the motivation and design rationale.

## Directory Layout

- **`spec/`** — Language specification (numbered markdown files) and YAML conformance test suite
- **`go/`** — Go reference implementation (Pratt parser, tree-walking interpreter, spec test runner framework)
- **`tree-sitter/`** — Tree-sitter grammar for editor tooling (syntax highlighting, code folding)
- **`demo/`** — Interactive web playground with live execution and syntax highlighting

## Spec (`spec/`)

The specification is split across 13 numbered markdown files covering the full language — from lexical structure and type system through to the standard library. Start with [`spec/README.md`](spec/README.md) for the table of contents and a quick syntax reference.

The `spec/tests/` directory contains ~80 YAML test files organized by topic (types, operators, control flow, maps, lambdas, error handling, stdlib, edge cases, etc.). These are the canonical conformance tests — any correct implementation must pass them all. See [`spec/tests/README.md`](spec/tests/README.md) for the test schema.

## Go Implementation (`go/`)

A Pratt-parser-based compiler and tree-walking interpreter in `go/pratt/`. The compilation pipeline is:

1. **Parse** — `syntax.Parse()` produces an AST
2. **Optimize** — `syntax.Optimize()` does path collapse, constant folding, dead code elimination
3. **Resolve** — `syntax.Resolve()` performs semantic checks (name resolution, arity validation)
4. **Execute** — `eval.New()` + `interp.Run()` tree-walks the AST

The `go/spectest/` package provides a reusable test runner. Any implementation that satisfies the `spectest.Interpreter` interface can be validated against the full spec test suite. The top-level `bloblang2_test.go` does exactly this for the Go implementation:

```go
func TestBloblangV2Spec(t *testing.T) {
    spectest.RunT(t, "spec/tests", &Interp{})
}
```

## Tree-sitter Grammar (`tree-sitter/`)

A full tree-sitter grammar for Bloblang V2, suitable for syntax highlighting, code folding, and editor integration. Uses an external scanner (`src/scanner.c`) for context-sensitive newline handling.

Build tasks (requires `node_modules/tree-sitter-cli`):

```sh
cd tree-sitter
task generate   # Generate parser from grammar.js
task test       # Run corpus tests
task build-wasm # Compile to WASM
task sync-demo  # Copy WASM + highlights into demo/
task all        # Full rebuild
```

## Demo (`demo/`)

A local web playground that lets you write Bloblang V2 mappings with tree-sitter-powered syntax highlighting and autocomplete, execute them against JSON input, and see results live.

```sh
go run ./demo
# Opens http://localhost:4195 in your browser
```

Use `--addr` to change the listen address, `--no-open` to skip auto-opening the browser.

## Running Tests

From the `bloblang2/` directory:

```sh
go test ./...
```

This runs the full spec conformance suite plus unit tests for the scanner, parser, interpreter, and spectest framework.
