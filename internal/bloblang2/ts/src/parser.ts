// Pratt parser for Bloblang V2.

import { Scanner } from "./scanner.js";
import { type Token, type Pos, type PosError, TokenType, isKeyword } from "./token.js";
import type {
  Program,
  MapDecl,
  Param,
  ImportStmt,
  ExprBody,
  VarAssign,
  Stmt,
  Assignment,
  AssignTarget,
  IfStmt,
  IfBranch,
  MatchStmt,
  MatchStmtCase,
  MatchExprCase,
  Expr,
  CallArg,
  PathSegment,
  IfExprBranch,
} from "./ast.js";

// Binding powers.
const BP_NONE = 0;
const BP_OR = 10;
const BP_AND = 20;
const BP_EQUALITY = 40;
const BP_COMPARISON = 60;
const BP_ADDITIVE = 80;
const BP_MULTIPLY = 100;
const BP_UNARY = 120;
const BP_POSTFIX = 140;

export function parse(
  src: string,
  file: string,
  files: Map<string, string> | null,
): { program: Program; errors: PosError[] } {
  const p = new Parser(files ?? new Map(), new Set([file]), file);
  p.init(src, file);
  const program = p.parseProgram();
  return { program, errors: p.errors };
}

class Parser {
  private s!: Scanner;
  private tok!: Token;
  private files: Map<string, string>;
  private parsing: Set<string>;
  private currentFile: string;
  errors: PosError[] = [];

  constructor(files: Map<string, string>, parsing: Set<string>, currentFile: string) {
    this.files = files;
    this.parsing = parsing;
    this.currentFile = currentFile;
  }

  init(src: string, file: string): void {
    this.s = new Scanner(src, file);
    this.currentFile = file;
    this.advance();
  }

  private advance(): void {
    this.tok = this.s.next();
    if (this.s.errors.length > 0) {
      this.errors.push(...this.s.errors);
      this.s.errors.length = 0;
    }
  }

  private expect(type: TokenType): Token {
    const tok = this.tok;
    if (tok.type !== type) {
      this.error(tok.pos, `expected ${type}, got ${tok.type}`);
      return tok;
    }
    this.advance();
    return tok;
  }

  private at(type: TokenType): boolean {
    return this.tok.type === type;
  }

  private skipNL(): void {
    while (this.tok.type === TokenType.NL) {
      this.advance();
    }
  }

  private error(pos: Pos, msg: string): void {
    this.errors.push({ pos, msg });
  }

  private recover(): void {
    while (this.tok.type !== TokenType.NL && this.tok.type !== TokenType.EOF) {
      this.advance();
    }
  }

  // --- Top-level ---

  parseProgram(): Program {
    const prog: Program = {
      stmts: [],
      maps: [],
      imports: [],
      namespaces: new Map(),
    };

    this.skipNL();
    while (this.tok.type !== TokenType.EOF) {
      switch (this.tok.type) {
        case TokenType.MAP: {
          const m = this.parseMapDecl();
          if (m) prog.maps.push(m);
          break;
        }
        case TokenType.IMPORT: {
          const imp = this.parseImport(prog);
          if (imp) prog.imports.push(imp);
          break;
        }
        default: {
          const stmt = this.parseStatement();
          if (stmt) prog.stmts.push(stmt);
          break;
        }
      }
      if (this.at(TokenType.NL)) {
        this.advance();
        this.skipNL();
      } else if (!this.at(TokenType.EOF)) {
        this.error(this.tok.pos, `expected newline or end of input, got ${this.tok.type}`);
        this.recover();
        this.skipNL();
      }
    }

    return prog;
  }

  private parseMapDecl(): MapDecl | null {
    const pos = this.tok.pos;
    this.advance(); // skip 'map'

    const nameTok = this.expect(TokenType.IDENT);
    this.expect(TokenType.LPAREN);
    const params = this.parseParamList();
    this.expect(TokenType.RPAREN);
    this.expect(TokenType.LBRACE);

    const body = this.parseExprBody();

    this.skipNL();
    this.expect(TokenType.RBRACE);

    return {
      pos,
      name: nameTok.literal,
      params,
      body,
      namespaces: new Map(),
    };
  }

