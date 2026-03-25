// Tree-walking interpreter for Bloblang V2.

import type {
  Program,
  Stmt,
  Expr,
  ExprBody,
  VarAssign,
  MapDecl,
  CallArg,
  PathSegment,
  Param,
  MatchStmtCase,
  MatchExprCase,
  Assignment,
  IfStmt,
  MatchStmt,
  LiteralExpr,
  BinaryExpr,
  UnaryExpr,
  InputExpr,
  InputMetaExpr,
  OutputExpr,
  OutputMetaExpr,
  VarExpr,
  IdentExpr,
  CallExpr,
  FieldAccessExpr,
  MethodCallExpr,
  IndexExpr,
  IfExpr,
  MatchExpr,
  ArrayLiteral,
  ObjectLiteral,
  LambdaExpr,
  PathExpr,
} from "./ast.js";
import { TokenType } from "./token.js";
import {
  type Value,
  mkInt64,
  mkFloat64,
  mkString,
  mkBool,
  mkArray,
  mkObject,
  mkError,
  NULL,
  VOID,
  DELETED,
  TRUE,
  FALSE,
  isError,
  isVoid,
  isDeleted,
  isNull,
  isBool,
  isString,
  isArray,
  isObject,
  isNumeric,
  isInt64,
  isFloat64,
  isInt32,
  isUint32,
  isUint64,
  isFloat32,
  isTimestamp,
  isBytes,
  typeName,
  deepClone,
  valuesEqual,
} from "./value.js";
import { evalBinaryOp, numericNegate } from "./arithmetic.js";
import { Scope, type ScopeMode } from "./scope.js";

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

export class RuntimeError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "RuntimeError";
  }
}

class RecursionError extends Error {
  constructor() {
    super("maximum recursion depth exceeded");
    this.name = "RecursionError";
  }
}

// ---------------------------------------------------------------------------
// Type interfaces
// ---------------------------------------------------------------------------

export type MethodFunc = (
  interp: Interpreter,
  receiver: Value,
  args: Value[],
) => Value;

export type LambdaMethodFunc = (
  interp: Interpreter,
  receiver: Value,
  args: CallArg[],
) => Value;

export type FunctionFunc = (args: Value[]) => Value;

export interface MethodParam {
  name: string;
  default_: Value | null;
  hasDefault: boolean;
}

export interface MethodSpec {
  fn: MethodFunc | null;
  lambdaFn: LambdaMethodFunc | null;
  intrinsic: boolean;
  params: MethodParam[] | null;
  acceptsNull: boolean;
}

export interface FunctionParam {
  name: string;
  default_: Value | null;
  hasDefault: boolean;
}

export interface FunctionSpec {
  fn: FunctionFunc;
  params: FunctionParam[];
}

// ---------------------------------------------------------------------------
// Maximum recursion depth
// ---------------------------------------------------------------------------

const MAX_RECURSION_DEPTH = 10000;

// ---------------------------------------------------------------------------
// Interpreter
// ---------------------------------------------------------------------------

export class Interpreter {
  prog: Program | null;

  // Runtime state.
  input: Value = NULL;
  inputMeta: Value = mkObject(new Map());
  output: Value = mkObject(new Map());
  outputMeta: Value = mkObject(new Map());
  deleted: boolean = false;

  // Map table: local maps + namespaced imports.
  maps: Map<string, MapDecl> = new Map();
  namespaces: Map<string, Map<string, MapDecl>> = new Map();

  scope: Scope = new Scope(null, "statement");
  depth: number = 0;

  // Methods and functions (pluggable for extensibility).
  methods: Map<string, MethodSpec> = new Map();
  functions: Map<string, FunctionSpec> = new Map();

  constructor(prog: Program | null) {
    this.prog = prog;

    if (prog !== null) {
      // Hoist map declarations.
      for (const m of prog.maps) {
        this.maps.set(m.name, m);
      }

      // Build namespace tables from imports.
      for (const [ns, maps] of prog.namespaces) {
        const table = new Map<string, MapDecl>();
        for (const m of maps) {
          table.set(m.name, m);
        }
        this.namespaces.set(ns, table);
      }
    }
  }

  // --- Registration ---

  registerMethod(name: string, spec: MethodSpec): void {
    this.methods.set(name, spec);
  }

  registerFunction(name: string, spec: FunctionSpec): void {
    this.functions.set(name, spec);
  }

  // --- Public API ---

  /**
   * Exec runs the program against the given input and metadata.
   * Throws RuntimeError on failure.
   */
  exec(
    input: Value,
    metadata: Value,
  ): { output: Value; outputMeta: Value; deleted: boolean } {
    this.input = input;
    this.inputMeta = metadata;
    this.output = mkObject(new Map());
    this.outputMeta = mkObject(new Map());
    this.deleted = false;
    this.scope = new Scope(null, "statement");
    this.depth = 0;

    for (const stmt of this.prog!.stmts) {
      this.execStmt(stmt);
      if (this.deleted) {
        return { output: NULL, outputMeta: mkObject(new Map()), deleted: true };
      }
    }

    return {
      output: this.output,
      outputMeta: this.outputMeta,
      deleted: false,
    };
  }

  /**
   * Run executes the program with error recovery, converting runtime errors
   * to error returns.
   */
  run(
    input: Value,
    metadata: Value,
  ): {
    output: Value;
    outputMeta: Value;
    deleted: boolean;
    error: string | null;
  } {
    try {
      const result = this.exec(input, metadata);
      return { ...result, error: null };
    } catch (e) {
      if (e instanceof RuntimeError) {
        return {
          output: NULL,
          outputMeta: mkObject(new Map()),
          deleted: false,
          error: e.message,
        };
      }
      if (e instanceof RecursionError) {
        return {
          output: NULL,
          outputMeta: mkObject(new Map()),
          deleted: false,
          error: e.message,
        };
      }
      throw e; // re-throw unexpected errors
    }
  }

