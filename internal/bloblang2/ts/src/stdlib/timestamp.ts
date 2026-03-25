// Timestamp methods: ts_unix, ts_unix_milli, ts_unix_nano, ts_format,
// ts_parse, ts_add, ts_from_unix, ts_from_unix_milli, ts_from_unix_nano,
// ts_from_unix_micro, ts_unix_micro.
//
// Also exports strftime helpers used by type_conversion.ts and encoding.ts.

import type { Interpreter, MethodSpec } from "../interpreter.js";
import {
  type Value,
  mkInt64,
  mkFloat64,
  mkString,
  mkTimestamp,
  mkError,
  isString,
  isInt64,
  isInt32,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isTimestamp,
  isNumeric,
  typeName,
  MAX_INT64,
} from "../value.js";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

export const DEFAULT_TIMESTAMP_FORMAT = "%Y-%m-%dT%H:%M:%S%f%z";
const NANOS_PER_SECOND = 1_000_000_000n;
const NANOS_PER_MILLI = 1_000_000n;
const NANOS_PER_MICRO = 1_000n;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toInt64(v: Value): bigint | null {
  if (isInt64(v)) return v.value;
  if (isInt32(v)) return BigInt(v.value);
  if (isUint32(v)) return BigInt(v.value);
  if (isUint64(v)) {
    if (v.value > MAX_INT64) return null;
    return v.value;
  }
  if (isFloat64(v)) return BigInt(Math.trunc(v.value));
  if (isFloat32(v)) return BigInt(Math.trunc(v.value));
  return null;
}

function toFloat64(v: Value): number | null {
  if (isFloat64(v)) return v.value;
  if (isFloat32(v)) return v.value;
  if (isInt64(v)) return Number(v.value);
  if (isInt32(v)) return v.value;
  if (isUint32(v)) return v.value;
  if (isUint64(v)) return Number(v.value);
  return null;
}

/** Decompose bigint nanos into Date + remaining nanos. */
function nanosToDateParts(nanos: bigint): { date: Date; subMilliNanos: bigint } {
  const millis = nanos / NANOS_PER_MILLI;
  const remainder = nanos - millis * NANOS_PER_MILLI;
  return {
    date: new Date(Number(millis)),
    subMilliNanos: remainder < 0n ? remainder + NANOS_PER_MILLI : remainder,
  };
}

// ---------------------------------------------------------------------------
// Strftime implementation
// ---------------------------------------------------------------------------

function padN(n: number, width: number): string {
  let s = String(n);
  while (s.length < width) s = "0" + s;
  return s;
}

/**
 * Format a timestamp (bigint nanos since epoch) with a strftime format.
 * Supported directives: %Y, %m, %d, %H, %M, %S, %f, %z, %Z, %%.
 */
export function strftimeFormat(nanos: bigint, format: string): string {
  const { date, subMilliNanos } = nanosToDateParts(nanos);
  const totalNanos = Number(
    (nanos % NANOS_PER_SECOND + NANOS_PER_SECOND) % NANOS_PER_SECOND,
  );

  let result = "";
  let i = 0;
  while (i < format.length) {
    if (format[i] === "%" && i + 1 < format.length) {
      const directive = format[i + 1]!;
      switch (directive) {
        case "Y":
          result += padN(date.getUTCFullYear(), 4);
          break;
        case "m":
          result += padN(date.getUTCMonth() + 1, 2);
          break;
        case "d":
          result += padN(date.getUTCDate(), 2);
          break;
        case "H":
          result += padN(date.getUTCHours(), 2);
          break;
        case "M":
          result += padN(date.getUTCMinutes(), 2);
          break;
        case "S":
          result += padN(date.getUTCSeconds(), 2);
          break;
        case "f": {
          // Fractional seconds: shortest with leading dot, trimmed trailing zeros.
          // Empty when zero.
          if (totalNanos === 0) {
            // No fractional part.
          } else {
            let s = padN(totalNanos, 9);
            // Trim trailing zeros.
            s = s.replace(/0+$/, "");
            result += "." + s;
          }
          break;
        }
        case "z": {
          // For timestamps stored as UTC bigint nanos, offset is always UTC.
          // We emit 'Z' for UTC.
          result += "Z";
          break;
        }
        case "Z": {
          result += "UTC";
          break;
        }
        case "%":
          result += "%";
          break;
        default:
          // Unknown directive: pass through.
          result += "%" + directive;
          break;
      }
      i += 2;
    } else {
      result += format[i]!;
      i++;
    }
  }

  void subMilliNanos; // used indirectly via totalNanos
  return result;
}

