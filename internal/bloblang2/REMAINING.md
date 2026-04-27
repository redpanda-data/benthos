# Bloblang V2 — Remaining Items

Work outstanding to reach parity with the V1 integration in `public/service`.

## Config field metadata

- Add a `BloblangV2 bool` flag to `internal/docs.FieldSpec` (analog of `Bloblang`) and an `IsBloblangV2()` builder, so tooling can differentiate a V2 mapping field from a plain string.
- Wire `NewBloblangV2Field` in `public/service/config_bloblangv2.go` to use it once the flag exists.

## Linting

- Parse-based linting of V2 mapping fields (no deactivated mode needed — V2 parsing is side-effect free).
- A mechanism for field authors to attach custom V2 lint rules, equivalent to `FieldBloblang`'s custom-rule pathway.
- Integration with `component_config_linter.go` / `stream_config_linter.go` (V1 threads `bloblangEnv.Deactivated()` through `docs.LintConfig`; V2 will need an equivalent hook that uses `bloblangv2.Environment.Parse`).

## Schema generation

- V2 equivalent of `stream_schema.expandBloblEnvWithSchema` — expose registered V2 functions/methods in generated schemas.
- An introspection surface on `bloblangv2.Environment` to enumerate registered plugins (signatures, docs) for schema tooling.

## Interpolated strings

- V2 does not yet support interpolated string fields. `public/bloblangv2.Environment` has no `NewField` analog of the V1 surface, and `public/service/config_interpolated_string.go` still calls the V1 env only.
- Out of scope for the initial V2 integration; design needed before wiring.

## CLI surface

- `internal/cli/studio/sync_schema.go` still builds the schema from the V1 env only. Extend once the V2 schema hook above exists.
