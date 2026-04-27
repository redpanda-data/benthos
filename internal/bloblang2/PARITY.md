# Bloblang V1 → V2 Plugin Parity

Tracking sheet for porting V1 stdlib functions and methods to V2 within this
repo. Generated from a sweep of `internal/bloblang/query/` (V1 internal stdlib)
and `internal/impl/pure/bloblang_*.go` (V1 plugin registrations) against
`internal/bloblang2/go/pratt/eval/{stdlib,stdlib_lambda}.go` (V2 internal
stdlib).

Deprecated V1 entries are excluded.

## Status legend

- ✅ already in V2 (incl. batches 1 / 2 ports landed in `internal/impl/{pure,io}`)
- 🟢 batch 1 — pure stdlib port, mechanical (now ✅ except `format`)
- 🟡 batch 2 — array / object completeness (now ✅)
- 🔴 batch 3 — pipeline-coupled, needs design
- ❌ V1-only by architectural choice; V2 won't port
- ➕ V2-only addition (no V1 equivalent)

> **Batches 1 and 2 complete.** Pure ports live in
> `internal/impl/pure/bloblangv2_*.go`; non-pure (clock / randomness)
> live in `internal/impl/io/bloblangv2_*.go`. The single deferred entry
> is `format`, which V1 implemented as a variadic — V2 deliberately
> removed variadic params (commit `d73db44a6`), so a V2 `format` needs
> an array-param API redesign rather than a mechanical lift.

## Plugin lambda parameters (resolved)

`bloblangv2.PluginSpec` now supports lambda-typed parameters through
`NewLambdaParam(name)`. Plugin authors retrieve the lambda as a
`bloblangv2.Lambda` callable via `ParsedParams.GetLambda(name)` and
invoke it with positional argument values; bare map references
(spec §5.5) are accepted on the call-site and synthesised into
single-parameter lambdas automatically.

Plumbing sketch:

- `eval.MethodSpec` / `FunctionSpec` gained a `PluginFn` dispatch shape
  that receives the interpreter alongside unevaluated AST args. The
  plugin layer routes specs with any `paramKindLambda` parameter
  through this path, evaluates non-lambda args eagerly, and wraps
  lambda positions into a `Lambda` closure that calls back through
  `interp.CallLambda`.
- `eval.Interpreter.CallLambda` and `ExtractLambdaOrMapRef` are
  exported for the plugin layer to use.
- Static-arg folding is bypassed for plugins with lambda params
  (lambdas are not values), matching V2 stdlib semantics.

## Architectural choices we are NOT porting

| V1 name | V2 equivalent / reason |
|---------|------------------------|
| `from`, `from_all` | V1 batch context. V2 is per-message; batch operations belong in the processor layer. |
| `apply` | V1 named-map invocation. V2 has explicit `map name(arg) { ... }` and bare-name calls (spec §5). |
| `map_each` | Superseded by V2 `map`, which already takes a lambda. |
| `not` | V2 has the `!` operator (spec §3). |

## Comparison

### Type coercion

| Name | Type | Status | Notes |
|---|---|---|---|
| `bool` | method | ✅ | |
| `bytes` | method | ✅ | |
| `number` | method | ✅ | V2 has typed `int64`/`float64`/etc.; `number` would be a permissive coerce. |
| `string` | method | ✅ | |
| `timestamp` | method | ✅ | |
| `type` | method | ✅ | |

### Strings

