// Semantic analysis for Bloblang V2.
// Checks variable scoping, map isolation, lambda purity, arity, and naming.

import type { Pos, PosError } from "./token.js";
import { TokenType } from "./token.js";
import type {
  Program,
  Stmt,
  Expr,
  ExprBody,
  MapDecl,
  Param,
  Assignment,
  IfStmt,
  MatchStmt,
  IfExpr,
  MatchExpr,
  CallExpr,
  CallArg,
  IdentExpr,
  PathSegment,
} from "./ast.js";

/**
 * ArgFolder performs parse-time evaluation of a stdlib call's arguments
 * so the runtime can skip repeat work. The folder inspects the AST args
 * (typically checking for string-literal shapes) and returns a same-
 * length array of folded values, using null/undefined for argument
 * positions that aren't eligible for folding. On success the resolver
 * writes each non-null entry onto the matching CallArg.folded field,
 * and the interpreter substitutes the folded value for the arg at
 * runtime.
 *
 * Throwing an error surfaces as a resolver diagnostic anchored at the
 * call site — used e.g. to reject an invalid regex pattern at parse
 * time rather than on first call.
 */
export type ArgFolder = (args: CallArg[]) => Array<unknown | null>;

export interface FunctionInfo {
  required: number;
  /** Total params (required + optional). -1 means no arity checking. */
  total: number;
  /**
   * argFolder, if set, is invoked by the resolver to precompute literal
   * arguments (see ArgFolder docs).
   */
  argFolder?: ArgFolder;
}

export interface MethodInfo {
  required: number;
  /** Total params (required + optional). -1 means no arity checking. */
  total: number;
  /**
   * Per-parameter metadata, parallel to declared positions. Empty when the
   * method doesn't declare params (variadic — e.g. .sort); in that case
   * `acceptsLambda` is the method-level fallback.
   */
  params?: MethodParamInfo[];
  /**
   * Method-level fallback used when `params` is empty. Methods not marked as
   * lambda-accepting (e.g. .or()) reject lambdas at compile time
   * (spec Section 3.4).
   */
  acceptsLambda?: boolean;
  /**
   * argFolder, if set, is invoked by the resolver to precompute literal
   * arguments (see ArgFolder docs).
   */
  argFolder?: ArgFolder;
}

export interface MethodParamInfo {
  name: string;
  hasDefault: boolean;
  acceptsLambda: boolean;
}

/** Reports whether a lambda is accepted at the given argument position. */
export function paramAcceptsLambda(
  mi: MethodInfo,
  position: number,
  name: string,
): boolean {
  const params = mi.params;
  if (!params || params.length === 0) {
    return mi.acceptsLambda === true;
  }
  if (name) {
    for (const p of params) {
      if (p.name === name) return p.acceptsLambda;
    }
    return false;
  }
  if (position < 0 || position >= params.length) return false;
  return params[position]!.acceptsLambda;
}

export function resolve(
  prog: Program,
  knownMethods: Set<string> | Map<string, MethodInfo>,
  knownFunctions: Map<string, FunctionInfo>,
): PosError[] {
  const r = new Resolver(prog, knownMethods, knownFunctions);
  r.resolve();
  return r.errors;
}

class ResolveScope {
  parent: ResolveScope | null;
  vars = new Set<string>();
  params = new Set<string>();

  constructor(parent: ResolveScope | null) {
    this.parent = parent;
  }

  isDeclared(name: string): boolean {
    for (let cur: ResolveScope | null = this; cur; cur = cur.parent) {
      if (cur.vars.has(name) || cur.params.has(name)) return true;
    }
    return false;
  }

  // isParam checks whether a name is declared as a parameter (map param, lambda
  // param, match-as binding) without checking variables. Bare identifiers must
  // not resolve to $variables.
  isParam(name: string): boolean {
    for (let cur: ResolveScope | null = this; cur; cur = cur.parent) {
      if (cur.params.has(name)) return true;
    }
    return false;
  }
}

class Resolver {
  private prog: Program;
  private knownMethods: Set<string> | Map<string, MethodInfo>;
  private knownFunctions: Map<string, FunctionInfo>;
  errors: PosError[] = [];
  private scope!: ResolveScope;
  private inMap = false;
  private inMethodArg = false;

  constructor(
    prog: Program,
    knownMethods: Set<string> | Map<string, MethodInfo>,
    knownFunctions: Map<string, FunctionInfo>,
  ) {
    this.prog = prog;
    this.knownMethods = knownMethods;
    this.knownFunctions = knownFunctions;
  }

