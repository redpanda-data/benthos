import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "fs";
import { join } from "path";
import { parse as parseYaml } from "yaml";
import { parse } from "../src/parser.js";
import { optimize } from "../src/optimizer.js";
import { resolve } from "../src/resolver.js";
import { Interpreter } from "../src/interpreter.js";
import { registerStdlib, stdlibNames } from "../src/stdlib/index.js";
import {
  fromJSON, toJSON, Value, mkInt32, mkInt64, mkUint32, mkUint64,
  mkFloat32, mkFloat64, mkBytes, mkTimestamp, mkString, mkBool, mkArray, mkObject,
  isTimestamp, isBytes, isFloat64, isFloat32, isInt64, isInt32, isUint32, isUint64,
  NULL, VOID, DELETED, mkError,
} from "../src/value.js";

const SPEC_DIR = join(__dirname, "..", "..", "spec", "tests");

/**
 * Convert a spec JSON value (which may contain _type annotations) to a Bloblang Value.
 */
function specFromJSON(v: unknown): Value {
  if (v === null || v === undefined) return NULL;
  if (typeof v === "boolean") return mkBool(v);
  if (typeof v === "string") return mkString(v);
  if (typeof v === "number") {
    if (Number.isInteger(v)) return mkInt64(BigInt(v));
    return mkFloat64(v);
  }
  if (Array.isArray(v)) {
    return mkArray(v.map(specFromJSON));
  }
  if (typeof v === "object") {
    const obj = v as Record<string, unknown>;
    // Check for _type annotation.
    if (typeof obj._type === "string" && "value" in obj) {
      const strVal = String(obj.value);
      switch (obj._type) {
        case "int32": return mkInt32(parseInt(strVal, 10));
        case "int64": return mkInt64(BigInt(strVal));
        case "uint32": return mkUint32(parseInt(strVal, 10) >>> 0);
        case "uint64": return mkUint64(BigInt(strVal));
        case "float32": return mkFloat32(parseFloat(strVal));
        case "float64": {
          if (strVal === "NaN") return mkFloat64(NaN);
          if (strVal === "Infinity" || strVal === "+Infinity") return mkFloat64(Infinity);
          if (strVal === "-Infinity") return mkFloat64(-Infinity);
          return mkFloat64(parseFloat(strVal));
        }
        case "bytes": {
          // Base64 decode.
          const buf = Buffer.from(strVal, "base64");
          return mkBytes(new Uint8Array(buf));
        }
        case "timestamp": {
          // Parse RFC 3339 to nanoseconds since epoch.
          const d = new Date(strVal);
          return mkTimestamp(BigInt(d.getTime()) * 1000000n);
        }
      }
    }
    const m = new Map<string, Value>();
    for (const [key, val] of Object.entries(obj)) {
      m.set(key, specFromJSON(val));
    }
    return mkObject(m);
  }
  return mkError(`cannot convert ${typeof v} to Bloblang value`);
}

/**
 * Convert a Bloblang Value to a spec JSON value (with _type annotations for non-trivial types).
 */
function specToJSON(v: Value): unknown {
  switch (v.tag) {
    case "null": return null;
    case "bool": return v.value;
    case "int32": return { _type: "int32", value: String(v.value) };
    case "int64": {
      // Plain integers within safe range are just numbers.
      const n = Number(v.value);
      if (Number.isSafeInteger(n)) return n;
      return { _type: "int64", value: String(v.value) };
    }
    case "uint32": return { _type: "uint32", value: String(v.value) };
    case "uint64": {
      const n = Number(v.value);
      if (Number.isSafeInteger(n) && n >= 0) return n;
      return { _type: "uint64", value: String(v.value) };
    }
    case "float32": return { _type: "float32", value: float32Str(v.value) };
    case "float64": {
      if (isNaN(v.value)) return { _type: "float64", value: "NaN" };
      if (v.value === Infinity) return { _type: "float64", value: "Infinity" };
      if (v.value === -Infinity) return { _type: "float64", value: "-Infinity" };
      if (Object.is(v.value, -0)) return { _type: "float64", value: "-0" };
      return v.value;
    }
    case "string": return v.value;
    case "bytes": {
      const b64 = Buffer.from(v.value).toString("base64");
      return { _type: "bytes", value: b64 };
    }
    case "array": return v.value.map(specToJSON);
    case "object": {
      const obj: Record<string, unknown> = {};
      for (const [k, val] of v.value) {
        obj[k] = specToJSON(val);
      }
      return obj;
    }
    case "timestamp": {
      // Format as RFC 3339 with full nanosecond precision.
      const nanos = v.value;
      const ms = Number(nanos / 1000000n);
      const d = new Date(ms);
      // Build base: YYYY-MM-DDTHH:MM:SS
      const pad2 = (n: number) => String(n).padStart(2, "0");
      const base = `${d.getUTCFullYear()}-${pad2(d.getUTCMonth()+1)}-${pad2(d.getUTCDate())}T${pad2(d.getUTCHours())}:${pad2(d.getUTCMinutes())}:${pad2(d.getUTCSeconds())}`;
      // Fractional seconds from nanoseconds.
      let subSecNanos = Number(((nanos % 1000000000n) + 1000000000n) % 1000000000n);
      let frac = "";
      if (subSecNanos > 0) {
        // Format as up to 9 digits, trimming trailing zeros.
        const nanoStr = String(subSecNanos).padStart(9, "0");
        frac = "." + nanoStr.replace(/0+$/, "");
      }
      return { _type: "timestamp", value: `${base}${frac}Z` };
    }
    case "void": return undefined;
    case "deleted": return undefined;
    case "error": return `error: ${v.message}`;
  }
}

