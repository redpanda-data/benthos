// Token types for the Bloblang V2 lexer.

export enum TokenType {
  ILLEGAL = "ILLEGAL",
  EOF = "EOF",
  NL = "NL",

  // Literals
  INT = "INT",
  FLOAT = "FLOAT",
  STRING = "STRING",
  RAW_STRING = "RAW_STRING",

  // Identifiers and variables
  IDENT = "IDENT",
  VAR = "VAR",

  // Keywords
  INPUT = "input",
  OUTPUT = "output",
  IF = "if",
  ELSE = "else",
  MATCH = "match",
  AS = "as",
  MAP = "map",
  IMPORT = "import",
  TRUE = "true",
  FALSE = "false",
  NULL = "null",
  UNDERSCORE = "_",

  // Reserved function names
  DELETED = "deleted",
  THROW = "throw",

  // Operators
  DOT = ".",
  QDOT = "?.",
  AT = "@",
  DCOLON = "::",
  ASSIGN = "=",
  PLUS = "+",
  MINUS = "-",
  STAR = "*",
  SLASH = "/",
  PERCENT = "%",
  BANG = "!",
  GT = ">",
  GE = ">=",
  EQ = "==",
  NE = "!=",
  LT = "<",
  LE = "<=",
  AND = "&&",
  OR = "||",
  FATARROW = "=>",
  THINARROW = "->",

  // Delimiters
  LPAREN = "(",
  RPAREN = ")",
  LBRACE = "{",
  RBRACE = "}",
  LBRACKET = "[",
  RBRACKET = "]",
  QLBRACKET = "?[",
  COMMA = ",",
  COLON = ":",
}

const keywords: Record<string, TokenType> = {
  input: TokenType.INPUT,
  output: TokenType.OUTPUT,
  if: TokenType.IF,
  else: TokenType.ELSE,
  match: TokenType.MATCH,
  as: TokenType.AS,
  map: TokenType.MAP,
  import: TokenType.IMPORT,
  true: TokenType.TRUE,
  false: TokenType.FALSE,
  null: TokenType.NULL,
  _: TokenType.UNDERSCORE,
};

const reservedNames: Record<string, TokenType> = {
  deleted: TokenType.DELETED,
  throw: TokenType.THROW,
};

export function lookupIdent(word: string): TokenType {
  return keywords[word] ?? reservedNames[word] ?? TokenType.IDENT;
}

const KEYWORD_SET: ReadonlySet<TokenType> = new Set(Object.values(keywords));

export function isKeyword(t: TokenType): boolean {
  return KEYWORD_SET.has(t);
}

/** Reports whether this token suppresses a following newline. */
export function suppressesFollowingNL(t: TokenType): boolean {
  switch (t) {
    case TokenType.PLUS:
    case TokenType.MINUS:
    case TokenType.STAR:
    case TokenType.SLASH:
    case TokenType.PERCENT:
    case TokenType.EQ:
    case TokenType.NE:
    case TokenType.GT:
    case TokenType.GE:
    case TokenType.LT:
    case TokenType.LE:
    case TokenType.AND:
    case TokenType.OR:
    case TokenType.BANG:
    case TokenType.ASSIGN:
    case TokenType.FATARROW:
    case TokenType.THINARROW:
    case TokenType.COLON:
      return true;
    default:
      return false;
  }
}

/** Reports whether this token triggers postfix continuation. */
export function isPostfixContinuation(t: TokenType): boolean {
  switch (t) {
    case TokenType.DOT:
    case TokenType.QDOT:
    case TokenType.LBRACKET:
    case TokenType.QLBRACKET:
    case TokenType.ELSE:
      return true;
    default:
      return false;
  }
}

export interface Pos {
  file: string;
  line: number;
  column: number;
}

export function posToString(p: Pos): string {
  return p.file ? `${p.file}:${p.line}:${p.column}` : `${p.line}:${p.column}`;
}

export interface Token {
  type: TokenType;
  literal: string;
  pos: Pos;
}

export interface PosError {
  pos: Pos;
  msg: string;
}
