// Tokenizer for Bloblang V2 source code.

import {
  type Token,
  type Pos,
  type PosError,
  TokenType,
  lookupIdent,
  isReservedName,
  suppressesFollowingNL,
  isPostfixContinuation,
} from "./token.js";

export interface ScannerState {
  pos: number;
  line: number;
  col: number;
  prevTok: TokenType;
  parenDepth: number;
  bracketDepth: number;
  peeked: Token | null;
  errorLen: number;
}

export class Scanner {
  private src: string;
  private file: string;

  private pos = 0;
  private line = 1;
  private col = 1;
  private prevTok: TokenType = TokenType.NL; // suppress leading newlines

  private parenDepth = 0;
  private bracketDepth = 0;

  private peeked: Token | null = null;

  errors: PosError[] = [];

  constructor(src: string, file: string) {
    this.src = src;
    this.file = file;
  }

  /** Save scanner state for lookahead/backtracking. */
  saveState(): ScannerState {
    return {
      pos: this.pos,
      line: this.line,
      col: this.col,
      prevTok: this.prevTok,
      parenDepth: this.parenDepth,
      bracketDepth: this.bracketDepth,
      peeked: this.peeked,
      errorLen: this.errors.length,
    };
  }

  /** Restore scanner state from a previous save. */
  restoreState(state: ScannerState): void {
    this.pos = state.pos;
    this.line = state.line;
    this.col = state.col;
    this.prevTok = state.prevTok;
    this.parenDepth = state.parenDepth;
    this.bracketDepth = state.bracketDepth;
    this.peeked = state.peeked;
    this.errors.length = state.errorLen;
  }

  /** Returns the next token. Returns EOF repeatedly after input is exhausted. */
  next(): Token {
    if (this.peeked !== null) {
      const tok = this.peeked;
      this.peeked = null;
      this.trackToken(tok);
      return tok;
    }
    return this.scan();
  }

  private trackToken(tok: Token): void {
    if (tok.type !== TokenType.NL) {
      this.prevTok = tok.type;
    }
    switch (tok.type) {
      case TokenType.LPAREN:
        this.parenDepth++;
        break;
      case TokenType.RPAREN:
        if (this.parenDepth > 0) this.parenDepth--;
        break;
      case TokenType.LBRACKET:
      case TokenType.QLBRACKET:
        this.bracketDepth++;
        break;
      case TokenType.RBRACKET:
        if (this.bracketDepth > 0) this.bracketDepth--;
        break;
    }
  }

  /** Produces the next token with newline suppression applied. */
  private scan(): Token {
    for (;;) {
      const tok = this.scanRaw();
      if (tok.type !== TokenType.NL) {
        this.trackToken(tok);
        return tok;
      }

      // Mechanism 1: inside () or [].
      if (this.parenDepth > 0 || this.bracketDepth > 0) continue;

      // Mechanism 3: previous token suppresses NL.
      if (suppressesFollowingNL(this.prevTok)) continue;

      // Mechanism 2: next token is postfix continuation.
      const nextTok = this.peekNextNonNL();
      if (isPostfixContinuation(nextTok.type)) continue;

      // Collapse consecutive NLs.
      if (this.prevTok === TokenType.NL) continue;

      this.prevTok = TokenType.NL;
      return tok;
    }
  }

  /** Peek ahead past NL tokens to find the next substantive token. */
  private peekNextNonNL(): Token {
    const saved = this.saveState();
    for (;;) {
      const tok = this.scanRaw();
      if (tok.type !== TokenType.NL) {
        this.restoreState(saved);
        return tok;
      }
    }
  }

  /** Produces the next raw token without newline suppression. */
  private scanRaw(): Token {
    this.skipWhitespaceAndComments();

    if (this.pos >= this.src.length) {
      return this.makeToken(TokenType.EOF, "");
    }

    const ch = this.src[this.pos]!;

    // Newlines.
    if (ch === "\n") {
      const tok = this.makeToken(TokenType.NL, "\n");
      this.advance();
      return tok;
    }
    if (ch === "\r") {
      const tok = this.makeToken(TokenType.NL, "\n");
      this.advance();
      if (this.pos < this.src.length && this.src[this.pos] === "\n") {
        this.advance();
      }
      return tok;
    }

    // String literals.
    if (ch === '"') return this.scanString();
    if (ch === "`") return this.scanRawString();

    // Numbers.
    if (isDigit(ch)) return this.scanNumber();

    // Variable $name.
    if (ch === "$") return this.scanVar();

    // Identifiers and keywords.
    if (isIdentStart(ch)) return this.scanWord();

    // Operators and delimiters.
    return this.scanOperator();
  }

