// Tagged value type for the Bloblang V2 runtime.
//
// TypeScript doesn't have native int32/int64/uint32/uint64, so we use a
// discriminated union. 32-bit integers use `number`, 64-bit use `bigint`.
// float32 values are stored as `number` but rounded with Math.fround().

// --- Value types ---

export type Value =
  | NullValue
  | BoolValue
  | Int32Value
  | Int64Value
  | Uint32Value
  | Uint64Value
  | Float32Value
  | Float64Value
  | StringValue
  | BytesValue
  | ArrayValue
  | ObjectValue
  | TimestampValue
  | VoidValue
  | DeletedValue
  | ErrorValue;

export interface NullValue {
  tag: "null";
}
export interface BoolValue {
  tag: "bool";
  value: boolean;
}
export interface Int32Value {
  tag: "int32";
  value: number;
}
export interface Int64Value {
  tag: "int64";
  value: bigint;
}
export interface Uint32Value {
  tag: "uint32";
  value: number;
}
export interface Uint64Value {
  tag: "uint64";
  value: bigint;
}
export interface Float32Value {
  tag: "float32";
  value: number;
}
export interface Float64Value {
  tag: "float64";
  value: number;
}
export interface StringValue {
  tag: "string";
  value: string;
}
export interface BytesValue {
  tag: "bytes";
  value: Uint8Array;
}
export interface ArrayValue {
  tag: "array";
  value: Value[];
}
export interface ObjectValue {
  tag: "object";
  value: Map<string, Value>;
}
export interface TimestampValue {
  tag: "timestamp";
  /** Nanoseconds since Unix epoch. */
  value: bigint;
}
export interface VoidValue {
  tag: "void";
}
export interface DeletedValue {
  tag: "deleted";
}
export interface ErrorValue {
  tag: "error";
  message: string;
}

// --- Singletons ---

export const NULL: NullValue = { tag: "null" };
export const TRUE: BoolValue = { tag: "bool", value: true };
export const FALSE: BoolValue = { tag: "bool", value: false };
export const VOID: VoidValue = { tag: "void" };
export const DELETED: DeletedValue = { tag: "deleted" };

// --- Constructors ---

export function mkBool(v: boolean): BoolValue {
  return v ? TRUE : FALSE;
}

export function mkInt32(v: number): Int32Value {
  return { tag: "int32", value: v | 0 };
}

export function mkInt64(v: bigint): Int64Value {
  return { tag: "int64", value: v };
}

export function mkUint32(v: number): Uint32Value {
  return { tag: "uint32", value: v >>> 0 };
}

export function mkUint64(v: bigint): Uint64Value {
  return { tag: "uint64", value: v };
}

export function mkFloat32(v: number): Float32Value {
  return { tag: "float32", value: Math.fround(v) };
}

export function mkFloat64(v: number): Float64Value {
  return { tag: "float64", value: v };
}

export function mkString(v: string): StringValue {
  return { tag: "string", value: v };
}

export function mkBytes(v: Uint8Array): BytesValue {
  return { tag: "bytes", value: v };
}

export function mkArray(v: Value[]): ArrayValue {
  return { tag: "array", value: v };
}

export function mkObject(v: Map<string, Value>): ObjectValue {
  return { tag: "object", value: v };
}

export function mkTimestamp(nanos: bigint): TimestampValue {
  return { tag: "timestamp", value: nanos };
}

export function mkError(msg: string): ErrorValue {
  return { tag: "error", message: msg };
}

// --- Type guards ---

export function isNull(v: Value): v is NullValue {
  return v.tag === "null";
}
export function isBool(v: Value): v is BoolValue {
  return v.tag === "bool";
}
export function isInt32(v: Value): v is Int32Value {
  return v.tag === "int32";
}
export function isInt64(v: Value): v is Int64Value {
  return v.tag === "int64";
}
export function isUint32(v: Value): v is Uint32Value {
  return v.tag === "uint32";
}
export function isUint64(v: Value): v is Uint64Value {
  return v.tag === "uint64";
}
export function isFloat32(v: Value): v is Float32Value {
  return v.tag === "float32";
}
export function isFloat64(v: Value): v is Float64Value {
  return v.tag === "float64";
}
export function isString(v: Value): v is StringValue {
  return v.tag === "string";
}
export function isBytes(v: Value): v is BytesValue {
  return v.tag === "bytes";
}
export function isArray(v: Value): v is ArrayValue {
  return v.tag === "array";
}
export function isObject(v: Value): v is ObjectValue {
  return v.tag === "object";
}
export function isTimestamp(v: Value): v is TimestampValue {
  return v.tag === "timestamp";
}
export function isVoid(v: Value): v is VoidValue {
  return v.tag === "void";
}
export function isDeleted(v: Value): v is DeletedValue {
  return v.tag === "deleted";
}
export function isError(v: Value): v is ErrorValue {
  return v.tag === "error";
}

export function isNumeric(v: Value): boolean {
  switch (v.tag) {
    case "int32":
    case "int64":
    case "uint32":
    case "uint64":
    case "float32":
    case "float64":
      return true;
    default:
      return false;
  }
}

