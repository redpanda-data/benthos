// Type conversion methods: type, string, int32, int64, uint32, uint64,
// float32, float64, bool, bytes, not_null, char.

declare const TextDecoder: {
  new (label?: string, options?: { fatal?: boolean }): {
    decode(input: Uint8Array): string;
  };
};
declare const TextEncoder: { new (): { encode(s: string): Uint8Array } };

import type { Interpreter, MethodSpec } from "../interpreter.js";
import {
  type Value,
  mkInt32,
  mkInt64,
  mkUint32,
  mkUint64,
  mkFloat32,
  mkFloat64,
  mkString,
  mkBool,
  mkBytes,
  mkError,
  isNull,
  isString,
  isBool,
  isInt32,
  isInt64,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isBytes,
  isTimestamp,
  isArray,
  isObject,
  isNumeric,
  typeName,
  toJSON,
  MAX_INT32,
  MIN_INT32,
  MAX_UINT32,
  MAX_INT64,
  MAX_UINT64,
} from "../value.js";
import { strftimeFormat, DEFAULT_TIMESTAMP_FORMAT } from "./timestamp.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Check if a Value tree contains bytes anywhere. */
function containsBytes(v: Value): boolean {
  if (isBytes(v)) return true;
  if (isArray(v)) {
    for (const elem of v.value) {
      if (containsBytes(elem)) return true;
    }
  }
  if (isObject(v)) {
    for (const [, val] of v.value) {
      if (containsBytes(val)) return true;
    }
  }
  return false;
}

function formatFloat(f: number): string {
  if (Number.isNaN(f)) return "NaN";
  if (f === Infinity) return "Infinity";
  if (f === -Infinity) return "-Infinity";
  if (f === 0 && 1 / f === -Infinity) return "0.0"; // negative zero
  let s = String(f);
  // Ensure the string contains a decimal point or exponent.
  if (!s.includes(".") && !s.includes("e") && !s.includes("E")) {
    s += ".0";
  }
  return s;
}

/** Format a float32 value using the shortest representation that round-trips through float32. */
function formatFloat32(f: number): string {
  if (Number.isNaN(f)) return "NaN";
  if (f === Infinity) return "Infinity";
  if (f === -Infinity) return "-Infinity";
  if (f === 0 && 1 / f === -Infinity) return "0.0"; // negative zero
  // Find shortest representation that round-trips through float32.
  for (let prec = 1; prec <= 9; prec++) {
    const s = f.toPrecision(prec);
    if (Math.fround(parseFloat(s)) === f) {
      let result = cleanupTrailingZeros(s);
      // Ensure decimal point.
      if (!result.includes(".") && !result.includes("e") && !result.includes("E")) {
        result += ".0";
      }
      return result;
    }
  }
  let s = String(f);
  if (!s.includes(".") && !s.includes("e") && !s.includes("E")) {
    s += ".0";
  }
  return s;
}

/** Convert a Value to JSON with object keys sorted (matches Go's json.Marshal behavior). */
function sortedToJSON(v: Value): unknown {
  if (isArray(v)) return v.value.map(sortedToJSON);
  if (isObject(v)) {
    const obj: Record<string, unknown> = {};
    const keys = [...v.value.keys()].sort();
    for (const k of keys) {
      obj[k] = sortedToJSON(v.value.get(k)!);
    }
    return obj;
  }
  return toJSON(v);
}

function cleanupTrailingZeros(s: string): string {
  if (!s.includes(".")) return s;
  s = s.replace(/(\.\d*?)0+$/, "$1");
  s = s.replace(/\.$/, "");
  return s;
}

function valueToString(v: Value): Value {
  if (isNull(v)) return mkString("null");
  if (isString(v)) return v;
  if (isInt32(v)) return mkString(String(v.value));
  if (isInt64(v)) return mkString(String(v.value));
  if (isUint32(v)) return mkString(String(v.value));
  if (isUint64(v)) return mkString(String(v.value));
  if (isFloat32(v)) return mkString(formatFloat32(v.value));
  if (isFloat64(v)) return mkString(formatFloat(v.value));
  if (isBool(v)) return mkString(v.value ? "true" : "false");
  if (isTimestamp(v)) {
    return mkString(formatTimestampValue(v.value));
  }
  if (isBytes(v)) {
    // Check valid UTF-8 — in JS, TextDecoder with fatal option.
    try {
      const decoder = new TextDecoder("utf-8", { fatal: true });
      return mkString(decoder.decode(v.value));
    } catch {
      return mkError("bytes are not valid UTF-8");
    }
  }
  if (isArray(v)) {
    if (containsBytes(v)) {
      return mkError(
        "cannot convert array to string: contains bytes value (convert bytes explicitly before embedding in containers)",
      );
    }
    return mkString(JSON.stringify(toJSON(v)));
  }
  if (isObject(v)) {
    if (containsBytes(v)) {
      return mkError(
        "cannot convert object to string: contains bytes value (convert bytes explicitly before embedding in containers)",
      );
    }
    return mkString(JSON.stringify(sortedToJSON(v)));
  }
  return mkError(`cannot convert ${typeName(v)} to string`);
}

