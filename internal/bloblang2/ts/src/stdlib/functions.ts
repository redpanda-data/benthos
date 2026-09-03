// Stdlib functions: deleted, throw, uuid_v4, now, random_int, range,
// timestamp_unix, timestamp_unix_milli, timestamp_unix_nano, second, minute,
// hour, day, timestamp.

declare const crypto: {
  randomUUID?: () => string;
  getRandomValues?: (buf: Uint8Array) => Uint8Array;
};

import type { Interpreter, FunctionSpec } from "../interpreter.js";
import {
  type Value,
  mkInt64,
  mkFloat64,
  mkString,
  mkArray,
  mkTimestamp,
  mkError,
  DELETED,
  VOID,
  isString,
  isInt64,
  isUint64,
  isInt32,
  isUint32,
  isFloat32,
  isFloat64,
} from "../value.js";

function toInt64(v: Value): bigint | null {
  if (isInt64(v)) return v.value;
  if (isInt32(v)) return BigInt(v.value);
  if (isUint32(v)) return BigInt(v.value);
  if (isUint64(v)) return v.value;
  if (isFloat64(v)) return isInt64RangeWholeFloat(v.value) ? BigInt(v.value) : null;
  if (isFloat32(v)) return isInt64RangeWholeFloat(v.value) ? BigInt(v.value) : null;
  return null;
}

