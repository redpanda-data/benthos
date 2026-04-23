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
- **`migrator/`** — V1→V2 translator library, V1 AST package, and a side-by-side playground
- **`demo/`** — Interactive web playground with live execution and syntax highlighting

## Spec (`spec/`)

The specification is split across 13 numbered markdown files covering the full language — from lexical structure and type system through to the standard library. Start with [`spec/README.md`](spec/README.md) for the table of contents and a quick syntax reference.

The `spec/tests/` directory contains ~130 YAML test files organized into subdirectories by topic (types, operators, control flow, maps, lambdas, error handling, stdlib, edge cases, etc.). These are the canonical conformance tests — any correct implementation must pass them all. See [`spec/tests/README.md`](spec/tests/README.md) for the test schema.

## Go Implementation (`go/`)

A Pratt-parser-based compiler and tree-walking interpreter in `go/pratt/`. The compilation pipeline is:

1. **Parse** — `syntax.Parse()` produces an AST
2. **Optimize** — `syntax.Optimize()` does path collapse, constant folding, dead code elimination
3. **Resolve** — `syntax.Resolve()` performs semantic checks (name resolution, arity validation) and annotates AST nodes with opcode IDs and variable stack slot indices
4. **Execute** — `eval.NewWithStdlib()` + `interp.Run()` tree-walks the AST using opcode dispatch and stack-based variable access

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

Build the binary with `task build:nvim:lsp` (output at `plugins/nvim/bin/bloblang2-lsp`). Point any LSP client at it with stdio transport — no arguments needed.

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
task build:nvim:parser   # Compile tree-sitter .so
task build:nvim:lsp      # Build LSP binary
```

**Verify:** open a `.blobl2` file and run `:checkhealth bloblang2`.

## Tree-sitter Grammar (`tree-sitter/`)

A full tree-sitter grammar for Bloblang V2, suitable for syntax highlighting, code folding, and editor integration. Uses an external scanner (`src/scanner.c`) for context-sensitive newline handling.

Build tasks (requires `node_modules/tree-sitter-cli`):

```sh
task build:tree-sitter:generate    # Generate parser from grammar.js
task test:tree-sitter              # Run corpus tests
task build:tree-sitter:wasm        # Compile to WASM
task build:tree-sitter             # Full rebuild (generate, test, WASM, sync into demo)
```

## Demo Playgrounds

Two local web playgrounds are available. Both share the tree-sitter WASM and TypeScript bundle built by `task build:demo`.

**`demo/` — V2 playground.** Write V2 mappings with tree-sitter-powered syntax highlighting and autocomplete, execute them against JSON input, and see results live. An engine selector toggles between browser-side execution (via the TypeScript interpreter) and server-side execution (via the Go interpreter).

```sh
task demo
# Builds demo assets then opens http://localhost:4195 in your browser
```

**`migrator/demo/` — V1→V2 migrator playground.** Write a V1 mapping in the bottom-left editor and see the translator's V2 output appear in the bottom-right. A toggle picks whether the top-right output pane runs the V1 mapping (official Redpanda Connect engine) or the translated V2 mapping (Go Pratt interpreter).

```sh
task demo:migrator
# Opens http://localhost:4196 in your browser
```

## Performance

The Go implementation is benchmarked against Bloblang V1 using two non-trivial case studies (Stripe invoice normalization and GitHub webhook processing). V2 is faster than V1 on both, with lower memory usage and fewer allocations:

| | V2 | V1 | V2 vs V1 |
|---|---|---|---|
| Stripe (ns/op) | 8,483 | 9,620 | 12% faster |
| Stripe (B/op) | 5,112 | 7,528 | 32% less memory |
| Stripe (allocs/op) | 96 | 128 | 25% fewer |
| GitHub (ns/op) | 11,283 | 13,568 | 17% faster |
| GitHub (B/op) | 6,643 | 7,227 | 8% less memory |
| GitHub (allocs/op) | 155 | 189 | 18% fewer |

Key optimizations in the Go interpreter:

- **Opcode dispatch** — stdlib methods and functions are assigned compile-time integer IDs by the resolver. The interpreter dispatches via slice index instead of map lookup.
- **Variable stack** — the resolver assigns stack slot indices to all variables and parameters. The interpreter uses a flat `[]any` stack with frame-based indexing instead of a linked scope chain.
- **Interpreter reuse** — compiled mappings reuse the interpreter and its stack across executions.
- **Zero-alloc iterator args** — lambda arguments in iterator methods (map, filter, etc.) use stack-allocated buffers instead of heap-allocated slices.

Run the benchmarks with `task test:go` or directly:

```sh
go test ./... -bench=. -benchtime=3s -run='^$'
```

## Building & Testing

All build and test tasks are managed via [go-task](https://taskfile.dev) from the `bloblang2/` directory. Run `task --list` for the full list. Key tasks:

```sh
task test              # Run all tests (Go + TypeScript + tree-sitter)
task test:go           # Go spec conformance and unit tests only
task test:ts           # TypeScript spec conformance and unit tests only
task test:tree-sitter  # Tree-sitter corpus tests only

task build             # Build all artifacts (tree-sitter, TS bundle, nvim plugin)
task build:demo        # Build just the demo assets (tree-sitter WASM + TS bundle)
task demo              # Build demo assets and launch the V2 playground
task demo:migrator     # Launch the V1→V2 migrator playground

task clean             # Remove all build artifacts
```