function formatTimestampValue(nanos: bigint): string {
  return strftimeFormat(nanos, DEFAULT_TIMESTAMP_FORMAT);
}

function valueToInt64(v: Value): Value {
  if (isInt64(v)) return v;
  if (isInt32(v)) return mkInt64(BigInt(v.value));
  if (isUint32(v)) return mkInt64(BigInt(v.value));
  if (isUint64(v)) {
    if (v.value > BigInt(MAX_INT64)) {
      return mkError("uint64 value exceeds int64 range");
    }
    return mkInt64(BigInt(v.value));
  }
  if (isFloat64(v)) return isFinite(v.value) ? mkInt64(BigInt(Math.trunc(v.value))) : mkError("cannot convert NaN/Infinity to int64");
  if (isFloat32(v)) return isFinite(v.value) ? mkInt64(BigInt(Math.trunc(v.value))) : mkError("cannot convert NaN/Infinity to int64");
  if (isString(v)) {
    try {
      const n = BigInt(v.value);
      if (n > MAX_INT64 || n < BigInt("-9223372036854775808")) {
        return mkError("cannot convert string to int64: value out of range");
      }
      return mkInt64(n);
    } catch {
      return mkError("cannot convert string to int64: " + v.value);
    }
  }
  if (isBool(v)) return mkError("cannot convert bool to int64");
  return mkError(`cannot convert ${typeName(v)} to int64`);
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function registerTypeConversion(interp: Interpreter): void {
  const m = (
    fn: (interp: Interpreter, receiver: Value, args: Value[]) => Value,
    opts?: Partial<MethodSpec>,
  ): MethodSpec => ({
    fn,
    lambdaFn: null,
    intrinsic: false,
    params: null,
    acceptsNull: false,
    ...opts,
  });

  interp.registerMethod(
    "type",
    m((_interp, receiver) => mkString(typeName(receiver)), {
      acceptsNull: true,
    }),
  );

  interp.registerMethod(
    "string",
    m((_interp, receiver) => valueToString(receiver), { acceptsNull: true }),
  );

  interp.registerMethod(
    "int64",
    m((_interp, receiver) => valueToInt64(receiver)),
  );

  interp.registerMethod(
    "int32",
    m((_interp, receiver) => {
      const i64 = valueToInt64(receiver);
      if (i64.tag === "error") return i64;
      const n = (i64 as { tag: "int64"; value: bigint }).value;
      if (n > BigInt(MAX_INT32) || n < BigInt(MIN_INT32)) {
        return mkError("int32 overflow");
      }
      return mkInt32(Number(n));
    }),
  );

  interp.registerMethod(
    "uint32",
    m((_interp, receiver) => {
      if (isUint32(receiver)) return receiver;
      if (isInt64(receiver)) {
        if (receiver.value < 0n || receiver.value > BigInt(MAX_UINT32)) {
          return mkError("uint32 overflow");
        }
        return mkUint32(Number(receiver.value));
      }
      if (isString(receiver)) {
        try {
          const n = BigInt(receiver.value);
          if (n < 0n || n > BigInt(MAX_UINT32)) {
            return mkError("cannot convert string to uint32: value out of range");
          }
          return mkUint32(Number(n));
        } catch {
          return mkError("cannot convert string to uint32: " + receiver.value);
        }
      }
      const i64 = valueToInt64(receiver);
      if (i64.tag === "error") return i64;
      const n = (i64 as { tag: "int64"; value: bigint }).value;
      if (n < 0n || n > BigInt(MAX_UINT32)) {
        return mkError("uint32 overflow");
      }
      return mkUint32(Number(n));
    }),
  );

  interp.registerMethod(
    "uint64",
    m((_interp, receiver) => {
      if (isUint64(receiver)) return receiver;
      if (isInt64(receiver)) {
        if (receiver.value < 0n) {
          return mkError("uint64 overflow: negative value");
        }
        return mkUint64(receiver.value);
      }
      if (isString(receiver)) {
        try {
          const n = BigInt(receiver.value);
          if (n < 0n || n > MAX_UINT64) {
            return mkError("uint64 overflow: " + receiver.value);
          }
          return mkUint64(n);
        } catch {
          return mkError("uint64 overflow: " + receiver.value);
        }
      }
      const i64 = valueToInt64(receiver);
      if (i64.tag === "error") return i64;
      const n = (i64 as { tag: "int64"; value: bigint }).value;
      if (n < 0n) {
        return mkError("uint64 overflow: negative value");
      }
      return mkUint64(n);
    }),
  );

  interp.registerMethod(
    "float64",
    m((_interp, receiver) => {
      if (isFloat64(receiver)) return receiver;
      if (isFloat32(receiver)) return mkFloat64(receiver.value);
      if (isInt64(receiver)) return mkFloat64(Number(receiver.value));
      if (isInt32(receiver)) return mkFloat64(receiver.value);
      if (isUint32(receiver)) return mkFloat64(receiver.value);
      if (isUint64(receiver)) return mkFloat64(Number(receiver.value));
      if (isString(receiver)) {
        const f = Number(receiver.value);
        if (receiver.value.trim() === "" || Number.isNaN(f)) {
          return mkError(
            "cannot convert string to float64: " + receiver.value,
          );
        }
        return mkFloat64(f);
      }
      return mkError(`cannot convert ${typeName(receiver)} to float64`);
    }),
  );

  interp.registerMethod(
    "float32",
    m((_interp, receiver) => {
      if (isFloat32(receiver)) return receiver;
      // Go through float64 first.
      if (isFloat64(receiver)) return mkFloat32(receiver.value);
      if (isInt64(receiver)) return mkFloat32(Number(receiver.value));
      if (isInt32(receiver)) return mkFloat32(receiver.value);
      if (isUint32(receiver)) return mkFloat32(receiver.value);
      if (isUint64(receiver)) return mkFloat32(Number(receiver.value));
      if (isString(receiver)) {
        const f = Number(receiver.value);
        if (receiver.value.trim() === "" || Number.isNaN(f)) {
          return mkError(
            "cannot convert string to float32: " + receiver.value,
          );
        }
        return mkFloat32(f);
      }
      return mkError(`cannot convert ${typeName(receiver)} to float32`);
    }),
  );

  interp.registerMethod(
    "bool",
    m((_interp, receiver) => {
      if (isBool(receiver)) return receiver;
      if (isString(receiver)) {
        if (receiver.value === "true") return mkBool(true);
        if (receiver.value === "false") return mkBool(false);
        return mkError(
          `cannot convert string "${receiver.value}" to bool`,
        );
      }
      if (isInt64(receiver)) return mkBool(receiver.value !== 0n);
      if (isInt32(receiver)) return mkBool(receiver.value !== 0);
      if (isUint32(receiver)) return mkBool(receiver.value !== 0);
      if (isUint64(receiver)) return mkBool(receiver.value !== 0n);
      if (isFloat64(receiver)) {
        if (Number.isNaN(receiver.value)) {
          return mkError("NaN cannot be converted to bool");
        }
        return mkBool(receiver.value !== 0);
      }
      if (isFloat32(receiver)) {
        if (Number.isNaN(receiver.value)) {
          return mkError("NaN cannot be converted to bool");
        }
        return mkBool(receiver.value !== 0);
      }
      return mkError(`cannot convert ${typeName(receiver)} to bool`);
    }),
  );

  interp.registerMethod(
    "bytes",
    m(
      (_interp, receiver) => {
        if (isBytes(receiver)) return receiver;
        if (isString(receiver)) {
          return mkBytes(new TextEncoder().encode(receiver.value));
        }
        // Fall through to string conversion then to bytes.
        const s = valueToString(receiver);
        if (s.tag === "error") return s;
        return mkBytes(
          new TextEncoder().encode((s as { tag: "string"; value: string }).value),
        );
      },
      { acceptsNull: true },
    ),
  );

  interp.registerMethod(
    "char",
    m((_interp, receiver) => {
      let n: bigint | null = null;
      if (isInt64(receiver)) n = receiver.value;
      else if (isInt32(receiver)) n = BigInt(receiver.value);
      else if (isUint32(receiver)) n = BigInt(receiver.value);
      else if (isUint64(receiver)) n = receiver.value;
      else
        return mkError(`char() requires integer, got ${typeName(receiver)}`);

      if (n < 0n || n > 0x10ffffn) {
        return mkError("codepoint out of valid Unicode range");
      }
      return mkString(String.fromCodePoint(Number(n)));
    }),
  );

  interp.registerMethod("not_null", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (!isNull(receiver)) return receiver;
      let msg = "unexpected null value";
      if (args.length > 0 && isString(args[0]!)) {
        msg = args[0]!.value;
      }
      return mkError(msg);
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: true,
    params: [
      {
        name: "message",
        default_: mkString("unexpected null value"),
        hasDefault: true,
      },
    ],
  });
}
