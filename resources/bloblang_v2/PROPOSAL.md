# Bloblang V2: Proposal for Redpanda Connect V5

## Context

Redpanda Connect V5 is an opportunity to commit breaking changes. Any breaking change must meet a high bar: it should either reduce maintenance burden without impacting the majority of users, or improve the product in ways that aren't possible with backwards compatibility — ideally both.

After consulting with a large number of customers we've determined that Bloblang is well-liked. Users generally don't want to migrate away from it, but they do want improvements to the language and better development tooling. This positions Bloblang as something to improve upon, not replace.

## The Problem

Bloblang V1 was never backed by a formal specification. The language grew organically, accumulating inconsistencies and ambiguities that make it:

- **Hard to parse correctly** — edge cases and implicit behaviors make writing a correct parser difficult, and writing multiple parsers (e.g. in different languages) impractical.
- **Hard to build tooling for** — without a formal grammar, building LSP servers, linters, formatters, and other developer tools requires reverse-engineering behavior from the Go implementation.
- **Hard for AI to assist with** — LLMs cannot reliably help users write or debug mappings without a complete, unambiguous reference to ground their output against.
- **Expensive to maintain** — the existing parser and execution engine carry the weight of every historical ambiguity and implicit behavior.

## V1 Pain Points

These are concrete examples of ambiguities and traps in Bloblang V1 that cost us maintenance time and cost users debugging time. Each exists because V1 evolved without a formal spec, and each is resolved in V2.

### `this` silently changes meaning

The most significant design problem. At top level, `this` refers to the input document. Inside `map_each`, it silently shifts to refer to the current array element — or, for objects, to a synthetic `{"key": k, "value": v}` wrapper that appears from nowhere:

```bloblang
# V1: the two uses of `this` mean completely different things
root.names = this.users.map_each(this.name.uppercase())
#            ^^^^                ^^^^
#            input document      current array element
```

For objects it's worse — bare identifiers like `key` and `value` implicitly refer to fields on a hidden context object:

```bloblang
# V1: where do `key` and `value` come from? nowhere obvious
root.result = this.dict.map_each(key + ": " + value)
```

V2 fixes this with explicit lambda parameters:
```bloblang
# V2: no ambiguity
output.names = input.users.map(user -> user.name.uppercase())
output.result = input.dict.map_entries((k, v) -> {"key": k, "value": k + ": " + v})
```

### Bare identifiers silently resolve to `this.field`

Any unrecognized bare identifier is silently treated as `this.<identifier>`. A typo like `inpt.name` instead of `input.name` parses without error and returns `null` at runtime. The parser has a `TODO V5: Remove this and force this, root, or named contexts` comment acknowledging this.

### Assignment targets don't require `root`

`root.foo = bar` and `foo = bar` are silently identical — the `root.` prefix is stripped if present but not required. This means the left and right sides of an assignment use different naming conventions: on the left, `foo` means `root.foo`; on the right, `foo` means `this.foo`. The parser has a `TODO V5: Enforce root here` comment.

### `null` is silently treated as `false` in conditionals

