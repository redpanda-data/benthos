# Bloblang V2 Test Suite

Machine-readable test suite for Bloblang V2 implementations. See `../TEST_PLAN.md` for the full schema documentation.

## Quick Reference

Each YAML file contains a `tests` array. Each test has:

- `name` — unique identifier
- `mapping` — the Bloblang mapping to execute
- `input` — input document (default: `null`)
- `input_metadata` — input metadata (default: `{}`)
- Exactly one expectation:
  - `output` — expected output (order-independent deep equality for objects)
  - `deleted: true` — expect message deletion
  - `error` — expect runtime error (substring match)
  - `compile_error` — expect compile error (substring match)
- Optional: `output_metadata`, `no_output_check`, `output_type`

## Type Annotations

Use `{_type: "typename", value: "string_value"}` for precise types:

- `int32`, `int64`, `uint32`, `uint64`, `float32`, `float64`
- `bytes` (base64-encoded value)
- `timestamp` (RFC 3339 value)

All `value` fields are strings. Plain YAML integers default to int64, floats to float64.

## Output Semantics

- Output starts as `{}` before mapping runs
- Object comparison is order-independent
- `output_metadata` defaults to `{}` when not specified
