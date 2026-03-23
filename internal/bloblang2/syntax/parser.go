package syntax

import (
	"fmt"
	"strings"
)

// Parse parses a Bloblang V2 mapping and returns the AST.
// files provides a virtual filesystem for import resolution.
func Parse(src, file string, files map[string]string) (*Program, []PosError) {
	p := &parser{
		files:       files,
		parsing:     map[string]bool{file: true},
		currentFile: file,
	}
	p.init(src, file)
	prog := p.parseProgram()
	return prog, p.errors
}

type parser struct {
	s           *scanner
	tok         Token // current token
	files       map[string]string
	parsing     map[string]bool // files currently being parsed (circular import detection)
	currentFile string
	errors      []PosError
}

func (p *parser) init(src, file string) {
	p.s = newScanner(src, file)
	p.currentFile = file
	p.advance() // prime the first token
}

// advance consumes the current token and moves to the next.
func (p *parser) advance() {
	p.tok = p.s.next()
	// Collect scanner errors.
	for len(p.s.errors) > 0 {
		p.errors = append(p.errors, p.s.errors...)
		p.s.errors = p.s.errors[:0]
	}
}

// expect consumes the current token if it matches the expected type,
// otherwise adds an error.
func (p *parser) expect(typ TokenType) Token {
	tok := p.tok
	if tok.Type != typ {
		p.error(tok.Pos, fmt.Sprintf("expected %s, got %s", typ, tok.Type))
		return tok
	}
	p.advance()
	return tok
}

// at reports whether the current token is of the given type.
func (p *parser) at(typ TokenType) bool {
	return p.tok.Type == typ
}

// skipNL consumes any NL tokens.
func (p *parser) skipNL() {
	for p.tok.Type == NL {
		p.advance()
	}
}

func (p *parser) error(pos Pos, msg string) {
	p.errors = append(p.errors, PosError{Pos: pos, Msg: msg})
}

// recover skips tokens until the next NL or EOF for error recovery.
func (p *parser) recover() {
	for p.tok.Type != NL && p.tok.Type != EOF {
		p.advance()
	}
}

// -----------------------------------------------------------------------
// Top-level parsing
// -----------------------------------------------------------------------

func (p *parser) parseProgram() *Program {
	prog := &Program{
		Namespaces: make(map[string][]*MapDecl),
	}

	p.skipNL()
	for p.tok.Type != EOF {
		switch p.tok.Type {
		case MAP:
			m := p.parseMapDecl()
			if m != nil {
				prog.Maps = append(prog.Maps, m)
			}
		case IMPORT:
			imp := p.parseImport(prog)
			if imp != nil {
				prog.Imports = append(prog.Imports, imp)
			}
		default:
			stmt := p.parseStatement()
			if stmt != nil {
				prog.Stmts = append(prog.Stmts, stmt)
			}
		}
		// Consume statement separator.
		if p.tok.Type == NL {
			p.advance()
			p.skipNL()
		} else if p.tok.Type != EOF {
			p.error(p.tok.Pos, fmt.Sprintf("expected newline or end of input, got %s", p.tok.Type))
			p.recover()
			p.skipNL()
		}
	}

	return prog
}

func (p *parser) parseMapDecl() *MapDecl {
	pos := p.tok.Pos
	p.advance() // skip 'map'

	nameTok := p.expect(IDENT)
	p.expect(LPAREN)
	params := p.parseParamList()
	p.expect(RPAREN)
	p.expect(LBRACE)

	body := p.parseExprBody()

	p.skipNL()
	p.expect(RBRACE)

	return &MapDecl{
		TokenPos: pos,
		Name:     nameTok.Literal,
		Params:   params,
		Body:     body,
	}
}

func (p *parser) parseParamList() []Param {
	if p.at(RPAREN) {
		return nil
	}

	var params []Param
	params = append(params, p.parseParam())
	for p.at(COMMA) {
		p.advance()
		params = append(params, p.parseParam())
	}
	return params
}

