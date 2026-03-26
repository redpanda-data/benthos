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

## Spec Agent (`specagent/`)

A utility for testing a coding agent's comprehension of the Bloblang V2 specification. It generates isolated "clean rooms" from the spec test suite, invokes an agent, and scores the results.

Two challenges are generated:

- **Predict Output** — agent receives a mapping + input and must predict the output
- **Predict Mapping** — agent receives an input + output and must write a mapping that produces it

Each clean room contains only the spec prose and test files — no implementation code, examples, or other reference material. The agent gets a fresh context per clean room.

```sh
# 1. Generate clean rooms
task specagent:prepare -- --output /tmp/specagent

# 2. Run the agent in each clean room (uses claude CLI by default)
task specagent:run -- --dir /tmp/specagent

# 3. Score results
task specagent:evaluate -- --dir /tmp/specagent --verbose
```

Evaluation for predict-output compares JSON outputs directly. Evaluation for predict-mapping compiles and executes the agent's mapping through the real Bloblang V2 interpreter — any correct mapping passes, not just the original. Results are printed as a table broken down by test category.

Run `task specagent -- <subcommand> --help` for all available flags (model selection, mode filtering, max turns, etc.).

### Results

The first run of specagent against the full verbose spec gave these results:

+----------------+----------------------+----------------------+
| Category       | predict_output       | predict_mapping      |
+----------------+----------------------+----------------------+
| access         | 100.0% (85/85)       |  98.8% (84/85)       |
| case_studies   |  80.0% (8/10)        |  10.0% (1/10)        |
| control_flow   |  99.3% (140/141)     |  87.9% (124/141)     |
| edge_cases     | 100.0% (114/114)     |  85.1% (97/114)      |
| error_handling |  97.5% (77/79)       |  98.7% (78/79)       |
| input_output   |  89.0% (121/136)     | 100.0% (136/136)     |
| lambdas        | 100.0% (93/93)       |  89.2% (83/93)       |
| maps           |  97.5% (118/121)     |  90.9% (110/121)     |
| operators      |  99.1% (223/225)     |  99.1% (223/225)     |
| optimizations  | 100.0% (82/82)       |  90.2% (74/82)       |
| stdlib         |  99.1% (640/646)     |  51.4% (332/646)     |
| types          |  98.0% (296/302)     |  99.0% (299/302)     |
| variables      |  99.2% (125/126)     |  97.6% (123/126)     |
+----------------+----------------------+----------------------+
| TOTAL          |  98.2% (2122/2160)   |  81.7% (1764/2160)   |
+----------------+----------------------+----------------------+

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
