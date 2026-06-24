// Array/sequence methods: length, append, concat, flatten, reverse, sort,
// unique, contains, enumerate, sum, min, max, join, collect, values,
// iter (object→entries array).

import type { Interpreter, MethodSpec } from "../interpreter.js";
import { TokenType } from "../token.js";
import { evalBinaryOp } from "../arithmetic.js";
import {
  type Value,
  mkInt64,
  mkBool,
  mkString,
  mkArray,
  mkObject,
  mkError,
  isString,
  isInt64,
  isInt32,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isArray,
  isObject,
  isBytes,
  isTimestamp,
  isNumeric,
  isError as isErrorV,
  typeName,
  valuesEqual,
  promoteChecked,
} from "../value.js";

// ---------------------------------------------------------------------------
// Sort helpers
// ---------------------------------------------------------------------------

function isNaNValue(v: Value): boolean {
  return (isFloat64(v) && Number.isNaN(v.value)) || (isFloat32(v) && Number.isNaN(v.value));
}

function isSortable(v: Value): boolean {
  return (
    isInt32(v) || isInt64(v) || isUint32(v) || isUint64(v) ||
    isFloat32(v) || isFloat64(v) || isString(v) || isTimestamp(v)
  );
}

function compareForSort(a: Value, b: Value): Value {
  const aNaN = isNaNValue(a);
  const bNaN = isNaNValue(b);
  if (aNaN && bNaN) return mkInt64(0n);
  if (aNaN) return mkInt64(1n);
  if (bNaN) return mkInt64(-1n);

  // Numeric comparison.
  if (isNumeric(a) && isNumeric(b)) {
    const result = promoteChecked(a, b);
    if (result === null) {
      return mkError(`cannot sort: promotion failed for ${a.tag} and ${b.tag}`);
    }
    const [pa, pb, kind] = result;
    switch (kind) {
      case "int64": {
        const av = (pa as { value: bigint }).value;
        const bv = (pb as { value: bigint }).value;
        return mkInt64(av < bv ? -1n : av > bv ? 1n : 0n);
      }
      case "int32": {
        const av = (pa as { value: number }).value;
        const bv = (pb as { value: number }).value;
        return mkInt64(av < bv ? -1n : av > bv ? 1n : 0n);
      }
      case "uint32": {
        const av = (pa as { value: number }).value;
        const bv = (pb as { value: number }).value;
        return mkInt64(av < bv ? -1n : av > bv ? 1n : 0n);
      }
      case "uint64": {
        const av = (pa as { value: bigint }).value;
        const bv = (pb as { value: bigint }).value;
        return mkInt64(av < bv ? -1n : av > bv ? 1n : 0n);
      }
      case "float64":
      case "float32": {
        const av = (pa as { value: number }).value;
        const bv = (pb as { value: number }).value;
        return mkInt64(av < bv ? -1n : av > bv ? 1n : 0n);
      }
    }
    return mkInt64(0n);
  }

  // String comparison.
  if (isString(a) && isString(b)) {
    return mkInt64(
      a.value < b.value ? -1n : a.value > b.value ? 1n : 0n,
    );
  }

  // Timestamp comparison.
  if (isTimestamp(a) && isTimestamp(b)) {
    return mkInt64(
      a.value < b.value ? -1n : a.value > b.value ? 1n : 0n,
    );
  }

  return mkError(`cannot sort: incompatible types ${a.tag} and ${b.tag}`);
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function registerArrayMethods(interp: Interpreter): void {
  const m = (
    fn: (interp: Interpreter, receiver: Value, args: Value[]) => Value,
  ): MethodSpec => ({
    fn,
    lambdaFn: null,
    intrinsic: false,
    params: null,
    acceptsNull: false,
  });

  // --- length ---
  interp.registerMethod(
    "length",
    m((_i, recv) => {
      if (isString(recv)) {
        // Count codepoints, not UTF-16 code units.
        return mkInt64(BigInt([...recv.value].length));
      }
      if (isArray(recv)) return mkInt64(BigInt(recv.value.length));
      if (isBytes(recv)) return mkInt64(BigInt(recv.value.length));
      if (isObject(recv)) return mkInt64(BigInt(recv.value.size));
      return mkError(`length() not supported on ${typeName(recv)}`);
    }),
  );

  // --- contains (string + array + bytes) ---
  interp.registerMethod(
    "contains",
    m((_i, recv, args) => {
      if (args.length !== 1) {
        return mkError("contains() requires exactly one argument");
      }
      if (isString(recv)) {
        const target = args[0]!;
        if (!isString(target)) {
          return mkError("string contains() requires string argument");
        }
        return mkBool(recv.value.includes(target.value));
      }
      if (isArray(recv)) {
        for (const elem of recv.value) {
          if (valuesEqual(elem, args[0]!)) return mkBool(true);
        }
        return mkBool(false);
      }
      if (isBytes(recv)) {
        const target = args[0]!;
        if (!isBytes(target)) {
          return mkError("bytes contains() requires bytes argument");
        }
        // Search for subsequence.
        const haystack = recv.value;
        const needle = target.value;
        outer: for (let i = 0; i <= haystack.length - needle.length; i++) {
          for (let j = 0; j < needle.length; j++) {
            if (haystack[i + j] !== needle[j]) continue outer;
          }
          return mkBool(true);
        }
        return mkBool(false);
      }
      return mkError(`contains() not supported on ${typeName(recv)}`);
    }),
  );

  // --- reverse ---
  interp.registerMethod(
    "reverse",
    m((_i, recv) => {
      if (isString(recv)) {
        return mkString([...recv.value].reverse().join(""));
      }
      if (isArray(recv)) {
        return mkArray([...recv.value].reverse());
      }
      if (isBytes(recv)) {
        const result = new Uint8Array(recv.value.length);
        for (let i = 0, j = recv.value.length - 1; j >= 0; i++, j--) {
          result[i] = recv.value[j]!;
        }
        return { tag: "bytes", value: result };
      }
      return mkError(`reverse() not supported on ${typeName(recv)}`);
    }),
  );

  // --- append ---
  interp.registerMethod(
    "append",
    m((_i, recv, args) => {
      if (!isArray(recv)) {
        return mkError(`append() requires array, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("append() requires one argument");
      return mkArray([...recv.value, args[0]!]);
    }),
  );

  // --- concat ---
  interp.registerMethod(
    "concat",
    m((_i, recv, args) => {
      if (!isArray(recv)) {
        return mkError(`concat() requires array, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("concat() requires one argument");
      const other = args[0]!;
      if (!isArray(other)) return mkError("concat() argument must be array");
      return mkArray([...recv.value, ...other.value]);
    }),
  );

  // --- flatten ---
  interp.registerMethod(
    "flatten",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`flatten() requires array, got ${typeName(recv)}`);
      }
      const result: Value[] = [];
      for (const elem of recv.value) {
        if (isArray(elem)) {
          result.push(...elem.value);
        } else {
          result.push(elem);
        }
      }
      return mkArray(result);
    }),
  );

  // --- enumerate ---
  interp.registerMethod(
    "enumerate",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`enumerate() requires array, got ${typeName(recv)}`);
      }
      return mkArray(
        recv.value.map((v, i) =>
          mkObject(
            new Map<string, Value>([
              ["index", mkInt64(BigInt(i))],
              ["value", v],
            ]),
          ),
        ),
      );
    }),
  );

  // --- join ---
  interp.registerMethod(
    "join",
    m((_i, recv, args) => {
      if (!isArray(recv)) {
        return mkError(`join() requires array, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("join() requires one argument");
      const delim = args[0]!;
      if (!isString(delim)) return mkError("join() delimiter must be string");
      const parts: string[] = [];
      for (let i = 0; i < recv.value.length; i++) {
        const elem = recv.value[i]!;
        if (!isString(elem)) {
          return mkError(
            `join() requires all elements to be strings, element ${i} is ${typeName(elem)}`,
          );
        }
        parts.push(elem.value);
      }
      return mkString(parts.join(delim.value));
    }),
  );

  // --- sum ---
  interp.registerMethod(
    "sum",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`sum() requires array, got ${typeName(recv)}`);
      }
      if (recv.value.length === 0) return mkInt64(0n);
      let result = recv.value[0]!;
      if (!isNumeric(result)) {
        return mkError(
          `sum() requires numeric elements, got ${typeName(result)}`,
        );
      }
      for (let i = 1; i < recv.value.length; i++) {
        result = evalBinaryOp(TokenType.PLUS, result, recv.value[i]!);
        if (isErrorV(result)) return result;
      }
      return result;
    }),
  );

  // --- min ---
  interp.registerMethod(
    "min",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`min() requires array, got ${typeName(recv)}`);
      }
      if (recv.value.length === 0) {
        return mkError("min() requires non-empty array");
      }
      let result = recv.value[0]!;
      let widest = recv.value[0]!;
      for (let i = 1; i < recv.value.length; i++) {
        const elem = recv.value[i]!;
        const cmp = compareForSort(result, elem);
        if (isErrorV(cmp)) return cmp;
        if ((cmp as { value: bigint }).value > 0n) {
          result = elem;
        }
        const promoted = promoteChecked(widest, elem);
        if (promoted !== null) {
          widest = promoted[0];
        }
      }
      const finalP = promoteChecked(result, widest);
      if (finalP !== null) result = finalP[0];
      return result;
    }),
  );

  // --- max ---
  interp.registerMethod(
    "max",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`max() requires array, got ${typeName(recv)}`);
      }
      if (recv.value.length === 0) {
        return mkError("max() requires non-empty array");
      }
      let result = recv.value[0]!;
      let widest = recv.value[0]!;
      for (let i = 1; i < recv.value.length; i++) {
        const elem = recv.value[i]!;
        const cmp = compareForSort(result, elem);
        if (isErrorV(cmp)) return cmp;
        if ((cmp as { value: bigint }).value < 0n) {
          result = elem;
        }
        const promoted = promoteChecked(widest, elem);
        if (promoted !== null) {
          widest = promoted[0];
        }
      }
      const finalP = promoteChecked(result, widest);
      if (finalP !== null) result = finalP[0];
      return result;
    }),
  );

  // --- collect ---
  interp.registerMethod(
    "collect",
    m((_i, recv) => {
      if (!isArray(recv)) {
        return mkError(`collect() requires array, got ${typeName(recv)}`);
      }
      const result = new Map<string, Value>();
      for (const elem of recv.value) {
        if (!isObject(elem)) {
          return mkError("collect() requires array of {key, value} objects");
        }
        const key = elem.value.get("key");
        if (key === undefined || !isString(key)) {
          return mkError("collect() entry missing string 'key' field");
        }
        const val = elem.value.get("value");
        if (val === undefined) {
          return mkError("collect() entry missing 'value' field");
        }
        result.set(key.value, val);
      }
      return mkObject(result);
    }),
  );

  // --- iter (object → array of {key, value}) ---
  interp.registerMethod(
    "iter",
    m((_i, recv) => {
      if (!isObject(recv)) {
        return mkError(`iter() requires object, got ${typeName(recv)}`);
      }
      const result: Value[] = [];
      for (const [k, v] of recv.value) {
        result.push(
          mkObject(
            new Map<string, Value>([
              ["key", mkString(k)],
              ["value", v],
            ]),
          ),
        );
      }
      return mkArray(result);
    }),
  );
}

// Export compareForSort and isSortable for lambda_methods to use.
export { compareForSort, isSortable, isNaNValue };