  private parseParamList(): Param[] {
    if (this.at(TokenType.RPAREN)) return [];

    const params: Param[] = [this.parseParam()];
    while (this.at(TokenType.COMMA)) {
      this.advance();
      params.push(this.parseParam());
    }
    return params;
  }

  private parseParam(): Param {
    const pos = this.tok.pos;

    if (this.at(TokenType.UNDERSCORE)) {
      this.advance();
      if (this.at(TokenType.ASSIGN)) {
        this.error(pos, "discard parameter _ cannot have a default value");
        this.advance();
        this.parseLiteral();
      }
      return { name: "", discard: true, default_: null, pos };
    }

    const nameTok = this.expect(TokenType.IDENT);
    const param: Param = { name: nameTok.literal, discard: false, default_: null, pos };

    if (this.at(TokenType.ASSIGN)) {
      this.advance();
      param.default_ = this.parseLiteral();
      if (!this.at(TokenType.COMMA) && !this.at(TokenType.RPAREN)) {
        this.error(this.tok.pos, "default parameter values must be literals, not expressions");
        while (!this.at(TokenType.COMMA) && !this.at(TokenType.RPAREN) && !this.at(TokenType.EOF)) {
          this.advance();
        }
      }
    }

    return param;
  }

  private parseLiteral(): Expr {
    const tok = this.tok;
    switch (tok.type) {
      case TokenType.INT:
      case TokenType.FLOAT:
      case TokenType.STRING:
      case TokenType.RAW_STRING:
      case TokenType.TRUE:
      case TokenType.FALSE:
      case TokenType.NULL:
        this.advance();
        return { kind: "literal", pos: tok.pos, tokenType: tok.type, value: tok.literal };
      default:
        this.error(tok.pos, `expected literal value, got ${tok.type}`);
        return { kind: "literal", pos: tok.pos, tokenType: TokenType.NULL, value: "null" };
    }
  }

  private parseImport(prog: Program): ImportStmt | null {
    const pos = this.tok.pos;
    this.advance(); // skip 'import'

    const pathTok = this.tok;
    if (pathTok.type !== TokenType.STRING && pathTok.type !== TokenType.RAW_STRING) {
      this.error(pathTok.pos, "expected string literal for import path");
      this.recover();
      return null;
    }
    this.advance();

    this.expect(TokenType.AS);
    const nsTok = this.expect(TokenType.IDENT);

    const imp: ImportStmt = { pos, path: pathTok.literal, namespace: nsTok.literal };
    this.resolveImport(prog, imp);
    return imp;
  }

  private resolveImport(prog: Program, imp: ImportStmt): void {
    if (prog.namespaces.has(imp.namespace)) {
      this.error(imp.pos, `duplicate namespace "${imp.namespace}"`);
      return;
    }

    const src = this.files.get(imp.path);
    if (src === undefined) {
      this.error(imp.pos, `import file "${imp.path}" not found`);
      return;
    }

    if (this.parsing.has(imp.path)) {
      this.error(imp.pos, `circular import: "${imp.path}"`);
      return;
    }

    const sub = new Parser(this.files, this.parsing, imp.path);
    this.parsing.add(imp.path);
    sub.init(src, imp.path);
    const importProg = sub.parseProgram();
    this.parsing.delete(imp.path);

    this.errors.push(...sub.errors);

    if (importProg.stmts.length > 0) {
      this.error(imp.pos, `imported file "${imp.path}" contains statements (only map declarations and imports are allowed)`);
    }

    for (const m of importProg.maps) {
      for (const [ns, maps] of importProg.namespaces) {
        m.namespaces.set(ns, maps);
      }
    }
    prog.namespaces.set(imp.namespace, importProg.maps);
  }

  // --- Statement parsing ---

  private parseStatement(): Stmt | null {
    switch (this.tok.type) {
      case TokenType.IF:
        return this.parseIfStmt();
      case TokenType.MATCH:
        return this.parseMatchStmt();
      default:
        return this.parseAssignment();
    }
  }

  private parseAssignment(): Stmt | null {
    const target = this.parseAssignTarget();
    if (!target) {
      this.recover();
      return null;
    }

    this.expect(TokenType.ASSIGN);
    const value = this.parseExpr(BP_NONE);

    return { kind: "assignment", pos: target.pos, target, value } satisfies Assignment;
  }

