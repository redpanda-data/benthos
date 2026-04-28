# Decimal types in `public/schema`

This document describes the `Decimal` common type and its parameterised
representation, and lays out the contracts that schema-format converters and
data-source plugins must honour when handling decimal values.

## Why decimals are a special case

Most numeric types in the common schema (`Int32`, `Int64`, `Float32`, `Float64`)
have a fixed bit-width and need no further parameters. Decimals don't: a
decimal value's meaning depends on its **precision** (total significant digits)
and **scale** (digits to the right of the decimal point). The same byte
sequence `0x00 0x7B` encodes `123` at scale 0, `12.3` at scale 1, and `1.23` at
scale 2.

Therefore the common schema needs to carry these parameters alongside the type
identifier, and every downstream converter and data-source plugin must agree on
how the parameters and the values they describe travel together.

## Changes to the common schema

### New type

```go
const Decimal CommonType = 15
```

`Decimal` joins the existing primitive and structural types and stringifies as
`"DECIMAL"`.

### New parameter struct

A new optional field is added to `Common` for parameterised types in general,
not only decimal:

```go
type Common struct {
    Name     string
    Type     CommonType
    Optional bool
    Children []Common
    Logical  *LogicalParams // nil when no logical parameters are required
}

type LogicalParams struct {
    Decimal *DecimalParams
    // Future parameterised logical types add their own pointer field here.
}

type DecimalParams struct {
    Precision int32
    Scale     int32
}
```

Only the `LogicalParams` field corresponding to `Common.Type` is allowed to be
non-nil. Setting `Logical.Decimal` on a non-`Decimal` schema is a validation
error.

### Bounds

```go
const (
    DecimalMinPrecision int32 = 1
    DecimalMaxPrecision int32 = 38
)
```

Validation rules enforced by `Common.Validate()` and applied by
`ParseFromAny`:

- `Precision ∈ [DecimalMinPrecision, DecimalMaxPrecision]`
- `Scale ∈ [0, Precision]`

These bounds describe the **lossless intersection** across Avro `decimal`,
Parquet `DECIMAL`, and Oracle `NUMBER`. Oracle permits negative scale and
precisions up to its own internal limits, but those values cannot round-trip
through Avro or Parquet, so the common schema does not allow them. Sources that
encounter wider Oracle decimals should either narrow them or downgrade to
`String` and document the loss.

### Serialisation in `ToAny` / `ParseFromAny`

For decimals, `ToAny` adds two top-level fields to the map:

```json
{
    "type": "DECIMAL",
    "name": "amount",
    "precision": 18,
    "scale": 4,
    "fingerprint": "..."
}
```

`ParseFromAny` requires both fields when `type` is `DECIMAL`, rejects them on
any other type, and runs full validation before returning. Numeric values are
accepted as `int`, `int32`, `int64`, `float32` or `float64`, the latter two
provided they have no fractional part — JSON unmarshalling tends to produce
`float64`s.

### Fingerprinting

`writeFingerprint` includes a `D:<precision>:<scale>|` segment **only** when
the type is `Decimal`. Non-decimal schemas keep the byte-for-byte canonical
form they had before, so existing fingerprints (and cached conversions keyed by
them) remain stable.

### Inference

`InferFromAny` does not infer decimals. Go has no canonical decimal type and
there is no reliable way to recover precision and scale from a generic Go value
without context. Decimal schemas must be constructed explicitly by data-source
plugins from authoritative source metadata.

## Contract for schema-format converters

Converters live outside this package (Avro, Parquet, Iceberg, JSON Schema,
Protobuf, ...). When a converter encounters a `Decimal` common schema it
**must**:

1. Read precision and scale from `c.Logical.Decimal`. Treat `c.Logical == nil`
   or `c.Logical.Decimal == nil` as a programming error and return an error,
   not a default.
2. Pick the format-native decimal representation that preserves precision and
   scale exactly. See per-format guidance below.
