# Bloblang V2 — Remaining Items

Work outstanding to reach parity with the V1 integration in `public/service`.

## Custom lint rules

- A mechanism for field authors to attach custom V2 lint rules, equivalent to `FieldBloblang`'s custom-rule pathway. The built-in parse-based lint is now wired (see `LintBloblangV2Mapping` in `internal/docs/bloblang.go`); user-supplied rules need their own surface.

## Interpolated strings

- V2 does not yet support interpolated string fields. `public/bloblangv2.Environment` has no `NewField` analog of the V1 surface, and `public/service/config_interpolated_string.go` still calls the V1 env only.
- Out of scope for the initial V2 integration; design needed before wiring.

## Plugin spec round-trip cleanup

Tracked from a foundation review of `NewPluginSpecFromInfo`.

- The reverse builder in `public/bloblangv2/view.go` populates `PluginSpec` and `ParamDefinition` private fields directly rather than going through the public builders (`NewPluginSpec().Description(...)`, `NewStringParam(...).Default(...)`, etc.). This bypasses any future validation added to those builders. Either expose typed setters on `PluginSpec` (e.g. `Status(string)`, `Category(string)`, `Version(string)`) and a `ParamDefinition`-from-info constructor, or accept the divergence as documented best-effort behaviour.
- `PluginInfo.Default` is typed `any`; numeric defaults round-trip through JSON as `float64` regardless of the declared `Kind`. The decode path could normalise `Default` against `Kind` (e.g. coerce `float64(5)` back to `int64(5)` when `Kind == "int64"`) so dumped specs round-trip with their original Go types intact. Currently invisible because stub registrations never evaluate defaults.
