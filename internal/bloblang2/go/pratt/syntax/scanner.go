package syntax

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// scanner tokenizes Bloblang V2 source code.
type scanner struct {
	src  string // source code
	file string // filename for positions

	pos     int       // current byte offset
	line    int       // 1-based current line
	col     int       // 1-based current column
	prevTok TokenType // type of the last non-NL token emitted

	parenDepth   int // () nesting depth
	bracketDepth int // [] nesting depth

	// Buffered token for lookahead (used for newline suppression).
	peeked *Token

	errors []PosError
}

// PosError is a compile error with source position.
type PosError struct {
	Pos Pos
	Msg string
}

func (e PosError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

func newScanner(src, file string) *scanner {
	return &scanner{
		src:     src,
		file:    file,
		line:    1,
		col:     1,
		prevTok: NL, // suppress leading newlines
	}
}

// next returns the next token. Returns EOF repeatedly after input is exhausted.
func (s *scanner) next() Token {
	if s.peeked != nil {
		tok := *s.peeked
		s.peeked = nil
		s.trackToken(tok)
		return tok
	}
	return s.scan()
}

func (s *scanner) trackToken(tok Token) {
	if tok.Type != NL {
		s.prevTok = tok.Type
	}
	switch tok.Type {
	case LPAREN:
		s.parenDepth++
	case RPAREN:
		if s.parenDepth > 0 {
			s.parenDepth--
		}
	case LBRACKET, QLBRACKET:
		s.bracketDepth++
	case RBRACKET:
		if s.bracketDepth > 0 {
			s.bracketDepth--
		}
	}
}

// scan produces the next token, applying newline suppression rules.
func (s *scanner) scan() Token {
	for {
		tok := s.scanRaw()
		if tok.Type != NL {
			s.trackToken(tok)
			return tok
		}

		// Newline suppression mechanism 1: inside () or [].
		if s.parenDepth > 0 || s.bracketDepth > 0 {
			continue
		}

		// Newline suppression mechanism 3: previous token suppresses NL.
		if suppressesFollowingNL(s.prevTok) {
			continue
		}

		// Newline suppression mechanism 2: next token is postfix continuation.
		nextTok := s.peekNextNonNL()
		if isPostfixContinuation(nextTok.Type) {
			continue
		}

		// Collapse consecutive NLs: if previous emitted token was already NL, skip.
		if s.prevTok == NL {
			continue
		}

		// Emit the newline.
		s.prevTok = NL
		return tok
	}
}

// peekNextNonNL scans forward past any NL tokens to find the next
// substantive token, without consuming it.
func (s *scanner) peekNextNonNL() Token {
	// Save state.
	savedPos := s.pos
	savedLine := s.line
	savedCol := s.col
	savedErrors := len(s.errors)

	for {
		tok := s.scanRaw()
		if tok.Type != NL {
			// Restore scanner to before we started peeking.
			s.pos = savedPos
			s.line = savedLine
			s.col = savedCol
			s.errors = s.errors[:savedErrors]
			return tok
		}
	}
}

// scanRaw produces the next raw token without newline suppression.
func (s *scanner) scanRaw() Token {
	s.skipWhitespaceAndComments()

	if s.pos >= len(s.src) {
		return s.makeToken(EOF, "")
	}

	ch := s.src[s.pos]

	// Newlines.
	if ch == '\n' {
		tok := s.makeToken(NL, "\n")
		s.advance()
		return tok
	}
	if ch == '\r' {
		tok := s.makeToken(NL, "\n")
		s.advance()
		if s.pos < len(s.src) && s.src[s.pos] == '\n' {
			s.advance()
		}
		return tok
	}

	// String literals.
	if ch == '"' {
		return s.scanString()
	}
	if ch == '`' {
		return s.scanRawString()
	}

	// Numbers.
	if isDigit(ch) {
		return s.scanNumber()
	}

	// Variable $name.
	if ch == '$' {
		return s.scanVar()
	}

	// Identifiers and keywords.
	if isIdentStart(ch) {
		return s.scanWord()
	}

	// Multi-character operators and delimiters.
	return s.scanOperator()
}

func (s *scanner) scanString() Token {
	startPos := s.currentPos()
	s.advance() // skip opening "

	var sb strings.Builder
	for s.pos < len(s.src) {
		ch := s.src[s.pos]
		if ch == '"' {
			s.advance() // skip closing "
			return Token{Type: STRING, Literal: sb.String(), Pos: startPos}
		}
		if ch == '\n' || ch == '\r' {
			s.addError(s.currentPos(), "unterminated string literal")
			return Token{Type: ILLEGAL, Literal: sb.String(), Pos: startPos}
		}
		if ch == '\\' {
			s.advance()
			escaped, ok := s.scanEscapeSeq()
			if !ok {
				return Token{Type: ILLEGAL, Literal: sb.String(), Pos: startPos}
			}
			sb.WriteString(escaped)
			continue
		}
		// Regular character — read full UTF-8 rune.
		r, size := utf8.DecodeRuneInString(s.src[s.pos:])
		sb.WriteRune(r)
		s.advanceN(size)
	}
	s.addError(startPos, "unterminated string literal")
	return Token{Type: ILLEGAL, Literal: sb.String(), Pos: startPos}
}

func (s *scanner) scanEscapeSeq() (string, bool) {
	if s.pos >= len(s.src) {
		s.addError(s.currentPos(), "unterminated escape sequence")
		return "", false
	}
	ch := s.src[s.pos]
	chPos := s.currentPos()
	s.advance()
	switch ch {
	case '"':
		return "\"", true
	case '\\':
		return "\\", true
	case 'n':
		return "\n", true
	case 't':
		return "\t", true
	case 'r':
		return "\r", true
	case 'u':
		return s.scanUnicodeEscape()
	default:
		s.addError(chPos, fmt.Sprintf("invalid escape character %q", ch))
		return "", false
	}
}

func (s *scanner) scanUnicodeEscape() (string, bool) {
	if s.pos >= len(s.src) {
		s.addError(s.currentPos(), "unterminated unicode escape")
		return "", false
	}

	// \u{X...} form: 1-6 hex digits.
	if s.src[s.pos] == '{' {
		s.advance() // skip {
		start := s.pos
		for s.pos < len(s.src) && isHexDigit(s.src[s.pos]) {
			s.advance()
		}
		hexStr := s.src[start:s.pos]
		if len(hexStr) == 0 || len(hexStr) > 6 {
			s.addError(s.currentPos(), "\\u{} requires 1-6 hex digits")
			return "", false
		}
		if s.pos >= len(s.src) || s.src[s.pos] != '}' {
			s.addError(s.currentPos(), "unterminated \\u{} escape")
			return "", false
		}
		s.advance() // skip }
		codepoint, _ := strconv.ParseUint(hexStr, 16, 32)
		if codepoint > 0x10FFFF {
			s.addError(s.currentPos(), fmt.Sprintf("unicode codepoint U+%X out of range", codepoint))
			return "", false
		}
		if codepoint >= 0xD800 && codepoint <= 0xDFFF {
			s.addError(s.currentPos(), fmt.Sprintf("surrogate codepoint U+%X is invalid", codepoint))
			return "", false
		}
		return string(rune(codepoint)), true
	}

	// \uXXXX form: exactly 4 hex digits.
	if s.pos+4 > len(s.src) {
		s.addError(s.currentPos(), "\\uXXXX requires exactly 4 hex digits")
		return "", false
	}
	hexStr := s.src[s.pos : s.pos+4]
	for _, c := range []byte(hexStr) {
		if !isHexDigit(c) {
			s.addError(s.currentPos(), fmt.Sprintf("invalid hex digit %q in \\uXXXX", c))
			return "", false
		}
	}
	s.advanceN(4)
	codepoint, _ := strconv.ParseUint(hexStr, 16, 32)
	if codepoint >= 0xD800 && codepoint <= 0xDFFF {
		s.addError(s.currentPos(), fmt.Sprintf("surrogate codepoint U+%04X is invalid", codepoint))
		return "", false
	}
	return string(rune(codepoint)), true
}

func (s *scanner) scanRawString() Token {
	startPos := s.currentPos()
	s.advance() // skip opening `

	start := s.pos
	for s.pos < len(s.src) {
		if s.src[s.pos] == '`' {
			lit := s.src[start:s.pos]
			s.advance() // skip closing `
			return Token{Type: RAW_STRING, Literal: lit, Pos: startPos}
		}
		s.advance() // advance() handles newline tracking
	}
	s.addError(startPos, "unterminated raw string literal")
	return Token{Type: ILLEGAL, Literal: s.src[start:], Pos: startPos}
}

func (s *scanner) scanNumber() Token {
	startPos := s.currentPos()
	start := s.pos

	for s.pos < len(s.src) && isDigit(s.src[s.pos]) {
		s.advance()
	}

	// Check for float: digits.digits
	if s.pos < len(s.src) && s.src[s.pos] == '.' {
		// Peek ahead — must have digit after dot for float literal.
		if s.pos+1 < len(s.src) && isDigit(s.src[s.pos+1]) {
			s.advance() // skip .
			for s.pos < len(s.src) && isDigit(s.src[s.pos]) {
				s.advance()
			}
			return Token{Type: FLOAT, Literal: s.src[start:s.pos], Pos: startPos}
		}
		// Dot without digit after — it's an int followed by a dot operator.
	}

	// Integer — validate range at scan time.
	lit := s.src[start:s.pos]
	_, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		s.addError(startPos, fmt.Sprintf("integer literal %s exceeds int64 range", lit))
		return Token{Type: ILLEGAL, Literal: lit, Pos: startPos}
	}
	return Token{Type: INT, Literal: lit, Pos: startPos}
}

