// Numeric methods: abs, ceil, floor, round, min (scalar), max (scalar).

import type { Interpreter, MethodSpec } from "../interpreter.js";
import {
  type Value,
  mkInt32,
  mkInt64,
  mkFloat32,
  mkFloat64,
  mkError,
  isInt32,
  isInt64,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  typeName,
  MIN_INT64,
  MIN_INT32,
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

function roundFloat(f: number, decimals: bigint): number {
  const shift = Math.pow(10, Number(decimals));
  // Math.round with banker's rounding (round half to even).
  const shifted = f * shift;
  const floored = Math.floor(shifted);
  const diff = shifted - floored;
  let rounded: number;
  if (diff > 0.5) {
    rounded = floored + 1;
  } else if (diff < 0.5) {
    rounded = floored;
  } else {
    // Round to even.
    rounded = floored % 2 === 0 ? floored : floored + 1;
  }
  return rounded / shift;
}

export function registerNumericMethods(interp: Interpreter): void {
  const m = (
    fn: (interp: Interpreter, receiver: Value, args: Value[]) => Value,
  ): MethodSpec => ({
    fn,
    lambdaFn: null,
    intrinsic: false,
    params: null,
    acceptsNull: false,
  });

  // --- abs ---
  interp.registerMethod(
    "abs",
    m((_i, recv) => {
      if (isInt64(recv)) {
        if (recv.value === MIN_INT64) {
          return mkError("int64 overflow in abs()");
        }
        return mkInt64(recv.value < 0n ? -recv.value : recv.value);
      }
      if (isInt32(recv)) {
        if (recv.value === MIN_INT32) {
          return mkError("int32 overflow in abs()");
        }
        return mkInt32(recv.value < 0 ? -recv.value : recv.value);
      }
      if (isFloat64(recv)) return mkFloat64(Math.abs(recv.value));
      if (isFloat32(recv)) return mkFloat32(Math.abs(recv.value));
      if (isUint32(recv)) return recv;
      if (isUint64(recv)) return recv;
      return mkError(`abs() requires numeric, got ${typeName(recv)}`);
    }),
  );

  // --- floor ---
  interp.registerMethod(
    "floor",
    m((_i, recv) => {
      if (isFloat64(recv)) return mkFloat64(Math.floor(recv.value));
      if (isFloat32(recv)) return mkFloat32(Math.floor(recv.value));
      if (isInt32(recv) || isInt64(recv) || isUint32(recv) || isUint64(recv)) return recv;
      return mkError(`floor() requires numeric, got ${typeName(recv)}`);
    }),
  );

  // --- ceil ---
  interp.registerMethod(
    "ceil",
    m((_i, recv) => {
      if (isFloat64(recv)) return mkFloat64(Math.ceil(recv.value));
      if (isFloat32(recv)) return mkFloat32(Math.ceil(recv.value));
      if (isInt32(recv) || isInt64(recv) || isUint32(recv) || isUint64(recv)) return recv;
      return mkError(`ceil() requires numeric, got ${typeName(recv)}`);
    }),
  );

  // --- round ---
  interp.registerMethod(
    "round",
    m((_i, recv, args) => {
      let decimals = 0n;
      if (args.length > 0) {
        const d = toInt64(args[0]!);
        if (d === null) return mkError("round() argument must be integer");
        decimals = d;
      }
      if (isFloat64(recv)) return mkFloat64(roundFloat(recv.value, decimals));
      if (isFloat32(recv)) return mkFloat32(roundFloat(recv.value, decimals));
      if (isInt32(recv) || isInt64(recv) || isUint32(recv) || isUint64(recv)) return recv;
      return mkError(`round() requires numeric, got ${typeName(recv)}`);
    }),
  );
}
