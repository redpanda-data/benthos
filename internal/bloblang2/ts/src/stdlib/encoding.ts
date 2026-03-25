// Encoding methods: parse_json, format_json, encode, decode.

/* eslint-disable @typescript-eslint/no-explicit-any */
declare const TextEncoder: { new (): { encode(s: string): Uint8Array } };
declare const TextDecoder: {
  new (label?: string, options?: { fatal?: boolean }): {
    decode(input: Uint8Array): string;
  };
};
declare const Buffer: {
  from(data: Uint8Array): { toString(encoding: string): string };
  from(data: string, encoding: string): Uint8Array;
};
declare function btoa(s: string): string;
declare function atob(s: string): string;
/* eslint-enable @typescript-eslint/no-explicit-any */

import type { Interpreter, MethodSpec } from "../interpreter.js";
import {
  type Value,
  mkInt64,
  mkFloat64,
  mkString,
  mkBool,
  mkBytes,
  mkArray,
  mkObject,
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
  isArray,
  isObject,
  isBytes,
  isTimestamp,
  isNumeric,
  typeName,
  NULL,
  toJSON,
} from "../value.js";
import { strftimeFormat, DEFAULT_TIMESTAMP_FORMAT } from "./timestamp.js";

// ---------------------------------------------------------------------------
// JSON normalization (like Go's json.Number → int64/float64)
// ---------------------------------------------------------------------------

/**
 * Parse JSON string preserving Go-like number semantics:
 * - Numbers with decimal point or exponent notation → float64
 * - Integer numbers → int64
 */
function parseJSONToValue(data: string): Value {
  // Use a two-pass approach: first find all number literals and check if they have
  // decimal points or exponent notation, then parse normally.
  const floatPositions = new Set<string>();

  // Walk the JSON string to find number tokens that contain '.', 'e', or 'E'.
  let i = 0;
  const len = data.length;
  let path: string[] = [];
  let arrayIndices: number[] = [];

  // Simple approach: parse with reviver to detect the raw text.
  // JSON.parse raw source is available via the `source` parameter in modern Node.
  // Fallback: use regex to detect exponent numbers in the source.
  // Actually, simplest: custom reviver that gets the key, check source text.

  // Use JSON.parse with a reviver that gets the raw value.
  // In Node 22+, JSON.parse has `context.source` in the reviver.
  // For compatibility, use a different approach: parse, then re-scan source for number tokens.

  // Simplest correct approach: scan JSON for number tokens, tag any with e/E/. as float.
  const numberRegex = /(?<=[[{:,\s]|^)-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?(?=[\s,}\]\n]|$)/g;
  const floatNumbers = new Set<number>();
  let match;
  while ((match = numberRegex.exec(data)) !== null) {
    const numStr = match[0];
    if (numStr.includes('.') || numStr.includes('e') || numStr.includes('E')) {
      floatNumbers.add(parseFloat(numStr));
    }
  }

  // Parse with awareness of float numbers.
  const parsed = JSON.parse(data);
  return normalizeJSONValueWithFloats(parsed, floatNumbers);
}

function normalizeJSONValueWithFloats(v: unknown, floatNumbers: Set<number>): Value {
  if (v === null || v === undefined) return NULL;
  if (typeof v === "boolean") return mkBool(v);
  if (typeof v === "string") return mkString(v);
  if (typeof v === "number") {
    // If this number was written with exponent or decimal point, it's float64.
    if (!Number.isInteger(v) || floatNumbers.has(v)) {
      return mkFloat64(v);
    }
    return mkInt64(BigInt(v));
  }
  if (Array.isArray(v)) {
    return mkArray(v.map(e => normalizeJSONValueWithFloats(e, floatNumbers)));
  }
  if (typeof v === "object") {
    const m = new Map<string, Value>();
    for (const [key, val] of Object.entries(v as Record<string, unknown>)) {
      m.set(key, normalizeJSONValueWithFloats(val, floatNumbers));
    }
    return mkObject(m);
  }
  return mkError(`parse_json(): unsupported type ${typeof v}`);
}