func (p *parser) parseParam() Param {
	pos := p.tok.Pos

	if p.at(UNDERSCORE) {
		p.advance()
		if p.at(ASSIGN) {
			p.error(pos, "discard parameter _ cannot have a default value")
			p.advance()      // skip =
			p.parseLiteral() // consume the value
		}
		return Param{Discard: true, Pos: pos}
	}

	nameTok := p.expect(IDENT)
	param := Param{Name: nameTok.Literal, Pos: pos}

	if p.at(ASSIGN) {
		p.advance()
		param.Default = p.parseLiteral()
		// Check that the default is actually a single literal (not an expression).
		if !p.at(COMMA) && !p.at(RPAREN) {
			p.error(p.tok.Pos, "default parameter values must be literals, not expressions")
			// Skip the rest of the expression to recover.
			for !p.at(COMMA) && !p.at(RPAREN) && !p.at(EOF) {
				p.advance()
			}
		}
	}

	return param
}

func (p *parser) parseLiteral() Expr {
	tok := p.tok
	switch tok.Type {
	case INT, FLOAT, STRING, RAW_STRING, TRUE, FALSE, NULL:
		p.advance()
		return &LiteralExpr{TokenPos: tok.Pos, TokenType: tok.Type, Value: tok.Literal}
	default:
		p.error(tok.Pos, fmt.Sprintf("expected literal value, got %s", tok.Type))
		return &LiteralExpr{TokenPos: tok.Pos, TokenType: NULL, Value: "null"}
	}
}

func (p *parser) parseImport(prog *Program) *ImportStmt {
	pos := p.tok.Pos
	p.advance() // skip 'import'

	pathTok := p.tok
	if pathTok.Type != STRING && pathTok.Type != RAW_STRING {
		p.error(pathTok.Pos, "expected string literal for import path")
		p.recover()
		return nil
	}
	p.advance()

	p.expect(AS)
	nsTok := p.expect(IDENT)

	imp := &ImportStmt{
		TokenPos:  pos,
		Path:      pathTok.Literal,
		Namespace: nsTok.Literal,
	}

	// Resolve the import.
	p.resolveImport(prog, imp)

	return imp
}

func (p *parser) resolveImport(prog *Program, imp *ImportStmt) {
	if _, exists := prog.Namespaces[imp.Namespace]; exists {
		p.error(imp.TokenPos, fmt.Sprintf("duplicate namespace %q", imp.Namespace))
		return
	}

	src, ok := p.files[imp.Path]
	if !ok {
		p.error(imp.TokenPos, fmt.Sprintf("import file %q not found", imp.Path))
		return
	}

	if p.parsing[imp.Path] {
		p.error(imp.TokenPos, fmt.Sprintf("circular import: %q", imp.Path))
		return
	}

	// Parse the imported file recursively.
	sub := &parser{
		files:       p.files,
		parsing:     p.parsing,
		currentFile: imp.Path,
	}
	p.parsing[imp.Path] = true
	sub.init(src, imp.Path)
	importProg := sub.parseProgram()
	delete(p.parsing, imp.Path)

	// Collect errors from the imported file.
	p.errors = append(p.errors, sub.errors...)

	// Only map declarations are allowed in imported files.
	if len(importProg.Stmts) > 0 {
		p.error(imp.TokenPos, fmt.Sprintf("imported file %q contains statements (only map declarations and imports are allowed)", imp.Path))
	}

	// Collect maps from the imported file. Attach the imported file's
	// namespace tables to each map so the interpreter can resolve
	// qualified calls (e.g., core::square) within those maps.
	for _, m := range importProg.Maps {
		if m.Namespaces == nil {
			m.Namespaces = make(map[string][]*MapDecl)
		}
		// Merge the imported program's namespaces into each map.
		for ns, maps := range importProg.Namespaces {
			m.Namespaces[ns] = maps
		}
		// Also include the imported program's own maps (for self-references).
		// These are accessible without namespace qualification within the file.
	}
	prog.Namespaces[imp.Namespace] = importProg.Maps
}

// -----------------------------------------------------------------------
// Statement parsing
// -----------------------------------------------------------------------

func (p *parser) parseStatement() Stmt {
	switch p.tok.Type {
	case IF:
		return p.parseIfStmt()
	case MATCH:
		return p.parseMatchStmt()
	default:
		return p.parseAssignment()
	}
}

func (p *parser) parseAssignment() Stmt {
	target, ok := p.parseAssignTarget()
	if !ok {
		p.recover()
		return nil
	}

	p.expect(ASSIGN)
	value := p.parseExpr(0)

	return &Assignment{
		TokenPos: target.Pos,
		Target:   target,
		Value:    value,
	}
}

