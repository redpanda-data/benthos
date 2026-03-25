// Binary arithmetic and comparison operations for the Bloblang V2 interpreter.

import { TokenType } from "./token.js";
import {
  type Value,
  mkInt64,
  mkFloat64,
  mkString,
  mkBool,
  mkError,
  mkBytes,
  mkInt32,
  mkUint32,
  mkUint64,
  mkFloat32,
  NULL,
  TRUE,
  FALSE,
  isError,
  isNull,
  isString,
  isBytes,
  isNumeric,
  isFloat32,
  isFloat64,
  isInt32,
  isInt64,
  isUint32,
  isUint64,
  isTimestamp,
  typeName,
  valuesEqual,
  promoteWithError,
  MAX_INT64,
  MIN_INT64,
  MAX_INT32,
  MIN_INT32,
  MAX_UINT32,
  MAX_UINT64,
  MAX_SAFE_FLOAT64,
} from "./value.js";

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function evalBinaryOp(op: TokenType, left: Value, right: Value): Value {
  // Timestamp subtraction: ts - ts = int64 nanoseconds.
  if (op === TokenType.MINUS) {
    if (isTimestamp(left) && isTimestamp(right)) {
      const diff = left.value - right.value;
      if (diff > MAX_INT64 || diff < MIN_INT64) {
        return mkError(
          "timestamp subtraction overflow: difference exceeds int64 nanosecond range (~292 years)",
        );
      }
      return mkInt64(diff);
    }
    if (isTimestamp(left)) {
      return mkError("cannot subtract timestamp and " + typeName(right));
    }
  }

  // Timestamp comparison.
  if (isTimestamp(left)) {
    if (isTimestamp(right)) {
      switch (op) {
        case TokenType.GT:
          return mkBool(left.value > right.value);
        case TokenType.GE:
          return mkBool(left.value >= right.value);
        case TokenType.LT:
          return mkBool(left.value < right.value);
        case TokenType.LE:
          return mkBool(left.value <= right.value);
        case TokenType.EQ:
          return mkBool(left.value === right.value);
        case TokenType.NE:
          return mkBool(left.value !== right.value);
        default:
          return mkError(
            "cannot " +
              opVerb(op) +
              " timestamp and timestamp",
          );
      }
    }
    if (op === TokenType.EQ || op === TokenType.NE) {
      return mkBool(op === TokenType.NE); // cross-family
    }
    return mkError(
      "cannot " + opVerb(op) + " timestamp and " + typeName(right),
    );
  }
  if (isTimestamp(right)) {
    if (op === TokenType.EQ || op === TokenType.NE) {
      return mkBool(op === TokenType.NE); // cross-family
    }
    return mkError(
      "cannot " + opVerb(op) + " " + typeName(left) + " and timestamp",
    );
  }

  switch (op) {
    case TokenType.PLUS:
      return evalAdd(left, right);
    case TokenType.MINUS:
      return evalArith(left, right, "-");
    case TokenType.STAR:
      return evalArith(left, right, "*");
    case TokenType.SLASH:
      return evalDivide(left, right);
    case TokenType.PERCENT:
      return evalModulo(left, right);
    case TokenType.EQ:
      return mkBool(valuesEqual(left, right));
    case TokenType.NE:
      return mkBool(!valuesEqual(left, right));
    case TokenType.GT:
      return evalCompare(left, right, ">");
    case TokenType.GE:
      return evalCompare(left, right, ">=");
    case TokenType.LT:
      return evalCompare(left, right, "<");
    case TokenType.LE:
      return evalCompare(left, right, "<=");
    default:
      return mkError(`unknown binary operator ${op}`);
  }
}