// --- Type name ---

export function typeName(v: Value): string {
  return v.tag;
}

// --- Deep clone ---

export function deepClone(v: Value): Value {
  switch (v.tag) {
    case "array":
      return mkArray(v.value.map(deepClone));
    case "object": {
      const m = new Map<string, Value>();
      for (const [k, val] of v.value) {
        m.set(k, deepClone(val));
      }
      return mkObject(m);
    }
    case "bytes":
      return mkBytes(new Uint8Array(v.value));
    default:
      // Immutable types: null, bool, numbers, string, timestamp, void, deleted, error.
      return v;
  }
}

// --- JSON conversion ---

const MAX_SAFE_INT64 = 9007199254740991n; // 2^53 - 1
const MIN_SAFE_INT64 = -9007199254740991n;

/** Convert a JSON-compatible JavaScript value to a Bloblang Value. */
export function fromJSON(v: unknown): Value {
  if (v === null || v === undefined) return NULL;
  if (typeof v === "boolean") return mkBool(v);
  if (typeof v === "string") return mkString(v);
  if (typeof v === "number") {
    // Integer or float?
    if (Number.isInteger(v)) {
      return mkInt64(BigInt(v));
    }
    return mkFloat64(v);
  }
  if (Array.isArray(v)) {
    return mkArray(v.map(fromJSON));
  }
  if (typeof v === "object") {
    const m = new Map<string, Value>();
    for (const [key, val] of Object.entries(v as Record<string, unknown>)) {
      m.set(key, fromJSON(val));
    }
    return mkObject(m);
  }
  return mkError(`cannot convert ${typeof v} to Bloblang value`);
}

/** Convert a Bloblang Value to a JSON-compatible JavaScript value. */
export function toJSON(v: Value): unknown {
  switch (v.tag) {
    case "null":
      return null;
    case "bool":
      return v.value;
    case "int32":
      return v.value;
    case "int64":
      return Number(v.value);
    case "uint32":
      return v.value;
    case "uint64":
      return Number(v.value);
    case "float32":
      return v.value;
    case "float64":
      return v.value;
    case "string":
      return v.value;
    case "bytes":
      // Bytes are not directly JSON-serializable. Return base64 or array.
      return Array.from(v.value);
    case "array":
      return v.value.map(toJSON);
    case "object": {
      const obj: Record<string, unknown> = {};
      for (const [k, val] of v.value) {
        obj[k] = toJSON(val);
      }
      return obj;
    }
    case "timestamp":
      // ISO 8601 string representation.
      return new Date(Number(v.value / 1000000n)).toISOString();
    case "void":
      return undefined;
    case "deleted":
      return undefined;
    case "error":
      return `error: ${v.message}`;
  }
}

/** Deep equality comparison following Bloblang semantics. */
export function valuesEqual(a: Value, b: Value): boolean {
  if (a.tag === "null" && b.tag === "null") return true;
  if (a.tag === "null" || b.tag === "null") return false;

  // Numeric equality with promotion.
  if (isNumeric(a) && isNumeric(b)) {
    return numericEqual(a, b);
  }

  // Timestamp equality.
  if (a.tag === "timestamp" && b.tag === "timestamp") {
    return a.value === b.value;
  }

  // Bytes equality.
  if (a.tag === "bytes" && b.tag === "bytes") {
    if (a.value.length !== b.value.length) return false;
    for (let i = 0; i < a.value.length; i++) {
      if (a.value[i] !== b.value[i]) return false;
    }
    return true;
  }

  // Same tag required for non-numeric.
  if (a.tag !== b.tag) return false;

  switch (a.tag) {
    case "string":
      return a.value === (b as StringValue).value;
    case "bool":
      return a.value === (b as BoolValue).value;
    case "array": {
      const bArr = (b as ArrayValue).value;
      if (a.value.length !== bArr.length) return false;
      for (let i = 0; i < a.value.length; i++) {
        if (!valuesEqual(a.value[i]!, bArr[i]!)) return false;
      }
      return true;
    }
    case "object": {
      const bObj = (b as ObjectValue).value;
      if (a.value.size !== bObj.size) return false;
      for (const [k, v] of a.value) {
        const bv = bObj.get(k);
        if (bv === undefined || !valuesEqual(v, bv)) return false;
      }
      return true;
    }
    default:
      return false;
  }
}

// --- Numeric helpers ---