  // --- Statement execution ---

  private execStmt(stmt: Stmt): void {
    switch (stmt.kind) {
      case "assignment":
        this.execAssignment(stmt);
        break;
      case "if_stmt":
        this.execIfStmt(stmt);
        break;
      case "match_stmt":
        this.execMatchStmt(stmt);
        break;
    }
  }

  private execAssignment(a: Assignment): void {
    const value = this.evalExpr(a.value);

    // Error propagation: if value is an error, it halts the mapping.
    if (isError(value)) {
      throw new RuntimeError(value.message);
    }

    // Void handling.
    if (isVoid(value)) {
      // For variable targets: declaration with void is an error,
      // reassignment with void skips the assignment.
      if (
        a.target.root === "var" &&
        a.target.path.length === 0
      ) {
        if (this.scope.get(a.target.varName) === undefined) {
          throw new RuntimeError(
            "void in variable declaration (use .or() to provide a default)",
          );
        }
      }
      return;
    }

    switch (a.target.root) {
      case "output": {
        if (a.target.metaAccess) {
          // Metadata root assignment.
          if (a.target.path.length === 0) {
            if (isDeleted(value)) {
              throw new RuntimeError("cannot delete metadata object");
            }
            if (!isObject(value)) {
              throw new RuntimeError(
                `metadata must be an object, got ${typeName(value)}`,
              );
            }
            this.outputMeta = deepClone(value);
            return;
          }
          const metaRef: { v: Value } = { v: this.outputMeta };
          this.assignPath(metaRef, a.target.path, value);
          this.outputMeta = metaRef.v;
        } else {
          // Message drop: output = deleted()
          if (a.target.path.length === 0 && isDeleted(value)) {
            this.deleted = true;
            return;
          }
          const outputRef: { v: Value } = { v: this.output };
          this.assignPath(outputRef, a.target.path, value);
          this.output = outputRef.v;
        }
        break;
      }
      case "var": {
        if (isDeleted(value)) {
          if (a.target.path.length === 0) {
            throw new RuntimeError(
              "cannot assign deleted() to a variable",
            );
          }
        }
        if (a.target.path.length === 0) {
          this.scope.set(a.target.varName, deepClone(value));
        } else {
          const existing = this.scope.get(a.target.varName);
          if (existing === undefined) {
            throw new RuntimeError(
              `variable $${a.target.varName} not declared`,
            );
          }
          const clone = deepClone(existing);
          const ref: { v: Value } = { v: clone };
          this.assignPath(ref, a.target.path, value);
          this.scope.set(a.target.varName, ref.v);
        }
        break;
      }
    }
  }

  private execIfStmt(s: IfStmt): void {
    for (const branch of s.branches) {
      const cond = this.evalExpr(branch.cond);
      if (isError(cond)) {
        throw new RuntimeError(cond.message);
      }
      if (!isBool(cond)) {
        throw new RuntimeError(
          `if condition must be boolean, got ${typeName(cond)}`,
        );
      }
      if (cond.value) {
        const childScope = new Scope(this.scope, "statement");
        const saved = this.scope;
        this.scope = childScope;
        for (const stmt of branch.body) {
          this.execStmt(stmt);
          if (this.deleted) {
            this.scope = saved;
            return;
          }
        }
        this.scope = saved;
        return;
      }
    }

    if (s.else_ !== null) {
      const childScope = new Scope(this.scope, "statement");
      const saved = this.scope;
      this.scope = childScope;
      for (const stmt of s.else_) {
        this.execStmt(stmt);
        if (this.deleted) {
          this.scope = saved;
          return;
        }
      }
      this.scope = saved;
    }
  }

  private execMatchStmt(s: MatchStmt): void {
    let subject: Value = NULL;
    if (s.subject !== null) {
      subject = this.evalExpr(s.subject);
      if (isError(subject)) {
        throw new RuntimeError(subject.message);
      }
    }

    for (const c of s.cases) {
      const [matched, errVal] = this.matchCaseMatches(
        c.pattern,
        c.wildcard,
        subject,
        s.binding,
        s.subject !== null,
      );
      if (errVal !== null) {
        throw new RuntimeError(
          isError(errVal) ? errVal.message : "match error",
        );
      }
      if (matched) {
        const childScope = new Scope(this.scope, "statement");
        if (s.binding !== "") {
          childScope.vars.set(s.binding, subject);
        }
        const saved = this.scope;
        this.scope = childScope;
        for (const stmt of c.body) {
          this.execStmt(stmt);
          if (this.deleted) {
            this.scope = saved;
            return;
          }
        }
        this.scope = saved;
        return;
      }
    }
  }

  // --- Expression evaluation ---

