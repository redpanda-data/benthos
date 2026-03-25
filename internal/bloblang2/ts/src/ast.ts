// AST node types for Bloblang V2.

import type { Pos, TokenType } from "./token.js";

// --- Top-level ---

export interface Program {
  stmts: Stmt[];
  maps: MapDecl[];
  imports: ImportStmt[];
  namespaces: Map<string, MapDecl[]>;
}

export interface MapDecl {
  pos: Pos;
  name: string;
  params: Param[];
  body: ExprBody;
  namespaces: Map<string, MapDecl[]>;
}

export interface Param {
  name: string; // empty for discard (_)
  default_: Expr | null;
  discard: boolean;
  pos: Pos;
}

export interface ImportStmt {
  pos: Pos;
  path: string;
  namespace: string;
}

// --- Expression body ---

export interface ExprBody {
  assignments: VarAssign[];
  result: Expr;
}

export interface VarAssign {
  pos: Pos;
  name: string;
  path: PathSegment[];
  value: Expr;
}

// --- Statements ---

export type Stmt = Assignment | IfStmt | MatchStmt;

export interface Assignment {
  kind: "assignment";
  pos: Pos;
  target: AssignTarget;
  value: Expr;
}

export type AssignTargetRoot = "output" | "var";

export interface AssignTarget {
  pos: Pos;
  root: AssignTargetRoot;
  varName: string;
  metaAccess: boolean;
  path: PathSegment[];
}

export interface IfStmt {
  kind: "if_stmt";
  pos: Pos;
  branches: IfBranch[];
  else_: Stmt[] | null;
}

export interface IfBranch {
  cond: Expr;
  body: Stmt[];
}

export interface MatchStmt {
  kind: "match_stmt";
  pos: Pos;
  subject: Expr | null;
  binding: string;
  cases: MatchStmtCase[];
}

export interface MatchStmtCase {
  pattern: Expr | null; // null for wildcard
  wildcard: boolean;
  body: Stmt[];
}

export interface MatchExprCase {
  pattern: Expr | null; // null for wildcard
  wildcard: boolean;
  body: Expr | ExprBody;
}

// --- Expressions ---

export type Expr =
  | LiteralExpr
  | ArrayLiteral
  | ObjectLiteral
  | InputExpr
  | InputMetaExpr
  | OutputExpr
  | OutputMetaExpr
  | VarExpr
  | IdentExpr
  | BinaryExpr
  | UnaryExpr
  | CallExpr
  | MethodCallExpr
  | FieldAccessExpr
  | IndexExpr
  | LambdaExpr
  | IfExpr
  | MatchExpr
  | PathExpr;

export interface LiteralExpr {
  kind: "literal";
  pos: Pos;
  tokenType: TokenType;
  value: string;
}

export interface ArrayLiteral {
  kind: "array";
  pos: Pos;
  elements: Expr[];
}

export interface ObjectLiteral {
  kind: "object";
  pos: Pos;
  entries: ObjectEntry[];
}

export interface ObjectEntry {
  key: Expr;
  value: Expr;
}

export interface InputExpr {
  kind: "input";
  pos: Pos;
}

export interface InputMetaExpr {
  kind: "input_meta";
  pos: Pos;
}

export interface OutputExpr {
  kind: "output";
  pos: Pos;
}

export interface OutputMetaExpr {
  kind: "output_meta";
  pos: Pos;
}

export interface VarExpr {
  kind: "var";
  pos: Pos;
  name: string;
}

export interface IdentExpr {
  kind: "ident";
  pos: Pos;
  namespace: string;
  name: string;
}

export interface BinaryExpr {
  kind: "binary";
  left: Expr;
  op: TokenType;
  opPos: Pos;
  right: Expr;
}

export interface UnaryExpr {
  kind: "unary";
  op: TokenType;
  pos: Pos;
  operand: Expr;
}

export interface CallExpr {
  kind: "call";
  pos: Pos;
  name: string;
  namespace: string;
  args: CallArg[];
  named: boolean;
}

export interface CallArg {
  name: string;
  value: Expr;
}

export interface MethodCallExpr {
  kind: "method_call";
  receiver: Expr;
  method: string;
  methodPos: Pos;
  args: CallArg[];
  named: boolean;
  nullSafe: boolean;
}

export interface FieldAccessExpr {
  kind: "field_access";
  receiver: Expr;
  field: string;
  fieldPos: Pos;
  nullSafe: boolean;
}

export interface IndexExpr {
  kind: "index";
  receiver: Expr;
  index: Expr;
  pos: Pos;
  nullSafe: boolean;
}

export interface LambdaExpr {
  kind: "lambda";
  pos: Pos;
  params: Param[];
  body: ExprBody;
}

export interface IfExpr {
  kind: "if_expr";
  pos: Pos;
  branches: IfExprBranch[];
  else_: ExprBody | null;
}

export interface IfExprBranch {
  cond: Expr;
  body: ExprBody;
}

export interface MatchExpr {
  kind: "match_expr";
  pos: Pos;
  subject: Expr | null;
  binding: string;
  cases: MatchExprCase[];
}

// --- Path expressions (produced by optimizer) ---

export type PathRoot =
  | "input"
  | "input_meta"
  | "output"
  | "output_meta"
  | "var";

export interface PathExpr {
  kind: "path";
  pos: Pos;
  root: PathRoot;
  varName: string;
  segments: PathSegment[];
}

export type PathSegmentKind = "field" | "index" | "method";

export interface PathSegment {
  segKind: PathSegmentKind;
  name: string;
  index: Expr | null;
  args: CallArg[];
  named: boolean;
  nullSafe: boolean;
  pos: Pos;
}

// --- Helpers ---

export function exprPos(e: Expr): Pos {
  switch (e.kind) {
    case "literal":
    case "array":
    case "object":
    case "input":
    case "input_meta":
    case "output":
    case "output_meta":
    case "var":
    case "ident":
    case "call":
    case "lambda":
    case "if_expr":
    case "match_expr":
    case "path":
    case "unary":
    case "index":
      return e.pos;
    case "binary":
      return exprPos(e.left);
    case "method_call":
    case "field_access":
      return exprPos(e.receiver);
  }
}