function normalizeJSONValue(v: unknown): Value {
  if (v === null || v === undefined) return NULL;
  if (typeof v === "boolean") return mkBool(v);
  if (typeof v === "string") return mkString(v);
  if (typeof v === "number") {
    if (Number.isInteger(v)) {
      return mkInt64(BigInt(v));
    }
    return mkFloat64(v);
  }
  if (Array.isArray(v)) {
    return mkArray(v.map(normalizeJSONValue));
  }
  if (typeof v === "object") {
    const m = new Map<string, Value>();
    for (const [key, val] of Object.entries(v as Record<string, unknown>)) {
      m.set(key, normalizeJSONValue(val));
    }
    return mkObject(m);
  }
  return mkError(`parse_json(): unsupported type ${typeof v}`);
}

// ---------------------------------------------------------------------------
// format_json helpers
// ---------------------------------------------------------------------------

function checkJSONSerializable(v: Value): string {
  if (isFloat64(v) || isFloat32(v)) {
    const f = v.value;
    if (Number.isNaN(f)) return "format_json(): NaN is not representable in JSON";
    if (!Number.isFinite(f)) return "format_json(): Infinity is not representable in JSON";
  }
  if (isBytes(v)) {
    return "format_json(): bytes have no implicit JSON serialization";
  }
  if (isArray(v)) {
    for (const elem of v.value) {
      const err = checkJSONSerializable(elem);
      if (err) return err;
    }
  }
  if (isObject(v)) {
    for (const [, val] of v.value) {
      const err = checkJSONSerializable(val);
      if (err) return err;
    }
  }
  return "";
}

/**
 * Convert a Value to a JSON-compatible JS object with sorted keys
 * and timestamps formatted as strings.
 */
function sortedJSONValue(v: Value): unknown {
  if (isNull(v)) return null;
  if (isBool(v)) return v.value;
  if (isInt32(v)) return v.value;
  if (isInt64(v)) return Number(v.value);
  if (isUint32(v)) return v.value;
  if (isUint64(v)) return Number(v.value);
  if (isFloat32(v)) return v.value;
  if (isFloat64(v)) return v.value;
  if (isString(v)) return v.value;
  if (isTimestamp(v)) return strftimeFormat(v.value, DEFAULT_TIMESTAMP_FORMAT);
  if (isArray(v)) return v.value.map(sortedJSONValue);
  if (isObject(v)) {
    // Sort keys for deterministic output.
    const obj: Record<string, unknown> = {};
    const keys = [...v.value.keys()].sort();
    for (const k of keys) {
      obj[k] = sortedJSONValue(v.value.get(k)!);
    }
    return obj;
  }
  return toJSON(v);
}

// ---------------------------------------------------------------------------
// Base64 helpers (works in both browser and Node.js)
// ---------------------------------------------------------------------------