func (p *parser) parseAssignTarget() (AssignTarget, bool) {
	var target AssignTarget
	target.Pos = p.tok.Pos

	switch p.tok.Type {
	case OUTPUT:
		target.Root = AssignOutput
		p.advance()
		if p.at(AT) {
			target.MetaAccess = true
			p.advance()
		}

	case VAR:
		target.Root = AssignVar
		target.VarName = p.tok.Literal
		p.advance()

	default:
		p.error(p.tok.Pos, fmt.Sprintf("unexpected expression in statement context (expected output or $variable assignment, got %s)", p.tok.Type))
		return target, false
	}

	// Parse path components.
	target.Path = p.parsePathSegments()
	return target, true
}

func (p *parser) parsePathSegments() []PathSegment {
	var segs []PathSegment
	for {
		switch p.tok.Type {
		case DOT:
			pos := p.tok.Pos
			p.advance()
			name := p.expectWord()
			// Check for method call: name(
			if p.at(LPAREN) {
				p.advance()
				args, named := p.parseArgList()
				p.expect(RPAREN)
				segs = append(segs, PathSegment{Kind: PathSegMethod, Name: name, Args: args, Named: named, Pos: pos})
			} else {
				segs = append(segs, PathSegment{Kind: PathSegField, Name: name, Pos: pos})
			}
		case QDOT:
			pos := p.tok.Pos
			p.advance()
			name := p.expectWord()
			if p.at(LPAREN) {
				p.advance()
				args, named := p.parseArgList()
				p.expect(RPAREN)
				segs = append(segs, PathSegment{Kind: PathSegMethod, Name: name, Args: args, Named: named, NullSafe: true, Pos: pos})
			} else {
				segs = append(segs, PathSegment{Kind: PathSegField, Name: name, NullSafe: true, Pos: pos})
			}
		case LBRACKET:
			pos := p.tok.Pos
			p.advance()
			idx := p.parseExpr(0)
			p.expect(RBRACKET)
			segs = append(segs, PathSegment{Kind: PathSegIndex, Index: idx, Pos: pos})
		case QLBRACKET:
			pos := p.tok.Pos
			p.advance()
			idx := p.parseExpr(0)
			p.expect(RBRACKET)
			segs = append(segs, PathSegment{Kind: PathSegIndex, Index: idx, NullSafe: true, Pos: pos})
		default:
			return segs
		}
	}
}

// expectWord consumes the current token as a word (identifier, keyword,
// or quoted string). Keywords are valid as field names after dot.
// Quoted strings (."field with spaces") are also valid per spec Section 3.1.
func (p *parser) expectWord() string {
	tok := p.tok
	if tok.Type == IDENT || tok.Type.IsKeyword() || tok.Type == DELETED || tok.Type == THROW {
		p.advance()
		return tok.Literal
	}
	if tok.Type == STRING {
		p.advance()
		return tok.Literal
	}
	p.error(tok.Pos, fmt.Sprintf("expected field name, got %s", tok.Type))
	return ""
}

func (p *parser) parseIfStmt() Stmt {
	pos := p.tok.Pos
	p.advance() // skip 'if'

	var branches []IfBranch

	cond := p.parseExpr(0)
	p.expect(LBRACE)
	body := p.parseStmtBody()
	p.expect(RBRACE)
	branches = append(branches, IfBranch{Cond: cond, Body: body})

	// else-if / else
	var elseBody []Stmt
	for p.at(ELSE) {
		p.advance()
		if p.at(IF) {
			p.advance()
			cond := p.parseExpr(0)
			p.expect(LBRACE)
			body := p.parseStmtBody()
			p.expect(RBRACE)
			branches = append(branches, IfBranch{Cond: cond, Body: body})
		} else {
			p.expect(LBRACE)
			elseBody = p.parseStmtBody()
			p.expect(RBRACE)
			break
		}
	}

	return &IfStmt{TokenPos: pos, Branches: branches, Else: elseBody}
}