  private parseAssignTarget(): AssignTarget | null {
    const pos = this.tok.pos;

    switch (this.tok.type) {
      case TokenType.OUTPUT: {
        this.advance();
        let metaAccess = false;
        if (this.at(TokenType.AT)) {
          metaAccess = true;
          this.advance();
        }
        const path = this.parsePathSegments();
        return { pos, root: "output", varName: "", metaAccess, path };
      }
      case TokenType.VAR: {
        const varName = this.tok.literal;
        this.advance();
        const path = this.parsePathSegments();
        return { pos, root: "var", varName, metaAccess: false, path };
      }
      default:
        this.error(pos, `unexpected expression in statement context (expected output or $variable assignment, got ${this.tok.type})`);
        return null;
    }
  }

  private parsePathSegments(): PathSegment[] {
    const segs: PathSegment[] = [];
    for (;;) {
      switch (this.tok.type) {
        case TokenType.DOT:
        case TokenType.QDOT: {
          const nullSafe = this.tok.type === TokenType.QDOT;
          const pos = this.tok.pos;
          this.advance();
          const name = this.expectWord();
          if (this.at(TokenType.LPAREN)) {
            this.advance();
            const { args, named } = this.parseArgList();
            this.expect(TokenType.RPAREN);
            segs.push({ segKind: "method", name, index: null, args, named, nullSafe, pos });
          } else {
            segs.push({ segKind: "field", name, index: null, args: [], named: false, nullSafe, pos });
          }
          break;
        }
        case TokenType.LBRACKET:
        case TokenType.QLBRACKET: {
          const nullSafe = this.tok.type === TokenType.QLBRACKET;
          const pos = this.tok.pos;
          this.advance();
          const idx = this.parseExpr(BP_NONE);
          this.expect(TokenType.RBRACKET);
          segs.push({ segKind: "index", name: "", index: idx, args: [], named: false, nullSafe, pos });
          break;
        }
        default:
          return segs;
      }
    }
  }

  private expectWord(): string {
    const tok = this.tok;
    if (tok.type === TokenType.IDENT || isKeyword(tok.type) || tok.type === TokenType.DELETED || tok.type === TokenType.THROW || tok.type === TokenType.VOID) {
      this.advance();
      return tok.literal;
    }
    if (tok.type === TokenType.STRING) {
      this.advance();
      return tok.literal;
    }
    this.error(tok.pos, `expected field name, got ${tok.type}`);
    return "";
  }

  private parseIfStmt(): IfStmt {
    const pos = this.tok.pos;
    this.advance(); // skip 'if'

    const branches: IfBranch[] = [];
    const cond = this.parseExpr(BP_NONE);
    this.expect(TokenType.LBRACE);
    const body = this.parseStmtBody();
    this.expect(TokenType.RBRACE);
    branches.push({ cond, body });

    let else_: Stmt[] | null = null;
    while (this.at(TokenType.ELSE)) {
      this.advance();
      if (this.at(TokenType.IF)) {
        this.advance();
        const c = this.parseExpr(BP_NONE);
        this.expect(TokenType.LBRACE);
        const b = this.parseStmtBody();
        this.expect(TokenType.RBRACE);
        branches.push({ cond: c, body: b });
      } else {
        this.expect(TokenType.LBRACE);
        else_ = this.parseStmtBody();
        this.expect(TokenType.RBRACE);
        break;
      }
    }

    return { kind: "if_stmt", pos, branches, else_ };
  }

  private parseMatchStmt(): MatchStmt {
    const pos = this.tok.pos;
    this.advance(); // skip 'match'

    let subject: Expr | null = null;
    let binding = "";

    if (!this.at(TokenType.LBRACE)) {
      subject = this.parseExpr(BP_NONE);
      if (this.at(TokenType.AS)) {
        this.advance();
        binding = this.expect(TokenType.IDENT).literal;
      }
    }

    this.expect(TokenType.LBRACE);
    this.skipNL();

    const cases: MatchStmtCase[] = [];
    while (!this.at(TokenType.RBRACE) && !this.at(TokenType.EOF)) {
      cases.push(this.parseMatchCaseStmt());
      if (this.at(TokenType.COMMA)) this.advance();
      this.skipNL();
    }

    this.expect(TokenType.RBRACE);
    return { kind: "match_stmt", pos, subject, binding, cases };
  }

