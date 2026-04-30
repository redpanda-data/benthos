# Bloblang V1 → V2 Plugin Parity

Tracking sheet for porting V1 stdlib functions and methods to V2 within this
repo. Generated from a sweep of `internal/bloblang/query/` (V1 internal stdlib)
and `internal/impl/pure/bloblang_*.go` (V1 plugin registrations) against
`internal/bloblang2/go/pratt/eval/{stdlib,stdlib_lambda}.go` (V2 internal
stdlib).

Deprecated V1 entries are excluded.

## Status legend

- ✅ ported / available in V2
- ⏸ deferred (named-and-known follow-up)
- ❌ V1-only by architectural choice; V2 won't port
- ➕ V2-only addition (no V1 equivalent)

> **Batches 1, 2, and 3 are complete.** Pure ports live in
> `internal/impl/pure/bloblangv2_*.go`; non-pure (clock / randomness)
> live in `internal/impl/io/bloblangv2_*.go`. Message-coupled stdlib
> (batch_index, content, error, …) is registered internally in
> `internal/bloblang2/go/pratt/eval/stdlib_message.go` and wired
> through `Executor.QueryMessage(MessageContext)`.
>
> The remaining open items are tracked in [Deferred](#deferred-work)
> below — a `format` API redesign and several V1 → V2 migrator
> idiom-shift rules.

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
| `format` | method | ✅ | V2 takes a single array argument (`"%s".format([args])`); migrator rewrites V1 variadic callsites. |
| `parse_form_url_encoded` | method | ✅ | |
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
| `cos` | method | ✅ | |
| `floor` | method | ✅ | |
| `log` | method | ✅ | |
| `log10` | method | ✅ | |
| `pi` | function | ✅ | |
| `pow` | method | ✅ | |
| `round` | method | ✅ | |
| `sin` | method | ✅ | |
| `tan` | method | ✅ | |

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
| `zip` | method | ✅ | V2 takes a single array-of-arrays argument (V1 was variadic). |

### Objects

| Name | Type | Status | Notes |
|---|---|---|---|
| `array` | method | ✅ | |
| `assign` | method | ✅ | |
| `exists` | method | ✅ | |
| `explode` | method | ✅ | |
| `get` | method | ✅ | |
| `keys` | method | ✅ | |
| `map_each_key` | method | ✅ | Lambda predicate via plugin lambda support. |
| `merge` | method | ✅ | |
| `values` | method | ✅ | |
| `with` | method | ✅ | V2 takes a single array of dot-paths (V1 was variadic). |

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
| `parse_duration` | method | ✅ | |
| `timestamp_unix` | function | ✅ | Reads current time — non-pure. |
| `timestamp_unix_micro` | function | ✅ | |
| `timestamp_unix_milli` | function | ✅ | |
| `timestamp_unix_nano` | function | ✅ | |
| `ts_add` | method | ✅ | |
| `ts_format` | method | ✅ | Accepts strftime format strings (V1 `ts_strftime` is the same shape). |
| `ts_parse` | method | ✅ | Accepts strptime format strings (V1 `ts_strptime` is the same shape). |
| `ts_round` | method | ✅ | |
| `ts_sub` | method | ✅ | |
| `ts_tz` | method | ✅ | |
| `ts_unix` / `_milli` / `_micro` / `_nano` | method | ✅ | |
| `format_timestamp_strftime` / `parse_timestamp_strptime` | method | ✅ | Migrator renames to V2 `ts_format` / `ts_parse` (both V1 and V2 use strftime / strptime). |
| `format_timestamp_unix` / `_milli` / `_micro` / `_nano` | method | ✅ | Migrator renames to V2 `ts_unix` / `_milli` / `_micro` / `_nano`. |
| `format_timestamp` / `parse_timestamp` | method | ⏸ | V1 uses Go's reference-time layout; V2 `ts_format` / `ts_parse` use strftime/strptime exclusively. Migrator emits a Note pointing at the strftime variant — manual format-string conversion required. |
| `ts_strftime` / `ts_strptime` | method | ✅ | V2 has `ts_format` / `ts_parse`; migrator renames V1 callsites. |

### Encoding / parsing

| Name | Type | Status | Notes |
|---|---|---|---|
| `compress` | method | ✅ | V2 takes the same `algorithm` + optional `level` parameters. |
| `decompress` | method | ✅ | |
| `format_json` | method | ✅ | |
| `format_yaml` | method | ✅ | |
| `infer_schema` | method | ❌ | V1-specific JSON schema utility; not ported pending demand. |
| `json_schema` | method | ✅ | |
| `parse_csv` | method | ✅ | |
| `parse_json` | method | ✅ | |
| `parse_yaml` | method | ✅ | |
| `squash` | method | ❌ | V1-specific JSON schema utility; not ported pending demand. |

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
| `error` | function | ✅ | V2 returns structured `{what: string}` (was string in V1); migrator rewrites V1 `error()` → V2 `error().what`. |
| `error_source_label` | function | ❌ | V1 backwards-compat workaround; V2 surfaces source.* on the structured `error()` object in a future iteration. |
| `error_source_name` | function | ❌ | See `error_source_label`. |
| `error_source_path` | function | ❌ | See `error_source_label`. |
| `errored` | function | ✅ | |
| `not_null` | method | ✅ | |
| `or` | method | ✅ | |
| `throw` | function | ✅ | |

### Message / pipeline context

| Name | Type | Status | Notes |
|---|---|---|---|
| `batch_index` | function | ✅ | Bound via `Executor.QueryMessage(MessageContext)`. |
| `batch_size` | function | ✅ | |
| `content` | function | ✅ | Returns the raw bytes via `MessageContext.Bytes()`. |
| `json` | function | ❌ | Redundant in V2 — `input` is the parsed body; `content().parse_json()` re-parses from bytes. Migrator emits a Note. |
| `metadata` | function | ❌ | Redundant in V2 — `input@[key]` covers the read form. Migrator rewrites `metadata()` / `metadata("k")` to `input@` / `input@["k"]`. |
| `meta` | function | ❌ | V1's string-only metadata reader; replaced by V2 `input@`. Migrator rewrites with a type-change Note. |
| `root_meta` | function | ❌ | Redundant in V2 — `output@[key]` covers the read form. Migrator rewrites accordingly. |
| `tracing_id` | function | ✅ | Backed by `MessageContext.TraceID()`. |
| `tracing_span` | function | ✅ | Backed by `MessageContext.Span()`. |

### V2-only additions (➕)

Methods: `char`, `collect`, `filter_entries`, `float32`, `float64`, `has_key`,
`int32`, `int64`, `into`, `iter`, `map_entries`, `map_keys`, `map_values`,
`uint32`, `uint64`, `without_index`.

Functions: `day`, `hour`, `minute`, `random_int`, `range`, `second`,
`timestamp`, `void`.

## Deferred work

### Plugin / stdlib

- **`format` method** — V1 was variadic (`"%s/%d".format(name, age)`).
  V2 dropped variadic params. Needs an array-param API redesign
  (e.g. `"%s/%d".format([name, age])`) before the port can land.

### Migrator idiom-shift rules

Open follow-ups in the V1 → V2 migrator:

- V1 `.format_timestamp(fmt)` / `.parse_timestamp(fmt)` — V1 uses
  Go's reference-time layout, V2's `ts_format` / `ts_parse` use
  strftime/strptime. The format strings are not interchangeable, so
  the migrator can't safely auto-rename. A future enhancement could
  translate the format string at migrate time. For now, a Note
  points the user at the strftime-variant V1 method.
- V1 `error_source_label()` / `_name()` / `_path()` — no V2
  equivalent yet; revisit once `error()` grows the structured
  `source.*` fields (deferred from batch 3).
- V1 `json(path)` — no auto-rewrite. Migrator emits a Note pointing
  at `input` / `content().parse_json()`.