  evalExpr(expr: Expr): Value {
    switch (expr.kind) {
      case "literal":
        return this.evalLiteral(expr);
      case "binary":
        return this.evalBinary(expr);
      case "unary":
        return this.evalUnary(expr);
      case "input":
        return this.input;
      case "input_meta":
        return this.inputMeta;
      case "output":
        return deepClone(this.output);
      case "output_meta":
        return deepClone(this.outputMeta);
      case "var": {
        const v = this.scope.get(expr.name);
        if (v === undefined) {
          throw new RuntimeError("undefined variable $" + expr.name);
        }
        return v;
      }
      case "ident":
        return this.evalIdent(expr);
      case "call":
        return this.evalCall(expr);
      case "field_access":
        return this.evalFieldAccess(expr);
      case "method_call":
        return this.evalMethodCall(expr);
      case "index":
        return this.evalIndex(expr);
      case "if_expr":
        return this.evalIfExpr(expr);
      case "match_expr":
        return this.evalMatchExpr(expr);
      case "array":
        return this.evalArrayLiteral(expr);
      case "object":
        return this.evalObjectLiteral(expr);
      case "lambda":
        throw new RuntimeError(
          "lambda expression cannot be used as a value",
        );
      case "path":
        return this.evalPathExpr(expr);
    }
  }

  private evalLiteral(e: LiteralExpr): Value {
    switch (e.tokenType) {
      case TokenType.INT:
        return mkInt64(BigInt(e.value));
      case TokenType.FLOAT:
        return mkFloat64(parseFloat(e.value));
      case TokenType.STRING:
      case TokenType.RAW_STRING:
        return mkString(e.value);
      case TokenType.TRUE:
        return TRUE;
      case TokenType.FALSE:
        return FALSE;
      case TokenType.NULL:
        return NULL;
      default:
        return NULL;
    }
  }

  private evalBinary(e: BinaryExpr): Value {
    const left = this.evalExpr(e.left);
    if (isError(left)) return left;
    if (isVoid(left)) return mkError("void in expression");
    if (isDeleted(left)) return mkError("deleted value in expression");

    // Short-circuit for logical operators.
    if (e.op === TokenType.AND) {
      if (!isBool(left)) {
        return mkError(
          `&& requires boolean operands, got ${typeName(left)}`,
        );
      }
      if (!left.value) return FALSE;
      const right = this.evalExpr(e.right);
      if (isError(right)) return right;
      if (!isBool(right)) {
        return mkError(
          `&& requires boolean operands, got ${typeName(right)}`,
        );
      }
      return right;
    }
    if (e.op === TokenType.OR) {
      if (!isBool(left)) {
        return mkError(
          `|| requires boolean operands, got ${typeName(left)}`,
        );
      }
      if (left.value) return TRUE;
      const right = this.evalExpr(e.right);
      if (isError(right)) return right;
      if (!isBool(right)) {
        return mkError(
          `|| requires boolean operands, got ${typeName(right)}`,
        );
      }
      return right;
    }

    const right = this.evalExpr(e.right);
    if (isError(right)) return right;
    if (isVoid(right)) return mkError("void in expression");
    if (isDeleted(right)) return mkError("deleted value in expression");

    return evalBinaryOp(e.op, left, right);
  }

  private evalUnary(e: UnaryExpr): Value {
    const operand = this.evalExpr(e.operand);
    if (isError(operand)) return operand;
    if (isVoid(operand)) return mkError("void in expression");
    if (isDeleted(operand)) return mkError("deleted value in expression");

    switch (e.op) {
      case TokenType.MINUS:
        return numericNegate(operand);
      case TokenType.BANG: {
        if (!isBool(operand)) {
          return mkError(
            `! requires boolean operand, got ${typeName(operand)}`,
          );
        }
        return mkBool(!operand.value);
      }
      default:
        return mkError(`unknown unary operator ${e.op}`);
    }
  }

  private evalFieldAccess(e: FieldAccessExpr): Value {
    const receiver = this.evalExpr(e.receiver);
    if (isError(receiver)) return receiver;
    if (e.nullSafe && isNull(receiver)) return NULL;
    if (isNull(receiver)) {
      return mkError(`cannot access field "${e.field}" on null`);
    }
    if (!isObject(receiver)) {
      return mkError(
        `cannot access field "${e.field}" on ${typeName(receiver)}`,
      );
    }
    return receiver.value.get(e.field) ?? NULL;
  }

  private evalIndex(e: IndexExpr): Value {
    const receiver = this.evalExpr(e.receiver);
    if (isError(receiver)) return receiver;
    if (e.nullSafe && isNull(receiver)) return NULL;

    const index = this.evalExpr(e.index);
    if (isError(index)) return index;

    return this.indexValue(receiver, index);
  }

  private evalMethodCall(e: MethodCallExpr): Value {
    // Intrinsic: .catch()
    if (e.method === "catch") {
      return this.evalCatch(e);
    }

    // Intrinsic: .or()
    if (e.method === "or") {
      return this.evalOr(e);
    }

    const receiver = this.evalExpr(e.receiver);

    // Error propagation.
    if (isError(receiver)) return receiver;

    // Null-safe.
    if (e.nullSafe && isNull(receiver)) return NULL;

    // Look up the method.
    const spec = this.methods.get(e.method);
    if (spec === undefined) {
      if (isNull(receiver)) {
        return mkError(`.${e.method}() does not support null`);
      }
      return mkError(`unknown method .${e.method}()`);
    }

    // Null check using spec metadata.
    if (isNull(receiver) && !e.nullSafe && !spec.acceptsNull) {
      return mkError(`.${e.method}() does not support null`);
    }

    // Void and deleted in method calls.
    if (isVoid(receiver)) {
      return mkError("cannot call method on void");
    }
    if (isDeleted(receiver)) {
      return mkError("cannot call method on deleted value");
    }

    // Lambda methods: receive unevaluated AST args.
    if (spec.lambdaFn !== null) {
      let args = e.args;
      if (e.named && spec.params !== null) {
        args = reorderNamedCallArgs(args, spec.params);
      }
      return spec.lambdaFn(this, receiver, args);
    }

    // Evaluate arguments, resolving named args to positional if needed.
    let args: Value[];
    if (e.named) {
      const resolved = this.resolveNamedMethodArgs(e);
      if (isError(resolved)) return resolved;
      args = (resolved as { tag: "array"; value: Value[] }).value;
    } else {
      args = this.evalArgs(e.args);
    }
    for (const a of args) {
      if (isError(a)) return a;
    }

    return spec.fn!(this, receiver, args);
  }

