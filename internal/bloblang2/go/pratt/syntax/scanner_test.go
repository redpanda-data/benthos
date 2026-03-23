package syntax

import (
	"testing"
)

func scanAll(src string) []Token {
	s := newScanner(src, "")
	var tokens []Token
	for {
		tok := s.next()
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens
}

func scanTypes(src string) []TokenType {
	tokens := scanAll(src)
	types := make([]TokenType, len(tokens))
	for i, t := range tokens {
		types[i] = t.Type
	}
	return types
}

func requireTypes(t *testing.T, src string, expected ...TokenType) {
	t.Helper()
	got := scanTypes(src)
	if len(got) != len(expected) {
		t.Fatalf("token count: expected %d, got %d\nsource: %q\nexpected: %v\ngot:      %v", len(expected), len(got), src, expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("token %d: expected %s, got %s\nsource: %q\nfull: %v", i, expected[i], got[i], src, got)
		}
	}
}

func TestScanner_BasicTokens(t *testing.T) {
	requireTypes(t, "42", INT, EOF)
	requireTypes(t, "3.14", FLOAT, EOF)
	requireTypes(t, `"hello"`, STRING, EOF)
	requireTypes(t, "`raw`", RAW_STRING, EOF)
	requireTypes(t, "true", TRUE, EOF)
	requireTypes(t, "false", FALSE, EOF)
	requireTypes(t, "null", NULL, EOF)
	requireTypes(t, "foo", IDENT, EOF)
	requireTypes(t, "$x", VAR, EOF)
	requireTypes(t, "_", UNDERSCORE, EOF)
}

func TestScanner_Keywords(t *testing.T) {
	requireTypes(t, "input", INPUT, EOF)
	requireTypes(t, "output", OUTPUT, EOF)
	requireTypes(t, "if", IF, EOF)
	requireTypes(t, "else", ELSE, EOF)
	requireTypes(t, "match", MATCH, EOF)
	requireTypes(t, "as", AS, EOF)
	requireTypes(t, "map", MAP, EOF)
	requireTypes(t, "import", IMPORT, EOF)
	requireTypes(t, "deleted", DELETED, EOF)
	requireTypes(t, "throw", THROW, EOF)
}

func TestScanner_Operators(t *testing.T) {
	requireTypes(t, ".", DOT, EOF)
	requireTypes(t, "?.", QDOT, EOF)
	requireTypes(t, "@", AT, EOF)
	requireTypes(t, "::", DCOLON, EOF)
	requireTypes(t, "=", ASSIGN, EOF)
	requireTypes(t, "+", PLUS, EOF)
	requireTypes(t, "-", MINUS, EOF)
	requireTypes(t, "*", STAR, EOF)
	requireTypes(t, "/", SLASH, EOF)
	requireTypes(t, "%", PERCENT, EOF)
	requireTypes(t, "!", BANG, EOF)
	requireTypes(t, ">", GT, EOF)
	requireTypes(t, ">=", GE, EOF)
	requireTypes(t, "==", EQ, EOF)
	requireTypes(t, "!=", NE, EOF)
	requireTypes(t, "<", LT, EOF)
	requireTypes(t, "<=", LE, EOF)
	requireTypes(t, "&&", AND, EOF)
	requireTypes(t, "||", OR, EOF)
	requireTypes(t, "=>", FATARROW, EOF)
	requireTypes(t, "->", THINARROW, EOF)
	requireTypes(t, "?[", QLBRACKET, EOF)
}

func TestScanner_Delimiters(t *testing.T) {
	requireTypes(t, "(", LPAREN, EOF)
	requireTypes(t, ")", RPAREN, EOF)
	requireTypes(t, "{", LBRACE, EOF)
	requireTypes(t, "}", RBRACE, EOF)
	requireTypes(t, "[", LBRACKET, EOF)
	requireTypes(t, "]", RBRACKET, EOF)
	requireTypes(t, ",", COMMA, EOF)
	requireTypes(t, ":", COLON, EOF)
}

func TestScanner_StringEscapes(t *testing.T) {
	tests := []struct {
		src      string
		expected string
	}{
		{`"hello"`, "hello"},
		{`"line\none"`, "line\none"},
		{`"tab\there"`, "tab\there"},
		{`"cr\rhere"`, "cr\rhere"},
		{`"quote\"here"`, "quote\"here"},
		{`"slash\\here"`, "slash\\here"},
		{`"\u0041"`, "A"},
		{`"\u{41}"`, "A"},
		{`"\u{1F600}"`, "\U0001F600"},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			tokens := scanAll(tt.src)
			if tokens[0].Type != STRING {
				t.Fatalf("expected STRING, got %s", tokens[0].Type)
			}
			if tokens[0].Literal != tt.expected {
				t.Fatalf("expected literal %q, got %q", tt.expected, tokens[0].Literal)
			}
		})
	}
}