function floatStr(v: number): string {
  if (isNaN(v)) return "NaN";
  if (v === Infinity) return "Infinity";
  if (v === -Infinity) return "-Infinity";
  if (Object.is(v, -0)) return "-0";
  // Go-style shortest representation: ensure decimal point for whole numbers.
  const s = String(v);
  if (Number.isInteger(v) && !s.includes(".") && !s.includes("e")) {
    return s + ".0";
  }
  return s;
}

function float32Str(v: number): string {
  if (isNaN(v)) return "NaN";
  if (v === Infinity) return "Infinity";
  if (v === -Infinity) return "-Infinity";
  if (Object.is(v, -0)) return "-0";
  // For float32, use Go-style shortest representation for the float32 value.
  // Math.fround ensures it's the nearest float32, then format the float32 value.
  const f = Math.fround(v);
  // Use toPrecision to find shortest representation that round-trips.
  // Go's strconv.FormatFloat with 'G' format for float32 uses up to 8 significant digits.
  let s = formatShortest32(f);
  if (Number.isInteger(f) && !s.includes(".") && !s.includes("e")) {
    return s + ".0";
  }
  return s;
}

function formatShortest32(v: number): string {
  // Find shortest decimal representation that round-trips through float32.
  for (let prec = 1; prec <= 9; prec++) {
    const s = v.toPrecision(prec);
    if (Math.fround(parseFloat(s)) === v) {
      // Remove trailing zeros after decimal point, but keep at least one digit.
      return cleanupFloat(s);
    }
  }
  return String(v);
}

function cleanupFloat(s: string): string {
  if (!s.includes(".")) return s;
  // Remove trailing zeros.
  s = s.replace(/(\.\d*?)0+$/, "$1");
  // Remove trailing dot.
  s = s.replace(/\.$/, "");
  return s;
}

function buildMeta(input_metadata?: Record<string, unknown>): Value {
  const m = new Map<string, Value>();
  if (input_metadata) {
    for (const [k, v] of Object.entries(input_metadata)) {
      m.set(k, specFromJSON(v));
    }
  }
  return mkObject(m);
}

interface SpecFile {
  description?: string;
  files?: Record<string, string>;
  tests: SpecTest[];
}

interface SpecTest {
  name: string;
  mapping?: string;
  input?: unknown;
  input_metadata?: Record<string, unknown>;
  output?: unknown;
  output_metadata?: Record<string, unknown>;
  output_type?: string;
  no_output_check?: boolean;
  deleted?: boolean;
  error?: string;
  compile_error?: string;
  runtime_error?: string;
  files?: Record<string, string>;
}

function collectYamlFiles(dir: string): string[] {
  const result: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      result.push(...collectYamlFiles(full));
    } else if (entry.endsWith(".yaml")) {
      result.push(full);
    }
  }
  return result;
}

/**
 * Normalize a _type-annotated value to a plain JS value for comparison.
 * Returns the original value if not a _type annotation.
 */
function normalizeTyped(v: unknown): unknown {
  if (v === null || v === undefined || typeof v !== "object" || Array.isArray(v)) return v;
  const obj = v as Record<string, unknown>;
  if (typeof obj._type !== "string" || !("value" in obj)) return v;
  const strVal = String(obj.value);
  switch (obj._type) {
    case "int32": return { _type: "int32", value: strVal };
    case "int64": return { _type: "int64", value: strVal };
    case "uint32": return { _type: "uint32", value: strVal };
    case "uint64": return { _type: "uint64", value: strVal };
    case "float32": return { _type: "float32", value: strVal };
    case "float64": return { _type: "float64", value: strVal };
    case "bytes": return { _type: "bytes", value: strVal };
    case "timestamp": return { _type: "timestamp", value: strVal };
    default: return v;
  }
}