  private evalCatch(e: MethodCallExpr): Value {
    const receiver = this.evalExpr(e.receiver);

    // .catch() passes non-errors through unchanged.
    if (!isError(receiver)) return receiver;

    // Error: invoke the catch handler lambda.
    if (e.args.length !== 1) {
      return mkError(".catch() requires exactly one argument");
    }
    const lambdaExpr = e.args[0]!.value;
    if (lambdaExpr.kind !== "lambda") {
      return mkError(".catch() argument must be a lambda");
    }

    // Build the error object: {"what": "error message"}.
    const errObj = mkObject(
      new Map<string, Value>([["what", mkString(receiver.message)]]),
    );

    return this.callLambda(lambdaExpr, [errObj]);
  }

  private evalOr(e: MethodCallExpr): Value {
    const receiver = this.evalExpr(e.receiver);

    // .or() rescues null, void, and deleted.
    if (!isNull(receiver) && !isVoid(receiver) && !isDeleted(receiver)) {
      return receiver;
    }

    // Short-circuit: only evaluate the argument when rescuing.
    if (e.args.length !== 1) {
      return mkError(".or() requires exactly one argument");
    }
    return this.evalExpr(e.args[0]!.value);
  }

  /** Public for stdlib lambda methods that need to call lambdas. */
  callLambda(lambda: LambdaExpr, args: Value[]): Value {
    const lambdaScope = new Scope(this.scope, "expression");
    for (let i = 0; i < lambda.params.length; i++) {
      const p = lambda.params[i]!;
      if (p.discard) continue;
      if (i < args.length) {
        lambdaScope.vars.set(p.name, deepClone(args[i]!));
      } else if (p.default_ !== null) {
        lambdaScope.vars.set(p.name, this.evalExpr(p.default_));
      }
    }

    const saved = this.scope;
    this.scope = lambdaScope;
    const result = this.evalExprBody(lambda.body);
    this.scope = saved;

    return result;
  }

  private evalCall(e: CallExpr): Value {
    // Check for namespace-qualified call.
    if (e.namespace !== "") {
      return this.callNamespaced(e);
    }

    // Check for user-defined map.
    const mapDecl = this.maps.get(e.name);
    if (mapDecl !== undefined) {
      return this.callMap(mapDecl, e);
    }

    // Check stdlib functions.
    const spec = this.functions.get(e.name);
    if (spec !== undefined) {
      let args: Value[];
      if (e.named) {
        const resolved = this.resolveNamedFuncArgs(e, spec);
        if (isError(resolved)) return resolved;
        args = (resolved as { tag: "array"; value: Value[] }).value;
      } else {
        args = this.evalArgs(e.args);
      }
      for (const a of args) {
        if (isError(a)) return a;
      }
      return spec.fn(args);
    }

    return mkError(`unknown function ${e.name}()`);
  }

  private callNamespaced(e: CallExpr): Value {
    const ns = this.namespaces.get(e.namespace);
    if (ns === undefined) {
      return mkError(`unknown namespace "${e.namespace}"`);
    }
    const m = ns.get(e.name);
    if (m === undefined) {
      return mkError(`unknown function ${e.namespace}::${e.name}()`);
    }
    return this.callMap(m, e);
  }

  private callMap(m: MapDecl, e: CallExpr): Value {
    this.depth++;
    if (this.depth > MAX_RECURSION_DEPTH) {
      throw new RecursionError();
    }

    try {
      // Evaluate and bind parameters into an isolated scope.
      const mapScope = new Scope(null, "expression");
      if (e.named) {
        const err = this.bindNamedMapParams(mapScope, m, e);
        if (err !== "") return mkError(err);
      } else {
        const args = this.evalArgs(e.args);
        for (const a of args) {
          if (isError(a)) return a;
        }
        const err = this.bindPositionalParams(mapScope, m.params, args);
        if (err !== "") return mkError(err);
      }

      // Evaluate the map body. If the map has its own namespace context,
      // temporarily switch to it.
      const savedScope = this.scope;
      const savedNamespaces = this.namespaces;
      const savedMaps = this.maps;

      this.scope = mapScope;
      if (m.namespaces !== undefined && m.namespaces.size > 0) {
        const nsTable = new Map<string, Map<string, MapDecl>>();
        for (const [ns, maps] of m.namespaces) {
          const table = new Map<string, MapDecl>();
          for (const md of maps) {
            table.set(md.name, md);
          }
          nsTable.set(ns, table);
        }
        this.namespaces = nsTable;
      }

      const result = this.evalExprBody(m.body);

      this.scope = savedScope;
      this.namespaces = savedNamespaces;
      this.maps = savedMaps;

      return result;
    } finally {
      this.depth--;
    }
  }