  // methodInfo returns arity info for a known method, or null if the
  // registry is the legacy Set<string> (no arity) form.
  private methodInfo(name: string): MethodInfo | null {
    const km = this.knownMethods;
    if (km instanceof Map) {
      return km.get(name) ?? null;
    }
    return null;
  }

  private hasMethod(name: string): boolean {
    const km = this.knownMethods;
    if (km instanceof Map) return km.has(name);
    return km.has(name);
  }

  private mapIndex = new Map<string, MapDecl>();

  resolve(): void {
    // Check duplicate map names and build index.
    const seen = new Map<string, Pos>();
    for (const m of this.prog.maps) {
      const prev = seen.get(m.name);
      if (prev) {
        this.error(m.pos, `duplicate map name "${m.name}" (previously declared at ${prev.line}:${prev.column})`);
      }
      seen.set(m.name, m.pos);
      this.mapIndex.set(m.name, m);
    }

    this.scope = new ResolveScope(null);

    for (const m of this.prog.maps) {
      this.resolveMapDecl(m);
    }
    for (const stmt of this.prog.stmts) {
      this.resolveStmt(stmt);
    }
  }

  private error(pos: Pos, msg: string): void {
    this.errors.push({ pos, msg });
  }

  private resolveMapDecl(m: MapDecl): void {
    this.validateParams(m.params);

    const saved = this.scope;
    const savedInMap = this.inMap;

    this.inMap = true;
    const mapScope = new ResolveScope(null); // isolated
    for (const p of m.params) {
      if (!p.discard) mapScope.params.add(p.name);
    }
    this.scope = mapScope;
    this.resolveExprBody(m.body);

    this.scope = saved;
    this.inMap = savedInMap;
  }

  private validateParams(params: Param[]): void {
    let seenDefault = false;
    for (const p of params) {
      if (p.discard) {
        if (p.default_) this.error(p.pos, "discard parameter _ cannot have a default value");
        continue;
      }
      if (p.default_) {
        seenDefault = true;
      } else if (seenDefault) {
        this.error(p.pos, "required parameter after default parameter");
      }
    }
  }

  private resolveStmt(stmt: Stmt): void {
    switch (stmt.kind) {
      case "assignment":
        this.resolveAssignment(stmt);
        break;
      case "if_stmt":
        this.resolveIfStmt(stmt);
        break;
      case "match_stmt":
        this.resolveMatchStmt(stmt);
        break;
    }
  }

  private resolveAssignment(a: Assignment): void {
    // Lambdas in non-argument positions are caught by resolveExpr's "lambda"
    // case (spec Section 3.4).
    if (a.value.kind === "ident" && a.target.root === "var") {
      const isFn = this.knownFunctions.has(a.value.name);
      if (this.isKnownMap(a.value.name) || isFn) {
        this.error(a.pos, `cannot store ${a.value.name} in a variable (it is not a value)`);
      }
    }

    this.resolveExpr(a.value);

    if (a.target.root === "var" && !this.scope.isDeclared(a.target.varName)) {
      this.scope.vars.add(a.target.varName);
    }
  }

  private resolveIfStmt(s: IfStmt): void {
    for (const branch of s.branches) {
      this.resolveExpr(branch.cond);
      this.withScope(() => {
        for (const stmt of branch.body) this.resolveStmt(stmt);
      });
    }
    if (s.else_) {
      this.withScope(() => {
        for (const stmt of s.else_!) this.resolveStmt(stmt);
      });
    }
  }

  private resolveMatchStmt(s: MatchStmt): void {
    if (s.subject) this.resolveExpr(s.subject);
    for (const c of s.cases) {
      this.withScope(() => {
        if (s.binding) this.scope.params.add(s.binding);
        if (c.pattern && !c.wildcard) this.resolveExpr(c.pattern);
        for (const stmt of c.body) this.resolveStmt(stmt);
      });
    }
  }

  private resolveExprBody(body: ExprBody): void {
    for (const va of body.assignments) {
      // Lambdas in non-argument positions are caught by resolveExpr's
      // "lambda" case (spec Section 3.4).
      this.resolveExpr(va.value);
      if (!this.scope.isDeclared(va.name)) {
        this.scope.vars.add(va.name);
      }
    }
    this.resolveExpr(body.result);
  }