  private scanString(): Token {
    const startPos = this.currentPos();
    this.advance(); // skip opening "

    let s = "";
    while (this.pos < this.src.length) {
      const ch = this.src[this.pos]!;
      if (ch === '"') {
        this.advance(); // skip closing "
        return { type: TokenType.STRING, literal: s, pos: startPos };
      }
      if (ch === "\n" || ch === "\r") {
        this.addError(this.currentPos(), "unterminated string literal");
        return { type: TokenType.ILLEGAL, literal: s, pos: startPos };
      }
      if (ch === "\\") {
        this.advance();
        const escaped = this.scanEscapeSeq();
        if (escaped === null) {
          return { type: TokenType.ILLEGAL, literal: s, pos: startPos };
        }
        s += escaped;
        continue;
      }
      // Regular character — read full codepoint.
      const cp = this.src.codePointAt(this.pos)!;
      s += String.fromCodePoint(cp);
      this.advanceN(cp > 0xffff ? 2 : 1);
    }
    this.addError(startPos, "unterminated string literal");
    return { type: TokenType.ILLEGAL, literal: s, pos: startPos };
  }

  private scanEscapeSeq(): string | null {
    if (this.pos >= this.src.length) {
      this.addError(this.currentPos(), "unterminated escape sequence");
      return null;
    }
    const ch = this.src[this.pos]!;
    const chPos = this.currentPos();
    this.advance();
    switch (ch) {
      case '"':
        return '"';
      case "\\":
        return "\\";
      case "n":
        return "\n";
      case "t":
        return "\t";
      case "r":
        return "\r";
      case "u":
        return this.scanUnicodeEscape();
      default:
        this.addError(chPos, `invalid escape character '${ch}'`);
        return null;
    }
  }

  private scanUnicodeEscape(): string | null {
    if (this.pos >= this.src.length) {
      this.addError(this.currentPos(), "unterminated unicode escape");
      return null;
    }

    // \u{X...} form: 1-6 hex digits.
    if (this.src[this.pos] === "{") {
      this.advance(); // skip {
      const start = this.pos;
      while (this.pos < this.src.length && isHexDigit(this.src[this.pos]!)) {
        this.advance();
      }
      const hexStr = this.src.slice(start, this.pos);
      if (hexStr.length === 0 || hexStr.length > 6) {
        this.addError(this.currentPos(), "\\u{} requires 1-6 hex digits");
        return null;
      }
      if (this.pos >= this.src.length || this.src[this.pos] !== "}") {
        this.addError(this.currentPos(), "unterminated \\u{} escape");
        return null;
      }
      this.advance(); // skip }
      const codepoint = parseInt(hexStr, 16);
      if (codepoint > 0x10ffff) {
        this.addError(
          this.currentPos(),
          `unicode codepoint U+${codepoint.toString(16).toUpperCase()} out of range`,
        );
        return null;
      }
      if (codepoint >= 0xd800 && codepoint <= 0xdfff) {
        this.addError(
          this.currentPos(),
          `surrogate codepoint U+${codepoint.toString(16).toUpperCase()} is invalid`,
        );
        return null;
      }
      return String.fromCodePoint(codepoint);
    }

    // \uXXXX form: exactly 4 hex digits.
    if (this.pos + 4 > this.src.length) {
      this.addError(this.currentPos(), "\\uXXXX requires exactly 4 hex digits");
      return null;
    }
    const hexStr = this.src.slice(this.pos, this.pos + 4);
    for (const c of hexStr) {
      if (!isHexDigit(c)) {
        this.addError(this.currentPos(), `invalid hex digit '${c}' in \\uXXXX`);
        return null;
      }
    }
    this.advanceN(4);
    const codepoint = parseInt(hexStr, 16);
    if (codepoint >= 0xd800 && codepoint <= 0xdfff) {
      this.addError(
        this.currentPos(),
        `surrogate codepoint U+${codepoint.toString(16).toUpperCase().padStart(4, "0")} is invalid`,
      );
      return null;
    }
    return String.fromCodePoint(codepoint);
  }

