# Bloblang V2

A redesign of the Bloblang mapping language for Redpanda Connect V5. Bloblang V2 is backed by a formal specification, designed for explicit context management, predictable behavior, and first-class tooling support.

See [`spec/PROPOSAL.md`](spec/PROPOSAL.md) for the motivation and design rationale.

## Directory Layout

- **`spec/`** — Language specification (numbered markdown files) and YAML conformance test suite
- **`go/`** — Go reference implementation (Pratt parser, tree-walking interpreter, spec test runner framework)
- **`go/lsp/`** — Editor-agnostic LSP server for diagnostics and completions
- **`tree-sitter/`** — Tree-sitter grammar for editor tooling (syntax highlighting, code folding)
- **`plugins/nvim/`** — Neovim plugin (filetype detection, tree-sitter highlighting, LSP client)
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

## LSP Server (`go/lsp/`)

A minimal LSP server that wraps the Go compiler pipeline (Parse, Optimize, Resolve) to provide real-time editor feedback over JSON-RPC/stdio. Any editor that speaks LSP can use it.

**Diagnostics** — parse errors, undeclared variables, unknown functions/methods, arity mismatches, scope violations, map isolation, duplicate map names.

**Completions** — context-aware: methods after `.`, variables after `$`, keywords + stdlib functions + user-defined maps otherwise.

Build the binary:

```sh
cd plugins/nvim
task lsp
# Binary at plugins/nvim/bin/bloblang2-lsp
```

Point any LSP client at `bloblang2-lsp` with stdio transport. No arguments needed.

## Neovim Plugin (`plugins/nvim/`)

A Neovim plugin providing filetype detection (`.blobl2`), tree-sitter syntax highlighting, and LSP client wiring for diagnostics and completions.

**Setup** (vim-plug):

```vim
Plug '~/path/to/bloblang2/plugins/nvim'
" After plug#end():
lua require("bloblang2").setup()
```

**Build prerequisites** (run once, and after grammar changes):

```sh
cd plugins/nvim
task parser   # Compile tree-sitter .so
task lsp      # Build LSP binary
```

**Verify:** open a `.blobl2` file and run `:checkhealth bloblang2`.

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