/**
 * Check if two values are equivalent considering _type annotations.
 * A _type-annotated integer can match a plain number if numeric values are equal.
 */
function typedNumericEqual(a: unknown, b: unknown): boolean | null {
  // One is a _type annotation, the other is a plain number.
  const aTyped = isTypedAnnotation(a);
  const bTyped = isTypedAnnotation(b);
  if (!aTyped && !bTyped) return null; // both plain, use regular comparison
  if (aTyped && bTyped) return null; // both typed, use regular comparison

  const typed = aTyped ? a : b;
  const plain = aTyped ? b : a;
  if (typeof plain !== "number") return null;

  const obj = typed as Record<string, unknown>;
  const strVal = String(obj.value);
  const typeName = obj._type as string;

  switch (typeName) {
    case "int32":
    case "int64":
    case "uint32":
    case "uint64": {
      const n = Number(strVal);
      return Math.abs(n - plain) < 1e-9;
    }
    case "float32":
    case "float64": {
      const n = parseFloat(strVal);
      if (isNaN(n) && isNaN(plain)) return true;
      return Math.abs(n - plain) < 1e-9;
    }
    default:
      return null;
  }
}

function isTypedAnnotation(v: unknown): boolean {
  if (v === null || v === undefined || typeof v !== "object" || Array.isArray(v)) return false;
  const obj = v as Record<string, unknown>;
  return typeof obj._type === "string" && "value" in obj && Object.keys(obj).length === 2;
}

/**
 * Compare two timestamp _type annotations, treating them as equal if they
 * represent the same instant in time (ignoring timezone presentation).
 */
function timestampEqual(a: unknown, b: unknown): boolean | null {
  if (!isTypedAnnotation(a) || !isTypedAnnotation(b)) return null;
  const aObj = a as Record<string, unknown>;
  const bObj = b as Record<string, unknown>;
  if (aObj._type !== "timestamp" || bObj._type !== "timestamp") return null;
  const aTime = new Date(String(aObj.value)).getTime();
  const bTime = new Date(String(bObj.value)).getTime();
  if (isNaN(aTime) || isNaN(bTime)) return false;
  return aTime === bTime;
}

/**
 * Compare two float32 _type annotations with tolerance for representation differences.
 */
function float32Equal(a: unknown, b: unknown): boolean | null {
  if (!isTypedAnnotation(a) || !isTypedAnnotation(b)) return null;
  const aObj = a as Record<string, unknown>;
  const bObj = b as Record<string, unknown>;
  if (aObj._type !== "float32" || bObj._type !== "float32") return null;
  const aVal = parseFloat(String(aObj.value));
  const bVal = parseFloat(String(bObj.value));
  if (isNaN(aVal) && isNaN(bVal)) return true;
  return Math.abs(aVal - bVal) < 1e-9;
}

function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (a === null || b === null) return a === b;

  // Check typed numeric equality (e.g., {_type: "int64", value: "42"} vs 42)
  const typedResult = typedNumericEqual(a, b);
  if (typedResult !== null) return typedResult;

  // Check timestamp equality (timezone-insensitive)
  const tsResult = timestampEqual(a, b);
  if (tsResult !== null) return tsResult;

  // Check float32 equality (representation-insensitive)
  const f32Result = float32Equal(a, b);
  if (f32Result !== null) return f32Result;

  if (typeof a !== typeof b) return false;
  if (typeof a === "number" && typeof b === "number") {
    if (isNaN(a) && isNaN(b)) return true;
    if (Math.abs(a - b) < 1e-9) return true;
    return false;
  }
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i])) return false;
    }
    return true;
  }
  if (typeof a === "object" && typeof b === "object") {
    const aObj = a as Record<string, unknown>;
    const bObj = b as Record<string, unknown>;
    const aKeys = Object.keys(aObj);
    const bKeys = Object.keys(bObj);
    if (aKeys.length !== bKeys.length) return false;
    for (const key of aKeys) {
      if (!deepEqual(aObj[key], bObj[key])) return false;
    }
    return true;
  }
  return false;
}

const { methods, functions } = stdlibNames();
const files = collectYamlFiles(SPEC_DIR);