  private scanRawString(): Token {
    const startPos = this.currentPos();
    this.advance(); // skip opening `

    const start = this.pos;
    while (this.pos < this.src.length) {
      if (this.src[this.pos] === "`") {
        const lit = this.src.slice(start, this.pos);
        this.advance(); // skip closing `
        return { type: TokenType.RAW_STRING, literal: lit, pos: startPos };
      }
      this.advance(); // handles newline tracking
    }
    this.addError(startPos, "unterminated raw string literal");
    return {
      type: TokenType.ILLEGAL,
      literal: this.src.slice(start),
      pos: startPos,
    };
  }

  private scanNumber(): Token {
    const startPos = this.currentPos();
    const start = this.pos;

    while (this.pos < this.src.length && isDigit(this.src[this.pos]!)) {
      this.advance();
    }

    // Check for float: digits.digits
    if (this.pos < this.src.length && this.src[this.pos] === ".") {
      if (
        this.pos + 1 < this.src.length &&
        isDigit(this.src[this.pos + 1]!)
      ) {
        this.advance(); // skip .
        while (this.pos < this.src.length && isDigit(this.src[this.pos]!)) {
          this.advance();
        }
        return {
          type: TokenType.FLOAT,
          literal: this.src.slice(start, this.pos),
          pos: startPos,
        };
      }
    }

    // Integer — validate range.
    const lit = this.src.slice(start, this.pos);
    try {
      const n = BigInt(lit);
      if (n > 9223372036854775807n || n < -9223372036854775808n) {
        this.addError(startPos, `integer literal ${lit} exceeds int64 range`);
        return { type: TokenType.ILLEGAL, literal: lit, pos: startPos };
      }
    } catch {
      this.addError(startPos, `invalid integer literal ${lit}`);
      return { type: TokenType.ILLEGAL, literal: lit, pos: startPos };
    }
    return { type: TokenType.INT, literal: lit, pos: startPos };
  }

  private scanVar(): Token {
    const startPos = this.currentPos();
    this.advance(); // skip $

    if (this.pos >= this.src.length || !isIdentStart(this.src[this.pos]!)) {
      this.addError(startPos, "expected identifier after $");
      return { type: TokenType.ILLEGAL, literal: "$", pos: startPos };
    }

    const start = this.pos;
    while (this.pos < this.src.length && isIdentContinue(this.src[this.pos]!)) {
      this.advance();
    }

    const name = this.src.slice(start, this.pos);
    if (isReservedName(name)) {
      this.addError(
        startPos,
        `"${name}" is a reserved function name and cannot be used as a variable name`,
      );
    }
    return {
      type: TokenType.VAR,
      literal: name,
      pos: startPos,
    };
  }

  private scanWord(): Token {
    const startPos = this.currentPos();
    const start = this.pos;
    while (this.pos < this.src.length && isIdentContinue(this.src[this.pos]!)) {
      this.advance();
    }
    const word = this.src.slice(start, this.pos);
    return { type: lookupIdent(word), literal: word, pos: startPos };
  }