export function registerFunctions(interp: Interpreter): void {
  interp.registerFunction("deleted", {
    fn: () => DELETED,
    params: [],
  });

  interp.registerFunction("void", {
    fn: () => VOID,
    params: [],
  });

  interp.registerFunction("throw", {
    fn: (args: Value[]): Value => {
      if (args.length !== 1) {
        return mkError("throw() requires exactly one string argument");
      }
      const msg = args[0]!;
      if (!isString(msg)) {
        return mkError(`throw() requires a string argument, got ${msg.tag}`);
      }
      return mkError(msg.value);
    },
    params: [{ name: "message", default_: null, hasDefault: false }],
  });

  interp.registerFunction("uuid_v4", {
    fn: (): Value => {
      // crypto.randomUUID() is available in modern browsers and Node 19+.
      if (typeof crypto !== "undefined" && crypto.randomUUID) {
        return mkString(crypto.randomUUID());
      }
      // Fallback: manual v4 UUID.
      const bytes = new Uint8Array(16);
      if (typeof crypto !== "undefined" && crypto.getRandomValues) {
        crypto.getRandomValues(bytes);
      } else {
        for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
      }
      bytes[6] = (bytes[6]! & 0x0f) | 0x40;
      bytes[8] = (bytes[8]! & 0x3f) | 0x80;
      const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
      return mkString(
        `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`,
      );
    },
    params: [],
  });

  interp.registerFunction("now", {
    fn: (): Value => {
      const ms = Date.now();
      return mkTimestamp(BigInt(ms) * 1000000n);
    },
    params: [],
  });

  interp.registerFunction("random_int", {
    fn: (args: Value[]): Value => {
      if (args.length !== 2) {
        return mkError("random_int() requires min and max arguments");
      }
      const minVal = toInt64(args[0]!);
      const maxVal = toInt64(args[1]!);
      if (minVal === null || maxVal === null) {
        return mkError("random_int() requires integer arguments");
      }
      if (minVal > maxVal) {
        return mkError("random_int(): min must be <= max");
      }
      const range = maxVal - minVal + 1n;
      const rand = BigInt(Math.floor(Math.random() * Number(range)));
      return mkInt64(minVal + rand);
    },
    params: [
      { name: "min", default_: null, hasDefault: false },
      { name: "max", default_: null, hasDefault: false },
    ],
  });

  interp.registerFunction("range", {
    fn: (args: Value[]): Value => {
      if (args.length < 2 || args.length > 3) {
        return mkError("range() requires 2 or 3 arguments");
      }
      const start = toInt64(args[0]!);
      const stop = toInt64(args[1]!);
      if (start === null || stop === null) {
        return mkError("range() requires integer arguments");
      }
      let step: bigint;
      if (args.length === 3) {
        const s = toInt64(args[2]!);
        if (s === null) return mkError("range() step must be integer");
        if (s === 0n) return mkError("range() step cannot be zero");
        if ((start < stop && s < 0n) || (start > stop && s > 0n)) {
          return mkError("range() step direction contradicts start/stop");
        }
        step = s;
      } else {
        step = start <= stop ? 1n : -1n;
      }
      if (start === stop) return mkArray([]);
      const result: Value[] = [];
      if (step > 0n) {
        for (let i = start; i < stop; i += step) {
          result.push(mkInt64(i));
        }
      } else {
        for (let i = start; i > stop; i += step) {
          result.push(mkInt64(i));
        }
      }
      return mkArray(result);
    },
    params: [
      { name: "start", default_: null, hasDefault: false },
      { name: "stop", default_: null, hasDefault: false },
      { name: "step", default_: null, hasDefault: true },
    ],
  });

  // Duration constants (nanoseconds).
  interp.registerFunction("second", {
    fn: () => mkInt64(1_000_000_000n),
    params: [],
  });
  interp.registerFunction("minute", {
    fn: () => mkInt64(60_000_000_000n),
    params: [],
  });
  interp.registerFunction("hour", {
    fn: () => mkInt64(3_600_000_000_000n),
    params: [],
  });
  interp.registerFunction("day", {
    fn: () => mkInt64(86_400_000_000_000n),
    params: [],
  });

  interp.registerFunction("timestamp", {
    fn: (args: Value[]): Value => {
      if (args.length < 3) {
        return mkError("timestamp() requires at least year, month, day");
      }
      const year = toInt64(args[0]!);
      const month = toInt64(args[1]!);
      const day = toInt64(args[2]!);
      if (year === null || month === null || day === null) {
        return mkError("timestamp() requires integer year, month, day");
      }
      let hour = 0n,
        minute = 0n,
        sec = 0n,
        nano = 0n;
      let tz = "UTC";
      if (args.length > 3) {
        const h = toInt64(args[3]!);
        if (h !== null) hour = h;
      }
      if (args.length > 4) {
        const m = toInt64(args[4]!);
        if (m !== null) minute = m;
      }
      if (args.length > 5) {
        const s = toInt64(args[5]!);
        if (s !== null) sec = s;
      }
      if (args.length > 6) {
        const n = toInt64(args[6]!);
        if (n !== null) nano = n;
      }
      if (args.length > 7) {
        const tzArg = args[7]!;
        if (isString(tzArg)) tz = tzArg.value;
      }

      if (month < 1n || month > 12n) {
        return mkError(`timestamp(): month ${month} out of range (1-12)`);
      }
      if (day < 1n || day > 31n) {
        return mkError(`timestamp(): day ${day} out of range (1-31)`);
      }
      if (hour < 0n || hour > 23n) {
        return mkError(`timestamp(): hour ${hour} out of range (0-23)`);
      }
      if (minute < 0n || minute > 59n) {
        return mkError(`timestamp(): minute ${minute} out of range (0-59)`);
      }
      if (sec < 0n || sec > 59n) {
        return mkError(`timestamp(): second ${sec} out of range (0-59)`);
      }
      if (nano < 0n || nano > 999999999n) {
        return mkError(
          `timestamp(): nano ${nano} out of range (0-999999999)`,
        );
      }

      // Strict calendar validation (spec Section 13.1): a day that does
      // not exist in the given month/year errors — never normalized
      // (Feb 30 must not become Mar 1).
      const dim = daysInMonth(Number(year), Number(month));
      if (Number(day) > dim) {
        return mkError(
          `timestamp(): day ${day} does not exist in ${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")} (month has ${dim} days)`,
        );
      }

      // Build the Date. For non-UTC, try Intl API.
      // JavaScript Date doesn't natively support arbitrary IANA timezones
      // for construction, so we build in UTC and adjust for offset.
      let date: Date;
      if (tz === "UTC") {
        date = utcDateFromComponents(
          Number(year),
          Number(month),
          Number(day),
          Number(hour),
          Number(minute),
          Number(sec),
        );
        // Calendar round-trip (belt and braces on top of daysInMonth):
        // a normalized component means the requested date does not exist.
        if (
          date.getUTCFullYear() !== Number(year) ||
          date.getUTCMonth() + 1 !== Number(month) ||
          date.getUTCDate() !== Number(day)
        ) {
          return mkError(
            `timestamp(): day ${day} does not exist in ${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}`,
          );
        }
      } else {
        // Intl formats years before 1 CE era-less (year 0 renders as "1"),
        // which breaks the offset derivation below — reject explicitly
        // rather than emit a misleading error or a wrong instant.
        if (year < 1n) {
          return mkError(
            "timestamp(): years before 1 CE with a non-UTC timezone are not supported by this implementation",
          );
        }
        // Use a best-effort approach: construct in UTC, then try to find offset.
        try {
          // Build an ISO string and parse with the timezone.
          const isoStr =
            `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}T` +
            `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
          const formatter = new Intl.DateTimeFormat("en-US", {
            timeZone: tz,
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit",
            hour12: false,
          });
          // Verify timezone is valid by formatting.
          formatter.format(new Date());

          // Build a UTC date then find the offset at that point in time.
          const utcDate = utcDateFromComponents(
            Number(year),
            Number(month),
            Number(day),
            Number(hour),
            Number(minute),
            Number(sec),
          );
          // tzOffsetMs returns the zone's UTC offset at the given instant,
          // derived by formatting the instant in the target zone.
          const tzOffsetMs = (instant: Date): number => {
            const parts = new Intl.DateTimeFormat("en-US", {
              timeZone: tz,
              year: "numeric",
              month: "numeric",
              day: "numeric",
              hour: "numeric",
              minute: "numeric",
              second: "numeric",
              hourCycle: "h23",
            }).formatToParts(instant);
            const getPart = (type: string) =>
              parseInt(
                parts.find((p) => p.type === type)?.value ?? "0",
                10,
              );
            const tzDate = utcDateFromComponents(
              getPart("year"),
              getPart("month"),
              getPart("day"),
              getPart("hour"),
              getPart("minute"),
              getPart("second"),
            );
            return tzDate.getTime() - instant.getTime();
          };
          // Two-pass offset derivation: the offset at the naive instant can
          // be wrong within ~1 day of a DST transition, so re-derive the
          // offset at the first estimate and re-adjust. (The utc time we
          // want satisfies utc + offset(utc) = desired local time.)
          const offset1 = tzOffsetMs(utcDate);
          let candidate = new Date(utcDate.getTime() - offset1);
          const offset2 = tzOffsetMs(candidate);
          if (offset2 !== offset1) {
            candidate = new Date(utcDate.getTime() - offset2);
          }
          date = candidate;
          const tzOffsetMinutes = Math.round(tzOffsetMs(date) / 60000);

          void isoStr; // suppress unused warning

          // A wall-clock time skipped by a DST transition normalizes to
          // a different clock reading — reject rather than silently shift
          // (spec Section 13.1: non-existent local times error).
          const rt = new Intl.DateTimeFormat("en-US", {
            timeZone: tz,
            year: "numeric",
            month: "numeric",
            day: "numeric",
            hour: "numeric",
            minute: "numeric",
            hourCycle: "h23",
          }).formatToParts(date);
          const rtPart = (type: string) =>
            parseInt(rt.find((p) => p.type === type)?.value ?? "0", 10);
          if (
            rtPart("year") !== Number(year) ||
            rtPart("month") !== Number(month) ||
            rtPart("day") !== Number(day) ||
            rtPart("hour") !== Number(hour) ||
            rtPart("minute") !== Number(minute)
          ) {
            return mkError(
              `timestamp(): ${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")} ${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")} does not exist in ${tz} (skipped by a DST transition)`,
            );
          }

          const ms = BigInt(date.getTime());
          const nanos = ms * 1000000n + nano;
          return mkTimestamp(nanos, tzOffsetMinutes);
        } catch {
          return mkError("timestamp(): unknown timezone " + tz);
        }
      }

      const ms = BigInt(date.getTime());
      const nanos = ms * 1000000n + nano;
      return mkTimestamp(nanos);
    },
    params: [
      { name: "year", default_: null, hasDefault: false },
      { name: "month", default_: null, hasDefault: false },
      { name: "day", default_: null, hasDefault: false },
      { name: "hour", default_: mkInt64(0n), hasDefault: true },
      { name: "minute", default_: mkInt64(0n), hasDefault: true },
      { name: "second", default_: mkInt64(0n), hasDefault: true },
      { name: "nano", default_: mkInt64(0n), hasDefault: true },
      { name: "timezone", default_: mkString("UTC"), hasDefault: true },
    ],
  });
}

/**
 * Build a Date from proleptic-Gregorian wall-clock components read as UTC.
 * Date.UTC maps years 0-99 to 1900+year, so those years are constructed via
 * a +400-year proxy (identical leapness across the Gregorian 400-year cycle
 * — Feb 29 survives construction exactly when valid) and re-stamped with
 * setUTCFullYear, which recomputes the epoch from the stored components.
 */
function utcDateFromComponents(
  year: number,
  month: number,
  day: number,
  hour: number,
  minute: number,
  sec: number,
): Date {
  const proxied = year >= 0 && year < 100;
  const y = proxied ? year + 400 : year;
  const d = new Date(Date.UTC(y, month - 1, day, hour, minute, sec));
  if (proxied) d.setUTCFullYear(year);
  return d;
}

/** Days in the given month/year, leap-year aware (spec Section 13.1). */
function daysInMonth(year: number, month: number): number {
  const lengths = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month === 2) {
    const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    return leap ? 29 : 28;
  }
  return lengths[month - 1]!;
}

// isInt64RangeWholeFloat reports whether a float is a whole number that
// fits in int64. 2^63 is exactly representable as float64, so the upper
// comparison must be exclusive (spec Section 13 preamble: checked
// promotion — out-of-range values error, never wrap or saturate).
function isInt64RangeWholeFloat(f: number): boolean {
  return (
    isFinite(f) &&
    f === Math.trunc(f) &&
    f < 9223372036854775808 &&
    f >= -9223372036854775808
  );
}