  private parseMatchCaseStmt(): MatchStmtCase {
    let pattern: Expr | null = null;
    let wildcard = false;

    if (this.at(TokenType.UNDERSCORE)) {
      wildcard = true;
      this.advance();
    } else {
      pattern = this.parseExpr(BP_NONE);
    }

    this.expect(TokenType.FATARROW);
    this.expect(TokenType.LBRACE);
    const body = this.parseStmtBody();
    this.expect(TokenType.RBRACE);

    return { pattern, wildcard, body };
  }

  private parseStmtBody(): Stmt[] {
    this.skipNL();
    const stmts: Stmt[] = [];
    while (!this.at(TokenType.RBRACE) && !this.at(TokenType.EOF)) {
      const stmt = this.parseStatement();
      if (stmt) stmts.push(stmt);
      if (this.at(TokenType.NL)) {
        this.advance();
        this.skipNL();
      } else if (!this.at(TokenType.RBRACE) && !this.at(TokenType.EOF)) {
        this.error(this.tok.pos, `expected newline or }, got ${this.tok.type}`);
        this.recover();
        this.skipNL();
      }
    }
    return stmts;
  }

  // --- Expression parsing (Pratt) ---

  private parseExpr(minBP: number): Expr {
    let left = this.parsePrefix();

    for (;;) {
      const { leftBP, rightBP, nonAssoc } = infixBP(this.tok.type);
      if (leftBP === BP_NONE || leftBP < minBP) break;

      switch (this.tok.type) {
        case TokenType.DOT:
        case TokenType.QDOT:
          left = this.parsePostfixDot(left);
          break;
        case TokenType.LBRACKET:
        case TokenType.QLBRACKET:
          left = this.parsePostfixIndex(left);
          break;
        default: {
          const op = this.tok;
          this.advance();
          const right = this.parseExpr(rightBP);

          if (nonAssoc) {
            const next = infixBP(this.tok.type);
            if (next.leftBP === leftBP) {
              this.error(this.tok.pos, `cannot chain non-associative operator ${this.tok.type}`);
            }
          }

          left = { kind: "binary", left, op: op.type, opPos: op.pos, right };
          break;
        }
      }
    }

    return left;
  }

  // --- Prefix / atom parsers ---

  private parsePrefix(): Expr {
    const tok = this.tok;

    switch (tok.type) {
      case TokenType.INT:
      case TokenType.FLOAT:
      case TokenType.STRING:
      case TokenType.RAW_STRING:
      case TokenType.TRUE:
      case TokenType.FALSE:
      case TokenType.NULL:
        this.advance();
        return { kind: "literal", pos: tok.pos, tokenType: tok.type, value: tok.literal };

      case TokenType.MINUS:
        this.advance();
        return { kind: "unary", op: TokenType.MINUS, pos: tok.pos, operand: this.parseExpr(BP_UNARY) };

      case TokenType.BANG:
        this.advance();
        return { kind: "unary", op: TokenType.BANG, pos: tok.pos, operand: this.parseExpr(BP_UNARY) };

      case TokenType.LPAREN:
        return this.parseParenOrLambda();

      case TokenType.LBRACKET:
        return this.parseArrayLiteral();

      case TokenType.LBRACE:
        return this.parseObjectLiteral();

      case TokenType.IF:
        return this.parseIfExpr();

      case TokenType.MATCH:
        return this.parseMatchExpr();

      case TokenType.INPUT:
        this.advance();
        if (this.at(TokenType.AT)) {
          this.advance();
          return { kind: "input_meta", pos: tok.pos };
        }
        return { kind: "input", pos: tok.pos };

      case TokenType.OUTPUT:
        this.advance();
        if (this.at(TokenType.AT)) {
          this.advance();
          return { kind: "output_meta", pos: tok.pos };
        }
        return { kind: "output", pos: tok.pos };

      case TokenType.VAR:
        this.advance();
        if (this.at(TokenType.LPAREN)) {
          this.error(tok.pos, `$${tok.literal} is a variable, not a callable function (use a named map instead)`);
        }
        return { kind: "var", pos: tok.pos, name: tok.literal };

      case TokenType.IDENT:
        return this.parseIdentOrCall();

      case TokenType.DELETED:
      case TokenType.THROW:
      case TokenType.VOID:
        return this.parseReservedCall();

      case TokenType.UNDERSCORE:
        this.advance();
        if (this.at(TokenType.THINARROW)) {
          this.advance();
          const body = this.parseLambdaBody();
          return { kind: "lambda", pos: tok.pos, params: [{ name: "", discard: true, default_: null, pos: tok.pos }], body };
        }
        this.error(tok.pos, "unexpected _ in expression position");
        return { kind: "literal", pos: tok.pos, tokenType: TokenType.NULL, value: "null" };

      default:
        this.error(tok.pos, `expected expression, got ${tok.type}`);
        this.advance();
        return { kind: "literal", pos: tok.pos, tokenType: TokenType.NULL, value: "null" };
    }
  }