func (p *parser) parseMatchStmt() Stmt {
	pos := p.tok.Pos
	p.advance() // skip 'match'

	var subject Expr
	var binding string

	// Disambiguate: match { cases } vs match expr { cases }
	if !p.at(LBRACE) {
		subject = p.parseExpr(0)
		if p.at(AS) {
			p.advance()
			binding = p.expect(IDENT).Literal
		}
	}

	p.expect(LBRACE)
	p.skipNL()

	var cases []MatchCase
	for !p.at(RBRACE) && !p.at(EOF) {
		mc := p.parseMatchCaseStmt()
		cases = append(cases, mc)
		if p.at(COMMA) {
			p.advance()
		}
		p.skipNL()
	}

	p.expect(RBRACE)

	return &MatchStmt{TokenPos: pos, Subject: subject, Binding: binding, Cases: cases}
}

func (p *parser) parseMatchCaseStmt() MatchCase {
	var mc MatchCase

	if p.at(UNDERSCORE) {
		mc.Wildcard = true
		p.advance()
	} else {
		mc.Pattern = p.parseExpr(0)
	}

	p.expect(FATARROW)
	p.expect(LBRACE)
	body := p.parseStmtBody()
	p.expect(RBRACE)
	mc.Body = body

	return mc
}

func (p *parser) parseStmtBody() []Stmt {
	p.skipNL()
	var stmts []Stmt
	for !p.at(RBRACE) && !p.at(EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		if p.at(NL) {
			p.advance()
			p.skipNL()
		} else if !p.at(RBRACE) && !p.at(EOF) {
			p.error(p.tok.Pos, fmt.Sprintf("expected newline or }, got %s", p.tok.Type))
			p.recover()
			p.skipNL()
		}
	}
	return stmts
}

// -----------------------------------------------------------------------
// Expression parsing (Pratt parser)
// -----------------------------------------------------------------------

// Binding powers.
const (
	bpNone       = 0
	bpOr         = 10
	bpAnd        = 20
	bpEquality   = 40
	bpComparison = 60
	bpAdditive   = 80
	bpMultiply   = 100
	bpUnary      = 120
	bpPostfix    = 140
)

func (p *parser) parseExpr(minBP int) Expr {
	left := p.parsePrefix()

	for {
		bp, rightBP, nonAssoc := infixBP(p.tok.Type)
		if bp == bpNone || bp < minBP {
			break
		}

		switch p.tok.Type {
		case DOT, QDOT:
			left = p.parsePostfixDot(left)
		case LBRACKET, QLBRACKET:
			left = p.parsePostfixIndex(left)
		default:
			// Binary operator.
			op := p.tok
			p.advance()
			right := p.parseExpr(rightBP)

			// Non-associative check: if the next token is at the same level, error.
			if nonAssoc {
				nextBP, _, _ := infixBP(p.tok.Type)
				if nextBP == bp {
					p.error(p.tok.Pos, fmt.Sprintf("cannot chain non-associative operator %s", p.tok.Type))
				}
			}

			left = &BinaryExpr{Left: left, Op: op.Type, OpPos: op.Pos, Right: right}
		}
	}

	return left
}

func infixBP(typ TokenType) (leftBP, rightBP int, nonAssoc bool) {
	switch typ {
	case OR:
		return bpOr, bpOr + 1, false
	case AND:
		return bpAnd, bpAnd + 1, false
	case EQ, NE:
		return bpEquality, bpEquality + 1, true
	case GT, GE, LT, LE:
		return bpComparison, bpComparison + 1, true
	case PLUS, MINUS:
		return bpAdditive, bpAdditive + 1, false
	case STAR, SLASH, PERCENT:
		return bpMultiply, bpMultiply + 1, false
	case DOT, QDOT, LBRACKET, QLBRACKET:
		return bpPostfix, bpPostfix, false
	default:
		return bpNone, bpNone, false
	}
}

// -----------------------------------------------------------------------
// Prefix / atom parsers (null-denotation)
// -----------------------------------------------------------------------