  private scanOperator(): Token {
    const startPos = this.currentPos();
    const ch = this.src[this.pos]!;
    this.advance();

    switch (ch) {
      case ".":
        return { type: TokenType.DOT, literal: ".", pos: startPos };
      case "@":
        return { type: TokenType.AT, literal: "@", pos: startPos };
      case "(":
        return { type: TokenType.LPAREN, literal: "(", pos: startPos };
      case ")":
        return { type: TokenType.RPAREN, literal: ")", pos: startPos };
      case "{":
        return { type: TokenType.LBRACE, literal: "{", pos: startPos };
      case "}":
        return { type: TokenType.RBRACE, literal: "}", pos: startPos };
      case "[":
        return { type: TokenType.LBRACKET, literal: "[", pos: startPos };
      case "]":
        return { type: TokenType.RBRACKET, literal: "]", pos: startPos };
      case ",":
        return { type: TokenType.COMMA, literal: ",", pos: startPos };
      case "+":
        return { type: TokenType.PLUS, literal: "+", pos: startPos };
      case "*":
        return { type: TokenType.STAR, literal: "*", pos: startPos };
      case "/":
        return { type: TokenType.SLASH, literal: "/", pos: startPos };
      case "%":
        return { type: TokenType.PERCENT, literal: "%", pos: startPos };

      case "?":
        if (this.pos < this.src.length) {
          if (this.src[this.pos] === ".") {
            this.advance();
            return { type: TokenType.QDOT, literal: "?.", pos: startPos };
          }
          if (this.src[this.pos] === "[") {
            this.advance();
            return { type: TokenType.QLBRACKET, literal: "?[", pos: startPos };
          }
        }
        this.addError(startPos, "unexpected character '?'");
        return { type: TokenType.ILLEGAL, literal: "?", pos: startPos };

      case ":":
        if (this.pos < this.src.length && this.src[this.pos] === ":") {
          this.advance();
          return { type: TokenType.DCOLON, literal: "::", pos: startPos };
        }
        return { type: TokenType.COLON, literal: ":", pos: startPos };

      case "=":
        if (this.pos < this.src.length) {
          if (this.src[this.pos] === "=") {
            this.advance();
            return { type: TokenType.EQ, literal: "==", pos: startPos };
          }
          if (this.src[this.pos] === ">") {
            this.advance();
            return { type: TokenType.FATARROW, literal: "=>", pos: startPos };
          }
        }
        return { type: TokenType.ASSIGN, literal: "=", pos: startPos };

      case "!":
        if (this.pos < this.src.length && this.src[this.pos] === "=") {
          this.advance();
          return { type: TokenType.NE, literal: "!=", pos: startPos };
        }
        return { type: TokenType.BANG, literal: "!", pos: startPos };

      case ">":
        if (this.pos < this.src.length && this.src[this.pos] === "=") {
          this.advance();
          return { type: TokenType.GE, literal: ">=", pos: startPos };
        }
        return { type: TokenType.GT, literal: ">", pos: startPos };

      case "<":
        if (this.pos < this.src.length && this.src[this.pos] === "=") {
          this.advance();
          return { type: TokenType.LE, literal: "<=", pos: startPos };
        }
        return { type: TokenType.LT, literal: "<", pos: startPos };

      case "&":
        if (this.pos < this.src.length && this.src[this.pos] === "&") {
          this.advance();
          return { type: TokenType.AND, literal: "&&", pos: startPos };
        }
        this.addError(startPos, "unexpected character '&', did you mean '&&'?");
        return { type: TokenType.ILLEGAL, literal: "&", pos: startPos };

      case "|":
        if (this.pos < this.src.length && this.src[this.pos] === "|") {
          this.advance();
          return { type: TokenType.OR, literal: "||", pos: startPos };
        }
        this.addError(startPos, "unexpected character '|', did you mean '||'?");
        return { type: TokenType.ILLEGAL, literal: "|", pos: startPos };

      case "-":
        if (this.pos < this.src.length && this.src[this.pos] === ">") {
          this.advance();
          return { type: TokenType.THINARROW, literal: "->", pos: startPos };
        }
        return { type: TokenType.MINUS, literal: "-", pos: startPos };
    }

    this.addError(startPos, `unexpected character '${ch}'`);
    return { type: TokenType.ILLEGAL, literal: ch, pos: startPos };
  }

  private skipWhitespaceAndComments(): void {
    while (this.pos < this.src.length) {
      const ch = this.src[this.pos]!;
      if (ch === " " || ch === "\t") {
        this.advance();
        continue;
      }
      if (ch === "#") {
        // Comment: skip to end of line (don't consume newline).
        while (
          this.pos < this.src.length &&
          this.src[this.pos] !== "\n" &&
          this.src[this.pos] !== "\r"
        ) {
          this.advance();
        }
        continue;
      }
      break;
    }
  }

  private currentPos(): Pos {
    return { file: this.file, line: this.line, column: this.col };
  }

  private makeToken(type: TokenType, literal: string): Token {
    return { type, literal, pos: this.currentPos() };
  }

  private advance(): void {
    if (this.pos < this.src.length) {
      if (this.src[this.pos] === "\n") {
        this.line++;
        this.col = 1;
      } else {
        this.col++;
      }
      this.pos++;
    }
  }

  private advanceN(n: number): void {
    for (let i = 0; i < n; i++) {
      this.advance();
    }
  }

  private addError(pos: Pos, msg: string): void {
    this.errors.push({ pos, msg });
  }
}

function isDigit(ch: string): boolean {
  return ch >= "0" && ch <= "9";
}

function isHexDigit(ch: string): boolean {
  return (
    (ch >= "0" && ch <= "9") ||
    (ch >= "a" && ch <= "f") ||
    (ch >= "A" && ch <= "F")
  );
}

function isIdentStart(ch: string): boolean {
  return (ch >= "a" && ch <= "z") || (ch >= "A" && ch <= "Z") || ch === "_";
}

function isIdentContinue(ch: string): boolean {
  return isIdentStart(ch) || isDigit(ch);
}