  private parseIdentOrCall(): Expr {
    const tok = this.tok;
    this.advance();

    // Qualified: namespace::name or namespace::name(args)
    if (this.at(TokenType.DCOLON)) {
      this.advance();
      const name = this.expect(TokenType.IDENT);
      if (this.at(TokenType.LPAREN)) {
        this.advance();
        const { args, named } = this.parseArgList();
        this.expect(TokenType.RPAREN);
        return { kind: "call", pos: tok.pos, namespace: tok.literal, name: name.literal, args, named };
      }
      return { kind: "ident", pos: tok.pos, namespace: tok.literal, name: name.literal };
    }

    // Function call: name(
    if (this.at(TokenType.LPAREN)) {
      this.advance();
      const { args, named } = this.parseArgList();
      this.expect(TokenType.RPAREN);
      return { kind: "call", pos: tok.pos, namespace: "", name: tok.literal, args, named };
    }

    // Single-param lambda: ident ->
    if (this.at(TokenType.THINARROW)) {
      this.advance();
      const body = this.parseLambdaBody();
      return { kind: "lambda", pos: tok.pos, params: [{ name: tok.literal, discard: false, default_: null, pos: tok.pos }], body };
    }

    // Bare identifier.
    return { kind: "ident", pos: tok.pos, namespace: "", name: tok.literal };
  }

  private parseReservedCall(): Expr {
    const tok = this.tok;
    this.advance();
    this.expect(TokenType.LPAREN);
    const { args, named } = this.parseArgList();
    this.expect(TokenType.RPAREN);
    return { kind: "call", pos: tok.pos, namespace: "", name: tok.literal, args, named };
  }

  private parseParenOrLambda(): Expr {
    const pos = this.tok.pos;
    if (this.isLambdaAhead()) {
      return this.parseMultiParamLambda(pos);
    }
    this.advance(); // skip (
    const expr = this.parseExpr(BP_NONE);
    this.expect(TokenType.RPAREN);
    return expr;
  }

  private isLambdaAhead(): boolean {
    const savedTok = this.tok;
    const savedS = this.s.saveState();

    let depth = 0;
    this.advance(); // skip (
    depth++;
    while (depth > 0 && this.tok.type !== TokenType.EOF) {
      if (this.tok.type === TokenType.LPAREN) depth++;
      else if (this.tok.type === TokenType.RPAREN) depth--;
      if (depth > 0) this.advance();
    }
    this.advance(); // skip )
    const isLambda = this.tok.type === TokenType.THINARROW;

    this.s.restoreState(savedS);
    this.tok = savedTok;
    return isLambda;
  }

  private parseMultiParamLambda(pos: Pos): Expr {
    this.advance(); // skip (
    const params = this.parseParamList();
    this.expect(TokenType.RPAREN);
    this.expect(TokenType.THINARROW);
    const body = this.parseLambdaBody();
    return { kind: "lambda", pos, params, body };
  }

  private parseLambdaBody(): ExprBody {
    if (this.at(TokenType.LBRACE)) {
      if (this.isLambdaBlock()) {
        this.advance(); // skip {
        const body = this.parseExprBody();
        this.skipNL();
        this.expect(TokenType.RBRACE);
        return body;
      }
      const expr = this.parseExpr(BP_NONE);
      return { assignments: [], result: expr };
    }
    const expr = this.parseExpr(BP_NONE);
    return { assignments: [], result: expr };
  }