func (p *parser) parsePrefix() Expr {
	tok := p.tok

	switch tok.Type {
	case INT, FLOAT, STRING, RAW_STRING, TRUE, FALSE, NULL:
		p.advance()
		return &LiteralExpr{TokenPos: tok.Pos, TokenType: tok.Type, Value: tok.Literal}

	case MINUS:
		p.advance()
		operand := p.parseExpr(bpUnary)
		return &UnaryExpr{Op: MINUS, OpPos: tok.Pos, Operand: operand}

	case BANG:
		p.advance()
		operand := p.parseExpr(bpUnary)
		return &UnaryExpr{Op: BANG, OpPos: tok.Pos, Operand: operand}

	case LPAREN:
		return p.parseParenOrLambda()

	case LBRACKET:
		return p.parseArrayLiteral()

	case LBRACE:
		return p.parseObjectLiteral()

	case IF:
		return p.parseIfExpr()

	case MATCH:
		return p.parseMatchExpr()

	case INPUT:
		p.advance()
		if p.at(AT) {
			p.advance()
			return &InputMetaExpr{TokenPos: tok.Pos}
		}
		return &InputExpr{TokenPos: tok.Pos}

	case OUTPUT:
		p.advance()
		if p.at(AT) {
			p.advance()
			return &OutputMetaExpr{TokenPos: tok.Pos}
		}
		return &OutputExpr{TokenPos: tok.Pos}

	case VAR:
		p.advance()
		return &VarExpr{TokenPos: tok.Pos, Name: tok.Literal}

	case IDENT:
		return p.parseIdentOrCall()

	case DELETED, THROW:
		return p.parseReservedCall()

	case UNDERSCORE:
		p.advance()
		// _ -> body: discard lambda.
		if p.at(THINARROW) {
			p.advance()
			body := p.parseLambdaBody()
			return &LambdaExpr{
				TokenPos: tok.Pos,
				Params:   []Param{{Discard: true, Pos: tok.Pos}},
				Body:     body,
			}
		}
		// Underscore in other expression positions is not valid.
		p.error(tok.Pos, "unexpected _ in expression position")
		return &LiteralExpr{TokenPos: tok.Pos, TokenType: NULL, Value: "null"}

	default:
		p.error(tok.Pos, fmt.Sprintf("expected expression, got %s", tok.Type))
		p.advance()
		return &LiteralExpr{TokenPos: tok.Pos, TokenType: NULL, Value: "null"}
	}
}

func (p *parser) parseIdentOrCall() Expr {
	tok := p.tok
	p.advance()

	// Check for qualified name: namespace::name
	if p.at(DCOLON) {
		p.advance()
		name := p.expect(IDENT)
		p.expect(LPAREN)
		args, named := p.parseArgList()
		p.expect(RPAREN)
		return &CallExpr{
			TokenPos:  tok.Pos,
			Namespace: tok.Literal,
			Name:      name.Literal,
			Args:      args,
			Named:     named,
		}
	}

	// Check for function call: name(
	if p.at(LPAREN) {
		p.advance()
		args, named := p.parseArgList()
		p.expect(RPAREN)
		return &CallExpr{
			TokenPos: tok.Pos,
			Name:     tok.Literal,
			Args:     args,
			Named:    named,
		}
	}

	// Check for single-param lambda: ident ->
	if p.at(THINARROW) {
		p.advance()
		body := p.parseLambdaBody()
		return &LambdaExpr{
			TokenPos: tok.Pos,
			Params:   []Param{{Name: tok.Literal, Pos: tok.Pos}},
			Body:     body,
		}
	}

	// Bare identifier (parameter reference, map name reference).
	return &IdentExpr{TokenPos: tok.Pos, Name: tok.Literal}
}

func (p *parser) parseReservedCall() Expr {
	tok := p.tok
	p.advance()
	p.expect(LPAREN)
	args, named := p.parseArgList()
	p.expect(RPAREN)
	return &CallExpr{
		TokenPos: tok.Pos,
		Name:     tok.Literal,
		Args:     args,
		Named:    named,
	}
}

func (p *parser) parseParenOrLambda() Expr {
	pos := p.tok.Pos

	// Lookahead: scan past matching ) and check for ->.
	if p.isLambdaAhead() {
		return p.parseMultiParamLambda(pos)
	}

	// Grouped expression.
	p.advance() // skip (
	expr := p.parseExpr(0)
	p.expect(RPAREN)
	return expr
}