function base64Encode(data: Uint8Array): string {
  if (typeof Buffer !== "undefined") {
    return Buffer.from(data).toString("base64");
  }
  // Browser.
  let binary = "";
  for (const byte of data) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function base64Decode(s: string): Uint8Array | null {
  try {
    // Validate base64 characters before decoding (Node's Buffer silently ignores invalid chars).
    if (!/^[A-Za-z0-9+/]*={0,2}$/.test(s)) {
      return null;
    }
    if (typeof Buffer !== "undefined") {
      return new Uint8Array(Buffer.from(s, "base64"));
    }
    const binary = atob(s);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  } catch {
    return null;
  }
}

function base64UrlEncode(data: Uint8Array): string {
  const standard = base64Encode(data);
  return standard.replace(/\+/g, "-").replace(/\//g, "_");
}

function base64UrlDecode(s: string): Uint8Array | null {
  // Add padding if missing.
  let padded = s.replace(/-/g, "+").replace(/_/g, "/");
  while (padded.length % 4 !== 0) padded += "=";
  return base64Decode(padded);
}

function base64RawUrlEncode(data: Uint8Array): string {
  return base64UrlEncode(data).replace(/=/g, "");
}

function base64RawUrlDecode(s: string): Uint8Array | null {
  return base64UrlDecode(s);
}

function hexEncode(data: Uint8Array): string {
  return Array.from(data, (b) => b.toString(16).padStart(2, "0")).join("");
}

function hexDecode(s: string): Uint8Array | null {
  if (s.length % 2 !== 0) return null;
  const bytes = new Uint8Array(s.length / 2);
  for (let i = 0; i < s.length; i += 2) {
    const byte = parseInt(s.slice(i, i + 2), 16);
    if (Number.isNaN(byte)) return null;
    bytes[i / 2] = byte;
  }
  return bytes;
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function registerEncoding(interp: Interpreter): void {
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

  // --- parse_json ---
  interp.registerMethod(
    "parse_json",
    m((_i, recv) => {
      let data: string;
      if (isString(recv)) {
        data = recv.value;
      } else if (isBytes(recv)) {
        data = new TextDecoder().decode(recv.value);
      } else {
        return mkError(`parse_json() requires string or bytes, got ${typeName(recv)}`);
      }
      try {
        return parseJSONToValue(data);
      } catch (e) {
        return mkError("parse_json() failed: " + (e as Error).message);
      }
    }),
  );

  // --- format_json ---
  interp.registerMethod("format_json", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      let indent = "";
      let escapeHTML = true;

      if (args.length > 0 && isString(args[0]!)) {
        indent = args[0]!.value;
      }
      if (args.length > 1 && isBool(args[1]!) && args[1]!.value) {
        indent = ""; // no_indent overrides indent
      }
      if (args.length > 2 && isBool(args[2]!)) {
        escapeHTML = args[2]!.value;
      }

      const err = checkJSONSerializable(receiver);
      if (err) return mkError(err);

      const jsValue = sortedJSONValue(receiver);
      let result: string;
      if (indent !== "") {
        result = JSON.stringify(jsValue, null, indent);
      } else {
        result = JSON.stringify(jsValue);
      }

      // HTML escaping: JSON.stringify doesn't escape <, >, & by default.
      if (escapeHTML) {
        result = result
          .replace(/&/g, "\\u0026")
          .replace(/</g, "\\u003c")
          .replace(/>/g, "\\u003e");
      }

      return mkString(result);
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: true,
    params: [
      { name: "indent", default_: mkString(""), hasDefault: true },
      { name: "no_indent", default_: mkBool(false), hasDefault: true },
      { name: "escape_html", default_: mkBool(true), hasDefault: true },
    ],
  });

  // --- encode ---
  interp.registerMethod("encode", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (args.length !== 1) return mkError("encode() requires one argument (scheme)");
      const scheme = args[0]!;
      if (!isString(scheme)) return mkError("encode() scheme must be string");

      let data: Uint8Array;
      if (isString(receiver)) {
        data = new TextEncoder().encode(receiver.value);
      } else if (isBytes(receiver)) {
        data = receiver.value;
      } else {
        return mkError(`encode() requires string or bytes, got ${typeName(receiver)}`);
      }

      switch (scheme.value) {
        case "base64":
          return mkString(base64Encode(data));
        case "base64url":
          return mkString(base64UrlEncode(data));
        case "base64rawurl":
          return mkString(base64RawUrlEncode(data));
        case "hex":
          return mkString(hexEncode(data));
        default:
          return mkError("encode(): unknown scheme " + scheme.value);
      }
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: false,
    params: [{ name: "scheme", default_: null, hasDefault: false }],
  });

  // --- decode ---
  interp.registerMethod("decode", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (!isString(receiver)) {
        return mkError(`decode() requires string, got ${typeName(receiver)}`);
      }
      if (args.length !== 1) return mkError("decode() requires one argument (scheme)");
      const scheme = args[0]!;
      if (!isString(scheme)) return mkError("decode() scheme must be string");

      const s = receiver.value;
      switch (scheme.value) {
        case "base64": {
          const b = base64Decode(s);
          if (b === null) return mkError("decode() base64 failed");
          return mkBytes(b);
        }
        case "base64url": {
          const b = base64UrlDecode(s);
          if (b === null) return mkError("decode() base64url failed");
          return mkBytes(b);
        }
        case "base64rawurl": {
          const b = base64RawUrlDecode(s);
          if (b === null) return mkError("decode() base64rawurl failed");
          return mkBytes(b);
        }
        case "hex": {
          const b = hexDecode(s);
          if (b === null) return mkError("decode() hex failed");
          return mkBytes(b);
        }
        default:
          return mkError("decode(): unknown scheme " + scheme.value);
      }
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: false,
    params: [{ name: "scheme", default_: null, hasDefault: false }],
  });
}