func TestScanner_RawString(t *testing.T) {
	tokens := scanAll("`no\\escapes`")
	if tokens[0].Type != RAW_STRING {
		t.Fatalf("expected RAW_STRING, got %s", tokens[0].Type)
	}
	if tokens[0].Literal != "no\\escapes" {
		t.Fatalf("expected literal %q, got %q", "no\\escapes", tokens[0].Literal)
	}
}

func TestScanner_IntegerRange(t *testing.T) {
	// Max int64 is valid.
	tokens := scanAll("9223372036854775807")
	if tokens[0].Type != INT {
		t.Fatalf("expected INT, got %s", tokens[0].Type)
	}

	// Max int64 + 1 is illegal.
	s := newScanner("9223372036854775808", "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for overflow, got %s", tok.Type)
	}
	if len(s.errors) == 0 {
		t.Fatal("expected error for overflow")
	}
}

func TestScanner_FloatRequiresDigitsBothSides(t *testing.T) {
	// 5. is int + dot (not a float).
	requireTypes(t, "5.", INT, DOT, EOF)

	// .5 is dot + int (not a float).
	requireTypes(t, ".5", DOT, INT, EOF)

	// 5.0 is a float.
	requireTypes(t, "5.0", FLOAT, EOF)
}

func TestScanner_VarToken(t *testing.T) {
	tokens := scanAll("$count")
	if tokens[0].Type != VAR {
		t.Fatalf("expected VAR, got %s", tokens[0].Type)
	}
	if tokens[0].Literal != "count" {
		t.Fatalf("expected literal %q, got %q", "count", tokens[0].Literal)
	}
}

func TestScanner_Comments(t *testing.T) {
	requireTypes(t, "42 # comment", INT, EOF)
	requireTypes(t, "# full line comment\n42", INT, EOF)
}

func TestScanner_NewlineSuppression_ParenNesting(t *testing.T) {
	// Inside parens, newlines are suppressed.
	requireTypes(t, "(1\n+\n2)", LPAREN, INT, PLUS, INT, RPAREN, EOF)

	// Inside brackets, newlines are suppressed.
	requireTypes(t, "[1\n2\n3]", LBRACKET, INT, INT, INT, RBRACKET, EOF)

	// Inside braces, newlines are NOT suppressed.
	requireTypes(t, "{\n}", LBRACE, NL, RBRACE, EOF)
}

func TestScanner_NewlineSuppression_PostfixContinuation(t *testing.T) {
	// . on next line suppresses NL.
	requireTypes(t, "input\n.field", INPUT, DOT, IDENT, EOF)

	// ?. on next line suppresses NL.
	requireTypes(t, "input\n?.field", INPUT, QDOT, IDENT, EOF)

	// [ on next line suppresses NL.
	requireTypes(t, "arr\n[0]", IDENT, LBRACKET, INT, RBRACKET, EOF)

	// ?[ on next line suppresses NL.
	requireTypes(t, "arr\n?[0]", IDENT, QLBRACKET, INT, RBRACKET, EOF)

	// else on next line suppresses NL.
	requireTypes(t, "}\nelse", RBRACE, ELSE, EOF)
}

