// Post-parse AST optimizer for Bloblang V2.
// - Path collapse: chains of field/index/method access → PathExpr
// - Constant folding: literal-only expressions evaluated at compile time
// - Dead code elimination: unreachable if/match branches pruned

import { TokenType } from "./token.js";
import { MAX_INT64, MIN_INT64 } from "./value.js";
import type {
  Program,
  Stmt,
  Expr,
  ExprBody,
  LiteralExpr,
  BinaryExpr,
  UnaryExpr,
  IfStmt,
  IfExpr,
  MatchStmt,
  MatchExpr,
  PathExpr,
  PathRoot,
  PathSegment,
  IfExprBranch,
  IfBranch,
} from "./ast.js";

export function optimize(prog: Program): void {
  for (let i = 0; i < prog.stmts.length; i++) {
    prog.stmts[i] = optimizeStmt(prog.stmts[i]!);
  }
  for (const m of prog.maps) {
    optimizeExprBody(m.body);
  }
}

// --- Statement optimization ---

function optimizeStmt(stmt: Stmt): Stmt {
  switch (stmt.kind) {
    case "assignment":
      stmt.value = optimizeExpr(stmt.value);
      return stmt;
    case "if_stmt":
      optimizeIfStmt(stmt);
      return stmt;
    case "match_stmt":
      optimizeMatchStmt(stmt);
      return stmt;
  }
}

function optimizeIfStmt(s: IfStmt): void {
  const kept: IfBranch[] = [];
  for (const branch of s.branches) {
    branch.cond = optimizeExpr(branch.cond);
    if (branch.cond.kind === "literal") {
      if (branch.cond.tokenType === TokenType.TRUE) {
        for (let i = 0; i < branch.body.length; i++) {
          branch.body[i] = optimizeStmt(branch.body[i]!);
        }
        kept.push(branch);
        s.branches = kept;
        s.else_ = null;
        return;
      }
      if (branch.cond.tokenType === TokenType.FALSE) continue;
    }
    for (let i = 0; i < branch.body.length; i++) {
      branch.body[i] = optimizeStmt(branch.body[i]!);
    }
    kept.push(branch);
  }
  s.branches = kept;
  if (s.else_) {
    for (let i = 0; i < s.else_.length; i++) {
      s.else_[i] = optimizeStmt(s.else_[i]!);
    }
  }
}

function optimizeMatchStmt(s: MatchStmt): void {
  if (s.subject) s.subject = optimizeExpr(s.subject);
  for (const c of s.cases) {
    if (c.pattern) c.pattern = optimizeExpr(c.pattern);
    for (let i = 0; i < c.body.length; i++) {
      c.body[i] = optimizeStmt(c.body[i]!);
    }
  }
}

// --- Expression optimization ---

function optimizeExpr(expr: Expr): Expr {
  switch (expr.kind) {
    case "binary": {
      expr.left = optimizeExpr(expr.left);
      expr.right = optimizeExpr(expr.right);
      return foldBinary(expr) ?? expr;
    }
    case "unary": {
      expr.operand = optimizeExpr(expr.operand);
      return foldUnary(expr) ?? expr;
    }
    case "field_access":
      expr.receiver = optimizeExpr(expr.receiver);
      return tryCollapsePath(expr);
    case "index":
      expr.receiver = optimizeExpr(expr.receiver);
      expr.index = optimizeExpr(expr.index);
      return tryCollapsePath(expr);
    case "method_call":
      expr.receiver = optimizeExpr(expr.receiver);
      for (const arg of expr.args) arg.value = optimizeExpr(arg.value);
      return tryCollapsePath(expr);
    case "call":
      for (const arg of expr.args) arg.value = optimizeExpr(arg.value);
      return expr;
    case "array":
      for (let i = 0; i < expr.elements.length; i++) {
        expr.elements[i] = optimizeExpr(expr.elements[i]!);
      }
      return expr;
    case "object":
      for (const entry of expr.entries) {
        entry.key = optimizeExpr(entry.key);
        entry.value = optimizeExpr(entry.value);
      }
      return expr;
    case "if_expr":
      return optimizeIfExpr(expr);
    case "match_expr":
      return optimizeMatchExpr(expr);
    case "lambda":
      optimizeExprBody(expr.body);
      return expr;
    case "path":
      for (const seg of expr.segments) {
        if (seg.index) seg.index = optimizeExpr(seg.index);
        for (const arg of seg.args) arg.value = optimizeExpr(arg.value);
      }
      return expr;
    default:
      return expr;
  }
}

function optimizeExprBody(body: ExprBody): void {
  for (const va of body.assignments) {
    va.value = optimizeExpr(va.value);
  }
  body.result = optimizeExpr(body.result);
}