func (s *scanner) scanVar() Token {
	startPos := s.currentPos()
	s.advance() // skip $

	if s.pos >= len(s.src) || !isIdentStart(s.src[s.pos]) {
		s.addError(startPos, "expected identifier after $")
		return Token{Type: ILLEGAL, Literal: "$", Pos: startPos}
	}

	start := s.pos
	for s.pos < len(s.src) && isIdentContinue(s.src[s.pos]) {
		s.advance()
	}

	name := s.src[start:s.pos]
	return Token{Type: VAR, Literal: name, Pos: startPos}
}

func (s *scanner) scanWord() Token {
	startPos := s.currentPos()
	start := s.pos
	for s.pos < len(s.src) && isIdentContinue(s.src[s.pos]) {
		s.advance()
	}
	word := s.src[start:s.pos]
	return Token{Type: LookupIdent(word), Literal: word, Pos: startPos}
}

func (s *scanner) scanOperator() Token {
	startPos := s.currentPos()
	ch := s.src[s.pos]
	s.advance()

	switch ch {
	case '.':
		return Token{Type: DOT, Literal: ".", Pos: startPos}
	case '@':
		return Token{Type: AT, Literal: "@", Pos: startPos}
	case '(':
		return Token{Type: LPAREN, Literal: "(", Pos: startPos}
	case ')':
		return Token{Type: RPAREN, Literal: ")", Pos: startPos}
	case '{':
		return Token{Type: LBRACE, Literal: "{", Pos: startPos}
	case '}':
		return Token{Type: RBRACE, Literal: "}", Pos: startPos}
	case '[':
		return Token{Type: LBRACKET, Literal: "[", Pos: startPos}
	case ']':
		return Token{Type: RBRACKET, Literal: "]", Pos: startPos}
	case ',':
		return Token{Type: COMMA, Literal: ",", Pos: startPos}
	case '+':
		return Token{Type: PLUS, Literal: "+", Pos: startPos}
	case '*':
		return Token{Type: STAR, Literal: "*", Pos: startPos}
	case '/':
		return Token{Type: SLASH, Literal: "/", Pos: startPos}
	case '%':
		return Token{Type: PERCENT, Literal: "%", Pos: startPos}

	case '?':
		if s.pos < len(s.src) {
			switch s.src[s.pos] {
			case '.':
				s.advance()
				return Token{Type: QDOT, Literal: "?.", Pos: startPos}
			case '[':
				s.advance()
				return Token{Type: QLBRACKET, Literal: "?[", Pos: startPos}
			}
		}
		s.addError(startPos, "unexpected character '?'")
		return Token{Type: ILLEGAL, Literal: "?", Pos: startPos}

	case ':':
		if s.pos < len(s.src) && s.src[s.pos] == ':' {
			s.advance()
			return Token{Type: DCOLON, Literal: "::", Pos: startPos}
		}
		return Token{Type: COLON, Literal: ":", Pos: startPos}

	case '=':
		if s.pos < len(s.src) {
			switch s.src[s.pos] {
			case '=':
				s.advance()
				return Token{Type: EQ, Literal: "==", Pos: startPos}
			case '>':
				s.advance()
				return Token{Type: FATARROW, Literal: "=>", Pos: startPos}
			}
		}
		return Token{Type: ASSIGN, Literal: "=", Pos: startPos}

	case '!':
		if s.pos < len(s.src) && s.src[s.pos] == '=' {
			s.advance()
			return Token{Type: NE, Literal: "!=", Pos: startPos}
		}
		return Token{Type: BANG, Literal: "!", Pos: startPos}

	case '>':
		if s.pos < len(s.src) && s.src[s.pos] == '=' {
			s.advance()
			return Token{Type: GE, Literal: ">=", Pos: startPos}
		}
		return Token{Type: GT, Literal: ">", Pos: startPos}

	case '<':
		if s.pos < len(s.src) && s.src[s.pos] == '=' {
			s.advance()
			return Token{Type: LE, Literal: "<=", Pos: startPos}
		}
		return Token{Type: LT, Literal: "<", Pos: startPos}

	case '&':
		if s.pos < len(s.src) && s.src[s.pos] == '&' {
			s.advance()
			return Token{Type: AND, Literal: "&&", Pos: startPos}
		}
		s.addError(startPos, "unexpected character '&', did you mean '&&'?")
		return Token{Type: ILLEGAL, Literal: "&", Pos: startPos}

	case '|':
		if s.pos < len(s.src) && s.src[s.pos] == '|' {
			s.advance()
			return Token{Type: OR, Literal: "||", Pos: startPos}
		}
		s.addError(startPos, "unexpected character '|', did you mean '||'?")
		return Token{Type: ILLEGAL, Literal: "|", Pos: startPos}

	case '-':
		if s.pos < len(s.src) && s.src[s.pos] == '>' {
			s.advance()
			return Token{Type: THINARROW, Literal: "->", Pos: startPos}
		}
		return Token{Type: MINUS, Literal: "-", Pos: startPos}
	}

	r, _ := utf8.DecodeRuneInString(s.src[s.pos-1:])
	s.addError(startPos, fmt.Sprintf("unexpected character %q", r))
	return Token{Type: ILLEGAL, Literal: string(r), Pos: startPos}
}