  private resolveExpr(expr: Expr): void {
    switch (expr.kind) {
      case "literal":
        break;
      case "input":
      case "input_meta":
        if (this.inMap) this.error(expr.pos, "cannot access input inside a map body");
        break;
      case "output":
      case "output_meta":
        if (this.inMap) this.error(expr.pos, "cannot access output inside a map body");
        break;
      case "var":
        if (!this.scope.isDeclared(expr.name)) {
          this.error(expr.pos, "undeclared variable $" + expr.name);
        }
        break;
      case "ident":
        this.resolveIdent(expr);
        break;
      case "binary":
        this.resolveExpr(expr.left);
        this.resolveExpr(expr.right);
        break;
      case "unary":
        this.resolveExpr(expr.operand);
        break;
      case "call":
        this.resolveCall(expr);
        break;
      case "method_call": {
        this.resolveExpr(expr.receiver);
        this.checkMethodCallArity(expr);
        const mi = this.methodInfo(expr.method);
        this.applyArgFolder(mi?.argFolder, expr.args, expr.methodPos, `.${expr.method}()`);
        this.resolveMethodArgs(expr.args, mi, expr.method);
        break;
      }
      case "field_access":
        this.resolveExpr(expr.receiver);
        break;
      case "index":
        this.resolveExpr(expr.receiver);
        this.resolveExpr(expr.index);
        break;
      case "array":
        for (const elem of expr.elements) this.resolveExpr(elem);
        break;
      case "object":
        for (const entry of expr.entries) {
          this.resolveExpr(entry.key);
          this.resolveExpr(entry.value);
        }
        break;
      case "if_expr":
        this.resolveIfExpr(expr);
        break;
      case "match_expr":
        this.resolveMatchExpr(expr);
        break;
      case "lambda":
        this.error(expr.pos, "lambda is only valid as a call argument (spec Section 3.4)");
        // Still resolve the body so downstream passes don't see unresolved
        // parameter bindings; the emitted error will surface the problem.
        this.resolveLambda(expr);
        break;
      case "path":
        this.resolvePath(expr);
        break;
    }
  }

  private resolveIdent(e: IdentExpr): void {
    if (e.namespace) {
      if (!this.inMethodArg) {
        this.error(e.pos, `${e.namespace}::${e.name} is not a valid expression (call it with parentheses or pass to a method)`);
      }
      this.resolveQualifiedIdent(e);
    } else if (this.scope.isParam(e.name)) {
      // Resolves to a parameter (map param, lambda param, match-as binding).
      // Bare identifiers must NOT resolve to $variables (those require the $
      // prefix via VarExpr).
    } else {
      const isFn = this.knownFunctions.has(e.name);
      if (this.isKnownMap(e.name) || isFn) {
        if (!this.inMethodArg) {
          this.error(e.pos, `${e.name} is not a valid expression (call it with parentheses or pass to a method)`);
        }
      } else {
        this.error(e.pos, `undeclared identifier "${e.name}"`);
      }
    }
  }

  private resolveQualifiedIdent(e: IdentExpr): void {
    const maps = this.prog.namespaces.get(e.namespace);
    if (!maps) {
      this.error(e.pos, `unknown namespace "${e.namespace}"`);
      return;
    }
    if (!maps.some((m) => m.name === e.name)) {
      this.error(e.pos, `nonexistent map ${e.namespace}::${e.name}`);
    }
  }

  private resolveCall(e: CallExpr): void {
    // Validate named arg consistency.
    if (e.named && e.args.length > 0) {
      const seen = new Set<string>();
      for (const arg of e.args) {
        if (!arg.name) {
          this.error(e.pos, "cannot mix positional and named arguments");
          break;
        }
        if (seen.has(arg.name)) {
          this.error(e.pos, `duplicate named argument "${arg.name}"`);
        }
        seen.add(arg.name);
      }
    }

    if (!e.namespace) {
      const m = this.findMap(e.name);
      if (m) {
        this.checkMapArity(e, m);
      } else if (this.knownFunctions.has(e.name)) {
        const fi = this.knownFunctions.get(e.name)!;
        this.checkFunctionArity(e, fi);
        this.applyArgFolder(fi.argFolder, e.args, e.pos, `${e.name}()`);
      } else {
        this.error(e.pos, `unknown function or map "${e.name}"`);
      }

      if (e.name === "throw" && e.args.length === 1) {
        const arg = e.args[0]!.value;
        if (arg.kind === "literal" && arg.tokenType !== TokenType.STRING && arg.tokenType !== TokenType.RAW_STRING) {
          this.error(e.pos, "throw() requires a string argument");
        }
      }
    }

    if (e.namespace) {
      const maps = this.prog.namespaces.get(e.namespace);
      if (!maps) {
        this.error(e.pos, `unknown namespace "${e.namespace}"`);
      } else if (!maps.some((m) => m.name === e.name)) {
        this.error(e.pos, `nonexistent map ${e.namespace}::${e.name}()`);
      }
    }

    // No function or user map accepts a lambda argument.
    for (const arg of e.args) this.resolveArgValue(arg.value, false, e.name);
  }