function numericEqual(a: Value, b: Value): boolean {
  // Same type: direct comparison.
  if (a.tag === b.tag) {
    switch (a.tag) {
      case "int32":
        return a.value === (b as Int32Value).value;
      case "int64":
        return a.value === (b as Int64Value).value;
      case "uint32":
        return a.value === (b as Uint32Value).value;
      case "uint64":
        return a.value === (b as Uint64Value).value;
      case "float32":
        return !isNaN(a.value) && !isNaN((b as Float32Value).value) && a.value === (b as Float32Value).value;
      case "float64":
        return !isNaN(a.value) && !isNaN((b as Float64Value).value) && a.value === (b as Float64Value).value;
    }
  }

  // Different numeric types: checked promotion.
  const result = promoteChecked(a, b);
  if (result === null) return false; // promotion failed

  const [pa, pb, kind] = result;
  switch (kind) {
    case "int64":
      return (pa as Int64Value).value === (pb as Int64Value).value;
    case "int32":
      return (pa as Int32Value).value === (pb as Int32Value).value;
    case "uint32":
      return (pa as Uint32Value).value === (pb as Uint32Value).value;
    case "uint64":
      return (pa as Uint64Value).value === (pb as Uint64Value).value;
    case "float64": {
      const af = (pa as Float64Value).value;
      const bf = (pb as Float64Value).value;
      return !isNaN(af) && !isNaN(bf) && af === bf;
    }
    case "float32": {
      const af = (pa as Float32Value).value;
      const bf = (pb as Float32Value).value;
      return !isNaN(af) && !isNaN(bf) && af === bf;
    }
    default:
      return false;
  }
}

// --- Numeric promotion ---

type PromoteKind = "int32" | "int64" | "uint32" | "uint64" | "float32" | "float64";

function numericKind(v: Value): PromoteKind | null {
  return isNumeric(v) ? (v.tag as PromoteKind) : null;
}

function isFloatKind(k: PromoteKind): boolean {
  return k === "float32" || k === "float64";
}

export const MAX_SAFE_FLOAT64 = 9007199254740992n; // 2^53
export const MAX_INT64 = 9223372036854775807n;
export const MIN_INT64 = -9223372036854775808n;
export const MAX_INT32 = 2147483647;
export const MIN_INT32 = -2147483648;
export const MAX_UINT32 = 4294967295;
export const MAX_UINT64 = 18446744073709551615n;

/**
 * Promote two numeric values to a common type.
 * Returns [promoted_a, promoted_b, kind] or null on failure.
 */
export const promoteChecked = promote;

/**
 * Returns a specific error message for promotion failure, or null on success.
 */
export function promoteWithError(
  a: Value,
  b: Value,
): { promoted: [Value, Value, PromoteKind] } | { error: string } {
  const result = promote(a, b);
  if (result !== null) return { promoted: result };

  const ak = numericKind(a);
  const bk = numericKind(b);
  if (
    (ak === "uint64" || bk === "uint64") &&
    ak !== null &&
    bk !== null &&
    !isFloatKind(ak) &&
    !isFloatKind(bk)
  ) {
    return { error: "uint64 value exceeds int64 range" };
  }
  return { error: "integer exceeds float64 exact range (magnitude > 2^53)" };
}

function promote(
  a: Value,
  b: Value,
): [Value, Value, PromoteKind] | null {
  const ak = numericKind(a);
  const bk = numericKind(b);
  if (ak === null || bk === null) return null;

  // Same type: no promotion needed.
  if (ak === bk) return [a, b, ak];

  // Same signedness, different width: widen.
  // uint32 + uint64 → uint64.
  if (
    (ak === "uint32" && bk === "uint64") ||
    (ak === "uint64" && bk === "uint32")
  ) {
    return [toU64(a), toU64(b), "uint64"];
  }

  // Any float involved → float64.
  if (isFloatKind(ak) || isFloatKind(bk)) {
    const af = checkedToFloat64(a);
    const bf = checkedToFloat64(b);
    if (af === null || bf === null) return null;
    return [af, bf, "float64"];
  }

  // Both integers: widen to int64.
  const ai = toI64(a);
  const bi = toI64(b);
  if (ai === null || bi === null) return null;
  return [ai, bi, "int64"];
}

function toU64(v: Value): Uint64Value {
  switch (v.tag) {
    case "uint32":
      return mkUint64(BigInt(v.value));
    case "uint64":
      return v;
    default:
      throw new Error(`toU64: unexpected tag ${v.tag}`);
  }
}

function toI64(v: Value): Int64Value | null {
  switch (v.tag) {
    case "int32":
      return mkInt64(BigInt(v.value));
    case "int64":
      return v;
    case "uint32":
      return mkInt64(BigInt(v.value));
    case "uint64":
      if (v.value > MAX_INT64) return null;
      return mkInt64(BigInt(v.value));
    default:
      return null;
  }
}

function checkedToFloat64(v: Value): Float64Value | null {
  switch (v.tag) {
    case "int64":
      if (v.value > MAX_SAFE_FLOAT64 || v.value < -MAX_SAFE_FLOAT64)
        return null;
      return mkFloat64(Number(v.value));
    case "int32":
      return mkFloat64(v.value);
    case "uint32":
      return mkFloat64(v.value);
    case "uint64":
      if (v.value > MAX_SAFE_FLOAT64) return null;
      return mkFloat64(Number(v.value));
    case "float64":
      return v;
    case "float32":
      return mkFloat64(v.value);
    default:
      return null;
  }
}

/** Convert any numeric Value to float64, unchecked. */
export function toFloat64Unchecked(v: Value): number {
  switch (v.tag) {
    case "int32":
    case "uint32":
    case "float32":
    case "float64":
      return v.value;
    case "int64":
    case "uint64":
      return Number(v.value);
    default:
      return NaN;
  }
}