// isLambdaAhead scans forward from the current ( to the matching )
// and checks if -> follows. Does not consume tokens.
func (p *parser) isLambdaAhead() bool {
	// Save scanner state.
	savedTok := p.tok
	savedS := *p.s

	depth := 0
	p.advance() // skip (
	depth++
	for depth > 0 && p.tok.Type != EOF {
		switch p.tok.Type {
		case LPAREN:
			depth++
		case RPAREN:
			depth--
		}
		if depth > 0 {
			p.advance()
		}
	}
	// Now at the matching ) — peek past it.
	p.advance() // skip )
	isLambda := p.tok.Type == THINARROW

	// Restore state.
	*p.s = savedS
	p.tok = savedTok

	return isLambda
}

func (p *parser) parseMultiParamLambda(pos Pos) Expr {
	p.advance() // skip (
	params := p.parseParamList()
	p.expect(RPAREN)
	p.expect(THINARROW)
	body := p.parseLambdaBody()
	return &LambdaExpr{TokenPos: pos, Params: params, Body: body}
}

func (p *parser) parseLambdaBody() *ExprBody {
	if p.at(LBRACE) {
		// Disambiguate: lambda block vs object literal.
		// Per spec Section 10: {} is parsed as empty object literal.
		// A lambda block requires at least one var assignment ($var = ...)
		// followed by a final expression. If the content after { doesn't
		// start with $var, parse as a single expression (object literal).
		if p.isLambdaBlock() {
			p.advance() // skip {
			body := p.parseExprBody()
			p.skipNL()
			p.expect(RBRACE)
			return body
		}
		// Object literal or other expression starting with {.
		expr := p.parseExpr(0)
		return &ExprBody{Result: expr}
	}
	// Single expression.
	expr := p.parseExpr(0)
	return &ExprBody{Result: expr}
}

// isLambdaBlock peeks inside { to determine if it's a lambda block
// (has var assignments, output assignments, or identifier assignments)
// or an object literal.
func (p *parser) isLambdaBlock() bool {
	savedTok := p.tok
	savedS := *p.s

	p.advance() // skip {
	// Skip optional NL.
	for p.tok.Type == NL {
		p.advance()
	}

	var isBlock bool
	switch p.tok.Type {
	case VAR, OUTPUT:
		// Definitely a block.
		isBlock = true
	case IDENT:
		// Could be block (x = ...) or object literal (key: value).
		// Peek ahead: if followed by = it's an assignment attempt.
		savedInner := p.tok
		savedInnerS := *p.s
		p.advance()
		isBlock = p.tok.Type == ASSIGN
		*p.s = savedInnerS
		p.tok = savedInner
	}

	*p.s = savedS
	p.tok = savedTok
	return isBlock
}

func (p *parser) parseArrayLiteral() Expr {
	pos := p.tok.Pos
	p.advance() // skip [

	var elems []Expr
	for !p.at(RBRACKET) && !p.at(EOF) {
		elems = append(elems, p.parseExpr(0))
		if !p.at(RBRACKET) {
			p.expect(COMMA)
		}
	}
	p.expect(RBRACKET)

	return &ArrayLiteral{LBracketPos: pos, Elements: elems}
}

func (p *parser) parseObjectLiteral() Expr {
	pos := p.tok.Pos
	p.advance() // skip {
	p.skipNL()

	var entries []ObjectEntry
	for !p.at(RBRACE) && !p.at(EOF) {
		key := p.parseExpr(0)
		p.expect(COLON)
		value := p.parseExpr(0)
		entries = append(entries, ObjectEntry{Key: key, Value: value})
		if !p.at(RBRACE) {
			p.expect(COMMA)
			p.skipNL()
		}
	}
	p.skipNL()
	p.expect(RBRACE)

	return &ObjectLiteral{LBracePos: pos, Entries: entries}
}

// -----------------------------------------------------------------------
// If/match expressions
// -----------------------------------------------------------------------

func (p *parser) parseIfExpr() Expr {
	pos := p.tok.Pos
	p.advance() // skip 'if'

	var branches []IfExprBranch

	cond := p.parseExpr(0)
	p.expect(LBRACE)
	body := p.parseExprBody()
	p.skipNL()
	p.expect(RBRACE)
	branches = append(branches, IfExprBranch{Cond: cond, Body: body})

	var elseBody *ExprBody
	for p.at(ELSE) {
		p.advance()
		if p.at(IF) {
			p.advance()
			cond := p.parseExpr(0)
			p.expect(LBRACE)
			body := p.parseExprBody()
			p.skipNL()
			p.expect(RBRACE)
			branches = append(branches, IfExprBranch{Cond: cond, Body: body})
		} else {
			p.expect(LBRACE)
			elseBody = p.parseExprBody()
			p.skipNL()
			p.expect(RBRACE)
			break
		}
	}

	return &IfExpr{TokenPos: pos, Branches: branches, Else: elseBody}
}