function optimizeIfExpr(e: IfExpr): Expr {
  const kept: IfExprBranch[] = [];
  for (const branch of e.branches) {
    branch.cond = optimizeExpr(branch.cond);
    if (branch.cond.kind === "literal") {
      if (branch.cond.tokenType === TokenType.TRUE) {
        optimizeExprBody(branch.body);
        kept.push(branch);
        e.branches = kept;
        e.else_ = null;
        return e;
      }
      if (branch.cond.tokenType === TokenType.FALSE) continue;
    }
    optimizeExprBody(branch.body);
    kept.push(branch);
  }
  e.branches = kept;
  if (e.else_) optimizeExprBody(e.else_);
  return e;
}

function optimizeMatchExpr(e: MatchExpr): Expr {
  if (e.subject) e.subject = optimizeExpr(e.subject);
  for (const c of e.cases) {
    if (c.pattern) c.pattern = optimizeExpr(c.pattern);
    if ("kind" in c.body) {
      c.body = optimizeExpr(c.body);
    } else {
      optimizeExprBody(c.body);
    }
  }
  return e;
}

// --- Path collapse ---

function tryCollapsePath(expr: Expr): Expr {
  const segments: PathSegment[] = [];
  let current: Expr = expr;

  for (;;) {
    switch (current.kind) {
      case "field_access":
        segments.push({
          segKind: "field",
          name: current.field,
          index: null,
          args: [],
          named: false,
          nullSafe: current.nullSafe,
          pos: current.fieldPos,
        });
        current = current.receiver;
        continue;
      case "index":
        segments.push({
          segKind: "index",
          name: "",
          index: current.index,
          args: [],
          named: false,
          nullSafe: current.nullSafe,
          pos: current.pos,
        });
        current = current.receiver;
        continue;
      case "method_call":
        // Intrinsic methods (catch, or) require special dispatch in the
        // interpreter (short-circuit evaluation, error interception) and
        // cannot be collapsed into PathExpr segments.
        if (current.method === "catch" || current.method === "or") {
          return expr;
        }
        segments.push({
          segKind: "method",
          name: current.method,
          index: null,
          args: current.args,
          named: current.named,
          nullSafe: current.nullSafe,
          pos: current.methodPos,
        });
        current = current.receiver;
        continue;
      case "input":
      case "input_meta":
      case "output":
      case "output_meta":
      case "var": {
        if (segments.length === 0) return expr;
        segments.reverse();
        return {
          kind: "path",
          pos: current.pos,
          root: current.kind as PathRoot,
          varName: current.kind === "var" ? current.name : "",
          segments,
        } satisfies PathExpr;
      }
      default:
        return expr;
    }
  }
}

// --- Constant folding ---

function foldBinary(e: BinaryExpr): Expr | null {
  if (e.left.kind !== "literal" || e.right.kind !== "literal") return null;
  const left = e.left;
  const right = e.right;
  const pos = left.pos;

  // String concatenation.
  if (e.op === TokenType.PLUS && isStringLit(left) && isStringLit(right)) {
    return lit(pos, TokenType.STRING, left.value + right.value);
  }

  // Integer arithmetic.
  if (left.tokenType === TokenType.INT && right.tokenType === TokenType.INT) {
    try {
      const a = BigInt(left.value);
      const b = BigInt(right.value);
      const result = foldIntOp(a, b, e.op);
      if (result === null) return null;
      return lit(pos, TokenType.INT, result.toString());
    } catch {
      return null;
    }
  }

  // Float arithmetic.
  if (isNumericLit(left) && isNumericLit(right) && (left.tokenType === TokenType.FLOAT || right.tokenType === TokenType.FLOAT)) {
    if (!canSafelyPromoteToFloat(left) || !canSafelyPromoteToFloat(right)) return null;
    const a = parseFloat(left.value);
    const b = parseFloat(right.value);
    if (isNaN(a) || isNaN(b)) return null;
    const result = foldFloatOp(a, b, e.op);
    if (result === null) return null;
    return lit(pos, TokenType.FLOAT, formatFloat(result));
  }

  // Boolean logic.
  if (isBoolLit(left) && isBoolLit(right)) {
    const a = left.tokenType === TokenType.TRUE;
    const b = right.tokenType === TokenType.TRUE;
    let result: boolean;
    switch (e.op) {
      case TokenType.AND: result = a && b; break;
      case TokenType.OR: result = a || b; break;
      case TokenType.EQ: result = a === b; break;
      case TokenType.NE: result = a !== b; break;
      default: return null;
    }
    return boolLit(pos, result);
  }

  // Equality of same-type literals.
  if (e.op === TokenType.EQ || e.op === TokenType.NE) {
    if (left.tokenType === right.tokenType) {
      let eq = left.value === right.value;
      if (e.op === TokenType.NE) eq = !eq;
      return boolLit(pos, eq);
    }
    if (isLiteralCrossType(left, right)) {
      return boolLit(pos, e.op === TokenType.NE);
    }
  }

  return null;
}