  private evalIdent(e: IdentExpr): Value {
    // Qualified reference — only valid in higher-order method args.
    if (e.namespace !== "") {
      return mkError(
        e.namespace +
          "::" +
          e.name +
          " cannot be used as a value (pass to a higher-order method or call with parentheses)",
      );
    }
    // Check scope (parameters, variables).
    const v = this.scope.get(e.name);
    if (v !== undefined) return v;
    // Bare map name without call — error per spec.
    if (this.maps.has(e.name)) {
      return mkError(
        "map " +
          e.name +
          " cannot be used as a value (call it with parentheses)",
      );
    }
    return mkError("undefined identifier " + e.name);
  }

  private evalIfExpr(e: IfExpr): Value {
    for (const branch of e.branches) {
      const cond = this.evalExpr(branch.cond);
      if (isError(cond)) return cond;
      if (!isBool(cond)) {
        return mkError(
          `if condition must be boolean, got ${typeName(cond)}`,
        );
      }
      if (cond.value) {
        const childScope = new Scope(this.scope, "expression");
        const saved = this.scope;
        this.scope = childScope;
        const result = this.evalExprBody(branch.body);
        this.scope = saved;
        return result;
      }
    }

    if (e.else_ !== null) {
      const childScope = new Scope(this.scope, "expression");
      const saved = this.scope;
      this.scope = childScope;
      const result = this.evalExprBody(e.else_);
      this.scope = saved;
      return result;
    }

    return VOID;
  }

  private evalMatchExpr(e: MatchExpr): Value {
    let subject: Value = NULL;
    if (e.subject !== null) {
      subject = this.evalExpr(e.subject);
      if (isError(subject)) return subject;
    }

    for (const c of e.cases) {
      const [matched, errVal] = this.matchCaseMatches(
        c.pattern,
        c.wildcard,
        subject,
        e.binding,
        e.subject !== null,
      );
      if (errVal !== null) return errVal;
      if (matched) {
        const childScope = new Scope(this.scope, "expression");
        if (e.binding !== "") {
          childScope.vars.set(e.binding, subject);
        }
        const saved = this.scope;
        this.scope = childScope;

        let result: Value;
        const body = c.body;
        if ("assignments" in body) {
          // ExprBody
          result = this.evalExprBody(body as ExprBody);
        } else {
          // Expr
          result = this.evalExpr(body as Expr);
        }

        this.scope = saved;
        return result;
      }
    }

    return VOID;
  }

  /**
   * matchCaseMatches returns [matched, errorValue]. If errorValue is non-null,
   * the case expression produced an error that should be propagated.
   */
  private matchCaseMatches(
    pattern: Expr | null,
    wildcard: boolean,
    subject: Value,
    binding: string,
    hasSubject: boolean,
  ): [boolean, Value | null] {
    if (wildcard) return [true, null];

    if (hasSubject && binding === "") {
      // Equality match: compare pattern against subject.
      const patternVal = this.evalExpr(pattern!);
      if (isError(patternVal)) return [false, patternVal];
      // Boolean case values are a runtime error in equality match.
      if (isBool(patternVal)) {
        return [
          false,
          mkError(
            "boolean case value in equality match (use 'as' for boolean conditions)",
          ),
        ];
      }
      return [valuesEqual(subject, patternVal), null];
    }

    // Boolean match (with or without 'as'): case must evaluate to bool.
    if (binding !== "") {
      const childScope = new Scope(this.scope, this.scope.mode);
      childScope.vars.set(binding, subject);
      const saved = this.scope;
      this.scope = childScope;
      const patternVal = this.evalExpr(pattern!);
      this.scope = saved;
      if (isError(patternVal)) return [false, patternVal];
      if (!isBool(patternVal)) {
        return [
          false,
          mkError(
            `boolean match case must evaluate to bool, got ${typeName(patternVal)}`,
          ),
        ];
      }
      return [patternVal.value, null];
    }

    const patternVal = this.evalExpr(pattern!);
    if (isError(patternVal)) return [false, patternVal];
    if (!isBool(patternVal)) {
      return [
        false,
        mkError(
          `boolean match case must evaluate to bool, got ${typeName(patternVal)}`,
        ),
      ];
    }
    return [patternVal.value, null];
  }

  private evalArrayLiteral(e: ArrayLiteral): Value {
    const result: Value[] = [];
    for (const elem of e.elements) {
      const val = this.evalExpr(elem);
      if (isError(val)) return val;
      if (isVoid(val)) {
        return mkError(
          "void in array literal (use deleted() to omit elements, or add an else branch)",
        );
      }
      if (isDeleted(val)) continue; // deleted elements are removed
      result.push(val);
    }
    return mkArray(result);
  }

  private evalObjectLiteral(e: ObjectLiteral): Value {
    const result = new Map<string, Value>();
    for (const entry of e.entries) {
      const key = this.evalExpr(entry.key);
      if (isError(key)) return key;
      if (!isString(key)) {
        return mkError(
          `object key must be string, got ${typeName(key)}`,
        );
      }
      const val = this.evalExpr(entry.value);
      if (isError(val)) return val;
      if (isVoid(val)) {
        return mkError(
          "void in object literal (use deleted() to omit fields, or add an else branch)",
        );
      }
      if (isDeleted(val)) continue; // deleted fields are removed
      result.set(key.value, val);
    }
    return mkObject(result);
  }

