// Bloblang V2 — TypeScript implementation.

export { parse } from "./parser.js";
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
