// Bloblang V2 — TypeScript implementation.

export { parse } from "./parser.js";
export { optimize } from "./optimizer.js";
export { resolve } from "./resolver.js";
export { Interpreter } from "./interpreter.js";
export type { MethodSpec, FunctionSpec, MethodFunc, LambdaMethodFunc, FunctionFunc } from "./interpreter.js";
export type { FunctionInfo } from "./resolver.js";
export type { Program, Expr, Stmt } from "./ast.js";
export type { PosError, Pos } from "./token.js";
export {
  type Value,
  fromJSON,
  toJSON,
  mkString,
  mkInt64,
  mkFloat64,
  mkArray,
  mkObject,
  mkBool,
  mkError,
  NULL,
  VOID,
  DELETED,
  isError,
  isVoid,
  isDeleted,
  typeName,
  deepClone,
  valuesEqual,
} from "./value.js";
