// Higher-order lambda methods: filter, map, sort, sort_by, fold, any, all,
// find, unique, map_values, map_keys, map_entries, filter_entries,
// for_each, group_by, without_index, index_of, slice, or, catch.

import type { Interpreter, MethodSpec, LambdaMethodFunc, MethodParam } from "../interpreter.js";
import type { CallArg, LambdaExpr } from "../ast.js";
import {
  type Value,
  mkInt64,
  mkBool,
  mkString,
  mkArray,
  mkObject,
  mkError,
  VOID,
  isString,
  isBool,
  isInt64,
  isInt32,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isArray,
  isObject,
  isBytes,
  isError as isErrorV,
  isVoid,
  isDeleted,
  isNumeric,
  typeName,
  valuesEqual,
} from "../value.js";
import { compareForSort, isSortable, isNaNValue } from "./array_methods.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toInt64(v: Value): bigint | null {
  if (isInt64(v)) return v.value;
  if (isInt32(v)) return BigInt(v.value);
  if (isUint32(v)) return BigInt(v.value);
  if (isUint64(v)) return v.value;
  if (isFloat64(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  if (isFloat32(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  return null;
}

function clampSlice(low: bigint, high: bigint, length: bigint): [bigint, bigint] {
  if (low < 0n) low += length;
  if (high < 0n) high += length;
  if (low < 0n) low = 0n;
  if (high > length) high = length;
  if (low > high) low = high;
  return [low, high];
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function registerLambdaMethods(interp: Interpreter): void {
  const lm = (
    fn: LambdaMethodFunc,
    params?: MethodParam[],
  ): MethodSpec => ({
    fn: null,
    lambdaFn: fn,
    intrinsic: false,
    params: params ?? null,
    acceptsNull: false,
  });

  const fnParam: MethodParam = {
    name: "fn",
    default_: null,
    hasDefault: false,
    acceptsLambda: true,
  };

  // --- filter ---
  interp.registerMethod(
    "filter",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`filter() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("filter() requires a lambda argument");

      const result: Value[] = [];
      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("filter() lambda returned void");
        if (!isBool(val)) {
          return mkError(`filter() lambda must return bool, got ${val.tag}`);
        }
        if (val.value) result.push(elem);
      }
      return mkArray(result);
    }, [fnParam]),
  );

  // --- map ---
  interp.registerMethod(
    "map",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`map() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("map() requires a lambda argument");

      const result: Value[] = [];
      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) {
          return mkError("map() lambda returned void (must return a value for every element)");
        }
        if (isDeleted(val)) continue;
        result.push(val);
      }
      return mkArray(result);
    }, [fnParam]),
  );

  // --- sort (lambda version — no required params) ---
  interp.registerMethod(
    "sort",
    lm((interp, receiver, _args) => {
      if (!isArray(receiver)) {
        return mkError(`sort() requires array, got ${typeName(receiver)}`);
      }
      const arr = receiver.value;
      if (arr.length === 0) return mkArray([]);
      if (!isSortable(arr[0]!)) {
        return mkError(`sort(): ${arr[0]!.tag} is not a sortable type`);
      }

      const sorted = [...arr];
      let sortErr: Value | null = null;
      sorted.sort((a, b) => {
        if (sortErr !== null) return 0;
        const cmp = compareForSort(a, b);
        if (isErrorV(cmp)) {
          sortErr = cmp;
          return 0;
        }
        return Number((cmp as { value: bigint }).value);
      });
      if (sortErr !== null) return sortErr;
      return mkArray(sorted);
    }),
  );

  // --- sort_by ---
  interp.registerMethod(
    "sort_by",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`sort_by() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("sort_by() requires a lambda argument");

      const arr = receiver.value;
      // Extract keys.
      const keys: Value[] = new Array(arr.length);
      for (let i = 0; i < arr.length; i++) {
        const key = interp.callLambda(lambda, [arr[i]!]);
        if (isErrorV(key)) return key;
        keys[i] = key;
      }

      const indices = Array.from({ length: arr.length }, (_, i) => i);
      let sortErr: Value | null = null;
      indices.sort((i, j) => {
        if (sortErr !== null) return 0;
        const cmp = compareForSort(keys[i]!, keys[j]!);
        if (isErrorV(cmp)) {
          sortErr = cmp;
          return 0;
        }
        return Number((cmp as { value: bigint }).value);
      });
      if (sortErr !== null) return sortErr;

      return mkArray(indices.map((i) => arr[i]!));
    }, [fnParam]),
  );

  // --- any ---
  interp.registerMethod(
    "any",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`any() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("any() requires a lambda argument");

      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("any() lambda returned void");
        if (!isBool(val)) {
          return mkError(`any() lambda must return bool, got ${val.tag}`);
        }
        if (val.value) return mkBool(true);
      }
      return mkBool(false);
    }, [fnParam]),
  );

  // --- all ---
  interp.registerMethod(
    "all",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`all() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("all() requires a lambda argument");

      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("all() lambda returned void");
        if (!isBool(val)) {
          return mkError(`all() lambda must return bool, got ${val.tag}`);
        }
        if (!val.value) return mkBool(false);
      }
      return mkBool(true);
    }, [fnParam]),
  );

  // --- find ---
  interp.registerMethod(
    "find",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`find() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("find() requires a lambda argument");

      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("find() lambda returned void");
        if (!isBool(val)) {
          return mkError(`find() lambda must return bool, got ${val.tag}`);
        }
        if (val.value) return elem;
      }
      return VOID;
    }, [fnParam]),
  );

  // --- fold ---
  interp.registerMethod(
    "fold",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`fold() requires array, got ${typeName(receiver)}`);
      }
      if (args.length !== 2) {
        return mkError("fold() requires initial value and lambda arguments");
      }
      const initial = interp.evalExpr(args[0]!.value);
      if (isErrorV(initial)) return initial;

      const lambdaArg = args[1]!.value;
      if (lambdaArg.kind !== "lambda") {
        return mkError("fold() second argument must be a lambda");
      }
      const lambda = lambdaArg as LambdaExpr;

      let tally: Value = initial;
      for (const elem of receiver.value) {
        tally = interp.callLambda(lambda, [tally, elem]);
        if (isErrorV(tally)) return tally;
        if (isVoid(tally)) return mkError("fold() lambda returned void");
      }
      return tally;
    }, [
      { name: "initial", default_: null, hasDefault: false },
      fnParam,
    ]),
  );

  // --- unique ---
  interp.registerMethod(
    "unique",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`unique() requires array, got ${typeName(receiver)}`);
      }

      let keyFn: LambdaExpr | null = null;
      if (args.length > 0) {
        keyFn = interp.extractLambdaOrMapRef(args);
      }

      const seenList: Value[] = [];
      let seenNaN = false;
      const contains = (key: Value): boolean => {
        if (isNaNValue(key)) {
          if (seenNaN) return true;
          seenNaN = true;
          return false;
        }
        for (const s of seenList) {
          if (valuesEqual(s, key)) return true;
        }
        return false;
      };

      const result: Value[] = [];
      for (const elem of receiver.value) {
        let key: Value;
        if (keyFn !== null) {
          key = interp.callLambda(keyFn, [elem]);
          if (isErrorV(key)) return key;
        } else {
          key = elem;
        }
        if (!contains(key)) {
          seenList.push(key);
          result.push(elem);
        }
      }
      return mkArray(result);
    }, [{ name: "fn", default_: null, hasDefault: true, acceptsLambda: true }]),
  );

  // --- without_index ---
  interp.registerMethod(
    "without_index",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`without_index() requires array, got ${typeName(receiver)}`);
      }
      if (args.length !== 1) return mkError("without_index() requires one argument");
      const idxVal = interp.evalExpr(args[0]!.value);
      if (isErrorV(idxVal)) return idxVal;
      let idx = toInt64(idxVal);
      if (idx === null) return mkError("without_index() argument must be integer");
      const len = BigInt(receiver.value.length);
      if (idx < 0n) idx += len;
      if (idx < 0n || idx >= len) {
        return mkError("without_index(): index out of bounds");
      }
      const n = Number(idx);
      return mkArray([
        ...receiver.value.slice(0, n),
        ...receiver.value.slice(n + 1),
      ]);
    }, [{ name: "index", default_: null, hasDefault: false }]),
  );

  // --- index_of ---
  interp.registerMethod(
    "index_of",
    lm((interp, receiver, args) => {
      if (args.length !== 1) return mkError("index_of() requires one argument");
      const target = interp.evalExpr(args[0]!.value);
      if (isErrorV(target)) return target;

      if (isString(receiver)) {
        if (!isString(target)) {
          return mkError("string index_of() requires string argument");
        }
        // Codepoint-based index.
        const runes = [...receiver.value];
        const targetRunes = [...target.value];
        for (let i = 0; i <= runes.length - targetRunes.length; i++) {
          if (runes.slice(i, i + targetRunes.length).join("") === target.value) {
            return mkInt64(BigInt(i));
          }
        }
        return mkInt64(-1n);
      }
      if (isArray(receiver)) {
        for (let i = 0; i < receiver.value.length; i++) {
          if (valuesEqual(receiver.value[i]!, target)) {
            return mkInt64(BigInt(i));
          }
        }
        return mkInt64(-1n);
      }
      if (isBytes(receiver)) {
        if (!isBytes(target)) {
          return mkError("bytes index_of() requires bytes argument");
        }
        const haystack = receiver.value;
        const needle = target.value;
        outer: for (let i = 0; i <= haystack.length - needle.length; i++) {
          for (let j = 0; j < needle.length; j++) {
            if (haystack[i + j] !== needle[j]) continue outer;
          }
          return mkInt64(BigInt(i));
        }
        return mkInt64(-1n);
      }
      return mkError(`index_of() not supported on ${typeName(receiver)}`);
    }, [{ name: "target", default_: null, hasDefault: false }]),
  );

  // --- slice ---
  interp.registerMethod(
    "slice",
    lm((interp, receiver, args) => {
      if (args.length < 1 || args.length > 2) {
        return mkError("slice() requires 1 or 2 arguments");
      }
      const lowVal = interp.evalExpr(args[0]!.value);
      if (isErrorV(lowVal)) return lowVal;
      const low = toInt64(lowVal);
      if (low === null) return mkError("slice() low must be integer");

      if (isString(receiver)) {
        const runes = [...receiver.value];
        const length = BigInt(runes.length);
        let high = length;
        if (args.length === 2) {
          const hVal = interp.evalExpr(args[1]!.value);
          if (isErrorV(hVal)) return hVal;
          const h = toInt64(hVal);
          if (h === null) return mkError("slice() high must be integer");
          high = h;
        }
        const [lo, hi] = clampSlice(low, high, length);
        return mkString(runes.slice(Number(lo), Number(hi)).join(""));
      }
      if (isArray(receiver)) {
        const length = BigInt(receiver.value.length);
        let high = length;
        if (args.length === 2) {
          const hVal = interp.evalExpr(args[1]!.value);
          if (isErrorV(hVal)) return hVal;
          const h = toInt64(hVal);
          if (h === null) return mkError("slice() high must be integer");
          high = h;
        }
        const [lo, hi] = clampSlice(low, high, length);
        return mkArray(receiver.value.slice(Number(lo), Number(hi)));
      }
      if (isBytes(receiver)) {
        const length = BigInt(receiver.value.length);
        let high = length;
        if (args.length === 2) {
          const hVal = interp.evalExpr(args[1]!.value);
          if (isErrorV(hVal)) return hVal;
          const h = toInt64(hVal);
          if (h === null) return mkError("slice() high must be integer");
          high = h;
        }
        const [lo, hi] = clampSlice(low, high, length);
        return { tag: "bytes", value: receiver.value.slice(Number(lo), Number(hi)) };
      }
      return mkError(`slice() not supported on ${typeName(receiver)}`);
    }, [
      { name: "low", default_: null, hasDefault: false },
      { name: "high", default_: null, hasDefault: true },
    ]),
  );

  // --- map_values ---
  interp.registerMethod(
    "map_values",
    lm((interp, receiver, args) => {
      if (!isObject(receiver)) {
        return mkError(`map_values() requires object, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("map_values() requires a lambda argument");

      const result = new Map<string, Value>();
      for (const [k, v] of receiver.value) {
        const val = interp.callLambda(lambda, [v]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("map_values() lambda returned void");
        if (isDeleted(val)) continue;
        result.set(k, val);
      }
      return mkObject(result);
    }, [fnParam]),
  );

  // --- map_keys ---
  interp.registerMethod(
    "map_keys",
    lm((interp, receiver, args) => {
      if (!isObject(receiver)) {
        return mkError(`map_keys() requires object, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("map_keys() requires a lambda argument");

      const result = new Map<string, Value>();
      for (const [k, v] of receiver.value) {
        const newKey = interp.callLambda(lambda, [mkString(k)]);
        if (isErrorV(newKey)) return newKey;
        if (isVoid(newKey)) return mkError("map_keys() lambda returned void");
        if (isDeleted(newKey)) continue;
        if (!isString(newKey)) {
          return mkError(`map_keys() lambda must return string, got ${newKey.tag}`);
        }
        result.set(newKey.value, v);
      }
      return mkObject(result);
    }, [fnParam]),
  );

  // --- map_entries ---
  interp.registerMethod(
    "map_entries",
    lm((interp, receiver, args) => {
      if (!isObject(receiver)) {
        return mkError(`map_entries() requires object, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("map_entries() requires a lambda argument");

      const result = new Map<string, Value>();
      for (const [k, v] of receiver.value) {
        const entry = interp.callLambda(lambda, [mkString(k), v]);
        if (isErrorV(entry)) return entry;
        if (isVoid(entry)) return mkError("map_entries() lambda returned void");
        if (isDeleted(entry)) continue;
        if (!isObject(entry)) {
          return mkError("map_entries() lambda must return {key, value} object");
        }
        const keyVal = entry.value.get("key");
        if (keyVal === undefined || !isString(keyVal)) {
          return mkError("map_entries() returned entry missing string 'key'");
        }
        const valVal = entry.value.get("value");
        if (valVal === undefined) {
          return mkError("map_entries() returned entry missing 'value'");
        }
        result.set(keyVal.value, valVal);
      }
      return mkObject(result);
    }, [fnParam]),
  );

  // --- filter_entries ---
  interp.registerMethod(
    "filter_entries",
    lm((interp, receiver, args) => {
      if (!isObject(receiver)) {
        return mkError(`filter_entries() requires object, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("filter_entries() requires a lambda argument");

      const result = new Map<string, Value>();
      for (const [k, v] of receiver.value) {
        const val = interp.callLambda(lambda, [mkString(k), v]);
        if (isErrorV(val)) return val;
        if (isVoid(val)) return mkError("filter_entries() lambda returned void");
        if (!isBool(val)) {
          return mkError(`filter_entries() lambda must return bool, got ${val.tag}`);
        }
        if (val.value) result.set(k, v);
      }
      return mkObject(result);
    }, [fnParam]),
  );

  // --- for_each ---
  interp.registerMethod(
    "for_each",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`for_each() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("for_each() requires a lambda argument");

      for (const elem of receiver.value) {
        const val = interp.callLambda(lambda, [elem]);
        if (isErrorV(val)) return val;
      }
      return receiver; // Return the original array.
    }, [fnParam]),
  );

  // --- group_by ---
  interp.registerMethod(
    "group_by",
    lm((interp, receiver, args) => {
      if (!isArray(receiver)) {
        return mkError(`group_by() requires array, got ${typeName(receiver)}`);
      }
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("group_by() requires a lambda argument");

      const groups = new Map<string, Value[]>();
      for (const elem of receiver.value) {
        const key = interp.callLambda(lambda, [elem]);
        if (isErrorV(key)) return key;
        if (!isString(key)) {
          return mkError(`group_by() lambda must return string, got ${key.tag}`);
        }
        const existing = groups.get(key.value);
        if (existing !== undefined) {
          existing.push(elem);
        } else {
          groups.set(key.value, [elem]);
        }
      }
      const result = new Map<string, Value>();
      for (const [k, v] of groups) {
        result.set(k, mkArray(v));
      }
      return mkObject(result);
    }, [fnParam]),
  );

  // --- into ---
  // Pass the receiver to a single-parameter lambda and return the result.
  // Errors / void / deleted() from the lambda propagate unchanged.
  interp.registerMethod(
    "into",
    lm((interp, receiver, args) => {
      const lambda = interp.extractLambdaOrMapRef(args);
      if (lambda === null) return mkError("into() requires a lambda argument");
      if (lambda.params.length !== 1) {
        return mkError(
          `into() requires a one-parameter lambda, got ${lambda.params.length} parameters`,
        );
      }
      return interp.callLambda(lambda, [receiver]);
    }, [fnParam]),
  );

  // --- Intrinsic methods (registered for name resolution only) ---
  interp.registerMethod("catch", {
    fn: null,
    lambdaFn: null,
    intrinsic: true,
    params: [{ name: "fn", default_: null, hasDefault: false, acceptsLambda: true }],
    acceptsNull: false,
  });

  interp.registerMethod("or", {
    fn: null,
    lambdaFn: null,
    intrinsic: true,
    params: [{ name: "default", default_: null, hasDefault: false }],
    acceptsNull: false,
  });
}