// skipWhitespaceAndComments skips horizontal whitespace and comments
// (but not newlines — those are significant tokens).
func (s *scanner) skipWhitespaceAndComments() {
	for s.pos < len(s.src) {
		ch := s.src[s.pos]
		if ch == ' ' || ch == '\t' {
			s.advance()
			continue
		}
		if ch == '#' {
			// Comment: skip to end of line (but don't consume the newline).
			for s.pos < len(s.src) && s.src[s.pos] != '\n' && s.src[s.pos] != '\r' {
				s.advance()
			}
			continue
		}
		break
	}
}

func (s *scanner) currentPos() Pos {
	return Pos{File: s.file, Line: s.line, Column: s.col}
}

func (s *scanner) makeToken(typ TokenType, lit string) Token {
	return Token{Type: typ, Literal: lit, Pos: s.currentPos()}
}

func (s *scanner) advance() {
	if s.pos < len(s.src) {
		if s.src[s.pos] == '\n' {
			s.line++
			s.col = 1
		} else {
			s.col++
		}
		s.pos++
	}
}

func (s *scanner) advanceN(n int) {
	for range n {
		s.advance()
	}
}

func (s *scanner) addError(pos Pos, msg string) {
	s.errors = append(s.errors, PosError{Pos: pos, Msg: msg})
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