  private isLambdaBlock(): boolean {
    const savedTok = this.tok;
    const savedS = this.s.saveState();

    this.advance(); // skip {
    while (this.tok.type === TokenType.NL) this.advance();

    let isBlock = false;
    switch (this.tok.type) {
      case TokenType.VAR:
      case TokenType.OUTPUT:
        isBlock = true;
        break;
      case TokenType.IDENT: {
        const savedInner = this.tok;
        const savedInnerS = this.s.saveState();
        this.advance();
        isBlock = this.at(TokenType.ASSIGN);
        this.s.restoreState(savedInnerS);
        this.tok = savedInner;
        break;
      }
    }

    this.s.restoreState(savedS);
    this.tok = savedTok;
    return isBlock;
  }

  private parseArrayLiteral(): Expr {
    const pos = this.tok.pos;
    this.advance(); // skip [

    const elements: Expr[] = [];
    while (!this.at(TokenType.RBRACKET) && !this.at(TokenType.EOF)) {
      elements.push(this.parseExpr(BP_NONE));
      if (!this.at(TokenType.RBRACKET)) {
        this.expect(TokenType.COMMA);
      }
    }
    this.expect(TokenType.RBRACKET);
    return { kind: "array", pos, elements };
  }

  private parseObjectLiteral(): Expr {
    const pos = this.tok.pos;
    this.advance(); // skip {
    this.skipNL();

    const entries: { key: Expr; value: Expr }[] = [];
    while (!this.at(TokenType.RBRACE) && !this.at(TokenType.EOF)) {
      const key = this.parseExpr(BP_NONE);
      this.expect(TokenType.COLON);
      const value = this.parseExpr(BP_NONE);
      entries.push({ key, value });
      if (!this.at(TokenType.RBRACE)) {
        this.expect(TokenType.COMMA);
        this.skipNL();
      }
    }
    this.skipNL();
    this.expect(TokenType.RBRACE);
    return { kind: "object", pos, entries };
  }

  // --- If/match expressions ---

  private parseIfExpr(): Expr {
    const pos = this.tok.pos;
    this.advance(); // skip 'if'

    const branches: IfExprBranch[] = [];
    const cond = this.parseExpr(BP_NONE);
    this.expect(TokenType.LBRACE);
    const body = this.parseExprBody();
    this.skipNL();
    this.expect(TokenType.RBRACE);
    branches.push({ cond, body });

    let else_: ExprBody | null = null;
    while (this.at(TokenType.ELSE)) {
      this.advance();
      if (this.at(TokenType.IF)) {
        this.advance();
        const c = this.parseExpr(BP_NONE);
        this.expect(TokenType.LBRACE);
        const b = this.parseExprBody();
        this.skipNL();
        this.expect(TokenType.RBRACE);
        branches.push({ cond: c, body: b });
      } else {
        this.expect(TokenType.LBRACE);
        else_ = this.parseExprBody();
        this.skipNL();
        this.expect(TokenType.RBRACE);
        break;
      }
    }

    return { kind: "if_expr", pos, branches, else_ };
  }

  private parseMatchExpr(): Expr {
    const pos = this.tok.pos;
    this.advance(); // skip 'match'

    let subject: Expr | null = null;
    let binding = "";

    if (!this.at(TokenType.LBRACE)) {
      subject = this.parseExpr(BP_NONE);
      if (this.at(TokenType.AS)) {
        this.advance();
        binding = this.expect(TokenType.IDENT).literal;
      }
    }

    this.expect(TokenType.LBRACE);
    this.skipNL();

    const cases: MatchExprCase[] = [];
    while (!this.at(TokenType.RBRACE) && !this.at(TokenType.EOF)) {
      cases.push(this.parseMatchCaseExpr());
      if (this.at(TokenType.COMMA)) this.advance();
      this.skipNL();
    }

    this.expect(TokenType.RBRACE);
    return { kind: "match_expr", pos, subject, binding, cases };
  }

