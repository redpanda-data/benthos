# Bloblang V2

A redesign of the Bloblang mapping language for Redpanda Connect V5. Bloblang V2 is backed by a formal specification, designed for explicit context management, predictable behavior, and first-class tooling support.

See [`spec/PROPOSAL.md`](spec/PROPOSAL.md) for the motivation and design rationale.

## Directory Layout

- **`spec/`** — Language specification (numbered markdown files) and YAML conformance test suite
- **`go/`** — Go reference implementation (Pratt parser, tree-walking interpreter, spec test runner framework)
- **`go/lsp/`** — Editor-agnostic LSP server for diagnostics and completions
- **`ts/`** — TypeScript implementation (scanner, parser, optimizer, resolver, interpreter, stdlib)
- **`tree-sitter/`** — Tree-sitter grammar for editor tooling (syntax highlighting, code folding)
- **`plugins/nvim/`** — Neovim plugin (filetype detection, tree-sitter highlighting, LSP client)
- **`demo/`** — Interactive web playground with live execution and syntax highlighting

## Spec (`spec/`)

The specification is split across 13 numbered markdown files covering the full language — from lexical structure and type system through to the standard library. Start with [`spec/README.md`](spec/README.md) for the table of contents and a quick syntax reference.

The `spec/tests/` directory contains ~95 YAML test files organized into subdirectories by topic (types, operators, control flow, maps, lambdas, error handling, stdlib, edge cases, etc.). These are the canonical conformance tests — any correct implementation must pass them all. See [`spec/tests/README.md`](spec/tests/README.md) for the test schema.

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

## TypeScript Implementation (`ts/`)

A full TypeScript implementation of the Bloblang V2 language: scanner, parser, optimizer, resolver, interpreter, and standard library. Follows the same compilation pipeline as the Go implementation and is validated against the spec conformance suite via Vitest.

The bundled output (`bloblang2.mjs`) is used by the demo playground for browser-side execution.

## LSP Server (`go/lsp/`)

A minimal LSP server that wraps the Go compiler pipeline (Parse, Optimize, Resolve) to provide real-time editor feedback over JSON-RPC/stdio. Any editor that speaks LSP can use it.

**Diagnostics** — parse errors, undeclared variables, unknown functions/methods, arity mismatches, scope violations, map isolation, duplicate map names.

**Completions** — context-aware: methods after `.`, variables after `$`, keywords + stdlib functions + user-defined maps otherwise.

Build the binary with `task nvim:lsp` (output at `plugins/nvim/bin/bloblang2-lsp`). Point any LSP client at it with stdio transport — no arguments needed.

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
task nvim:parser   # Compile tree-sitter .so
task nvim:lsp      # Build LSP binary
```

**Verify:** open a `.blobl2` file and run `:checkhealth bloblang2`.

## Tree-sitter Grammar (`tree-sitter/`)

A full tree-sitter grammar for Bloblang V2, suitable for syntax highlighting, code folding, and editor integration. Uses an external scanner (`src/scanner.c`) for context-sensitive newline handling.

Build tasks (requires `node_modules/tree-sitter-cli`):

```sh
task tree-sitter:generate    # Generate parser from grammar.js
task tree-sitter:test        # Run corpus tests
task tree-sitter:build-wasm  # Compile to WASM
task tree-sitter:sync-demo   # Copy WASM + highlights into demo/
task tree-sitter:all         # Full rebuild
```

## Demo (`demo/`)

A local web playground that lets you write Bloblang V2 mappings with tree-sitter-powered syntax highlighting and autocomplete, execute them against JSON input, and see results live. An engine selector lets you choose between browser-side execution (via the TypeScript interpreter) and server-side execution (via the Go interpreter).

```sh
task demo
# Builds demo assets then opens http://localhost:4195 in your browser
```

## Building & Testing

All build and test tasks are managed via [go-task](https://taskfile.dev) from the `bloblang2/` directory. Run `task --list` for the full list. Key tasks:

```sh
task test              # Run all tests (Go + TypeScript + tree-sitter)
task test:go           # Go spec conformance and unit tests only
task ts:test           # TypeScript spec conformance and unit tests only
task tree-sitter:test  # Tree-sitter corpus tests only

task build             # Build all artifacts (tree-sitter, TS bundle, nvim plugin)
task build:demo        # Build just the demo assets (tree-sitter WASM + TS bundle)
task demo              # Build demo assets and launch the playground

task clean             # Remove all build artifacts
```