3. Refuse precisions or scales the target format cannot represent rather than
   silently truncating. The common schema's bounds are conservative, so most
   targets will never need to reject; those that do must surface a clear
   error.

When **producing** a `Common` schema from a format-native schema, the converter
constructs `&LogicalParams{Decimal: &DecimalParams{...}}` from the source
precision and scale and runs `Common.Validate()` before returning.

### Avro

Avro's `decimal` is a logical type built on top of `bytes` or `fixed`.
Converters should:

- For schemas read from Avro: take `precision` and `scale` from the logical
  type annotation. If `scale` is absent, default it to `0` (Avro spec
  default).
- For schemas written to Avro: prefer `bytes` as the underlying primitive
  unless the conversion target requires `fixed` (e.g. for fixed-width on-wire
  framing). When using `fixed`, compute `size = ceil((precision * log2(10) +
  1) / 8)`.
- Reject Avro schemas where `scale > precision` or `precision <= 0` — these
  are invalid in Avro itself and would fail validation in the common schema
  too.

The on-wire Avro decimal value is a two's-complement signed big-endian
integer. Converters that operate on Avro records will need to multiply the
incoming value by `10^scale` (conceptually) to reconstruct the unscaled
integer, and divide on the way out.

### Parquet

Parquet's `DECIMAL` logical type wraps one of four physical types, chosen by
precision:

| Precision range | Physical type             |
|-----------------|---------------------------|
| 1 – 9           | `INT32`                   |
| 10 – 18         | `INT64`                   |
| 19 – 38         | `FIXED_LEN_BYTE_ARRAY`    |
| arbitrary       | `BYTE_ARRAY`              |

Converters should:

- For schemas written to Parquet: select the smallest physical type capable of
  representing the precision. `FIXED_LEN_BYTE_ARRAY` length is
  `ceil((precision * log2(10) + 1) / 8)`.
- For schemas read from Parquet: require both `precision` and `scale`
  annotations. Reject decimals encoded as bare `BYTE_ARRAY` without a logical
  type annotation, since precision and scale are not recoverable from the
  bytes alone.

Parquet shares Avro's two's-complement big-endian wire format for the
byte-backed cases, and uses native two's-complement for the integer-backed
cases.

### Oracle / databases with `NUMBER(p, s)`

Sources reading from `NUMBER(p, s)` set `Precision = p` and `Scale = s`. The
following conditions must be handled explicitly:

- `NUMBER` with **no** declared precision (Oracle's "floating decimal"): there
  is no fixed precision to record. Sources must either pick a sentinel
  precision (e.g. 38) and warn, or downgrade to `String`.
- `NUMBER` with declared precision but **no** scale: `Scale = 0`.
- `NUMBER` with **negative** scale: not supported. Sources must either round
  to scale 0, downgrade to `String`, or refuse the column.

### Postgres `NUMERIC` / MySQL `DECIMAL` / `NUMERIC`

These map directly: precision and scale from the column metadata translate
straight to `DecimalParams`. Both databases enforce `0 ≤ scale ≤ precision`,
so values from these sources will always validate.

`NUMERIC` columns with no precision (Postgres "arbitrary precision") fall into
the same bucket as undeclared Oracle `NUMBER`: pick a precision and warn, or
downgrade to `String`.

### JSON Schema

JSON Schema has no native decimal. Converters should map `Decimal` to
`{"type": "string", "pattern": ...}` with a regex that matches the precision
and scale, and document the loss of arithmetic semantics in the
roundtripped schema. Inbound conversion (JSON Schema → common) cannot recover
`Decimal` and should retain the value as `String`.

## Contract for data-source plugins

Data-source plugins (CDC inputs like `mysql_cdc`, `postgres_cdc`, `oracle_cdc`,
batch inputs like `sql_select`, etc.) emit two things: a **schema** describing
each column, and **values** for each row.

### Producing the schema

When a source identifies a column as a fixed-precision decimal, prefer the
constructor helper:

```go
col, err := schema.NewDecimal("amount", precisionFromSource, scaleFromSource, nullable)
if err != nil {
    return err
}
```

`NewDecimal` validates the precision and scale once at schema-discovery time.
Per-row validation is unnecessary and should be avoided on hot paths.

The constructor is shorthand for the equivalent struct literal, which remains
available for cases that need it (e.g. constructing a parent [Common] schema
in a single expression):

```go
col := schema.Common{
    Name:     "amount",
    Type:     schema.Decimal,
    Optional: nullable,
    Logical: &schema.LogicalParams{
        Decimal: &schema.DecimalParams{
            Precision: precisionFromSource,
            Scale:     scaleFromSource,
        },
    },
}
if err := col.Validate(); err != nil {
    return err
}
```

### Producing values

The benthos message body that travels alongside the schema should encode each
decimal value in **canonical string form**:

- A leading minus sign for negative values; no leading plus sign.
- No leading zeros except for the single `0` before a decimal point.
- A decimal point appears if and only if `scale > 0`.
- Exactly `scale` digits after the decimal point — sources must pad with
  trailing zeros if necessary so that `"1.5"` for a `(p, 4)` column is
  emitted as `"1.5000"`.
- No scientific notation, thousands separators, or whitespace.

Examples for `Precision=18, Scale=4`:

| Source value | Emitted string |
|--------------|----------------|
| `12345`      | `"12345.0000"` |
| `-0.1`       | `"-0.1000"`    |
| `0`          | `"0.0000"`     |

Strings are chosen as the canonical form because they:

- Survive JSON round-trips without floating-point loss.
- Pass cleanly through Bloblang's existing string-handling primitives.
- Can be parsed by every downstream converter (Avro, Parquet, ...) into the
  format-native unscaled integer when needed.
- Don't bind benthos to a specific Go decimal library.

To produce and consume the canonical form consistently across plugins, use
the helpers in this package:

```go
// Producing a value (e.g. in a CDC source after reading the raw decimal):
unscaled := big.NewInt(15000)
str, err := schema.FormatDecimal(unscaled, scale)        // "1.5000" at scale 4

// Or, with precision enforcement:
params := schema.DecimalParams{Precision: 18, Scale: 4}
str, err := params.Format(unscaled)

// Consuming a value (e.g. in a converter writing to Avro/Parquet):
unscaled, err := schema.ParseDecimal("1.5000", scale)    // big.NewInt(15000)
unscaled, err := params.Parse("1.5000")                  // also enforces precision
```

Plugins that roll their own formatting are likely to drift from the contract
(scientific notation, trailing-zero handling, sign-zero, leading zeros). Use
the helpers.

### Optional fast paths for converters

Converters that want to avoid string parsing on hot paths **may** accept
additional value forms — but the canonical string form is mandatory and is
what data-source plugins are required to emit. Suggested optional forms a
converter can opt in to:

- `[]byte` containing the two's-complement big-endian unscaled integer
  (matches the Avro/Parquet wire format).
- `*big.Int` containing the unscaled integer (the form returned by
  `schema.ParseDecimal` and accepted by `schema.FormatDecimal`).

These fast paths are **opt-in for the converter, not optional for the
source**. A new data-source plugin that does not emit canonical strings is
non-conformant.

### Null values

A nullable decimal column emits a Go `nil` value. The schema's `Optional`
field carries the nullability information; the value form is unchanged
otherwise.

## Migration notes for existing converters and sources

This change is additive:

- Existing schemas that did not previously contain decimals are unaffected.
  Their fingerprints are byte-for-byte identical to before, so cached
  conversions remain valid.
- Existing converters that do not handle the `Decimal` type should continue to
  reject it with `"unsupported type"` until updated.
- Existing data sources that previously surfaced decimal columns as `String`
  may continue to do so for backwards compatibility, but should migrate to
  emitting `Decimal` schemas with canonical-string values when possible.
