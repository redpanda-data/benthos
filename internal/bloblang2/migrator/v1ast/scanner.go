// Package v1ast implements a dedicated parser for Bloblang V1 that produces an
// inspectable, position-preserving AST. It is intended for migration tooling
// (V1 -> V2) and is deliberately separate from internal/bloblang/parser, which
// produces closures rather than AST nodes.
package v1ast

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Pos is a source position.
type Pos struct {
	Line   int // 1-based
	Column int // 1-based
	Offset int // byte offset from start of input
}

// String renders Pos as "line:col".
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Column) }

// TokenKind identifies the type of a lexical token.
type TokenKind int

// Token kinds.
const (
	TokEOF TokenKind = iota
	TokNewline
	TokIdent     // lenient: [A-Za-z0-9_]+
	TokInt       // digits (no sign)
	TokFloat     // digits "." digits
	TokString    // double-quoted (already unescaped)
	TokRawString // triple-quoted (raw)
	TokDollar    // $
	TokAt        // @
	TokLParen    // (
	TokRParen    // )
	TokLBracket  // [
	TokRBracket  // ]
	TokLBrace    // {
	TokRBrace    // }
	TokDot       // .
	TokComma     // ,
	TokColon     // :
	TokBang      // !
	TokAssign    // =
	TokEq        // ==
	TokNeq       // !=
	TokLt        // <
	TokLte       // <=
	TokGt        // >
	TokGte       // >=
	TokAnd       // &&
	TokOr        // ||
	TokPipe      // | (coalesce)
	TokPlus      // +
	TokMinus     // -
	TokStar      // *
	TokSlash     // /
	TokPercent   // %
	TokArrow     // ->
	TokFatArrow  // =>
	TokIllegal
)

var tokenNames = map[TokenKind]string{
	TokEOF: "EOF", TokNewline: "NEWLINE", TokIdent: "IDENT",
	TokInt: "INT", TokFloat: "FLOAT", TokString: "STRING", TokRawString: "RAW_STRING",
	TokDollar: "$", TokAt: "@", TokLParen: "(", TokRParen: ")",
	TokLBracket: "[", TokRBracket: "]", TokLBrace: "{", TokRBrace: "}",
	TokDot: ".", TokComma: ",", TokColon: ":", TokBang: "!",
	TokAssign: "=", TokEq: "==", TokNeq: "!=",
	TokLt: "<", TokLte: "<=", TokGt: ">", TokGte: ">=",
	TokAnd: "&&", TokOr: "||", TokPipe: "|",
	TokPlus: "+", TokMinus: "-", TokStar: "*", TokSlash: "/", TokPercent: "%",
	TokArrow: "->", TokFatArrow: "=>", TokIllegal: "ILLEGAL",
}

// String returns a human-readable token name.
func (t TokenKind) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	return fmt.Sprintf("TOK(%d)", int(t))
}

// Token is one lexical unit.
type Token struct {
	Kind TokenKind
	Text string // raw text for idents/numbers, unescaped content for strings
	Pos  Pos

	// PrecededBySpace indicates that the token was preceded by inline whitespace
	// (space or tab) but not by a newline. Used for assignment = whitespace rule
	// and for rejecting `a .b` where a space precedes a dot.
	PrecededBySpace bool
	// PrecededByNewline indicates the token is on a different line than the
	// previous token. (Comments do not count as newlines, but an actual newline
	// rune does.)
	PrecededByNewline bool
}

// Scanner produces tokens from source input.
type Scanner struct {
	src    []rune
	offset int // index into src
	line   int
	col    int

	// next-token hints
	precSpace, precNL bool
}

// NewScanner constructs a scanner over the given source.
func NewScanner(src string) *Scanner {
	return &Scanner{src: []rune(src), line: 1, col: 1}
}

// pos returns the current source position.
func (s *Scanner) pos() Pos {
	return Pos{Line: s.line, Column: s.col, Offset: s.byteOffset()}
}

func (s *Scanner) byteOffset() int {
	// Convert rune offset to approximate byte offset. Cheap & good enough.
	b := 0
	for i := 0; i < s.offset && i < len(s.src); i++ {
		b += utf8.RuneLen(s.src[i])
	}
	return b
}

func (s *Scanner) peek() rune {
	if s.offset >= len(s.src) {
		return 0
	}
	return s.src[s.offset]
}

func (s *Scanner) peekAt(i int) rune {
	if s.offset+i >= len(s.src) {
		return 0
	}
	return s.src[s.offset+i]
}

func (s *Scanner) advance() rune {
	if s.offset >= len(s.src) {
		return 0
	}
	r := s.src[s.offset]
	s.offset++
	if r == '\n' {
		s.line++
		s.col = 1
	} else {
		s.col++
	}
	return r
}