  evalExprBody(body: ExprBody): Value {
    for (const va of body.assignments) {
      const val = this.evalExpr(va.value);
      if (isError(val)) return val;
      if (isVoid(val)) {
        // Void in variable declaration is an error.
        // Void in reassignment (variable exists in any reachable scope) skips.
        if (this.scope.get(va.name) !== undefined) {
          continue;
        }
        return mkError(
          "void in variable declaration (use .or() to provide a default)",
        );
      }
      if (isDeleted(val)) {
        if (va.path.length === 0) {
          return mkError("cannot assign deleted() to a variable");
        }
      }
      if (va.path.length === 0) {
        this.scope.set(va.name, deepClone(val));
      } else {
        const existing = this.scope.get(va.name);
        if (existing === undefined) {
          return mkError(`variable $${va.name} not declared`);
        }
        const clone = deepClone(existing);
        const ref: { v: Value } = { v: clone };
        this.assignPath(ref, va.path, val);
        this.scope.set(va.name, ref.v);
      }
    }
    return this.evalExpr(body.result);
  }

  private evalPathExpr(e: PathExpr): Value {
    let root: Value;
    switch (e.root) {
      case "input":
        root = this.input;
        break;
      case "input_meta":
        root = this.inputMeta;
        break;
      case "output":
        root = deepClone(this.output);
        break;
      case "output_meta":
        root = deepClone(this.outputMeta);
        break;
      case "var": {
        const v = this.scope.get(e.varName);
        if (v === undefined) {
          return mkError("undefined variable $" + e.varName);
        }
        root = v;
        break;
      }
    }

    let current: Value = root;
    for (const seg of e.segments) {
      if (isError(current)) return current;

      switch (seg.segKind) {
        case "field": {
          if (seg.nullSafe && isNull(current)) {
            return NULL;
          }
          if (!isObject(current)) {
            return mkError(
              `cannot access field "${seg.name}" on ${typeName(current)}`,
            );
          }
          current = current.value.get(seg.name) ?? NULL;
          break;
        }
        case "index": {
          if (seg.nullSafe && isNull(current)) {
            return NULL;
          }
          const idx = this.evalExpr(seg.index!);
          if (isError(idx)) return idx;
          current = this.indexValue(current, idx);
          if (isError(current)) return current;
          break;
        }
        case "method": {
          if (seg.nullSafe && isNull(current)) {
            return NULL;
          }
          const spec = this.methods.get(seg.name);
          if (spec === undefined) {
            return mkError(`unknown method .${seg.name}()`);
          }
          // Intrinsic methods cannot appear in path expressions.
          if (spec.intrinsic) {
            return mkError(
              `.${seg.name}() cannot be used in path expressions`,
            );
          }
          if (
            isNull(current) &&
            !seg.nullSafe &&
            !spec.acceptsNull
          ) {
            return mkError(`.${seg.name}() does not support null`);
          }
          if (isVoid(current)) {
            return mkError("cannot call method on void");
          }
          if (isDeleted(current)) {
            return mkError("cannot call method on deleted value");
          }
          if (spec.lambdaFn !== null) {
            let lambdaArgs = seg.args;
            if (seg.named && spec.params !== null) {
              lambdaArgs = reorderNamedCallArgs(
                lambdaArgs,
                spec.params,
              );
            }
            current = spec.lambdaFn(this, current, lambdaArgs);
          } else {
            const args = this.evalArgs(seg.args);
            for (const a of args) {
              if (isError(a)) return a;
            }
            current = spec.fn!(this, current, args);
          }
          break;
        }
      }
    }
    return current;
  }

  // --- Helpers ---

  private resolveNamedArgs(
    callArgs: CallArg[],
    params: { name: string; default_: Value | null; hasDefault: boolean }[],
    context: string,
  ): Value {
    if (params.length === 0) {
      // No parameter metadata — evaluate named args by name order.
      const args: Value[] = [];
      for (const arg of callArgs) {
        const v = this.evalExpr(arg.value);
        if (isError(v)) return v;
        args.push(v);
      }
      return mkArray(args);
    }

    // Build named arg map.
    const named = new Map<string, Value>();
    for (const arg of callArgs) {
      const v = this.evalExpr(arg.value);
      if (isError(v)) return v;
      named.set(arg.name, v);
    }

    // Map to positional based on parameter metadata.
    const args: Value[] = new Array(params.length);
    for (let i = 0; i < params.length; i++) {
      const p = params[i]!;
      const v = named.get(p.name);
      if (v !== undefined) {
        args[i] = v;
      } else if (p.hasDefault) {
        args[i] = p.default_ ?? NULL;
      } else {
        return mkError(
          `${context}: missing required argument "${p.name}"`,
        );
      }
    }
    return mkArray(args);
  }

  private resolveNamedMethodArgs(e: MethodCallExpr): Value {
    const spec = this.methods.get(e.method);
    let params: {
      name: string;
      default_: Value | null;
      hasDefault: boolean;
    }[] = [];
    if (spec !== undefined && spec.params !== null) {
      params = spec.params;
    }
    return this.resolveNamedArgs(e.args, params, "." + e.method + "()");
  }

  private resolveNamedFuncArgs(e: CallExpr, spec: FunctionSpec): Value {
    const params = spec.params.map((p) => ({
      name: p.name,
      default_: p.default_,
      hasDefault: p.hasDefault,
    }));
    const resolved = this.resolveNamedArgs(e.args, params, e.name + "()");
    if (isError(resolved)) return resolved;
    const args = (resolved as { tag: "array"; value: Value[] }).value;

    // Truncate trailing default-filled args.
    const provided = new Set<string>();
    for (const arg of e.args) {
      provided.add(arg.name);
    }
    let lastExplicit = -1;
    for (let i = 0; i < spec.params.length; i++) {
      if (provided.has(spec.params[i]!.name)) {
        lastExplicit = i;
      }
    }
    if (lastExplicit >= 0 && lastExplicit < args.length - 1) {
      return mkArray(args.slice(0, lastExplicit + 1));
    }
    return mkArray(args);
  }

