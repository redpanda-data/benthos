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
  if (isFloat64(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  if (isFloat32(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  return null;
}

export function registerFunctions(interp: Interpreter): void {
  interp.registerFunction("deleted", {
    fn: () => DELETED,
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

      // Build the Date. For non-UTC, try Intl API.
      // JavaScript Date doesn't natively support arbitrary IANA timezones
      // for construction, so we build in UTC and adjust for offset.
      let date: Date;
      if (tz === "UTC") {
        date = new Date(
          Date.UTC(
            Number(year),
            Number(month) - 1,
            Number(day),
            Number(hour),
            Number(minute),
            Number(sec),
          ),
        );
        // Fix year < 100.
        if (year >= 0n && year < 100n) {
          date.setUTCFullYear(Number(year));
        }
      } else {
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
          const utcDate = new Date(
            Date.UTC(
              Number(year),
              Number(month) - 1,
              Number(day),
              Number(hour),
              Number(minute),
              Number(sec),
            ),
          );
          // Get the offset by formatting the UTC date in the target timezone.
          const parts = new Intl.DateTimeFormat("en-US", {
            timeZone: tz,
            year: "numeric",
            month: "numeric",
            day: "numeric",
            hour: "numeric",
            minute: "numeric",
            second: "numeric",
            hour12: false,
          }).formatToParts(utcDate);
          const getPart = (type: string) =>
            parseInt(
              parts.find((p) => p.type === type)?.value ?? "0",
              10,
            );
          const tzDate = new Date(
            Date.UTC(
              getPart("year"),
              getPart("month") - 1,
              getPart("day"),
              getPart("hour"),
              getPart("minute"),
              getPart("second"),
            ),
          );
          const offsetMs = tzDate.getTime() - utcDate.getTime();
          // The local time in the tz is utcDate + offset. We want the UTC time
          // such that utc + offset = desired local time. So utc = local - offset.
          date = new Date(utcDate.getTime() - offsetMs);
          const tzOffsetMinutes = Math.round(offsetMs / 60000);

          void isoStr; // suppress unused warning

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
