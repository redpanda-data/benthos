package syntax

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// ILLEGAL represents an invalid token.
	ILLEGAL TokenType = iota
	// EOF signals end of input.
	EOF
	// NL is a newline statement separator.
	NL

	// INT is an integer literal (e.g., 42).
	INT
	// FLOAT is a float literal (e.g., 3.14).
	FLOAT
	// STRING is an escape-processed string literal.
	STRING
	// RAW_STRING is a raw backtick string literal.
	RAW_STRING

	// IDENT is a user-defined identifier (excludes keywords and reserved names).
	IDENT
	// VAR is a variable reference (e.g., $name — literal is "name" without the $).
	VAR

	// INPUT is the "input" keyword.
	INPUT
	// OUTPUT is the "output" keyword.
	OUTPUT
	// IF is the "if" keyword.
	IF
	// ELSE is the "else" keyword.
	ELSE
	// MATCH is the "match" keyword.
	MATCH
	// AS is the "as" keyword.
	AS
	// MAP is the "map" keyword.
	MAP
	// IMPORT is the "import" keyword.
	IMPORT
	// TRUE is the "true" keyword.
	TRUE
	// FALSE is the "false" keyword.
	FALSE
	// NULL is the "null" keyword.
	NULL
	// UNDERSCORE is the "_" keyword.
	UNDERSCORE

	// DELETED is the reserved function name "deleted".
	DELETED
	// THROW is the reserved function name "throw".
	THROW

	// DOT is the "." operator.
	DOT
	// QDOT is the "?." null-safe operator.
	QDOT
	// AT is the "@" metadata accessor.
	AT
	// DCOLON is the "::" namespace separator.
	DCOLON
	// ASSIGN is the "=" assignment operator.
	ASSIGN
	// PLUS is the "+" operator.
	PLUS
	// MINUS is the "-" operator.
	MINUS
	// STAR is the "*" operator.
	STAR
	// SLASH is the "/" operator.
	SLASH
	// PERCENT is the "%" operator.
	PERCENT
	// BANG is the "!" operator.
	BANG
	// GT is the ">" operator.
	GT
	// GE is the ">=" operator.
	GE
	// EQ is the "==" operator.
	EQ
	// NE is the "!=" operator.
	NE
	// LT is the "<" operator.
	LT
	// LE is the "<=" operator.
	LE
	// AND is the "&&" operator.
	AND
	// OR is the "||" operator.
	OR
	// FATARROW is the "=>" operator.
	FATARROW
	// THINARROW is the "->" operator.
	THINARROW

	// LPAREN is the "(" delimiter.
	LPAREN
	// RPAREN is the ")" delimiter.
	RPAREN
	// LBRACE is the "{" delimiter.
	LBRACE
	// RBRACE is the "}" delimiter.
	RBRACE
	// LBRACKET is the "[" delimiter.
	LBRACKET
	// RBRACKET is the "]" delimiter.
	RBRACKET
	// QLBRACKET is the "?[" null-safe index delimiter.
	QLBRACKET
	// COMMA is the "," delimiter.
	COMMA
	// COLON is the ":" delimiter.
	COLON
)

var tokenNames = map[TokenType]string{
	ILLEGAL:    "ILLEGAL",
	EOF:        "EOF",
	NL:         "NL",
	INT:        "INT",
	FLOAT:      "FLOAT",
	STRING:     "STRING",
	RAW_STRING: "RAW_STRING",
	IDENT:      "IDENT",
	VAR:        "VAR",
	INPUT:      "input",
	OUTPUT:     "output",
	IF:         "if",
	ELSE:       "else",
	MATCH:      "match",
	AS:         "as",
	MAP:        "map",
	IMPORT:     "import",
	TRUE:       "true",
	FALSE:      "false",
	NULL:       "null",
	UNDERSCORE: "_",
	DELETED:    "deleted",
	THROW:      "throw",
	DOT:        ".",
	QDOT:       "?.",
	AT:         "@",
	DCOLON:     "::",
	ASSIGN:     "=",
	PLUS:       "+",
	MINUS:      "-",
	STAR:       "*",
	SLASH:      "/",
	PERCENT:    "%",
	BANG:       "!",
	GT:         ">",
	GE:         ">=",
	EQ:         "==",
	NE:         "!=",
	LT:         "<",
	LE:         "<=",
	AND:        "&&",
	OR:         "||",
	FATARROW:   "=>",
	THINARROW:  "->",
	LPAREN:     "(",
	RPAREN:     ")",
	LBRACE:     "{",
	RBRACE:     "}",
	LBRACKET:   "[",
	RBRACKET:   "]",
	QLBRACKET:  "?[",
	COMMA:      ",",
	COLON:      ":",
}

func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// keywords maps keyword strings to their token types.
var keywords = map[string]TokenType{
	"input":  INPUT,
	"output": OUTPUT,
	"if":     IF,
	"else":   ELSE,
	"match":  MATCH,
	"as":     AS,
	"map":    MAP,
	"import": IMPORT,
	"true":   TRUE,
	"false":  FALSE,
	"null":   NULL,
	"_":      UNDERSCORE,
}

// reservedNames maps reserved function names to their token types.
var reservedNames = map[string]TokenType{
	"deleted": DELETED,
	"throw":   THROW,
}

// LookupIdent returns the token type for a word: keyword, reserved name,
// or IDENT for user-defined identifiers.
func LookupIdent(word string) TokenType {
	if tok, ok := keywords[word]; ok {
		return tok
	}
	if tok, ok := reservedNames[word]; ok {
		return tok
	}
	return IDENT
}

// IsKeyword reports whether the token type is a keyword.
func (t TokenType) IsKeyword() bool {
	_, ok := tokenNames[t]
	return ok && t >= INPUT && t <= UNDERSCORE
}

// Pos represents a source position.
type Pos struct {
	File   string // filename (empty for the main mapping)
	Line   int    // 1-based line number
	Column int    // 1-based column number (byte offset in line)
}

func (p Pos) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Token represents a single lexical token with its position and literal value.
type Token struct {
	Type    TokenType
	Literal string // the literal text of the token
	Pos     Pos
}

func (t Token) String() string {
	if t.Literal != "" {
		return fmt.Sprintf("%s(%q) at %s", t.Type, t.Literal, t.Pos)
	}
	return fmt.Sprintf("%s at %s", t.Type, t.Pos)
}

// suppressesFollowingNL reports whether this token type suppresses a
// following newline (mechanism 3: operator continuation). These are
// tokens that cannot be the final token of a complete expression —
// the spec lists them explicitly.
func suppressesFollowingNL(t TokenType) bool {
	switch t {
	case PLUS, MINUS, STAR, SLASH, PERCENT, // binary/unary operators
		EQ, NE, GT, GE, LT, LE,
		AND, OR,
		BANG,      // unary not
		ASSIGN,    // =
		FATARROW,  // =>
		THINARROW, // ->
		COLON:     // :
		return true
	default:
		return false
	}
}

// isPostfixContinuation reports whether this token type triggers
// newline suppression when it appears at the start of the next line
// (mechanism 2: postfix continuation).
func isPostfixContinuation(t TokenType) bool {
	switch t {
	case DOT, QDOT, LBRACKET, QLBRACKET, ELSE:
		return true
	default:
		return false
	}
}