  evalArgs(args: CallArg[]): Value[] {
    const result: Value[] = new Array(args.length);
    for (let i = 0; i < args.length; i++) {
      const v = this.evalExpr(args[i]!.value);
      if (isVoid(v)) {
        result[i] = mkError(
          "void passed as argument (use .or() to provide a default)",
        );
      } else if (isDeleted(v)) {
        result[i] = mkError("deleted() passed as argument");
      } else {
        result[i] = v;
      }
    }
    return result;
  }

  private bindPositionalParams(
    s: Scope,
    params: Param[],
    args: Value[],
  ): string {
    let argIdx = 0;
    for (const p of params) {
      if (p.discard) {
        if (argIdx < args.length) argIdx++;
        continue;
      }
      if (argIdx < args.length) {
        s.vars.set(p.name, deepClone(args[argIdx]!));
        argIdx++;
      } else if (p.default_ !== null) {
        s.vars.set(p.name, this.evalExpr(p.default_));
      } else {
        return `missing argument for parameter "${p.name}"`;
      }
    }
    return "";
  }

  private bindNamedMapParams(
    s: Scope,
    m: MapDecl,
    e: CallExpr,
  ): string {
    // Build namedArgParam descriptors, evaluating AST defaults.
    const params: {
      name: string;
      default_: Value | null;
      hasDefault: boolean;
    }[] = [];
    for (const p of m.params) {
      if (p.discard) continue;
      const nap: {
        name: string;
        default_: Value | null;
        hasDefault: boolean;
      } = { name: p.name, default_: null, hasDefault: false };
      if (p.default_ !== null) {
        nap.hasDefault = true;
        nap.default_ = this.evalExpr(p.default_);
      }
      params.push(nap);
    }

    const resolved = this.resolveNamedArgs(e.args, params, e.name + "()");
    if (isError(resolved)) return resolved.message;
    const args = (resolved as { tag: "array"; value: Value[] }).value;

    // Bind into scope.
    for (let i = 0; i < params.length; i++) {
      if (i < args.length) {
        s.vars.set(params[i]!.name, deepClone(args[i]!));
      }
    }
    return "";
  }

  // --- Path assignment ---

  private assignPath(
    root: { v: Value },
    path: PathSegment[],
    value: Value,
  ): void {
    if (path.length === 0) {
      root.v = deepClone(value);
      return;
    }
    this.assignPathRecursive(root, path, 0, value);
  }

  private assignPathRecursive(
    current: { v: Value },
    path: PathSegment[],
    pathIdx: number,
    value: Value,
  ): void {
    const seg = path[pathIdx]!;
    const isLast = pathIdx === path.length - 1;

    switch (seg.segKind) {
      case "field": {
        // Ensure current is an object. Auto-create only from null.
        let obj: Map<string, Value>;
        if (isObject(current.v)) {
          obj = current.v.value;
        } else if (isNull(current.v)) {
          obj = new Map();
          current.v = mkObject(obj);
        } else {
          throw new RuntimeError(
            `cannot access field "${seg.name}" on ${typeName(current.v)} (expected object)`,
          );
        }

        if (isLast) {
          if (isDeleted(value)) {
            obj.delete(seg.name);
          } else {
            obj.set(seg.name, value);
          }
          return;
        }

        let child = obj.get(seg.name);
        if (child === undefined) {
          child = NULL;
        }
        const childRef: { v: Value } = { v: child };
        this.assignPathRecursive(childRef, path, pathIdx + 1, value);
        obj.set(seg.name, childRef.v);
        break;
      }
      case "index": {
        const idx = this.evalExpr(seg.index!);
        if (isError(idx)) return;

        // String index → object field.
        if (isString(idx)) {
          let obj: Map<string, Value>;
          if (isObject(current.v)) {
            obj = current.v.value;
          } else {
            obj = new Map();
            current.v = mkObject(obj);
          }
          if (isLast) {
            if (isDeleted(value)) {
              obj.delete(idx.value);
            } else {
              obj.set(idx.value, value);
            }
            return;
          }
          let child = obj.get(idx.value);
          if (child === undefined) {
            child = mkObject(new Map());
          }
          const childRef: { v: Value } = { v: child };
          this.assignPathRecursive(childRef, path, pathIdx + 1, value);
          obj.set(idx.value, childRef.v);
          return;
        }

        // Integer index → array element.
        const i64 = valueToInt64(idx);
        if (i64 === null) return;

        let arr: Value[];
        if (isArray(current.v)) {
          arr = current.v.value;
        } else if (isNull(current.v)) {
          arr = [];
          current.v = mkArray(arr);
        } else {
          throw new RuntimeError(
            `cannot index into ${typeName(current.v)} (expected array)`,
          );
        }

        let i = Number(i64);
        // Handle negative indexing.
        if (i < 0) {
          i += arr.length;
        }

        if (isLast && isDeleted(value)) {
          // Delete array element: remove and shift.
          if (i < 0 || i >= arr.length) {
            throw new RuntimeError(
              "array index deletion: index out of bounds",
            );
          }
          arr.splice(i, 1);
          return;
        }

        // Grow array with null gaps if needed.
        while (arr.length <= i) {
          arr.push(NULL);
        }

        if (isLast) {
          arr[i] = value;
          return;
        }

        let child = arr[i]!;
        if (isNull(child)) {
          child = mkObject(new Map());
        }
        const childRef: { v: Value } = { v: child };
        this.assignPathRecursive(childRef, path, pathIdx + 1, value);
        arr[i] = childRef.v;
        break;
      }
    }
  }

