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
  IdentExpr,
} from "./ast.js";

export interface FunctionInfo {
  required: number;
  /** Total params (required + optional). -1 means no arity checking. */
  total: number;
}

export function resolve(
  prog: Program,
  knownMethods: Set<string>,
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
}

class Resolver {
  private prog: Program;
  private knownMethods: Set<string>;
  private knownFunctions: Map<string, FunctionInfo>;
  errors: PosError[] = [];
  private scope!: ResolveScope;
  private inMap = false;
  private inMethodArg = false;

  constructor(
    prog: Program,
    knownMethods: Set<string>,
    knownFunctions: Map<string, FunctionInfo>,
  ) {
    this.prog = prog;
    this.knownMethods = knownMethods;
    this.knownFunctions = knownFunctions;
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
    if (a.value.kind === "lambda") {
      this.error(a.pos, "lambda expressions cannot be stored in a variable or assigned to output");
    }
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
      if (va.value.kind === "lambda") {
        this.error(va.pos, "lambda expressions cannot be stored as values");
      }
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
      case "method_call":
        this.resolveExpr(expr.receiver);
        this.resolveMethodArgs(expr.args);
        break;
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
    } else if (!this.scope.isDeclared(e.name)) {
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
        this.checkFunctionArity(e, this.knownFunctions.get(e.name)!);
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

    for (const arg of e.args) this.resolveExpr(arg.value);
  }

  private resolveMethodArgs(args: { name: string; value: Expr }[]): void {
    const saved = this.inMethodArg;
    this.inMethodArg = true;
    for (const arg of args) {
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
      this.resolveExpr(arg.value);
    }
    this.inMethodArg = saved;
  }

  private resolvePath(expr: {
    kind: "path";
    pos: Pos;
    root: string;
    varName: string;
    segments: { index: Expr | null; args: { name: string; value: Expr }[] }[];
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
      if (seg.args.length > 0) this.resolveMethodArgs(seg.args);
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