describe("spec compatibility — execute", () => {
  for (const file of files) {
    const relPath = file.slice(SPEC_DIR.length + 1);
    const data = readFileSync(file, "utf-8");
    const spec = parseYaml(data) as SpecFile;

    for (const tc of spec.tests) {
      if (!tc.mapping) continue;

      const testName = `${relPath}/${tc.name}`;

      // Build files map for imports.
      const filesMap = new Map<string, string>();
      if (spec.files) {
        for (const [name, content] of Object.entries(spec.files)) {
          filesMap.set(name, content);
        }
      }
      if (tc.files) {
        for (const [name, content] of Object.entries(tc.files)) {
          filesMap.set(name, content);
        }
      }

      if (tc.compile_error) {
        it(`compile error: ${testName}`, () => {
          const { program, errors: parseErrors } = parse(tc.mapping!, "", filesMap);
          if (parseErrors.length > 0) return; // parse error counts as compile error

          optimize(program);
          const resolveErrors = resolve(program, methods, functions);
          expect(resolveErrors.length).toBeGreaterThan(0);
        });
        continue;
      }

      if (tc.runtime_error !== undefined || tc.error !== undefined) {
        const errorSubstring = tc.runtime_error ?? tc.error!;
        it(`runtime error: ${testName}`, () => {
          const { program, errors: parseErrors } = parse(tc.mapping!, "", filesMap);
          expect(parseErrors).toHaveLength(0);

          optimize(program);
          const resolveErrors = resolve(program, methods, functions);
          expect(resolveErrors).toHaveLength(0);

          const interp = new Interpreter(program);
          registerStdlib(interp);

          const input = tc.input !== undefined ? specFromJSON(tc.input) : NULL;
          const meta = buildMeta(tc.input_metadata);
          const { error } = interp.run(input, meta);
          if (error === null) {
            expect.fail(`Expected runtime error containing "${errorSubstring}" but execution succeeded`);
          }
        });
        continue;
      }

      if (tc.deleted) {
        it(`exec: ${testName}`, () => {
          const { program, errors: parseErrors } = parse(tc.mapping!, "", filesMap);
          if (parseErrors.length > 0) {
            const msgs = parseErrors.map(e => `${e.pos.line}:${e.pos.column}: ${e.msg}`);
            expect.fail(`Parse errors:\n${msgs.join("\n")}`);
          }
          optimize(program);
          const resolveErrors = resolve(program, methods, functions);
          if (resolveErrors.length > 0) {
            const msgs = resolveErrors.map(e => `${e.pos.line}:${e.pos.column}: ${e.msg}`);
            expect.fail(`Resolve errors:\n${msgs.join("\n")}`);
          }
          const interp = new Interpreter(program);
          registerStdlib(interp);
          const input = tc.input !== undefined ? specFromJSON(tc.input) : NULL;
          const { deleted, error } = interp.run(input, buildMeta(tc.input_metadata));
          if (error) expect.fail(`Runtime error: ${error}`);
          expect(deleted).toBe(true);
        });
        continue;
      }

      // Normal test: execute and compare output.
      it(`exec: ${testName}`, () => {
        const { program, errors: parseErrors } = parse(tc.mapping!, "", filesMap);
        if (parseErrors.length > 0) {
          const msgs = parseErrors.map(e => `${e.pos.line}:${e.pos.column}: ${e.msg}`);
          expect.fail(`Parse errors:\n${msgs.join("\n")}`);
        }

        optimize(program);
        const resolveErrors = resolve(program, methods, functions);
        if (resolveErrors.length > 0) {
          const msgs = resolveErrors.map(e => `${e.pos.line}:${e.pos.column}: ${e.msg}`);
          expect.fail(`Resolve errors:\n${msgs.join("\n")}`);
        }

        const interp = new Interpreter(program);
        registerStdlib(interp);

        const input = tc.input !== undefined ? specFromJSON(tc.input) : NULL;
        const meta = buildMeta(tc.input_metadata);
        const { output, error, deleted } = interp.run(input, meta);

        if (error) {
          expect.fail(`Runtime error: ${error}`);
        }

        if (tc.no_output_check) return;

        if (deleted) {
          expect(tc.output).toBeUndefined();
          return;
        }

        const actual = specToJSON(output);
        if (!deepEqual(actual, tc.output)) {
          expect.fail(
            `Output mismatch:\n  expected: ${JSON.stringify(tc.output, null, 2)}\n  actual:   ${JSON.stringify(actual, null, 2)}`,
          );
        }
      });
    }
  }
});
