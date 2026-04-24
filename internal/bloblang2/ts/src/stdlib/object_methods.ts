// Object methods: keys, values, has_key, merge, without, assign.

import type { Interpreter, MethodSpec } from "../interpreter.js";
import {
  type Value,
  mkString,
  mkBool,
  mkArray,
  mkObject,
  mkError,
  isString,
  isArray,
  isObject,
  typeName,
} from "../value.js";

export function registerObjectMethods(interp: Interpreter): void {
  const m = (
    fn: (interp: Interpreter, receiver: Value, args: Value[]) => Value,
  ): MethodSpec => ({
    fn,
    lambdaFn: null,
    intrinsic: false,
    params: null,
    acceptsNull: false,
  });

  // --- keys ---
  interp.registerMethod(
    "keys",
    m((_i, recv) => {
      if (!isObject(recv)) {
        return mkError(`keys() requires object, got ${typeName(recv)}`);
      }
      const keys = [...recv.value.keys()].sort();
      return mkArray(keys.map(mkString));
    }),
  );

  // --- values ---
  interp.registerMethod(
    "values",
    m((_i, recv) => {
      if (!isObject(recv)) {
        return mkError(`values() requires object, got ${typeName(recv)}`);
      }
      // Sort by keys for deterministic order.
      const keys = [...recv.value.keys()].sort();
      return mkArray(keys.map((k) => recv.value.get(k)!));
    }),
  );

  // --- has_key ---
  interp.registerMethod(
    "has_key",
    m((_i, recv, args) => {
      if (!isObject(recv)) {
        return mkError(`has_key() requires object, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("has_key() requires one argument");
      const key = args[0]!;
      if (!isString(key)) return mkError("has_key() argument must be string");
      return mkBool(recv.value.has(key.value));
    }),
  );

  // --- merge ---
  interp.registerMethod(
    "merge",
    m((_i, recv, args) => {
      if (!isObject(recv)) {
        return mkError(`merge() requires object, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("merge() requires one argument");
      const other = args[0]!;
      if (!isObject(other)) return mkError("merge() argument must be object");
      const result = new Map<string, Value>(recv.value);
      for (const [k, v] of other.value) {
        result.set(k, v);
      }
      return mkObject(result);
    }),
  );

  // --- without (object version: takes array of string keys) ---
  interp.registerMethod(
    "without",
    m((_i, recv, args) => {
      if (!isObject(recv)) {
        return mkError(`without() requires object, got ${typeName(recv)}`);
      }
      if (args.length !== 1) return mkError("without() requires one argument");
      const keys = args[0]!;
      if (!isArray(keys)) {
        return mkError("without() argument must be array of strings");
      }
      const exclude = new Set<string>();
      for (let i = 0; i < keys.value.length; i++) {
        const k = keys.value[i]!;
        if (!isString(k)) {
          return mkError(
            `without() keys must be strings, element ${i} is ${typeName(k)}`,
          );
        }
        exclude.add(k.value);
      }
      const result = new Map<string, Value>();
      for (const [k, v] of recv.value) {
        if (!exclude.has(k)) result.set(k, v);
      }
      return mkObject(result);
    }),
  );

  // --- assign (alias for merge, may accept multiple objects) ---
  interp.registerMethod(
    "assign",
    m((_i, recv, args) => {
      if (!isObject(recv)) {
        return mkError(`assign() requires object, got ${typeName(recv)}`);
      }
      const result = new Map<string, Value>(recv.value);
      for (const arg of args) {
        if (!isObject(arg)) {
          return mkError("assign() arguments must be objects");
        }
        for (const [k, v] of arg.value) {
          result.set(k, v);
        }
      }
      return mkObject(result);
    }),
  );
}
