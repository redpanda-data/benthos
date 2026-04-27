# Bloblang V2 — Remaining Items

Work outstanding to reach parity with the V1 integration in `public/service`.

## Custom lint rules

- A mechanism for field authors to attach custom V2 lint rules, equivalent to `FieldBloblang`'s custom-rule pathway. The built-in parse-based lint is now wired (see `LintBloblangV2Mapping` in `internal/docs/bloblang.go`); user-supplied rules need their own surface.

## Schema generation

- Round-tripping V2 plugin specs through a serialised schema, equivalent to `expandBloblEnvWithSchema` for V1. `bloblangv2.PluginSpec` would need JSON encode/decode and a stub-registration path so a `LoadConfigSchemaFromJSON` consumer can lint configs against V2 plugins it does not implement. The encode side (enumeration via `Environment.WalkFunctions` / `WalkMethods` and embedding `bloblangv2.PluginInfo` in `schema.Full`) is wired; only the decode/stub path remains.

## Interpolated strings

- V2 does not yet support interpolated string fields. `public/bloblangv2.Environment` has no `NewField` analog of the V1 surface, and `public/service/config_interpolated_string.go` still calls the V1 env only.
- Out of scope for the initial V2 integration; design needed before wiring.