func TestScanner_NewlineSuppression_OperatorContinuation(t *testing.T) {
	// Trailing + suppresses NL.
	requireTypes(t, "a +\nb", IDENT, PLUS, IDENT, EOF)

	// Trailing && suppresses NL.
	requireTypes(t, "a &&\nb", IDENT, AND, IDENT, EOF)

	// Trailing = suppresses NL.
	requireTypes(t, "output.x =\n42", OUTPUT, DOT, IDENT, ASSIGN, INT, EOF)

	// Trailing => suppresses NL.
	requireTypes(t, "\"a\" =>\n1", STRING, FATARROW, INT, EOF)

	// Trailing -> suppresses NL.
	requireTypes(t, "x ->\nx", IDENT, THINARROW, IDENT, EOF)
}

func TestScanner_NewlineEmitted(t *testing.T) {
	// Normal statement separator.
	requireTypes(t, "a = 1\nb = 2", IDENT, ASSIGN, INT, NL, IDENT, ASSIGN, INT, EOF)

	// Multiple newlines collapse to one.
	requireTypes(t, "a = 1\n\n\nb = 2", IDENT, ASSIGN, INT, NL, IDENT, ASSIGN, INT, EOF)
}

func TestScanner_Positions(t *testing.T) {
	tokens := scanAll("ab\ncd")
	// ab at 1:1
	if tokens[0].Pos.Line != 1 || tokens[0].Pos.Column != 1 {
		t.Fatalf("expected ab at 1:1, got %s", tokens[0].Pos)
	}
	// NL
	// cd at 2:1
	if tokens[2].Pos.Line != 2 || tokens[2].Pos.Column != 1 {
		t.Fatalf("expected cd at 2:1, got %s", tokens[2].Pos)
	}
}

func TestScanner_Expression(t *testing.T) {
	requireTypes(t, "output.result = input.x + 5",
		OUTPUT, DOT, IDENT, ASSIGN, INPUT, DOT, IDENT, PLUS, INT, EOF)
}

func TestScanner_MethodChain(t *testing.T) {
	requireTypes(t, `input.name.uppercase().length()`,
		INPUT, DOT, IDENT, DOT, IDENT, LPAREN, RPAREN, DOT, IDENT, LPAREN, RPAREN, EOF)
}

func TestScanner_MatchExpression(t *testing.T) {
	requireTypes(t, `match input.x as v { v > 0 => "pos", _ => "neg" }`,
		MATCH, INPUT, DOT, IDENT, AS, IDENT, LBRACE,
		IDENT, GT, INT, FATARROW, STRING, COMMA,
		UNDERSCORE, FATARROW, STRING,
		RBRACE, EOF)
}

func TestScanner_Lambda(t *testing.T) {
	requireTypes(t, "x -> x * 2",
		IDENT, THINARROW, IDENT, STAR, INT, EOF)
}

func TestScanner_MultiParamLambda(t *testing.T) {
	requireTypes(t, "(a, b) -> a + b",
		LPAREN, IDENT, COMMA, IDENT, RPAREN, THINARROW, IDENT, PLUS, IDENT, EOF)
}

func TestScanner_QualifiedName(t *testing.T) {
	requireTypes(t, "math::double(5)",
		IDENT, DCOLON, IDENT, LPAREN, INT, RPAREN, EOF)
}

func TestScanner_MultilineMethodChain(t *testing.T) {
	src := "input.items\n  .filter(x -> x > 0)\n  .map(x -> x * 2)"
	requireTypes(t, src,
		INPUT, DOT, IDENT,
		// NL suppressed by postfix continuation (.)
		DOT, IDENT, LPAREN, IDENT, THINARROW, IDENT, GT, INT, RPAREN,
		// NL suppressed by postfix continuation (.)
		DOT, MAP, LPAREN, IDENT, THINARROW, IDENT, STAR, INT, RPAREN,
		// Note: .map() scans "map" as the MAP keyword. The parser
		// handles keywords-as-method-names after dot.
		EOF)
}

func TestScanner_IfElseMultiline(t *testing.T) {
	src := "if true { 1 }\nelse { 2 }"
	requireTypes(t, src,
		IF, TRUE, LBRACE, INT, RBRACE,
		// NL suppressed by postfix continuation (else)
		ELSE, LBRACE, INT, RBRACE, EOF)
}