func (p *parser) parseMatchExpr() Expr {
	pos := p.tok.Pos
	p.advance() // skip 'match'

	var subject Expr
	var binding string

	if !p.at(LBRACE) {
		subject = p.parseExpr(0)
		if p.at(AS) {
			p.advance()
			binding = p.expect(IDENT).Literal
		}
	}

	p.expect(LBRACE)
	p.skipNL()

	var cases []MatchCase
	for !p.at(RBRACE) && !p.at(EOF) {
		mc := p.parseMatchCaseExpr()
		cases = append(cases, mc)
		if p.at(COMMA) {
			p.advance()
		}
		p.skipNL()
	}

	p.expect(RBRACE)

	return &MatchExpr{TokenPos: pos, Subject: subject, Binding: binding, Cases: cases}
}

func (p *parser) parseMatchCaseExpr() MatchCase {
	var mc MatchCase

	if p.at(UNDERSCORE) {
		mc.Wildcard = true
		p.advance()
	} else {
		mc.Pattern = p.parseExpr(0)
	}

	p.expect(FATARROW)

	// Case body: bare expression or braced expr body.
	if p.at(LBRACE) {
		p.advance()
		body := p.parseExprBody()
		p.skipNL()
		p.expect(RBRACE)
		mc.Body = body
	} else {
		mc.Body = p.parseExpr(0)
	}

	return mc
}

// -----------------------------------------------------------------------
// Expression body
// -----------------------------------------------------------------------

func (p *parser) parseExprBody() *ExprBody {
	p.skipNL()
	body := &ExprBody{}

	for {
		// Check for output assignment in expression context — not allowed.
		if p.at(OUTPUT) && p.isOutputAssignAhead() {
			p.error(p.tok.Pos, "cannot assign to output in expression context (only $variable assignments are allowed)")
			p.recover()
			p.skipNL()
			continue
		}

		// Check for bare identifier assignment (param = value) — parameters are read-only.
		if p.at(IDENT) {
			savedTok := p.tok
			savedS := *p.s
			p.advance()
			isAssign := p.tok.Type == ASSIGN
			*p.s = savedS
			p.tok = savedTok
			if isAssign {
				p.error(p.tok.Pos, "cannot assign to identifier (parameters are read-only, use $variable for local assignments)")
				p.recover()
				p.skipNL()
				continue
			}
		}

		// Try to parse var assignment: $var[.path...] = expr
		if p.at(VAR) && p.isVarAssignAhead() {
			va := p.parseVarAssign()
			body.Assignments = append(body.Assignments, va)
			if p.at(NL) {
				p.advance()
				p.skipNL()
			}
			continue
		}
		break
	}

	// Final expression.
	body.Result = p.parseExpr(0)
	return body
}

// isOutputAssignAhead checks whether output is being used as an assignment
// target by scanning forward for `=` after `output[.path...]` or `output@[.path...]`.
func (p *parser) isOutputAssignAhead() bool {
	savedTok := p.tok
	savedS := *p.s

	p.advance() // skip OUTPUT
	if p.tok.Type == AT {
		p.advance() // skip @
	}
	// Skip path components.
	for p.tok.Type == DOT || p.tok.Type == LBRACKET || p.tok.Type == QLBRACKET || p.tok.Type == QDOT {
		if p.tok.Type == LBRACKET || p.tok.Type == QLBRACKET {
			depth := 1
			p.advance()
			for depth > 0 && p.tok.Type != EOF {
				switch p.tok.Type {
				case LBRACKET, QLBRACKET:
					depth++
				case RBRACKET:
					depth--
				}
				p.advance()
			}
		} else {
			p.advance() // skip . or ?.
			p.advance() // skip field name
		}
	}
	isAssign := p.tok.Type == ASSIGN

	*p.s = savedS
	p.tok = savedTok
	return isAssign
}