  private resolveArgValue(value: Expr, acceptsLambda: boolean, calleeName: string): void {
    if (value.kind === "lambda") {
      if (!acceptsLambda) {
        this.error(value.pos, `${calleeName}() does not accept a lambda argument`);
      }
      this.resolveLambda(value);
      return;
    }
    this.resolveExpr(value);
  }

  private resolveMethodArgs(
    args: CallArg[],
    mi: MethodInfo | null,
    calleeName: string,
  ): void {
    const saved = this.inMethodArg;
    this.inMethodArg = true;
    for (let i = 0; i < args.length; i++) {
      const arg = args[i]!;
      if (arg.value.kind === "ident") {
        const ident = arg.value;
        if (ident.namespace) {
          const m = this.findNamespacedMap(ident.namespace, ident.name);
          if (m) this.checkMapRefArity(ident.pos, `${ident.namespace}::${ident.name}`, m);
        } else {
          const m = this.findMap(ident.name);
          if (m) this.checkMapRefArity(ident.pos, ident.name, m);
        }
      }
      const acceptsLambda = mi === null ? true : paramAcceptsLambda(mi, i, arg.name);
      this.resolveArgValue(arg.value, acceptsLambda, calleeName);
    }
    this.inMethodArg = saved;
  }

  private resolvePath(expr: {
    kind: "path";
    pos: Pos;
    root: string;
    varName: string;
    segments: PathSegment[];
  }): void {
    if (this.inMap) {
      if (expr.root === "input" || expr.root === "input_meta") {
        this.error(expr.pos, "cannot access input inside a map body");
      }
      if (expr.root === "output" || expr.root === "output_meta") {
        this.error(expr.pos, "cannot access output inside a map body");
      }
    }
    if (expr.root === "var" && !this.scope.isDeclared(expr.varName)) {
      this.error(expr.pos, "undeclared variable $" + expr.varName);
    }
    for (const seg of expr.segments) {
      if (seg.index) this.resolveExpr(seg.index);
      if (seg.args.length > 0) {
        const mi = seg.segKind === "method" ? this.methodInfo(seg.name) : null;
        if (mi?.argFolder) {
          this.applyArgFolder(mi.argFolder, seg.args, seg.pos, `.${seg.name}()`);
        }
        this.resolveMethodArgs(seg.args, mi, seg.name);
      }
    }
  }

  /**
   * applyArgFolder runs folder against args and, on success, attaches
   * non-null folded values to the matching CallArg.folded field. A
   * folder throwing an error is recorded as a resolver diagnostic at
   * pos. Silently tolerates folder-returned arrays of the wrong length
   * (contract violation we don't want to block compilation for).
   */
  private applyArgFolder(folder: ArgFolder | undefined, args: CallArg[], pos: Pos, calleeLabel: string): void {
    if (!folder || args.length === 0) return;
    let folded: Array<unknown | null>;
    try {
      folded = folder(args);
    } catch (e) {
      this.error(pos, `${calleeLabel}: ${(e as Error).message}`);
      return;
    }
    if (folded.length !== args.length) return;
    for (let i = 0; i < args.length; i++) {
      if (folded[i] !== null && folded[i] !== undefined) {
        args[i]!.folded = folded[i];
      }
    }
  }

  private resolveIfExpr(e: IfExpr): void {
    for (const branch of e.branches) {
      this.resolveExpr(branch.cond);
      this.withScope(() => this.resolveExprBody(branch.body));
    }
    if (e.else_) {
      this.withScope(() => this.resolveExprBody(e.else_!));
    }
  }