| Name | Type | Status | Notes |
|---|---|---|---|
| `capitalize` | method | ✅ | |
| `decode` | method | ✅ | |
| `encode` | method | ✅ | |
| `escape_html` | method | ✅ | |
| `escape_url_query` | method | ✅ | |
| `filepath_join` | method | ✅ | Path manipulation — pure, no FS access. |
| `filepath_split` | method | ✅ | |
| `format` | method | 🟢 | printf-style. |
| `has_prefix` | method | ✅ | |
| `has_suffix` | method | ✅ | |
| `hash` | method | ✅ | |
| `index_of` | method | ✅ | |
| `join` | method | ✅ | |
| `lowercase` | method | ✅ | |
| `parse_url` | method | ✅ | |
| `quote` | method | ✅ | |
| `repeat` | method | ✅ | |
| `replace` | method | ✅ | |
| `replace_all` | method | ✅ | |
| `replace_all_many` | method | ✅ | |
| `replace_many` | method | ✅ | |
| `reverse` | method | ✅ | |
| `split` | method | ✅ | |
| `trim` | method | ✅ | |
| `trim_prefix` | method | ✅ | |
| `trim_suffix` | method | ✅ | |
| `unescape_html` | method | ✅ | |
| `unescape_url_query` | method | ✅ | |
| `unquote` | method | ✅ | |
| `uppercase` | method | ✅ | |

### Numbers

| Name | Type | Status | Notes |
|---|---|---|---|
| `abs` | method | ✅ | |
| `bitwise_and` | method | ✅ | |
| `bitwise_or` | method | ✅ | |
| `bitwise_xor` | method | ✅ | |
| `ceil` | method | ✅ | |
| `floor` | method | ✅ | |
| `log` | method | ✅ | |
| `log10` | method | ✅ | |
| `round` | method | ✅ | |

### Arrays / sequences

| Name | Type | Status | Notes |
|---|---|---|---|
| `all` | method | ✅ | |
| `any` | method | ✅ | |
| `append` | method | ✅ | |
| `collapse` | method | ✅ | |
| `contains` | method | ✅ | |
| `enumerated` | method | ✅ | V2 has `enumerate`; confirm shape parity. |
| `filter` | method | ✅ | |
| `find` | method | ✅ | V2's `index_of` covers V1 `find(value) → index`; V2's stdlib `find(lambda)` returns the matching element instead. |
| `find_all` | method | ✅ | |
| `find_all_by` | method | ✅ | Lambda predicate via plugin lambda support. |
| `find_by` | method | ✅ | Lambda predicate via plugin lambda support. |
| `flatten` | method | ✅ | |
| `fold` | method | ✅ | |
| `index` | method | ✅ | |
| `key_values` | method | ✅ | |
| `length` | method | ✅ | |
| `map_each` | method | ❌ | V2 `map` covers it. |
| `max` | method | ✅ | |
| `min` | method | ✅ | |
| `not_empty` | method | ✅ | |
| `slice` | method | ✅ | |
| `sort` | method | ✅ | |
| `sort_by` | method | ✅ | |
| `sum` | method | ✅ | |
| `unique` | method | ✅ | |
| `without` | method | ✅ | |

### Objects

| Name | Type | Status | Notes |
|---|---|---|---|
| `array` | method | ✅ | |
| `assign` | method | ✅ | |
| `exists` | method | ✅ | |
| `explode` | method | ✅ | |
| `get` | method | ✅ | |
| `keys` | method | ✅ | |
| `map_each_key` | method | ⏸ | Lambda predicate; needs plugin lambda support. |
| `merge` | method | ✅ | |
| `values` | method | ✅ | |

### Regex

| Name | Type | Status | Notes |
|---|---|---|---|
| `re_find_all` | method | ✅ | |
| `re_find_all_object` | method | ✅ | |
| `re_find_all_submatch` | method | ✅ | |
| `re_find_object` | method | ✅ | |
| `re_match` | method | ✅ | |
| `re_replace` | method | ✅ | |
| `re_replace_all` | method | ✅ | |

### Time

| Name | Type | Status | Notes |
|---|---|---|---|
| `now` | function | ✅ | |
| `timestamp_unix` | function | ✅ | Reads current time — non-pure. |
| `timestamp_unix_micro` | function | ✅ | |
| `timestamp_unix_milli` | function | ✅ | |
| `timestamp_unix_nano` | function | ✅ | |
| `ts_*` | method | ✅ | All ts_* methods present. |

### Encoding / parsing

| Name | Type | Status | Notes |
|---|---|---|---|
| `format_json` | method | ✅ | |
| `format_yaml` | method | ✅ | |
| `json_schema` | method | ✅ | |
| `parse_csv` | method | ✅ | |
| `parse_json` | method | ✅ | |
| `parse_yaml` | method | ✅ | |

