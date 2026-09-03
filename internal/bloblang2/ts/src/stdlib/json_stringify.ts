// Canonical JSON serialization for Bloblang V2 values (spec Sections 2.3,
// 13.2, 13.11): object keys in ascending Unicode-codepoint order, int64 and
// uint64 emitted exactly (never through float64), timestamps as
// shortest-precision RFC 3339.
//
// This module exists because JSON.stringify over a plain JS object cannot
// meet the spec: JS objects enumerate integer-like keys numerically first
// (defeating canonical key order), and Number(bigint) loses precision above
// 2^53. Both .format_json() and .string() on containers serialize through
// here so the two paths cannot drift apart.
//
// Pretty-printing (non-empty indent) reproduces JSON.stringify(x, null,
// indent) formatting exactly: elements one per line, `": "` after keys,
// accumulated indentation, empty containers as `{}` / `[]`.

import {
  type Value,
  isNull,
  isBool,
  isInt32,
  isInt64,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isString,
  isTimestamp,
  isArray,
  isObject,
  compareCodepoints,
  formatFloat,
  formatFloat32,
} from "../value.js";
import { strftimeFormat, DEFAULT_TIMESTAMP_FORMAT } from "./timestamp.js";

/**
 * Serialize a value to a JSON string. Callers are responsible for
 * pre-validating non-serializable content (bytes, NaN/Infinity) — see
 * checkJSONSerializable / containsBytes at the call sites.
 */
export function stringifyValue(v: Value, indent: string): string {
  return write(v, indent, "");
}

function write(v: Value, indent: string, pad: string): string {
  if (isNull(v)) return "null";
  if (isBool(v)) return v.value ? "true" : "false";
  if (isInt64(v) || isUint64(v)) return v.value.toString(); // exact — never via float64
  if (isInt32(v) || isUint32(v)) return String(v.value);
  if (isFloat64(v)) return jsonFloat(formatFloat(v.value));
  if (isFloat32(v)) return jsonFloat(formatFloat32(v.value));
  if (isString(v)) return JSON.stringify(v.value);
  if (isTimestamp(v)) {
    return JSON.stringify(
      strftimeFormat(v.value, DEFAULT_TIMESTAMP_FORMAT, v.offsetMinutes),
    );
  }
  if (isArray(v)) {
    if (v.value.length === 0) return "[]";
    const childPad = pad + indent;
    const items = v.value.map((e) => write(e, indent, childPad));
    if (indent === "") return "[" + items.join(",") + "]";
    return "[\n" + childPad + items.join(",\n" + childPad) + "\n" + pad + "]";
  }
  if (isObject(v)) {
    const keys = [...v.value.keys()].sort(compareCodepoints);
    if (keys.length === 0) return "{}";
    const childPad = pad + indent;
    const colon = indent === "" ? ":" : ": ";
    const items = keys.map(
      (k) => JSON.stringify(k) + colon + write(v.value.get(k)!, indent, childPad),
    );
    if (indent === "") return "{" + items.join(",") + "}";
    return "{\n" + childPad + items.join(",\n" + childPad) + "\n" + pad + "}";
  }
  // Bytes / other non-serializable tags: callers pre-validate; this line is
  // unreachable through the public paths.
  return "null";
}

// jsonFloat guards the spec float rendering for JSON contexts: NaN and
// Infinity are not representable (format_json pre-validates and never hits
// this; .string() on containers catches the throw and errors).
function jsonFloat(s: string): string {
  if (s === "NaN" || s === "Infinity" || s === "-Infinity") {
    throw new Error(s + " is not representable in JSON");
  }
  return s;
}