// isVarAssignAhead checks whether the current VAR token starts a var
// assignment ($var[.path...] = expr) by scanning ahead for '='.
func (p *parser) isVarAssignAhead() bool {
	savedTok := p.tok
	savedS := *p.s

	p.advance() // skip VAR
	// Skip path components.
	for p.tok.Type == DOT || p.tok.Type == LBRACKET || p.tok.Type == QLBRACKET || p.tok.Type == QDOT {
		if p.tok.Type == LBRACKET || p.tok.Type == QLBRACKET {
			// Skip past bracket contents (count depth).
			depth := 1
			p.advance()
			for depth > 0 && p.tok.Type != EOF {
				switch p.tok.Type {
				case LBRACKET, QLBRACKET:
					depth++
				case RBRACKET:
					depth--
				}
				p.advance()
			}
		} else {
			p.advance() // skip . or ?.
			p.advance() // skip field name
		}
	}
	isAssign := p.tok.Type == ASSIGN

	// Restore.
	*p.s = savedS
	p.tok = savedTok

	return isAssign
}

func (p *parser) parseVarAssign() *VarAssign {
	pos := p.tok.Pos
	name := p.tok.Literal
	p.advance() // skip VAR

	path := p.parsePathSegments()
	p.expect(ASSIGN)
	value := p.parseExpr(0)

	return &VarAssign{
		TokenPos: pos,
		Name:     name,
		Path:     path,
		Value:    value,
	}
}

// -----------------------------------------------------------------------
// Postfix parsers (left-denotation)
// -----------------------------------------------------------------------

func (p *parser) parsePostfixDot(receiver Expr) Expr {
	nullSafe := p.tok.Type == QDOT
	dotPos := p.tok.Pos
	p.advance() // skip . or ?.

	name := p.expectWord()

	// Method call: .name(args)
	if p.at(LPAREN) {
		p.advance()
		args, named := p.parseArgList()
		p.expect(RPAREN)
		return &MethodCallExpr{
			Receiver:  receiver,
			Method:    name,
			MethodPos: dotPos,
			Args:      args,
			Named:     named,
			NullSafe:  nullSafe,
		}
	}

	// Field access: .name
	return &FieldAccessExpr{
		Receiver: receiver,
		Field:    name,
		FieldPos: dotPos,
		NullSafe: nullSafe,
	}
}

func (p *parser) parsePostfixIndex(receiver Expr) Expr {
	nullSafe := p.tok.Type == QLBRACKET
	pos := p.tok.Pos
	p.advance() // skip [ or ?[

	index := p.parseExpr(0)
	p.expect(RBRACKET)

	return &IndexExpr{
		Receiver:    receiver,
		Index:       index,
		LBracketPos: pos,
		NullSafe:    nullSafe,
	}
}

// -----------------------------------------------------------------------
// Argument lists
// -----------------------------------------------------------------------

func (p *parser) parseArgList() ([]CallArg, bool) {
	if p.at(RPAREN) {
		return nil, false
	}

	// Detect named vs positional: peek for "ident :" pattern.
	named := p.isNamedArgList()

	var args []CallArg
	for {
		if named {
			nameTok := p.expect(IDENT)
			p.expect(COLON)
			value := p.parseExpr(0)
			args = append(args, CallArg{Name: nameTok.Literal, Value: value})
		} else {
			value := p.parseExpr(0)
			args = append(args, CallArg{Value: value})
		}
		if !p.at(COMMA) {
			break
		}
		p.advance() // skip comma
	}
	return args, named
}

// isNamedArgList checks if the argument list uses named arguments
// by peeking for the "ident :" pattern.
func (p *parser) isNamedArgList() bool {
	if p.tok.Type != IDENT {
		return false
	}
	savedTok := p.tok
	savedS := *p.s

	p.advance() // skip ident
	isNamed := p.tok.Type == COLON

	*p.s = savedS
	p.tok = savedTok
	return isNamed
}

// -----------------------------------------------------------------------
// Trailing comma support
// -----------------------------------------------------------------------

// Note: trailing commas in arrays, objects, and arg lists are handled
// by the parsing loops — they consume a comma then check if the closing
// delimiter follows. The grammar allows optional trailing commas:
// array := '[' [expression (',' expression)* ','?] ']'

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// FormatErrors returns the collected parse errors as a formatted string.
func FormatErrors(errs []PosError) string {
	if len(errs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, e := range errs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(e.Error())
	}
	return sb.String()
}
