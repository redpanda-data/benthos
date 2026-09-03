# Bloblang V2 — Remaining Items

Work outstanding to reach parity with the V1 integration in `public/service`.

## Custom lint rules

The built-in parse-based lint is now wired (see `LintBloblangV2Mapping` in `internal/docs/bloblang.go`), so `bloblang_v2` fields surface compile errors at config-load time.

Still missing: a public surface for field authors to attach **custom** V2 lint rules, equivalent to `FieldBloblang`'s custom-rule pathway. Plugin authors who currently rely on custom V1 lint rules (e.g. to require a particular field on the mapping result) have no V2-side hook.

## Interpolated strings

V2 does not yet plug into the interpolated-string surface. `public/bloblangv2.Environment` has no `NewField` analog of the V1 surface, and `public/service/config_interpolated_string.go` still calls the V1 environment only.

User-visible consequence: a plugin registered as a V2-only method **will not be available inside `${! ... }` fields**, even when the field's host component also accepts a `bloblang_v2` mapping field. Users of V1 + V2 plugins in the same binary should be warned about this in release notes.

Out of scope for the initial V2 integration; design needed before wiring.

## Plugin bridge between V1 and V2

V1 and V2 maintain separate plugin registries (`public/bloblang.Environment` and `public/bloblangv2.Environment` respectively), and plugins registered against one are invisible to the other. The current V1 stdlib parity ports under `internal/impl/{pure,io}/bloblangv2_*.go` had to be written by hand against the V2 plugin API.

Two follow-ups worth considering:

- A bridging helper that adapts a V1 plugin spec into a V2 registration. The adapter would have to declare any semantic shifts (variadic → array arguments, error-object shape, etc.) explicitly so authors opt in rather than silently accept a behavioural delta.
- A migration guide for plugin maintainers that mirrors the per-method notes in `PARITY.md`.

Out of scope for this branch; tracked here so users porting plugins know the work isn't done.