  // --- Indexing ---

  indexValue(receiver: Value, index: Value): Value {
    if (isObject(receiver)) {
      if (!isString(index)) {
        return mkError(
          `non-string index on object: got ${typeName(index)}`,
        );
      }
      return receiver.value.get(index.value) ?? NULL;
    }

    if (isArray(receiver)) {
      return indexSequence(
        index,
        receiver.value.length,
        (i) => receiver.value[i]!,
      );
    }

    if (isString(receiver)) {
      const codepoints = [...receiver.value]; // splits into codepoints
      return indexSequence(index, codepoints.length, (i) =>
        mkInt64(BigInt(codepoints[i]!.codePointAt(0)!)),
      );
    }

    if (isBytes(receiver)) {
      return indexSequence(index, receiver.value.length, (i) =>
        mkInt64(BigInt(receiver.value[i]!)),
      );
    }

    if (isNull(receiver)) {
      return mkError("cannot index null value");
    }

    return mkError(`cannot index ${typeName(receiver)}`);
  }

  // --- Lambda / map-ref extraction (for stdlib) ---

  /**
   * Extract a lambda expression from the first argument, handling both
   * direct lambdas and bare map-name references.
   */
  extractLambdaOrMapRef(args: CallArg[]): LambdaExpr | null {
    if (args.length === 0) return null;

    const firstValue = args[0]!.value;

    // Direct lambda.
    if (firstValue.kind === "lambda") return firstValue;

    // Bare identifier or qualified reference → map name reference.
    if (firstValue.kind === "ident") {
      if (firstValue.namespace !== "") {
        return this.synthesizeNamespacedMapLambda(firstValue);
      }
      const m = this.maps.get(firstValue.name);
      if (m !== undefined) {
        return this.synthesizeMapLambda(
          firstValue.pos,
          firstValue.name,
          "",
          m,
        );
      }
    }

    return null;
  }

  private synthesizeMapLambda(
    pos: { file: string; line: number; column: number },
    name: string,
    namespace: string,
    m: MapDecl,
  ): LambdaExpr | null {
    let required = 0;
    for (const p of m.params) {
      if (p.default_ === null && !p.discard) required++;
    }
    if (required !== 1) return null;
    return {
      kind: "lambda",
      pos,
      params: [{ name: "__arg", default_: null, discard: false, pos }],
      body: {
        assignments: [],
        result: {
          kind: "call",
          pos,
          namespace,
          name,
          args: [
            {
              name: "",
              value: { kind: "ident", pos, namespace: "", name: "__arg" },
            },
          ],
          named: false,
        },
      },
    };
  }

  private synthesizeNamespacedMapLambda(
    ident: IdentExpr,
  ): LambdaExpr | null {
    const ns = this.namespaces.get(ident.namespace);
    if (ns === undefined) return null;
    const m = ns.get(ident.name);
    if (m === undefined) return null;
    return this.synthesizeMapLambda(
      ident.pos,
      ident.name,
      ident.namespace,
      m,
    );
  }
}

// ---------------------------------------------------------------------------
// Module-level helpers
// ---------------------------------------------------------------------------

function reorderNamedCallArgs(
  args: CallArg[],
  params: MethodParam[],
): CallArg[] {
  const byName = new Map<string, CallArg>();
  for (const arg of args) {
    byName.set(arg.name, arg);
  }
  const result: CallArg[] = [];
  for (const p of params) {
    const arg = byName.get(p.name);
    if (arg !== undefined) {
      result.push(arg);
    } else if (!p.hasDefault) {
      // Required param missing — append placeholder.
      result.push({ name: "", value: { kind: "literal", pos: { file: "", line: 0, column: 0 }, tokenType: TokenType.NULL, value: "null" } });
    }
    // Optional param missing: omit.
  }
  return result;
}

/**
 * Convert a Value to a bigint index, or return null if not an integer-like value.
 */
function valueToInt64(v: Value): bigint | null {
  switch (v.tag) {
    case "int64":
      return v.value;
    case "int32":
      return BigInt(v.value);
    case "uint32":
      return BigInt(v.value);
    case "uint64":
      if (v.value > BigInt("9223372036854775807")) return null;
      return BigInt(v.value);
    case "float64": {
      if (!Number.isFinite(v.value) || v.value !== Math.trunc(v.value))
        return null;
      if (
        v.value > Number.MAX_SAFE_INTEGER ||
        v.value < Number.MIN_SAFE_INTEGER
      )
        return null;
      return BigInt(v.value);
    }
    case "float32": {
      const f = v.value;
      if (!Number.isFinite(f) || f !== Math.trunc(f)) return null;
      if (f > Number.MAX_SAFE_INTEGER || f < Number.MIN_SAFE_INTEGER)
        return null;
      return BigInt(f);
    }
    default:
      return null;
  }
}

function indexSequence(
  index: Value,
  length: number,
  get: (i: number) => Value,
): Value {
  const i64 = valueToInt64(index);
  if (i64 === null) {
    // Distinguish non-numeric from non-whole-number float.
    if (isFloat64(index) || isFloat32(index)) {
      const f = index.value;
      if (f !== Math.trunc(f)) {
        return mkError(
          "index must be a whole number, got float with fractional part",
        );
      }
    }
    return mkError(`non-numeric index: got ${typeName(index)}`);
  }
  let idx = Number(i64);
  if (idx < 0) idx += length;
  if (idx < 0 || idx >= length) {
    return mkError("index out of bounds");
  }
  return get(idx);
}