  private parseMatchCaseExpr(): MatchExprCase {
    let pattern: Expr | null = null;
    let wildcard = false;

    if (this.at(TokenType.UNDERSCORE)) {
      wildcard = true;
      this.advance();
    } else {
      pattern = this.parseExpr(BP_NONE);
    }

    this.expect(TokenType.FATARROW);

    let body: Expr | ExprBody;
    if (this.at(TokenType.LBRACE)) {
      this.advance();
      body = this.parseExprBody();
      this.skipNL();
      this.expect(TokenType.RBRACE);
    } else {
      body = this.parseExpr(BP_NONE);
    }

    return { pattern, wildcard, body };
  }

  // --- Expression body ---

  private parseExprBody(): ExprBody {
    this.skipNL();
    const assignments: VarAssign[] = [];

    for (;;) {
      // Output assignment in expression context — error.
      if (this.at(TokenType.OUTPUT) && this.isOutputAssignAhead()) {
        this.error(this.tok.pos, "cannot assign to output in expression context (only $variable assignments are allowed)");
        this.recover();
        this.skipNL();
        continue;
      }

      // Bare ident = ... — parameters are read-only.
      if (this.at(TokenType.IDENT)) {
        const savedTok = this.tok;
        const savedS = this.s.saveState();
        this.advance();
        const isAssign = this.tok.type === TokenType.ASSIGN;
        this.s.restoreState(savedS);
        this.tok = savedTok;
        if (isAssign) {
          this.error(this.tok.pos, "cannot assign to identifier (parameters are read-only, use $variable for local assignments)");
          this.recover();
          this.skipNL();
          continue;
        }
      }

      // Var assignment: $var[.path...] = expr
      if (this.at(TokenType.VAR) && this.isVarAssignAhead()) {
        const va = this.parseVarAssign();
        assignments.push(va);
        if (this.at(TokenType.NL)) {
          this.advance();
          this.skipNL();
        }
        continue;
      }
      break;
    }

    const result = this.parseExpr(BP_NONE);
    return { assignments, result };
  }

  private isOutputAssignAhead(): boolean {
    const savedTok = this.tok;
    const savedS = this.s.saveState();

    this.advance(); // skip OUTPUT
    if (this.at(TokenType.AT)) this.advance();
    while (this.at(TokenType.DOT) || this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET) || this.at(TokenType.QDOT)) {
      if (this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET)) {
        let depth = 1;
        this.advance();
        while (depth > 0 && !this.at(TokenType.EOF)) {
          if (this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET)) depth++;
          else if (this.at(TokenType.RBRACKET)) depth--;
          this.advance();
        }
      } else {
        this.advance(); // skip . or ?.
        this.advance(); // skip field name
      }
    }
    const isAssign = this.at(TokenType.ASSIGN);

    this.s.restoreState(savedS);
    this.tok = savedTok;
    return isAssign;
  }

  private isVarAssignAhead(): boolean {
    const savedTok = this.tok;
    const savedS = this.s.saveState();

    this.advance(); // skip VAR
    while (this.at(TokenType.DOT) || this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET) || this.at(TokenType.QDOT)) {
      if (this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET)) {
        let depth = 1;
        this.advance();
        while (depth > 0 && !this.at(TokenType.EOF)) {
          if (this.at(TokenType.LBRACKET) || this.at(TokenType.QLBRACKET)) depth++;
          else if (this.at(TokenType.RBRACKET)) depth--;
          this.advance();
        }
      } else {
        this.advance();
        this.advance();
      }
    }
    const isAssign = this.at(TokenType.ASSIGN);

    this.s.restoreState(savedS);
    this.tok = savedTok;
    return isAssign;
  }

  private parseVarAssign(): VarAssign {
    const pos = this.tok.pos;
    const name = this.tok.literal;
    this.advance(); // skip VAR

    const path = this.parsePathSegments();
    this.expect(TokenType.ASSIGN);
    const value = this.parseExpr(BP_NONE);

    return { pos, name, path, value };
  }

  // --- Postfix ---

  private parsePostfixDot(receiver: Expr): Expr {
    const nullSafe = this.tok.type === TokenType.QDOT;
    const dotPos = this.tok.pos;
    this.advance(); // skip . or ?.

    const name = this.expectWord();

    if (this.at(TokenType.LPAREN)) {
      this.advance();
      const { args, named } = this.parseArgList();
      this.expect(TokenType.RPAREN);
      return { kind: "method_call", receiver, method: name, methodPos: dotPos, args, named, nullSafe };
    }

    return { kind: "field_access", receiver, field: name, fieldPos: dotPos, nullSafe };
  }

  private parsePostfixIndex(receiver: Expr): Expr {
    const nullSafe = this.tok.type === TokenType.QLBRACKET;
    const pos = this.tok.pos;
    this.advance(); // skip [ or ?[

    const index = this.parseExpr(BP_NONE);
    this.expect(TokenType.RBRACKET);

    return { kind: "index", receiver, index, pos, nullSafe };
  }

  // --- Argument lists ---

  private parseArgList(): { args: CallArg[]; named: boolean } {
    if (this.at(TokenType.RPAREN)) return { args: [], named: false };

    const named = this.isNamedArgList();
    const args: CallArg[] = [];

    for (;;) {
      if (named) {
        if (this.tok.type !== TokenType.IDENT) {
          this.error(this.tok.pos, "cannot mix named and positional arguments in the same call");
          while (!this.at(TokenType.RPAREN) && !this.at(TokenType.EOF)) this.advance();
          break;
        }
        const nameTok = this.expect(TokenType.IDENT);
        this.expect(TokenType.COLON);
        const value = this.parseExpr(BP_NONE);
        args.push({ name: nameTok.literal, value });
      } else {
        const value = this.parseExpr(BP_NONE);
        if (this.at(TokenType.COLON)) {
          this.error(this.tok.pos, "cannot mix positional and named arguments in the same call");
          while (!this.at(TokenType.RPAREN) && !this.at(TokenType.EOF)) this.advance();
          break;
        }
        args.push({ name: "", value });
      }
      if (!this.at(TokenType.COMMA)) break;
      this.advance();
    }

    return { args, named };
  }

  private isNamedArgList(): boolean {
    if (!this.at(TokenType.IDENT)) return false;
    const savedTok = this.tok;
    const savedS = this.s.saveState();
    this.advance();
    const isNamed = this.at(TokenType.COLON);
    this.s.restoreState(savedS);
    this.tok = savedTok;
    return isNamed;
  }
}