func TestScanner_SurrogateCodepointError(t *testing.T) {
	s := newScanner(`"\uD800"`, "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for surrogate, got %s", tok.Type)
	}
}

func TestScanner_NullSafeIndexBracketDepth(t *testing.T) {
	// Newlines inside ?[...] should be suppressed by bracket nesting.
	requireTypes(t, "arr?[\n0\n]", IDENT, QLBRACKET, INT, RBRACKET, EOF)
}

func TestScanner_WindowsLineEndings(t *testing.T) {
	requireTypes(t, "a = 1\r\nb = 2", IDENT, ASSIGN, INT, NL, IDENT, ASSIGN, INT, EOF)
}

func TestScanner_UnterminatedString(t *testing.T) {
	s := newScanner(`"unterminated`, "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for unterminated string, got %s", tok.Type)
	}
	if len(s.errors) == 0 {
		t.Fatal("expected error for unterminated string")
	}
}

func TestScanner_UnterminatedRawString(t *testing.T) {
	s := newScanner("`unterminated", "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for unterminated raw string, got %s", tok.Type)
	}
}

func TestScanner_LoneAmpersand(t *testing.T) {
	s := newScanner("&", "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for lone &, got %s", tok.Type)
	}
}

func TestScanner_LonePipe(t *testing.T) {
	s := newScanner("|", "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for lone |, got %s", tok.Type)
	}
}

func TestScanner_DollarWithoutIdent(t *testing.T) {
	s := newScanner("$ ", "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for bare $, got %s", tok.Type)
	}
}

func TestScanner_UnicodeEscapeEdgeCases(t *testing.T) {
	// Empty braces.
	s := newScanner(`"\u{}"`, "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for empty \\u{}, got %s", tok.Type)
	}

	// Too many hex digits.
	s = newScanner(`"\u{0000000}"`, "")
	tok = s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for 7-digit \\u{}, got %s", tok.Type)
	}

	// Out of unicode range.
	s = newScanner(`"\u{110000}"`, "")
	tok = s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for out-of-range codepoint, got %s", tok.Type)
	}
}

func TestScanner_MultipleErrors(t *testing.T) {
	// Two illegal tokens on separate lines — both errors collected.
	s := newScanner("&\n|", "")
	s.next() // ILLEGAL &
	s.next() // NL (or suppressed)
	s.next() // ILLEGAL |
	if len(s.errors) < 2 {
		t.Fatalf("expected at least 2 errors, got %d", len(s.errors))
	}
}

func TestScanner_RawStringMultiline(t *testing.T) {
	// Raw string spanning lines — positions after it should be correct.
	tokens := scanAll("`a\nb`\nfoo")
	// raw string, NL, foo, EOF
	if tokens[0].Type != RAW_STRING {
		t.Fatalf("expected RAW_STRING, got %s", tokens[0].Type)
	}
	if tokens[0].Literal != "a\nb" {
		t.Fatalf("expected literal %q, got %q", "a\nb", tokens[0].Literal)
	}
	// "foo" should be on line 3.
	fooTok := tokens[2]
	if fooTok.Type != IDENT {
		t.Fatalf("expected IDENT, got %s", fooTok.Type)
	}
	if fooTok.Pos.Line != 3 {
		t.Fatalf("expected foo on line 3, got line %d", fooTok.Pos.Line)
	}
}

func TestScanner_FilePosition(t *testing.T) {
	s := newScanner("42", "test.blobl")
	tok := s.next()
	if tok.Pos.File != "test.blobl" {
		t.Fatalf("expected file %q, got %q", "test.blobl", tok.Pos.File)
	}
}

func TestScanner_InvalidEscapeCharacter(t *testing.T) {
	s := newScanner(`"\x"`, "")
	tok := s.next()
	if tok.Type != ILLEGAL {
		t.Fatalf("expected ILLEGAL for invalid escape, got %s", tok.Type)
	}
	if len(s.errors) == 0 {
		t.Fatal("expected error for invalid escape")
	}
}