function foldUnary(e: UnaryExpr): Expr | null {
  if (e.operand.kind !== "literal") return null;
  const l = e.operand;
  const pos = l.pos;

  switch (e.op) {
    case TokenType.BANG:
      if (l.tokenType === TokenType.TRUE) return boolLit(pos, false);
      if (l.tokenType === TokenType.FALSE) return boolLit(pos, true);
      break;
    case TokenType.MINUS:
      if (l.tokenType === TokenType.INT) {
        const n = BigInt(l.value);
        if (n === -9223372036854775808n) return null; // -MinInt64 overflows
        return lit(pos, TokenType.INT, (-n).toString());
      }
      if (l.tokenType === TokenType.FLOAT) {
        const f = parseFloat(l.value);
        if (isNaN(f)) return null;
        return lit(pos, TokenType.FLOAT, formatFloat(-f));
      }
      break;
  }
  return null;
}

// --- Folding helpers ---

import type { Pos } from "./token.js";

function lit(pos: Pos, tokenType: TokenType, value: string): LiteralExpr {
  return { kind: "literal", pos, tokenType, value };
}

function boolLit(pos: Pos, v: boolean): LiteralExpr {
  return v
    ? { kind: "literal", pos, tokenType: TokenType.TRUE, value: "true" }
    : { kind: "literal", pos, tokenType: TokenType.FALSE, value: "false" };
}

function isStringLit(l: LiteralExpr): boolean {
  return l.tokenType === TokenType.STRING || l.tokenType === TokenType.RAW_STRING;
}

function isNumericLit(l: LiteralExpr): boolean {
  return l.tokenType === TokenType.INT || l.tokenType === TokenType.FLOAT;
}

function isBoolLit(l: LiteralExpr): boolean {
  return l.tokenType === TokenType.TRUE || l.tokenType === TokenType.FALSE;
}

function canSafelyPromoteToFloat(l: LiteralExpr): boolean {
  if (l.tokenType === TokenType.FLOAT) return true;
  if (l.tokenType !== TokenType.INT) return false;
  try {
    const n = BigInt(l.value);
    return n >= -9007199254740992n && n <= 9007199254740992n;
  } catch {
    return false;
  }
}

function isLiteralCrossType(a: LiteralExpr, b: LiteralExpr): boolean {
  const af = literalFamily(a);
  const bf = literalFamily(b);
  return af !== bf && af !== 0 && bf !== 0;
}

function literalFamily(l: LiteralExpr): number {
  switch (l.tokenType) {
    case TokenType.INT:
    case TokenType.FLOAT:
      return 1;
    case TokenType.STRING:
    case TokenType.RAW_STRING:
      return 2;
    case TokenType.TRUE:
    case TokenType.FALSE:
      return 3;
    case TokenType.NULL:
      return 4;
    default:
      return 0;
  }
}


function foldIntOp(a: bigint, b: bigint, op: TokenType): bigint | null {
  let r: bigint;
  switch (op) {
    case TokenType.PLUS:
      r = a + b;
      if (r > MAX_INT64 || r < MIN_INT64) return null;
      return r;
    case TokenType.MINUS:
      r = a - b;
      if (r > MAX_INT64 || r < MIN_INT64) return null;
      return r;
    case TokenType.STAR:
      r = a * b;
      if (r > MAX_INT64 || r < MIN_INT64) return null;
      return r;
    case TokenType.PERCENT:
      if (b === 0n) return null;
      return a % b;
    default:
      return null;
  }
}

function foldFloatOp(a: number, b: number, op: TokenType): number | null {
  switch (op) {
    case TokenType.PLUS:
      return a + b;
    case TokenType.MINUS:
      return a - b;
    case TokenType.STAR:
      return a * b;
    case TokenType.SLASH:
      if (b === 0) return null;
      return a / b;
    case TokenType.PERCENT:
      if (b === 0) return null;
      return a % b;
    default:
      return null;
  }
}

function formatFloat(v: number): string {
  const s = String(v);
  // Ensure it looks like a float (has a dot).
  if (!s.includes(".") && !s.includes("e") && !s.includes("E") && isFinite(v)) {
    return s + ".0";
  }
  return s;
}