// All tokenises the entire input and returns every token (including the
// terminating EOF).
func (s *Scanner) All() ([]Token, error) {
	var out []Token
	for {
		tok, err := s.Next()
		if err != nil {
			return out, err
		}
		out = append(out, tok)
		if tok.Kind == TokEOF {
			return out, nil
		}
	}
}

// skipInlineWhitespace consumes spaces, tabs, and \r (treating \r\n as a
// newline at \n). It stops at \n, # (comment), or a non-whitespace rune.
// Returns whether any inline space was seen.
func (s *Scanner) skipInlineWhitespace() bool {
	sawSpace := false
	for s.offset < len(s.src) {
		r := s.src[s.offset]
		switch r {
		case ' ', '\t':
			s.advance()
			sawSpace = true
		case '\r':
			// Treat \r as whitespace unless followed by \n (handled by Next as newline).
			if s.peekAt(1) == '\n' {
				return sawSpace
			}
			s.advance()
			sawSpace = true
		case '#':
			// Line comment — consume to EOL but do NOT consume the newline.
			for s.offset < len(s.src) && s.src[s.offset] != '\n' {
				s.advance()
			}
		default:
			return sawSpace
		}
	}
	return sawSpace
}

// Next returns the next token.
func (s *Scanner) Next() (Token, error) {
	// Reset "preceded by" flags for this token.
	sawSpace := s.skipInlineWhitespace()
	if sawSpace {
		s.precSpace = true
	}

	// Handle newlines as tokens.
	if s.offset < len(s.src) {
		r := s.src[s.offset]
		if r == '\n' || (r == '\r' && s.peekAt(1) == '\n') {
			p := s.pos()
			if r == '\r' {
				s.advance()
			}
			s.advance()
			tok := Token{Kind: TokNewline, Text: "\n", Pos: p, PrecededBySpace: s.precSpace, PrecededByNewline: s.precNL}
			s.precSpace = false
			s.precNL = true
			return tok, nil
		}
	}

	if s.offset >= len(s.src) {
		tok := Token{Kind: TokEOF, Pos: s.pos(), PrecededBySpace: s.precSpace, PrecededByNewline: s.precNL}
		s.precSpace = false
		s.precNL = false
		return tok, nil
	}

	p := s.pos()
	precSpace, precNL := s.precSpace, s.precNL
	s.precSpace = false
	s.precNL = false

	r := s.src[s.offset]

	// Strings.
	if r == '"' {
		if s.peekAt(1) == '"' && s.peekAt(2) == '"' {
			return s.scanRawString(p, precSpace, precNL)
		}
		return s.scanString(p, precSpace, precNL)
	}

	// Numbers: must start with a digit. (Unary minus is produced separately.)
	if r >= '0' && r <= '9' {
		return s.scanNumber(p, precSpace, precNL)
	}

	// Identifiers (lenient): letters, digits, underscore, but must not start
	// with a digit because digits alone are numbers. The scanner emits any
	// [A-Za-z_] run (with digits allowed from position 2). Path segment parser
	// allows leading-digit segments via TokInt reinterpretation.
	if isIdentStart(r) {
		return s.scanIdent(p, precSpace, precNL)
	}

	// Operators and punctuation.
	switch r {
	case '(':
		s.advance()
		return Token{Kind: TokLParen, Text: "(", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case ')':
		s.advance()
		return Token{Kind: TokRParen, Text: ")", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '[':
		s.advance()
		return Token{Kind: TokLBracket, Text: "[", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case ']':
		s.advance()
		return Token{Kind: TokRBracket, Text: "]", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '{':
		s.advance()
		return Token{Kind: TokLBrace, Text: "{", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '}':
		s.advance()
		return Token{Kind: TokRBrace, Text: "}", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '.':
		s.advance()
		return Token{Kind: TokDot, Text: ".", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case ',':
		s.advance()
		return Token{Kind: TokComma, Text: ",", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case ':':
		s.advance()
		return Token{Kind: TokColon, Text: ":", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '$':
		s.advance()
		return Token{Kind: TokDollar, Text: "$", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '@':
		s.advance()
		return Token{Kind: TokAt, Text: "@", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '+':
		s.advance()
		return Token{Kind: TokPlus, Text: "+", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '*':
		s.advance()
		return Token{Kind: TokStar, Text: "*", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '/':
		s.advance()
		return Token{Kind: TokSlash, Text: "/", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '%':
		s.advance()
		return Token{Kind: TokPercent, Text: "%", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '-':
		s.advance()
		if s.peek() == '>' {
			s.advance()
			return Token{Kind: TokArrow, Text: "->", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokMinus, Text: "-", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '=':
		s.advance()
		if s.peek() == '=' {
			s.advance()
			return Token{Kind: TokEq, Text: "==", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		if s.peek() == '>' {
			s.advance()
			return Token{Kind: TokFatArrow, Text: "=>", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokAssign, Text: "=", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '!':
		s.advance()
		if s.peek() == '=' {
			s.advance()
			return Token{Kind: TokNeq, Text: "!=", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokBang, Text: "!", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '<':
		s.advance()
		if s.peek() == '=' {
			s.advance()
			return Token{Kind: TokLte, Text: "<=", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokLt, Text: "<", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '>':
		s.advance()
		if s.peek() == '=' {
			s.advance()
			return Token{Kind: TokGte, Text: ">=", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokGt, Text: ">", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	case '&':
		s.advance()
		if s.peek() == '&' {
			s.advance()
			return Token{Kind: TokAnd, Text: "&&", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokIllegal, Text: "&", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
			fmt.Errorf("%s: unexpected '&'", p)
	case '|':
		s.advance()
		if s.peek() == '|' {
			s.advance()
			return Token{Kind: TokOr, Text: "||", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		return Token{Kind: TokPipe, Text: "|", Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	}

	return Token{Kind: TokIllegal, Text: string(r), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
		fmt.Errorf("%s: unexpected character %q", p, r)
}

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func (s *Scanner) scanIdent(p Pos, precSpace, precNL bool) (Token, error) {
	start := s.offset
	for s.offset < len(s.src) && isIdentPart(s.src[s.offset]) {
		s.advance()
	}
	return Token{Kind: TokIdent, Text: string(s.src[start:s.offset]), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
}

func (s *Scanner) scanNumber(p Pos, precSpace, precNL bool) (Token, error) {
	start := s.offset
	for s.offset < len(s.src) && s.src[s.offset] >= '0' && s.src[s.offset] <= '9' {
		s.advance()
	}
	// Optional fractional part: must have digits on both sides.
	if s.peek() == '.' && s.peekAt(1) >= '0' && s.peekAt(1) <= '9' {
		s.advance() // dot
		for s.offset < len(s.src) && s.src[s.offset] >= '0' && s.src[s.offset] <= '9' {
			s.advance()
		}
		return Token{Kind: TokFloat, Text: string(s.src[start:s.offset]), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
	}
	return Token{Kind: TokInt, Text: string(s.src[start:s.offset]), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
}

func (s *Scanner) scanString(p Pos, precSpace, precNL bool) (Token, error) {
	start := s.offset
	s.advance() // opening "
	var buf strings.Builder
	buf.WriteByte('"')
	for s.offset < len(s.src) {
		r := s.src[s.offset]
		if r == '\\' {
			buf.WriteRune(r)
			s.advance()
			if s.offset < len(s.src) {
				buf.WriteRune(s.src[s.offset])
				s.advance()
			}
			continue
		}
		if r == '"' {
			buf.WriteRune(r)
			s.advance()
			// Use strconv.Unquote for standard Go-style unescape.
			raw := buf.String()
			val, err := strconv.Unquote(raw)
			if err != nil {
				return Token{Kind: TokIllegal, Text: raw, Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
					fmt.Errorf("%s: invalid string literal %q: %w", p, raw, err)
			}
			return Token{Kind: TokString, Text: val, Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		if r == '\n' {
			return Token{Kind: TokIllegal, Text: string(s.src[start:s.offset]), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
				fmt.Errorf("%s: unterminated string literal", p)
		}
		buf.WriteRune(r)
		s.advance()
	}
	return Token{Kind: TokIllegal, Text: string(s.src[start:s.offset]), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
		fmt.Errorf("%s: unterminated string literal", p)
}

func (s *Scanner) scanRawString(p Pos, precSpace, precNL bool) (Token, error) {
	// Already know src[offset..+2] == `"""`.
	s.advance()
	s.advance()
	s.advance()
	var buf strings.Builder
	for s.offset < len(s.src) {
		if s.src[s.offset] == '"' && s.peekAt(1) == '"' && s.peekAt(2) == '"' {
			s.advance()
			s.advance()
			s.advance()
			return Token{Kind: TokRawString, Text: buf.String(), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL}, nil
		}
		buf.WriteRune(s.src[s.offset])
		s.advance()
	}
	return Token{Kind: TokIllegal, Text: buf.String(), Pos: p, PrecededBySpace: precSpace, PrecededByNewline: precNL},
		fmt.Errorf("%s: unterminated triple-quoted string", p)
}
