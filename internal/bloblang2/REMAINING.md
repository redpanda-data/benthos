# Bloblang V2 — Remaining Items

Work outstanding to reach parity with the V1 integration in `public/service`.

## Custom lint rules

- A mechanism for field authors to attach custom V2 lint rules, equivalent to `FieldBloblang`'s custom-rule pathway. The built-in parse-based lint is now wired (see `LintBloblangV2Mapping` in `internal/docs/bloblang.go`); user-supplied rules need their own surface.

## Schema generation

- V2 equivalent of `stream_schema.expandBloblEnvWithSchema` — expose registered V2 functions/methods in generated schemas.
- An introspection surface on `bloblangv2.Environment` to enumerate registered plugins (signatures, docs) for schema tooling.

## Interpolated strings

- V2 does not yet support interpolated string fields. `public/bloblangv2.Environment` has no `NewField` analog of the V1 surface, and `public/service/config_interpolated_string.go` still calls the V1 env only.
- Out of scope for the initial V2 integration; design needed before wiring.

## CLI surface

- `internal/cli/studio/sync_schema.go` still builds the schema from the V1 env only. Extend once the V2 schema hook above exists.