### Crypto / IDs

| Name | Type | Status | Notes |
|---|---|---|---|
| `decrypt_aes` | method | ✅ | Deterministic given key — pure. |
| `encrypt_aes` | method | ✅ | |
| `ksuid` | function | ✅ | Generates IDs; non-pure. |
| `nanoid` | function | ✅ | |
| `uuid_v4` | function | ✅ | |
| `uuid_v5` | method | ✅ | Deterministic — pure. |
| `uuid_v7` | function | ✅ | Timestamp-based; non-pure. |

### Error handling

| Name | Type | Status | Notes |
|---|---|---|---|
| `catch` | method | ✅ | |
| `deleted` | function | ✅ | |
| `error` | function | 🔴 | Pipeline-level error introspection. |
| `error_source_label` | function | 🔴 | |
| `error_source_name` | function | 🔴 | |
| `error_source_path` | function | 🔴 | |
| `errored` | function | 🔴 | |
| `not_null` | method | ✅ | |
| `or` | method | ✅ | |
| `throw` | function | ✅ | |

### Message / pipeline context

| Name | Type | Status | Notes |
|---|---|---|---|
| `batch_index` | function | 🔴 | V2 spec doesn't define batches. Needs design. |
| `batch_size` | function | 🔴 | |
| `content` | function | 🔴 | V2 has `input` for this; possibly redundant. |
| `json` | function | 🔴 | Same — `input` covers most use cases. |
| `metadata` | function | 🔴 | V2 has `input@.key` for reads. |
| `tracing_id` | function | 🔴 | Reads runtime tracer context. |
| `tracing_span` | function | 🔴 | |

### V2-only additions (➕)

Methods: `char`, `collect`, `filter_entries`, `float32`, `float64`, `has_key`,
`int32`, `int64`, `into`, `iter`, `map_entries`, `map_keys`, `map_values`,
`uint32`, `uint64`, `without_index`.

Functions: `day`, `hour`, `minute`, `random_int`, `range`, `second`,
`timestamp`, `void`.

## Rollout plan

### Batch 1 — pure stdlib ports (🟢)

Mechanical lifts from `internal/bloblang/query/methods_*.go`. Each goes in
`./internal/impl/pure/` because none read external state. Sub-batches by
source file for focused commits:

- **strings**: `capitalize`, `escape_html`, `unescape_html`,
  `escape_url_query`, `unescape_url_query`, `quote`, `unquote`, `format`,
  `hash`, `replace`, `replace_many`, `replace_all_many`, `filepath_join`,
  `filepath_split`, `parse_url`
- **numbers**: `bitwise_and`, `bitwise_or`, `bitwise_xor`, `log`, `log10`,
  `number`
- **encoding**: `parse_yaml`, `format_yaml`, `parse_csv`, `json_schema`
- **crypto**: `encrypt_aes`, `decrypt_aes`, `uuid_v5`

### Batch 1b — non-pure stdlib ports (🔴, IO-only subset)

Goes in `./internal/impl/io/` because they read external state (current
time) or generate randomness:

- `timestamp_unix`, `timestamp_unix_milli`, `timestamp_unix_micro`,
  `timestamp_unix_nano`
- `ksuid`, `nanoid`, `uuid_v7`

### Batch 2 — array / object completeness (🟡)

Behavioural ports of: `find_all`, `find_all_by`, `find_by`, `index`,
`not_empty`, `enumerated`, `collapse`, `key_values`, `assign`, `get`,
`map_each_key`, `array`, `exists`, `explode`, `re_replace`,
`re_find_object`, `re_find_all_object`, `re_find_all_submatch`.

### Batch 3 — pipeline-coupled (🔴, needs design)

`batch_index`, `batch_size`, `content`, `json`, `metadata`, `error`,
`errored`, `error_source_*`, `tracing_id`, `tracing_span`. These need a
design conversation about how V2's interpreter sees the surrounding
benthos pipeline (batch position, message bytes, runtime metadata, tracer
context) before being ported.