  private resolveMatchExpr(e: MatchExpr): void {
    if (e.subject) this.resolveExpr(e.subject);
    const isEqualityMatch = e.subject !== null && !e.binding;

    for (const c of e.cases) {
      if (c.pattern && !c.wildcard) {
        if (isEqualityMatch && c.pattern.kind === "literal") {
          if (c.pattern.tokenType === TokenType.TRUE || c.pattern.tokenType === TokenType.FALSE) {
            this.error(c.pattern.pos, "boolean literal as case value in equality match (use 'as' for boolean conditions)");
          }
        }
        this.withScope(() => {
          if (e.binding) this.scope.params.add(e.binding);
          this.resolveExpr(c.pattern!);
        });
      }
      this.withScope(() => {
        if (e.binding) this.scope.params.add(e.binding);
        if ("kind" in c.body) {
          this.resolveExpr(c.body);
        } else {
          this.resolveExprBody(c.body);
        }
      });
    }
  }

  private resolveLambda(e: { params: Param[]; body: ExprBody; pos: Pos }): void {
    this.validateParams(e.params);
    this.withScope(() => {
      for (const p of e.params) {
        if (!p.discard) this.scope.params.add(p.name);
      }
      this.resolveExprBody(e.body);
    });
  }

  private withScope(fn: () => void): void {
    const saved = this.scope;
    this.scope = new ResolveScope(this.scope);
    fn();
    this.scope = saved;
  }

  private findMap(name: string): MapDecl | null {
    return this.mapIndex.get(name) ?? null;
  }

  private findNamespacedMap(namespace: string, name: string): MapDecl | null {
    const maps = this.prog.namespaces.get(namespace);
    return maps?.find((m) => m.name === name) ?? null;
  }

  private isKnownMap(name: string): boolean {
    return this.mapIndex.has(name);
  }

  private checkMapArity(e: CallExpr, m: MapDecl): void {
    let required = 0;
    let total = 0;
    let hasDiscard = false;
    for (const p of m.params) {
      total++;
      if (p.discard) {
        hasDiscard = true;
        required++;
      } else if (!p.default_) {
        required++;
      }
    }

    if (e.named && hasDiscard) {
      this.error(e.pos, "cannot use named arguments with discard parameters");
      return;
    }

    if (e.named) {
      const paramNames = new Set(m.params.filter((p) => !p.discard).map((p) => p.name));
      for (const arg of e.args) {
        if (!paramNames.has(arg.name)) {
          this.error(e.pos, `unknown named argument "${arg.name}"`);
        }
      }
      const provided = new Set(e.args.map((a) => a.name));
      for (const p of m.params) {
        if (!p.discard && !provided.has(p.name) && !p.default_) {
          this.error(e.pos, `arity mismatch: missing required named argument "${p.name}"`);
        }
      }
    } else {
      if (e.args.length < required) {
        this.error(e.pos, `arity mismatch: ${e.name}() requires at least ${required} arguments, got ${e.args.length}`);
      }
      if (e.args.length > total) {
        this.error(e.pos, `arity mismatch: ${e.name}() accepts at most ${total} arguments, got ${e.args.length}`);
      }
    }
  }

  private checkFunctionArity(e: CallExpr, fi: FunctionInfo): void {
    if (fi.total < 0) return;
    if (e.args.length < fi.required) {
      this.error(e.pos, `${e.name}() requires at least ${fi.required} arguments, got ${e.args.length}`);
    }
    if (e.args.length > fi.total) {
      this.error(e.pos, `${e.name}() accepts at most ${fi.total} arguments, got ${e.args.length}`);
    }
  }

  private checkMethodCallArity(e: {
    method: string;
    methodPos: Pos;
    args: { name: string; value: Expr }[];
  }): void {
    const info = this.methodInfo(e.method);
    if (!info || info.total < 0) return;
    if (e.args.length < info.required) {
      this.error(
        e.methodPos,
        `.${e.method}() requires at least ${info.required} arguments, got ${e.args.length}`,
      );
    }
    if (e.args.length > info.total) {
      this.error(
        e.methodPos,
        `.${e.method}() accepts at most ${info.total} arguments, got ${e.args.length}`,
      );
    }
  }

  private checkMapRefArity(pos: Pos, displayName: string, m: MapDecl): void {
    let required = 0;
    for (const p of m.params) {
      if (!p.default_ && !p.discard) required++;
    }
    if (required !== 1) {
      this.error(pos, `arity mismatch: ${displayName}() requires ${required} arguments, but higher-order methods pass 1`);
    }
  }
}