interface InfixInfo {
  leftBP: number;
  rightBP: number;
  nonAssoc: boolean;
}

const INFIX_NONE: InfixInfo = { leftBP: BP_NONE, rightBP: BP_NONE, nonAssoc: false };
const INFIX_OR: InfixInfo = { leftBP: BP_OR, rightBP: BP_OR + 1, nonAssoc: false };
const INFIX_AND: InfixInfo = { leftBP: BP_AND, rightBP: BP_AND + 1, nonAssoc: false };
const INFIX_EQ: InfixInfo = { leftBP: BP_EQUALITY, rightBP: BP_EQUALITY + 1, nonAssoc: true };
const INFIX_CMP: InfixInfo = { leftBP: BP_COMPARISON, rightBP: BP_COMPARISON + 1, nonAssoc: true };
const INFIX_ADD: InfixInfo = { leftBP: BP_ADDITIVE, rightBP: BP_ADDITIVE + 1, nonAssoc: false };
const INFIX_MUL: InfixInfo = { leftBP: BP_MULTIPLY, rightBP: BP_MULTIPLY + 1, nonAssoc: false };
const INFIX_POST: InfixInfo = { leftBP: BP_POSTFIX, rightBP: BP_POSTFIX, nonAssoc: false };

function infixBP(type: TokenType): InfixInfo {
  switch (type) {
    case TokenType.OR:
      return INFIX_OR;
    case TokenType.AND:
      return INFIX_AND;
    case TokenType.EQ:
    case TokenType.NE:
      return INFIX_EQ;
    case TokenType.GT:
    case TokenType.GE:
    case TokenType.LT:
    case TokenType.LE:
      return INFIX_CMP;
    case TokenType.PLUS:
    case TokenType.MINUS:
      return INFIX_ADD;
    case TokenType.STAR:
    case TokenType.SLASH:
    case TokenType.PERCENT:
      return INFIX_MUL;
    case TokenType.DOT:
    case TokenType.QDOT:
    case TokenType.LBRACKET:
    case TokenType.QLBRACKET:
      return INFIX_POST;
    default:
      return INFIX_NONE;
  }
}