export function numericNegate(v: Value): Value {
  switch (v.tag) {
    case "int64":
      if (v.value === MIN_INT64) return mkError("int64 overflow");
      return mkInt64(-v.value);
    case "int32":
      if (v.value === MIN_INT32) return mkError("int32 overflow");
      return mkInt32(-v.value);
    case "float64":
      return mkFloat64(-v.value);
    case "float32":
      return mkFloat32(-v.value);
    case "uint32":
      return mkInt64(-BigInt(v.value));
    case "uint64":
      if (v.value > MAX_INT64) {
        return mkError(
          "cannot negate uint64 value exceeding int64 range",
        );
      }
      return mkInt64(-v.value);
    default:
      return mkError(`cannot negate ${typeName(v)}`);
  }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

function evalAdd(left: Value, right: Value): Value {
  // String concatenation.
  if (isString(left)) {
    if (!isString(right)) {
      return mkError(
        "cannot add string and " + typeName(right) + ": not numeric",
      );
    }
    return mkString(left.value + right.value);
  }
  if (isString(right)) {
    return mkError(
      "cannot add " + typeName(left) + " and string: not numeric",
    );
  }
  // Bytes concatenation.
  if (isBytes(left)) {
    if (!isBytes(right)) {
      return mkError("cannot add bytes and " + typeName(right));
    }
    const result = new Uint8Array(left.value.length + right.value.length);
    result.set(left.value);
    result.set(right.value, left.value.length);
    return mkBytes(result);
  }
  // Numeric addition.
  return evalArith(left, right, "+");
}

function evalArith(left: Value, right: Value, op: string): Value {
  if (!isNumeric(left) || !isNumeric(right)) {
    return arithError(left, right, op);
  }

  const result = promoteWithError(left, right);
  if ("error" in result) return mkError(result.error);
  const [pl, pr, kind] = result.promoted;

  switch (kind) {
    case "int64":
      return checkedInt64Arith(
        (pl as { tag: "int64"; value: bigint }).value,
        (pr as { tag: "int64"; value: bigint }).value,
        op,
      );
    case "int32":
      return checkedInt32Arith(
        (pl as { tag: "int32"; value: number }).value,
        (pr as { tag: "int32"; value: number }).value,
        op,
      );
    case "uint32":
      return checkedUint32Arith(
        (pl as { tag: "uint32"; value: number }).value,
        (pr as { tag: "uint32"; value: number }).value,
        op,
      );
    case "uint64":
      return checkedUint64Arith(
        (pl as { tag: "uint64"; value: bigint }).value,
        (pr as { tag: "uint64"; value: bigint }).value,
        op,
      );
    case "float64":
      return floatArith(
        (pl as { tag: "float64"; value: number }).value,
        (pr as { tag: "float64"; value: number }).value,
        op,
      );
    case "float32":
      return float32Arith(
        (pl as { tag: "float32"; value: number }).value,
        (pr as { tag: "float32"; value: number }).value,
        op,
      );
    default:
      return mkError("unexpected promotion result");
  }
}

function evalDivide(left: Value, right: Value): Value {
  if (!isNumeric(left) || !isNumeric(right)) {
    return mkError(
      `cannot divide ${typeName(left)} by ${typeName(right)}`,
    );
  }

  // Division always produces float.
  // float32 / float32 → float32, all else → float64.
  if (isFloat32(left) && isFloat32(right)) {
    if (right.value === 0) return mkError("division by zero");
    return mkFloat32(left.value / right.value);
  }

  const af = checkedToFloat64(left);
  const bf = checkedToFloat64(right);
  if (af === null || bf === null) {
    return mkError(
      "integer exceeds float64 exact range (magnitude > 2^53)",
    );
  }
  if (bf === 0) return mkError("division by zero");
  return mkFloat64(af / bf);
}

function evalModulo(left: Value, right: Value): Value {
  if (!isNumeric(left) || !isNumeric(right)) {
    return mkError(
      `cannot modulo ${typeName(left)} by ${typeName(right)}`,
    );
  }

  const result = promoteWithError(left, right);
  if ("error" in result) return mkError(result.error);
  const [pl, pr, kind] = result.promoted;

  switch (kind) {
    case "int64": {
      const a = (pl as { tag: "int64"; value: bigint }).value;
      const b = (pr as { tag: "int64"; value: bigint }).value;
      if (b === 0n) return mkError("modulo by zero");
      return mkInt64(a % b);
    }
    case "int32": {
      const a = (pl as { tag: "int32"; value: number }).value;
      const b = (pr as { tag: "int32"; value: number }).value;
      if (b === 0) return mkError("modulo by zero");
      return mkInt32(a % b);
    }
    case "uint32": {
      const a = (pl as { tag: "uint32"; value: number }).value;
      const b = (pr as { tag: "uint32"; value: number }).value;
      if (b === 0) return mkError("modulo by zero");
      return mkUint32(a % b);
    }
    case "uint64": {
      const a = (pl as { tag: "uint64"; value: bigint }).value;
      const b = (pr as { tag: "uint64"; value: bigint }).value;
      if (b === 0n) return mkError("modulo by zero");
      return mkUint64(a % b);
    }
    case "float64": {
      const a = (pl as { tag: "float64"; value: number }).value;
      const b = (pr as { tag: "float64"; value: number }).value;
      if (b === 0) return mkError("modulo by zero");
      return mkFloat64(a % b);
    }
    case "float32": {
      const a = (pl as { tag: "float32"; value: number }).value;
      const b = (pr as { tag: "float32"; value: number }).value;
      if (b === 0) return mkError("modulo by zero");
      return mkFloat32(Math.fround(a % b));
    }
    default:
      return mkError("unexpected promotion result");
  }
}

function evalCompare(left: Value, right: Value, op: string): Value {
  if (!isNumeric(left) && !isNumeric(right)) {
    // String comparison.
    if (isString(left)) {
      if (!isString(right)) {
        return mkError(
          `cannot compare string and ${typeName(right)}: not comparable`,
        );
      }
      return stringCompare(left.value, right.value, op);
    }
    // Bytes comparison (lexicographic).
    if (isBytes(left)) {
      if (!isBytes(right)) {
        return mkError(
          `cannot compare bytes and ${typeName(right)}: not comparable`,
        );
      }
      return bytesCompare(left.value, right.value, op);
    }
    return mkError(
      `cannot compare ${typeName(left)} and ${typeName(right)}: not comparable types`,
    );
  }
  if (!isNumeric(left) || !isNumeric(right)) {
    return mkError(
      `cannot compare ${typeName(left)} and ${typeName(right)}: not comparable types`,
    );
  }

  // Promote using checked rules.
  const result = promoteWithError(left, right);
  if ("error" in result) return mkError(result.error);
  const [pl, pr, kind] = result.promoted;

  switch (kind) {
    case "int64":
      return compareOrdered(
        (pl as { tag: "int64"; value: bigint }).value,
        (pr as { tag: "int64"; value: bigint }).value,
        op,
      );
    case "int32":
      return compareOrdered(
        BigInt((pl as { tag: "int32"; value: number }).value),
        BigInt((pr as { tag: "int32"; value: number }).value),
        op,
      );
    case "uint32":
      return compareOrdered(
        BigInt((pl as { tag: "uint32"; value: number }).value),
        BigInt((pr as { tag: "uint32"; value: number }).value),
        op,
      );
    case "uint64":
      return compareOrdered(
        (pl as { tag: "uint64"; value: bigint }).value,
        (pr as { tag: "uint64"; value: bigint }).value,
        op,
      );
    case "float64":
      return compareFloat(
        (pl as { tag: "float64"; value: number }).value,
        (pr as { tag: "float64"; value: number }).value,
        op,
      );
    case "float32":
      return compareFloat(
        (pl as { tag: "float32"; value: number }).value,
        (pr as { tag: "float32"; value: number }).value,
        op,
      );
    default:
      return mkError("unexpected promotion result");
  }
}

// ---------------------------------------------------------------------------
// Checked integer arithmetic
// ---------------------------------------------------------------------------


function checkedInt64Arith(a: bigint, b: bigint, op: string): Value {
  switch (op) {
    case "+": {
      if (
        (b > 0n && a > MAX_INT64 - b) ||
        (b < 0n && a < MIN_INT64 - b)
      ) {
        return mkError("int64 overflow");
      }
      return mkInt64(a + b);
    }
    case "-": {
      if (
        (b < 0n && a > MAX_INT64 + b) ||
        (b > 0n && a < MIN_INT64 + b)
      ) {
        return mkError("int64 overflow");
      }
      return mkInt64(a - b);
    }
    case "*": {
      const result = a * b;
      if (result > MAX_INT64 || result < MIN_INT64) {
        return mkError("int64 overflow");
      }
      return mkInt64(result);
    }
    default:
      return mkError("unsupported int64 operation " + op);
  }
}

function checkedInt32Arith(a: number, b: number, op: string): Value {
  // Promote to int64, check, then narrow.
  const result = checkedInt64Arith(BigInt(a), BigInt(b), op);
  if (isError(result)) return result;
  const r = (result as { tag: "int64"; value: bigint }).value;
  if (r > BigInt(MAX_INT32) || r < BigInt(MIN_INT32)) {
    return mkError("int32 overflow");
  }
  return mkInt32(Number(r));
}

function checkedUint32Arith(a: number, b: number, op: string): Value {
  switch (op) {
    case "+": {
      if (a > MAX_UINT32 - b) return mkError("uint32 overflow");
      return mkUint32(a + b);
    }
    case "-": {
      if (a < b) return mkError("uint32 overflow");
      return mkUint32(a - b);
    }
    case "*": {
      if (a !== 0 && b !== 0) {
        const result = a * b;
        if (Math.trunc(result / a) !== b) {
          return mkError("uint32 overflow");
        }
        return mkUint32(result);
      }
      return mkUint32(a * b);
    }
    default:
      return mkError("unsupported uint32 operation " + op);
  }
}

function checkedUint64Arith(a: bigint, b: bigint, op: string): Value {
  switch (op) {
    case "+": {
      if (a > MAX_UINT64 - b) return mkError("uint64 overflow");
      return mkUint64(a + b);
    }
    case "-": {
      if (a < b) return mkError("uint64 overflow");
      return mkUint64(a - b);
    }
    case "*": {
      const result = a * b;
      if (result > MAX_UINT64) return mkError("uint64 overflow");
      return mkUint64(result);
    }
    default:
      return mkError("unsupported uint64 operation " + op);
  }
}

function floatArith(a: number, b: number, op: string): Value {
  switch (op) {
    case "+":
      return mkFloat64(a + b);
    case "-":
      return mkFloat64(a - b);
    case "*":
      return mkFloat64(a * b);
    default:
      return mkError("unsupported float64 operation " + op);
  }
}

function float32Arith(a: number, b: number, op: string): Value {
  switch (op) {
    case "+":
      return mkFloat32(a + b);
    case "-":
      return mkFloat32(a - b);
    case "*":
      return mkFloat32(a * b);
    default:
      return mkError("unsupported float32 operation " + op);
  }
}

// ---------------------------------------------------------------------------
// Comparison helpers
// ---------------------------------------------------------------------------

function compareOrdered(a: bigint, b: bigint, op: string): Value {
  switch (op) {
    case ">":
      return mkBool(a > b);
    case ">=":
      return mkBool(a >= b);
    case "<":
      return mkBool(a < b);
    case "<=":
      return mkBool(a <= b);
    default:
      return FALSE;
  }
}

function compareFloat(a: number, b: number, op: string): Value {
  switch (op) {
    case ">":
      return mkBool(a > b);
    case ">=":
      return mkBool(a >= b);
    case "<":
      return mkBool(a < b);
    case "<=":
      return mkBool(a <= b);
    default:
      return FALSE;
  }
}

function stringCompare(a: string, b: string, op: string): Value {
  switch (op) {
    case ">":
      return mkBool(a > b);
    case ">=":
      return mkBool(a >= b);
    case "<":
      return mkBool(a < b);
    case "<=":
      return mkBool(a <= b);
    default:
      return FALSE;
  }
}

function bytesCompare(a: Uint8Array, b: Uint8Array, op: string): Value {
  let cmp = 0;
  const len = Math.min(a.length, b.length);
  for (let i = 0; i < len; i++) {
    if (a[i]! < b[i]!) {
      cmp = -1;
      break;
    }
    if (a[i]! > b[i]!) {
      cmp = 1;
      break;
    }
  }
  if (cmp === 0) {
    if (a.length < b.length) cmp = -1;
    else if (a.length > b.length) cmp = 1;
  }
  switch (op) {
    case ">":
      return mkBool(cmp > 0);
    case ">=":
      return mkBool(cmp >= 0);
    case "<":
      return mkBool(cmp < 0);
    case "<=":
      return mkBool(cmp <= 0);
    default:
      return FALSE;
  }
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

function opVerb(op: TokenType | string): string {
  switch (op) {
    case TokenType.PLUS:
    case "+":
      return "add";
    case TokenType.MINUS:
    case "-":
      return "subtract";
    case TokenType.STAR:
    case "*":
      return "multiply";
    case TokenType.GT:
    case TokenType.GE:
    case TokenType.LT:
    case TokenType.LE:
    case ">":
    case ">=":
    case "<":
    case "<=":
      return "compare";
    default:
      return "perform arithmetic on";
  }
}

function arithError(left: Value, right: Value, op: string): Value {
  return mkError(
    `cannot ${opVerb(op)} ${typeName(left)} and ${typeName(right)}: arithmetic requires numeric types`,
  );
}

function checkedToFloat64(v: Value): number | null {
  switch (v.tag) {
    case "int64":
      if (v.value > MAX_SAFE_FLOAT64 || v.value < -MAX_SAFE_FLOAT64) return null;
      return Number(v.value);
    case "int32":
      return v.value;
    case "uint32":
      return v.value;
    case "uint64":
      if (v.value > MAX_SAFE_FLOAT64) return null;
      return Number(v.value);
    case "float64":
      return v.value;
    case "float32":
      return v.value;
    default:
      return null;
  }
}