`if this.field { ... }` treats a `null` value (field doesn't exist) the same as `false` (field exists and is explicitly false). These are semantically different — a missing field is not the same as a false field. The implementation has a `TODO V5: Remove this` comment explaining this was discovered after users were already relying on it, so it couldn't be fixed without a breaking change.

### `or()` conflates null, errors, deletion, and "nothing"

The `.or()` method activates on errors, null values, `deleted()` markers, and the internal `Nothing` type — four semantically distinct conditions handled by one operator. Users cannot distinguish between "the value was legitimately null" and "an error occurred during evaluation":

```bloblang
# V1: all of these return "fallback" — but for very different reasons
this.missing_field.or("fallback")   # null
(5 / 0).or("fallback")             # error
deleted().or("fallback")            # deletion marker
nothing().or("fallback")            # internal nothing type
```

V2 separates these cleanly: `.or()` handles null and void (absence of a value), `.catch()` handles only errors.

### The pipe `|` looks like logical OR but is coalesce

The `|` operator is a null/error coalesce, not a logical OR — but it uses the same symbol most developers associate with boolean logic. Like `.or()`, it silently swallows both errors and null values:

```bloblang
# V1: this silently catches JSON parse errors, not just null
root.city = this.user.address.city | "Unknown"
```

V2 removes `|` entirely and provides `?.` for null-safe navigation and `.or()` for explicit null/void fallback.

### Numbers silently coerce to booleans

The `&&` and `||` operators accept numbers, coercing nonzero to `true` and zero to `false`. This means `5 && true` silently evaluates to `true` and `0 || false` evaluates to `false` — C-style implicit coercion that leads to subtle bugs. V2 requires booleans for logical operators.

### Overlapping metadata APIs

V1 has four-plus ways to access metadata: `meta("key")` (deprecated, string-only, reads input), `root_meta("key")` (deprecated, string-only, reads output), `metadata("key")` (any-typed, reads input), and `@key` (any-typed, reads output). The deprecated `meta()` function also can't distinguish between an empty string value and a missing key — both return `null`. V2 has exactly two: `input@.key` and `output@.key`.

### No null-safe navigation

V1 has no `?.` operator. The only way to handle potentially-null nested access is the pipe operator `|`, which also swallows errors (see above). There is no way to say "navigate this path, short-circuiting on null but still surfacing type errors."

### Match mixes equality and boolean cases

A V1 `match` block decides between equality comparison and boolean evaluation based on whether each case is a literal value or a dynamic expression. You can mix both forms in the same block, and the parser silently picks the mode per-case:

```bloblang
# V1: first case is boolean evaluation, second is equality comparison
match this.score {
  this.score >= 100 => "gold"   # boolean: is the result true?
  50 => "exactly fifty"         # equality: is score == 50?
  _ => "other"
}
```

V2 has three explicit match forms and makes mixing them a compile error.

## The Proposal

I propose introducing **Bloblang V2**: a redesigned version of the language with a formal specification, developed and validated with the assistance of AI coding agents. The language preserves what users like about Bloblang (expressive, composable, familiar) while fixing what costs us (ambiguity, inconsistency, implicit behavior).

### Design Principles

1. **Radical Explicitness** — no implicit context shifting; all references are explicit (`input`/`output` instead of overloaded `this`/`root`).
2. **One Clear Way** — a single obvious approach for each operation, reducing cognitive load and eliminating "which way do I do this?" moments.
3. **Consistent Syntax** — symmetrical keywords, consistent prefixes, predictable patterns.
4. **Fail Loudly** — errors are explicit, not silent. No more wondering whether a mapping silently dropped data.

### Key Language Improvements

- **Explicit contexts**: `input`/`output` replace overloaded `this`/`root`
- **Expanded type system**: 14 runtime types including explicit timestamps, lambdas, and multiple integer/float widths
- **Null-safe navigation**: `?.` and `?[]` operators
- **Isolated maps with parameters**: maps become proper functions — no implicit access to input/output context
- **Namespace imports**: `import "./utils.blobl" as utils` with `utils::function()` syntax
- **Formal grammar**: unambiguous, machine-parseable specification

## Development Plan

The plan leverages AI coding agents throughout the process — not as a shortcut, but as a forcing function. If the spec is good enough for AI to build correct implementations and tooling from, it's good enough for humans too.

### Phase 1: Specification (in progress)

Build a formal spec and iterate until both humans and AI are satisfied with its consistency and ergonomics. The spec must be:
- Unambiguous enough for a parser to be generated from it
- Complete enough that an AI agent can answer any question about the language
- Compressed enough to fit in an LLM context window

**Status**: In progress. The specification lives on a branch as 13 sections covering the full language, from lexical structure through a formal grammar to a complete standard library reference.

### Phase 2: Test Suite

Generate a thorough test suite of mappings covering the entire breadth of the language. These are spec-level conformance tests — any correct implementation of Bloblang V2 should pass them all.

### Phase 3: Multi-Implementation Validation

Prove the robustness of the spec by having multiple independent AI agents generate implementations, then run those implementations against the conformance test suite from Phase 2. If different agents, working only from the spec, produce implementations that agree on all test cases, the spec is unambiguous. Where they disagree, we've found a gap in the spec to fix.

### Phase 4: Tooling Generation

Prove that agents can generate useful development tooling directly from the spec: LSP servers, syntax highlighters, formatters, linters. This validates that the spec supports the tooling ecosystem users have asked for.

### Phase 5: AI-Assisted Development

Test agents' ability to assist with language development when provided the spec. Can they help users write mappings? Debug them? Explain error messages? This validates the spec as a grounding document for AI-assisted workflows.

### Phase 6: Opt-In Introduction

If phases 2–5 are successful, introduce Bloblang V2 as an optional processor in Redpanda Connect (still V4), and gather real user feedback before any forced migration.

### Phase 7: Migration Tooling

Build a tool that converts Bloblang V1 mappings to V2. Critically, this tool must surface areas where V1 mappings rely on poorly defined or implicit behavior, so users can verify the converted output rather than trusting a silent best-effort translation.

### Phase 8: Full Config Migration

Expand the migration tool to convert entire Redpanda Connect V4 configs to V5, including Bloblang V1→V2 conversion as one component of the broader migration.

## Why This Approach

### AI changes the cost equation for DSLs

Maintaining a domain-specific language has historically been expensive: parsers, tooling, documentation, and support all require ongoing investment. AI coding agents are changing this equation. With a good formal spec, much of this work — parser generation, tooling, documentation, user assistance — can be substantially automated. The investment shifts from "maintain everything by hand" to "maintain one excellent spec and generate the rest."

### AI will be helping users write Bloblang regardless

Users are already asking AI to help them write and debug Bloblang mappings. Today that assistance is grounded in whatever the LLM absorbed from documentation and blog posts during training — an incomplete and sometimes inaccurate picture. A formal spec gives AI a complete, authoritative reference to work from, making the assistance users are already seeking dramatically more reliable.

### A spec-first approach de-risks the whole effort

By validating the spec through multiple independent implementations and generated tooling before committing to a release, we catch design problems early. If agents struggle to implement a feature correctly from the spec, that's a signal the feature is under-specified or poorly designed — and we can fix it before users ever see it.

### Multi-language parsers unlock new deployment models

Bloblang V1 exists only as a Go implementation. A spec that's proven to be implementable by AI agents in multiple languages opens the door to native Bloblang support in other runtimes — WebAssembly for browser-based tooling, TypeScript for VS Code extensions, Rust for performance-critical paths — without maintaining each implementation by hand.