/**
 * Parse a string with a strftime format into bigint nanos.
 * Supported directives: %Y, %m, %d, %H, %M, %S, %f, %z, %%.
 */
export function strftimeParse(input: string, format: string): bigint | string {
  let pos = 0;
  let year = 0,
    month = 1,
    day = 1,
    hour = 0,
    minute = 0,
    second = 0;
  let fracNanos = 0;
  let tzOffsetMinutes = 0;
  let hasTz = false;

  let fi = 0;
  while (fi < format.length) {
    if (format[fi] === "%" && fi + 1 < format.length) {
      const directive = format[fi + 1]!;
      fi += 2;

      switch (directive) {
        case "Y": {
          const m = input.slice(pos).match(/^(\d{4})/);
          if (!m) return "expected 4-digit year";
          year = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "m": {
          const m = input.slice(pos).match(/^(\d{1,2})/);
          if (!m) return "expected month";
          month = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "d": {
          const m = input.slice(pos).match(/^(\d{1,2})/);
          if (!m) return "expected day";
          day = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "H": {
          const m = input.slice(pos).match(/^(\d{1,2})/);
          if (!m) return "expected hour";
          hour = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "M": {
          const m = input.slice(pos).match(/^(\d{1,2})/);
          if (!m) return "expected minute";
          minute = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "S": {
          const m = input.slice(pos).match(/^(\d{1,2})/);
          if (!m) return "expected second";
          second = parseInt(m[1]!, 10);
          pos += m[1]!.length;
          break;
        }
        case "f": {
          // Optional fractional seconds: '.' followed by 1-9 digits.
          const m = input.slice(pos).match(/^\.(\d{1,9})/);
          if (m) {
            let digits = m[1]!;
            while (digits.length < 9) digits += "0";
            fracNanos = parseInt(digits.slice(0, 9), 10);
            pos += m[0]!.length;
          }
          // If no match, that's fine — %f is optional.
          break;
        }
        case "z": {
          hasTz = true;
          const rest = input.slice(pos);
          if (rest.startsWith("Z")) {
            tzOffsetMinutes = 0;
            pos += 1;
          } else {
            // Match ±HH:MM or ±HHMM.
            const m = rest.match(/^([+-])(\d{2}):?(\d{2})/);
            if (!m) return "expected timezone offset (Z, +HH:MM, or -HHMM)";
            const sign = m[1] === "+" ? 1 : -1;
            const h = parseInt(m[2]!, 10);
            const min = parseInt(m[3]!, 10);
            tzOffsetMinutes = sign * (h * 60 + min);
            pos += m[0]!.length;
          }
          break;
        }
        case "Z": {
          // Named timezone — just consume alphabetic chars.
          const m = input.slice(pos).match(/^([A-Za-z/_]+)/);
          if (m) {
            if (m[1] === "UTC" || m[1] === "GMT") {
              tzOffsetMinutes = 0;
              hasTz = true;
            }
            pos += m[1]!.length;
          }
          break;
        }
        case "%": {
          if (input[pos] !== "%") return "expected literal %";
          pos++;
          break;
        }
        default:
          // Unknown: try to match literal.
          if (input[pos] === directive) {
            pos++;
          }
          break;
      }
    } else {
      // Literal character.
      if (input[pos] !== format[fi]) {
        return `expected '${format[fi]}' at position ${pos}, got '${input[pos] ?? "EOF"}'`;
      }
      pos++;
      fi++;
    }
  }

  // Build UTC date.
  const d = Date.UTC(year, month - 1, day, hour, minute, second);
  // Adjust for timezone offset.
  const adjustedMs = d - tzOffsetMinutes * 60 * 1000;
  return BigInt(adjustedMs) * NANOS_PER_MILLI + BigInt(fracNanos);
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function registerTimestamp(interp: Interpreter): void {
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

  // --- ts_unix ---
  interp.registerMethod(
    "ts_unix",
    m((_i, recv) => {
      if (!isTimestamp(recv)) {
        return mkError(`ts_unix() requires timestamp, got ${typeName(recv)}`);
      }
      return mkInt64(recv.value / NANOS_PER_SECOND);
    }),
  );

  // --- ts_unix_milli ---
  interp.registerMethod(
    "ts_unix_milli",
    m((_i, recv) => {
      if (!isTimestamp(recv)) {
        return mkError(`ts_unix_milli() requires timestamp, got ${typeName(recv)}`);
      }
      return mkInt64(recv.value / NANOS_PER_MILLI);
    }),
  );

  // --- ts_unix_micro ---
  interp.registerMethod(
    "ts_unix_micro",
    m((_i, recv) => {
      if (!isTimestamp(recv)) {
        return mkError(`ts_unix_micro() requires timestamp, got ${typeName(recv)}`);
      }
      return mkInt64(recv.value / NANOS_PER_MICRO);
    }),
  );

  // --- ts_unix_nano ---
  interp.registerMethod(
    "ts_unix_nano",
    m((_i, recv) => {
      if (!isTimestamp(recv)) {
        return mkError(`ts_unix_nano() requires timestamp, got ${typeName(recv)}`);
      }
      return mkInt64(recv.value);
    }),
  );

  // --- ts_from_unix ---
  interp.registerMethod(
    "ts_from_unix",
    m((_i, recv) => {
      if (!isNumeric(recv)) {
        return mkError(`ts_from_unix() requires numeric, got ${typeName(recv)}`);
      }
      if (isUint64(recv) && recv.value > MAX_INT64) {
        return mkError("ts_from_unix(): uint64 value exceeds int64 range");
      }
      const f = toFloat64(recv);
      if (f === null) return mkError("ts_from_unix() requires numeric");
      const sec = Math.trunc(f);
      const nsec = Math.round((f - sec) * 1e9);
      return mkTimestamp(BigInt(sec) * NANOS_PER_SECOND + BigInt(nsec));
    }),
  );

  // --- ts_from_unix_milli ---
  interp.registerMethod(
    "ts_from_unix_milli",
    m((_i, recv) => {
      const n = toInt64(recv);
      if (n === null) {
        return mkError(`ts_from_unix_milli() requires integer, got ${typeName(recv)}`);
      }
      return mkTimestamp(n * NANOS_PER_MILLI);
    }),
  );

  // --- ts_from_unix_micro ---
  interp.registerMethod(
    "ts_from_unix_micro",
    m((_i, recv) => {
      const n = toInt64(recv);
      if (n === null) {
        return mkError(`ts_from_unix_micro() requires integer, got ${typeName(recv)}`);
      }
      return mkTimestamp(n * NANOS_PER_MICRO);
    }),
  );

  // --- ts_from_unix_nano ---
  interp.registerMethod(
    "ts_from_unix_nano",
    m((_i, recv) => {
      const n = toInt64(recv);
      if (n === null) {
        return mkError(`ts_from_unix_nano() requires integer, got ${typeName(recv)}`);
      }
      return mkTimestamp(n);
    }),
  );

  // --- ts_parse ---
  interp.registerMethod("ts_parse", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (!isString(receiver)) {
        return mkError(`ts_parse() requires string, got ${typeName(receiver)}`);
      }
      let format = DEFAULT_TIMESTAMP_FORMAT;
      if (args.length > 0 && isString(args[0]!)) {
        format = args[0]!.value;
      }
      const result = strftimeParse(receiver.value, format);
      if (typeof result === "string") {
        return mkError("ts_parse() failed: " + result);
      }
      return mkTimestamp(result);
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: false,
    params: [
      {
        name: "format",
        default_: mkString(DEFAULT_TIMESTAMP_FORMAT),
        hasDefault: true,
      },
    ],
  });

  // --- ts_format ---
  interp.registerMethod("ts_format", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (!isTimestamp(receiver)) {
        return mkError(`ts_format() requires timestamp, got ${typeName(receiver)}`);
      }
      let format = DEFAULT_TIMESTAMP_FORMAT;
      if (args.length > 0 && isString(args[0]!)) {
        format = args[0]!.value;
      }
      return mkString(strftimeFormat(receiver.value, format));
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: false,
    params: [
      {
        name: "format",
        default_: mkString(DEFAULT_TIMESTAMP_FORMAT),
        hasDefault: true,
      },
    ],
  });

  // --- ts_add ---
  interp.registerMethod("ts_add", {
    fn: (_interp: Interpreter, receiver: Value, args: Value[]): Value => {
      if (!isTimestamp(receiver)) {
        return mkError(`ts_add() requires timestamp, got ${typeName(receiver)}`);
      }
      if (args.length !== 1) {
        return mkError("ts_add() requires one argument (nanoseconds)");
      }
      const nanos = toInt64(args[0]!);
      if (nanos === null) {
        return mkError("ts_add() argument must be integer nanoseconds");
      }
      return mkTimestamp(receiver.value + nanos);
    },
    lambdaFn: null,
    intrinsic: false,
    acceptsNull: false,
    params: [{ name: "nanos", default_: null, hasDefault: false }],
  });
}
