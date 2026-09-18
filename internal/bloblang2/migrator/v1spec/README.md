# V1 Spec Compliance Suite

A Go test harness + YAML corpus that verifies the **Bloblang V1 interpreter** behaves the way `../bloblang_v1_spec.md` says it does. This is not a test of the migrator — the migrator doesn't exist yet — it's groundwork: the same corpus will later serve as fixture data for migrator round-trip tests, and the V1 spec itself was refined by investigating discrepancies this suite surfaced.

## Layout

- **`tests/`** — 128 YAML files mirroring `../../spec/tests/`. Each is a V1 equivalent of the corresponding V2 conformance test. The schema is identical (reuses `internal/bloblang2/go/spectest` for loading), with one added field: `skip: "<reason>"` on tests that have no direct V1 equivalent.
- **`interp.go`** — `V1Interp` implements `spectest.Interpreter` using `public/bloblang` for compilation and `mapping.Executor.ExecOnto` for execution. Executes directly rather than via `MapPart` to preserve raw scalar types (`MapPart` stringifies through the message body, which would re-parse `"true"` as a bool).
- **`runner.go`** — `RunT` wraps `spectest.RunT` and pre-scans each YAML for the `skip` field, surfacing those tests via `t.Skip` rather than compiling them.
- **`v1spec_test.go`** — `TestBloblangV1Spec` entrypoint.

## Running

From `internal/bloblang2/`:

```sh
task test:v1spec                 # run the full suite
task test:v1spec -- -v           # verbose
task test:v1spec -- -run 'TestBloblangV1Spec/types/bool_null'   # one file
```

Or from the repo root:

```sh
go test ./internal/bloblang2/migrator/v1spec/... -run TestBloblangV1Spec
```

A test **passes** when the V1 interpreter produces the `output` / `deleted` / `error` / `compile_error` the YAML expects. **Skips** are V2-only constructs that can't be expressed in V1 at all. Current state: 2090 pass, 0 fail, 984 skip across 3074 test cases.

## Intended uses

1. **Spec validation** — fixing a failure generally exposes a V1 behaviour the spec should document more clearly. The spec and this corpus evolved together.
2. **Migrator fixture data** — when the migrator tool is built, it will be fed the V2 source tests and asked to produce mappings equivalent to these V1 tests. Any divergence is a migration bug.

## Schema extension

The base schema is from `internal/bloblang2/go/spectest` (see its `TEST_PLAN.md` / `README.md`). One migrator-specific addition:

```yaml
- name: "uses V2-only typed numeric width"
  mapping: |
    root = this.x.int32()
  skip: "V1 has no typed numeric widths"
```

A test with `skip:` is surfaced via `t.Skip(reason)` and otherwise ignored. The runner does not compile or execute skipped mappings, so the mapping field can contain non-V1 code if needed for documentation.
