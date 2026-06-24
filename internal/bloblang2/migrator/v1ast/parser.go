package v1ast

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseError carries a position and a human-readable message.
type ParseError struct {
	Pos Pos
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

// Parse parses a V1 mapping source string into a *Program.
func Parse(src string) (*Program, error) {
	sc := NewScanner(src)
	toks, err := sc.All()
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	return p.parseProgram()
}

type parser struct {
	toks []Token
	pos  int

	// Trivia collection. Comment tokens are transparently skipped by peek()
	// and advance() so existing non-trivia parsing logic is unchanged; the
	// skipped comments + blank-line markers land in one of two buckets:
	//   - pendingTrailing: a comment on the same line as the preceding
	//     significant token. Drained into the just-finished statement's
	//     Trailing set.
	//   - pendingLeading: a comment on its own line, or a blank-line marker.
	//     Drained into the next statement's Leading set.
	pendingTrailing []Trivia
	pendingLeading  []Trivia
	// newlinesBuffered counts consecutive TokNewline advances since the
	// last leading-trivia decision. A run of 2+ produces a blank-line
	// marker inserted at the exact chronological moment (before the next
	// leading comment, or before returning to a significant token).
	newlinesBuffered int
	// lastSigLine tracks the line of the last consumed significant token,
	// used to classify a comment as trailing-vs-leading.
	lastSigLine int
}

//
// Token cursor helpers
//

// peek returns the next significant token, transparently skipping and
// stashing any comment tokens encountered.
func (p *parser) peek() Token {
	p.stashComments()
	return p.toks[p.pos]
}

// peekAt returns the i-th significant token ahead of the current position,
// skipping over any comment tokens in its count.
func (p *parser) peekAt(i int) Token {
	p.stashComments()
	j := p.pos
	for n := 0; n < i; n++ {
		j++
		if j >= len(p.toks) {
			return p.toks[len(p.toks)-1]
		}
		for j < len(p.toks) && p.toks[j].Kind == TokComment {
			j++
		}
	}
	if j >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[j]
}

// advance consumes the current significant token and returns it.
func (p *parser) advance() Token {
	p.stashComments()
	t := p.toks[p.pos]
	switch t.Kind {
	case TokNewline:
		p.newlinesBuffered++
	case TokEOF:
		// leave counters alone
	default:
		p.newlinesBuffered = 0
		p.lastSigLine = t.Pos.Line
	}
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// stashComments consumes TokComment tokens at the current cursor position,
// routing each into pendingTrailing (same-line as previous significant
// token, no newline between) or pendingLeading (own-line). Before stashing
// a leading comment, any pending blank-line marker is emitted so the
// trivia list stays in source order.
func (p *parser) stashComments() {
	for p.pos < len(p.toks) && p.toks[p.pos].Kind == TokComment {
		tok := p.toks[p.pos]
		tri := Trivia{Kind: TriviaComment, Text: tok.Text, Pos: tok.Pos}
		if p.newlinesBuffered == 0 && p.lastSigLine != 0 && tok.Pos.Line == p.lastSigLine {
			p.pendingTrailing = append(p.pendingTrailing, tri)
		} else {
			p.flushBlankLine()
			p.pendingLeading = append(p.pendingLeading, tri)
			// The comment itself occupies a line; subsequent newlines count
			// afresh toward a possible next blank-line marker.
			p.newlinesBuffered = 0
		}
		p.pos++
	}
}

// flushBlankLine emits a BlankLine marker into pendingLeading if
// newlinesBuffered is 2+ (meaning two or more consecutive newlines have
// passed without any content on one of the intervening lines).
// Collapses consecutive runs into a single marker and resets the counter.
func (p *parser) flushBlankLine() {
	if p.newlinesBuffered < 2 {
		return
	}
	if len(p.pendingLeading) == 0 || p.pendingLeading[len(p.pendingLeading)-1].Kind != TriviaBlankLine {
		p.pendingLeading = append(p.pendingLeading, Trivia{Kind: TriviaBlankLine})
	}
	p.newlinesBuffered = 0
}

// flushLeading returns and clears the pendingLeading buffer.
func (p *parser) flushLeading() []Trivia {
	out := p.pendingLeading
	p.pendingLeading = nil
	return out
}

// flushTrailing returns and clears the pendingTrailing buffer.
func (p *parser) flushTrailing() []Trivia {
	out := p.pendingTrailing
	p.pendingTrailing = nil
	return out
}

func (p *parser) errAt(pos Pos, format string, args ...any) *ParseError {
	return &ParseError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// skipNewlines discards newline tokens at a statement boundary. A run of
// two or more newlines produces a blank-line trivia entry, flushed either
// when a leading comment is stashed mid-run (see stashComments) or here,
// before the next significant token begins.
func (p *parser) skipNewlines() {
	for p.peek().Kind == TokNewline {
		p.advance()
	}
	p.flushBlankLine()
}

// skipInlineNewlines is used in contexts where a newline is tolerated
// (inside brackets, braces, after an operator/arrow, etc.). Does not
// record blank-line trivia — the newlines here are expression continuation
// whitespace, not structural blank lines between statements.
func (p *parser) skipInlineNewlines() {
	for p.peek().Kind == TokNewline {
		p.advance()
	}
}

// expect advances past a token of the given kind, returning an error otherwise.
func (p *parser) expect(kind TokenKind, what string) (Token, error) {
	t := p.peek()
	if t.Kind != kind {
		return t, p.errAt(t.Pos, "expected %s, got %s (%q)", what, t.Kind, t.Text)
	}
	return p.advance(), nil
}

//
// Program / statement layer
//

func (p *parser) parseProgram() (*Program, error) {
	prog := &Program{Pos: p.peek().Pos}
	p.skipNewlines()
	// If the input is a single bare expression, treat the whole thing as a
	// BareExprStmt. Heuristic: attempt statement parsing; if the first
	// top-level item is not an obvious statement start AND nothing follows,
	// interpret as bare expression.
	for p.peek().Kind != TokEOF {
		leading := p.flushLeading()
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			// Leading trivia accumulated before the statement, trailing
			// stashed during its parse (same-line comment, if any).
			tri := stmt.Trivia()
			tri.Leading = append(tri.Leading, leading...)
			tri.Trailing = append(tri.Trailing, p.flushTrailing()...)
			prog.Stmts = append(prog.Stmts, stmt)
			switch s := stmt.(type) {
			case *MapDecl:
				prog.Maps = append(prog.Maps, s)
			case *ImportStmt:
				prog.Imports = append(prog.Imports, s)
			}
		}
		// After each statement, require a newline or EOF.
		tok := p.peek()
		if tok.Kind == TokNewline {
			p.skipNewlines()
			continue
		}
		if tok.Kind == TokEOF {
			break
		}
		return nil, p.errAt(tok.Pos, "expected newline or EOF after statement, got %s", tok.Kind)
	}
	return prog, nil
}

// parseStmt dispatches on keyword / target shape.
func (p *parser) parseStmt() (Stmt, error) {
	t := p.peek()
	switch t.Kind {
	case TokIdent:
		switch t.Text {
		case "let":
			return p.parseLet()
		case "map":
			return p.parseMapDecl()
		case "import":
			return p.parseImport()
		case "from":
			return p.parseFrom()
		case "if":
			return p.parseIfStmt()
		case "meta":
			// Could be `meta <key> = v`, `meta = v`, or an expression
			// starting with meta(...).
			return p.parseMetaOrBare()
		case "root", "this":
			return p.parseAssignOrBare()
		}
		// Bare identifier — could be a target (bare path) or bare expression.
		return p.parseAssignOrBare()
	}
	// Anything else: bare expression.
	return p.parseBareExprStmt()
}

func (p *parser) parseLet() (Stmt, error) {
	tok := p.advance() // 'let'
	// Whitespace after 'let' is required, but the scanner already handles
	// spaces so we just need something after it.
	next := p.peek()
	st := &LetStmt{Pos: tok.Pos}
	switch next.Kind {
	case TokIdent:
		st.Name = next.Text
		st.NamePos = next.Pos
		p.advance()
	case TokString:
		st.Name = next.Text
		st.NameQuoted = true
		st.NamePos = next.Pos
		p.advance()
	default:
		return nil, p.errAt(next.Pos, "expected identifier or quoted string after 'let', got %s", next.Kind)
	}
	if err := p.consumeAssignEquals(); err != nil {
		return nil, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	st.Value = expr
	return st, nil
}

// consumeAssignEquals enforces whitespace-around-'=' at statement position
// (§14#68). It expects the current token to be `=` preceded by whitespace,
// and the next token to be preceded by whitespace.
func (p *parser) consumeAssignEquals() error {
	eq := p.peek()
	if eq.Kind != TokAssign {
		return p.errAt(eq.Pos, "expected '=', got %s", eq.Kind)
	}
	if !eq.PrecededBySpace && !eq.PrecededByNewline {
		return p.errAt(eq.Pos, "expected whitespace before '='")
	}
	p.advance()
	next := p.peek()
	if !next.PrecededBySpace && !next.PrecededByNewline {
		return p.errAt(next.Pos, "expected whitespace after '='")
	}
	return nil
}

func (p *parser) parseImport() (Stmt, error) {
	tok := p.advance()
	str, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ImportStmt{Path: str, Pos: tok.Pos}, nil
}

func (p *parser) parseFrom() (Stmt, error) {
	tok := p.advance()
	str, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &FromStmt{Path: str, Pos: tok.Pos}, nil
}

func (p *parser) parseMapDecl() (Stmt, error) {
	tok := p.advance() // 'map'
	nameTok := p.peek()
	if nameTok.Kind != TokIdent {
		return nil, p.errAt(nameTok.Pos, "expected map name, got %s", nameTok.Kind)
	}
	p.advance()
	if _, err := p.expect(TokLBrace, "'{'"); err != nil {
		return nil, err
	}
	p.skipNewlines()
	var body []Stmt
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		leading := p.flushLeading()
		st, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if st != nil {
			tri := st.Trivia()
			tri.Leading = append(tri.Leading, leading...)
			tri.Trailing = append(tri.Trailing, p.flushTrailing()...)
			body = append(body, st)
		}
		if p.peek().Kind == TokNewline {
			p.skipNewlines()
			continue
		}
		break
	}
	if _, err := p.expect(TokRBrace, "'}'"); err != nil {
		return nil, err
	}
	return &MapDecl{Name: nameTok.Text, NamePos: nameTok.Pos, Body: body, Pos: tok.Pos}, nil
}

func (p *parser) parseIfStmt() (Stmt, error) {
	tok := p.advance() // 'if'
	br, err := p.parseIfBranch(tok.Pos)
	if err != nil {
		return nil, err
	}
	st := &IfStmt{Pos: tok.Pos, Branches: []IfBranch{br}}

	for p.peek().Kind == TokIdent && p.peek().Text == "else" {
		elseTok := p.advance()
		if p.peek().Kind == TokIdent && p.peek().Text == "if" {
			p.advance()
			nb, err := p.parseIfBranch(elseTok.Pos)
			if err != nil {
				return nil, err
			}
			st.Branches = append(st.Branches, nb)
			continue
		}
		// final else
		if _, err := p.expect(TokLBrace, "'{'"); err != nil {
			return nil, err
		}
		body, err := p.parseStmtBlock()
		if err != nil {
			return nil, err
		}
		st.Else = body
		if _, err := p.expect(TokRBrace, "'}'"); err != nil {
			return nil, err
		}
		break
	}
	return st, nil
}

func (p *parser) parseIfBranch(pos Pos) (IfBranch, error) {
	cond, err := p.parseExpr()
	if err != nil {
		return IfBranch{}, err
	}
	if _, err := p.expect(TokLBrace, "'{'"); err != nil {
		return IfBranch{}, err
	}
	body, err := p.parseStmtBlock()
	if err != nil {
		return IfBranch{}, err
	}
	if _, err := p.expect(TokRBrace, "'}'"); err != nil {
		return IfBranch{}, err
	}
	return IfBranch{Cond: cond, Body: body, Pos: pos}, nil
}

// parseStmtBlock parses statements up to (but not including) the closing '}'.
func (p *parser) parseStmtBlock() ([]Stmt, error) {
	p.skipNewlines()
	var out []Stmt
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		leading := p.flushLeading()
		st, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if st != nil {
			tri := st.Trivia()
			tri.Leading = append(tri.Leading, leading...)
			tri.Trailing = append(tri.Trailing, p.flushTrailing()...)
			out = append(out, st)
		}
		if p.peek().Kind == TokNewline {
			p.skipNewlines()
			continue
		}
		break
	}
	return out, nil
}

// parseMetaOrBare handles `meta ... = ...`, `meta = ...`, and bare
// `meta(...)` expressions.
func (p *parser) parseMetaOrBare() (Stmt, error) {
	start := p.peek()
	// Look ahead: if we see `meta = …`, `meta <ident> = …`, or `meta
	// "<key>" = …`, it's an assignment. If we see `meta(` with no assignment
	// afterwards, it's a bare expression.
	save := p.pos
	_ = p.advance() // 'meta'
	next := p.peek()
	switch next.Kind {
	case TokAssign:
		// `meta = …`
		tgt := AssignTarget{Kind: TargetMeta, Pos: start.Pos}
		return p.finishAssignment(tgt, start.Pos)
	case TokIdent:
		// Could be `meta foo = …` or — if the next next is something else — a
		// bare expression. The ident form requires `=` eventually, so peek
		// ahead.
		if p.peekAt(1).Kind == TokAssign {
			keyTok := p.advance()
			tgt := AssignTarget{
				Kind: TargetMeta,
				Pos:  start.Pos,
				Path: []PathSegment{{Name: keyTok.Text, Pos: keyTok.Pos}},
			}
			return p.finishAssignment(tgt, start.Pos)
		}
	case TokString:
		if p.peekAt(1).Kind == TokAssign {
			keyTok := p.advance()
			tgt := AssignTarget{
				Kind: TargetMeta,
				Pos:  start.Pos,
				Path: []PathSegment{{Name: keyTok.Text, Quoted: true, Pos: keyTok.Pos}},
			}
			return p.finishAssignment(tgt, start.Pos)
		}
	}
	// Not a meta assignment — rewind and treat as bare expression.
	p.pos = save
	return p.parseBareExprStmt()
}

// parseAssignOrBare tries to parse an assignment target followed by `=
// <expr>`; if no `=` is present, parses the initial tokens as a bare
// expression.
func (p *parser) parseAssignOrBare() (Stmt, error) {
	save := p.pos
	tgt, ok := p.tryParseTarget()
	if ok {
		// Need to see `=` next.
		if p.peek().Kind == TokAssign && (p.peek().PrecededBySpace || p.peek().PrecededByNewline) {
			return p.finishAssignment(tgt, tgt.Pos)
		}
	}
	p.pos = save
	return p.parseBareExprStmt()
}

// tryParseTarget attempts to parse an assignment target. Returns (tgt, true)
// on success; on any failure the parser's position is not guaranteed to be
// reset — callers should save/restore.
func (p *parser) tryParseTarget() (AssignTarget, bool) {
	start := p.peek()
	if start.Kind != TokIdent {
		return AssignTarget{}, false
	}
	tgt := AssignTarget{Pos: start.Pos}
	switch start.Text {
	case "root":
		tgt.Kind = TargetRoot
		p.advance()
	case "this":
		tgt.Kind = TargetThis
		p.advance()
	case "meta":
		// Handled by parseMetaOrBare, but we tolerate here too.
		tgt.Kind = TargetMeta
		p.advance()
		if p.peek().Kind == TokIdent && p.peekAt(1).Kind == TokAssign {
			keyTok := p.advance()
			tgt.Path = []PathSegment{{Name: keyTok.Text, Pos: keyTok.Pos}}
		} else if p.peek().Kind == TokString && p.peekAt(1).Kind == TokAssign {
			keyTok := p.advance()
			tgt.Path = []PathSegment{{Name: keyTok.Text, Quoted: true, Pos: keyTok.Pos}}
		}
		return tgt, true
	default:
		// Bare identifier first segment — legacy root shorthand.
		tgt.Kind = TargetBare
		tgt.Path = []PathSegment{{Name: start.Text, Pos: start.Pos}}
		p.advance()
	}
	// Subsequent segments: `.ident` or `."quoted"`. Each dot must have no
	// leading whitespace.
	for p.peek().Kind == TokDot && !p.peek().PrecededBySpace && !p.peek().PrecededByNewline {
		p.advance()
		seg := p.peek()
		switch seg.Kind {
		case TokIdent:
			tgt.Path = append(tgt.Path, PathSegment{Name: seg.Text, Pos: seg.Pos})
			p.advance()
		case TokInt:
			// numeric segment (e.g. root.items.0)
			tgt.Path = append(tgt.Path, PathSegment{Name: seg.Text, Pos: seg.Pos})
			p.advance()
		case TokFloat:
			// Split merged "N.M" float into two numeric segments.
			p.advance()
			parts := strings.SplitN(seg.Text, ".", 2)
			tgt.Path = append(tgt.Path, PathSegment{Name: parts[0], Pos: seg.Pos})
			secondPos := seg.Pos
			secondPos.Column += len(parts[0]) + 1
			secondPos.Offset += len(parts[0]) + 1
			tgt.Path = append(tgt.Path, PathSegment{Name: parts[1], Pos: secondPos})
		case TokString:
			tgt.Path = append(tgt.Path, PathSegment{Name: seg.Text, Quoted: true, Pos: seg.Pos})
			p.advance()
		default:
			return tgt, false
		}
	}
	return tgt, true
}

func (p *parser) finishAssignment(tgt AssignTarget, pos Pos) (Stmt, error) {
	if err := p.consumeAssignEquals(); err != nil {
		return nil, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &Assignment{Target: tgt, Value: expr, Pos: pos}, nil
}

func (p *parser) parseBareExprStmt() (Stmt, error) {
	t := p.peek()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &BareExprStmt{Expr: expr, Pos: t.Pos}, nil
}

//
// Expression layer
//

// parseExpr parses a full expression, including binary chains.
//
// V1 parses arithmetic expressions flat and then reduces precedence in four
// passes. We mirror that: collect operand/operator sequence first, then
// reduce by precedence level.
func (p *parser) parseExpr() (Expr, error) {
	// Operand | op | operand | op | operand ...
	first, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	operands := []Expr{first}
	var ops []opTok
	for {
		t := p.peek()
		if t.PrecededByNewline {
			// A newline before a binary operator is rejected (§2.1). Exit the
			// chain.
			break
		}
		op, isOp := binaryOp(t.Kind)
		if !isOp {
			break
		}
		p.advance()
		// After a binary operator, newlines are tolerated.
		p.skipInlineNewlines()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		ops = append(ops, opTok{kind: op, pos: t.Pos})
		operands = append(operands, right)
	}
	return reducePrecedence(operands, ops), nil
}

type opTok struct {
	kind TokenKind
	pos  Pos
}

// binaryOp reports whether a token is a binary operator at expression
// position.
func binaryOp(t TokenKind) (TokenKind, bool) {
	switch t {
	case TokPlus, TokMinus, TokStar, TokSlash, TokPercent,
		TokEq, TokNeq, TokLt, TokLte, TokGt, TokGte,
		TokAnd, TokOr, TokPipe:
		return t, true
	}
	return 0, false
}

// precedence order — lower number binds tighter (matches V1).
//
//	level 1: * / % |
//	level 2: + -
//	level 3: == != < <= > >=
//	level 4: && ||  (flat; left-to-right)
func opLevel(k TokenKind) int {
	switch k {
	case TokStar, TokSlash, TokPercent, TokPipe:
		return 1
	case TokPlus, TokMinus:
		return 2
	case TokEq, TokNeq, TokLt, TokLte, TokGt, TokGte:
		return 3
	case TokAnd, TokOr:
		return 4
	}
	return 99
}

func reducePrecedence(operands []Expr, ops []opTok) Expr {
	for level := 1; level <= 4; level++ {
		operands, ops = reduceLevel(operands, ops, level)
	}
	if len(operands) != 1 {
		// Should not happen; return the first as a graceful fallback.
		return operands[0]
	}
	return operands[0]
}

func reduceLevel(operands []Expr, ops []opTok, level int) ([]Expr, []opTok) {
	newOperands := []Expr{operands[0]}
	var newOps []opTok
	for i, op := range ops {
		if opLevel(op.kind) == level {
			left := newOperands[len(newOperands)-1]
			right := operands[i+1]
			newOperands[len(newOperands)-1] = &BinaryExpr{
				Left: left, Op: op.kind, OpPos: op.pos, Right: right,
			}
		} else {
			newOps = append(newOps, op)
			newOperands = append(newOperands, operands[i+1])
		}
	}
	return newOperands, newOps
}

// parseTerm parses a prefix-unary-ed primary with method tails. V1 permits
// `- - x` (each term accepts an optional `-`); `!!x` is rejected.
func (p *parser) parseTerm() (Expr, error) {
	var negs []Token
	for p.peek().Kind == TokMinus {
		negs = append(negs, p.advance())
	}
	var not *Token
	if p.peek().Kind == TokBang {
		n := p.advance()
		// §5.1: `!!x` is a parse error.
		if p.peek().Kind == TokBang {
			return nil, p.errAt(p.peek().Pos, "stacked '!!' not permitted; write '!(!x)'")
		}
		not = &n
	}
	prim, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	prim, err = p.parseTails(prim)
	if err != nil {
		return nil, err
	}
	if not != nil {
		prim = &UnaryExpr{Op: TokBang, Operand: prim, OpPos: not.Pos}
	}
	for i := len(negs) - 1; i >= 0; i-- {
		prim = &UnaryExpr{Op: TokMinus, Operand: prim, OpPos: negs[i].Pos}
	}
	return prim, nil
}

// parseTails applies .field / .method() / .(expr) chains to `recv`.
func (p *parser) parseTails(recv Expr) (Expr, error) {
	for {
		t := p.peek()
		// Whitespace before the dot rejects the tail (§2.1).
		if t.Kind != TokDot || t.PrecededBySpace || t.PrecededByNewline {
			return recv, nil
		}
		dotTok := p.advance()
		// After '.', newlines/comments/whitespace are tolerated.
		p.skipInlineNewlines()

		next := p.peek()
		switch next.Kind {
		case TokLParen:
			// Map-expression — .(expr) or .(name -> body). Parse the inner
			// expression.
			p.advance()
			p.skipInlineNewlines()
			body, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			p.skipInlineNewlines()
			if _, err := p.expect(TokRParen, "')'"); err != nil {
				return nil, err
			}
			recv = &MapExpr{Recv: recv, Body: body, TokPos: dotTok.Pos}
		case TokIdent:
			nameTok := p.advance()
			// Method call? Look for `(` directly after (no space).
			if p.peek().Kind == TokLParen && !p.peek().PrecededBySpace && !p.peek().PrecededByNewline {
				p.advance()
				args, named, err := p.parseCallArgs()
				if err != nil {
					return nil, err
				}
				recv = &MethodCall{
					Recv: recv, Name: nameTok.Text, NamePos: nameTok.Pos,
					Args: args, Named: named,
				}
			} else {
				recv = &FieldAccess{Recv: recv, Seg: PathSegment{Name: nameTok.Text, Pos: nameTok.Pos}}
			}
		case TokInt:
			// Numeric segment — array index / stringy key.
			nameTok := p.advance()
			recv = &FieldAccess{Recv: recv, Seg: PathSegment{Name: nameTok.Text, Pos: nameTok.Pos}}
		case TokFloat:
			// The scanner merged `N.M` into a float because it's context-free,
			// but in a path-tail context N and M are independent numeric
			// segments. Split the float back into two segments.
			nameTok := p.advance()
			parts := strings.SplitN(nameTok.Text, ".", 2)
			recv = &FieldAccess{Recv: recv, Seg: PathSegment{Name: parts[0], Pos: nameTok.Pos}}
			// Second segment inherits a synthetic position one column after
			// the '.'.
			secondPos := nameTok.Pos
			secondPos.Column += len(parts[0]) + 1
			secondPos.Offset += len(parts[0]) + 1
			recv = &FieldAccess{Recv: recv, Seg: PathSegment{Name: parts[1], Pos: secondPos}}
		case TokString:
			nameTok := p.advance()
			recv = &FieldAccess{Recv: recv, Seg: PathSegment{Name: nameTok.Text, Quoted: true, Pos: nameTok.Pos}}
		default:
			return nil, p.errAt(next.Pos, "expected field, method call, or .(expr) after '.', got %s", next.Kind)
		}
	}
}

// parseCallArgs reads arguments up to and including the closing ')'. Newlines
// inside argument lists are tolerated.
func (p *parser) parseCallArgs() ([]CallArg, bool, error) {
	var args []CallArg
	named := false
	first := true
	for {
		p.skipInlineNewlines()
		if p.peek().Kind == TokRParen {
			p.advance()
			return args, named, nil
		}
		if !first {
			if _, err := p.expect(TokComma, "','"); err != nil {
				return nil, false, err
			}
			p.skipInlineNewlines()
		}
		// Tolerate trailing comma.
		if p.peek().Kind == TokRParen {
			p.advance()
			return args, named, nil
		}
		arg, err := p.parseCallArg()
		if err != nil {
			return nil, false, err
		}
		if arg.Name != "" {
			named = true
		}
		args = append(args, arg)
		first = false
	}
}

func (p *parser) parseCallArg() (CallArg, error) {
	// Named arg detection: ident ':' <expr> — but only if this ident is not
	// followed by '.' (which would start a path expression).
	t := p.peek()
	if t.Kind == TokIdent {
		next := p.peekAt(1)
		if next.Kind == TokColon {
			nameTok := p.advance()
			p.advance() // colon
			p.skipInlineNewlines()
			val, err := p.parseExpr()
			if err != nil {
				return CallArg{}, err
			}
			return CallArg{Name: nameTok.Text, Value: val, Pos: nameTok.Pos}, nil
		}
	}
	val, err := p.parseExpr()
	if err != nil {
		return CallArg{}, err
	}
	return CallArg{Value: val, Pos: t.Pos}, nil
}

// parsePrimary parses a primary expression without any tails or prefix ops.
func (p *parser) parsePrimary() (Expr, error) {
	t := p.peek()
	switch t.Kind {
	case TokInt:
		p.advance()
		n, _ := strconv.ParseInt(t.Text, 10, 64)
		return &Literal{Kind: LitInt, Raw: t.Text, Int: n, TokPos: t.Pos}, nil
	case TokFloat:
		p.advance()
		f, _ := strconv.ParseFloat(t.Text, 64)
		return &Literal{Kind: LitFloat, Raw: t.Text, Float: f, TokPos: t.Pos}, nil
	case TokString:
		p.advance()
		return &Literal{Kind: LitString, Raw: t.Text, Str: t.Text, TokPos: t.Pos}, nil
	case TokRawString:
		p.advance()
		return &Literal{Kind: LitRawString, Raw: t.Text, Str: t.Text, TokPos: t.Pos}, nil
	case TokLParen:
		p.advance()
		p.skipInlineNewlines()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipInlineNewlines()
		if _, err := p.expect(TokRParen, "')'"); err != nil {
			return nil, err
		}
		return &ParenExpr{Inner: inner, TokPos: t.Pos}, nil
	case TokLBracket:
		return p.parseArrayLit()
	case TokLBrace:
		return p.parseObjectLit()
	case TokDollar:
		p.advance()
		nameTok := p.peek()
		if nameTok.Kind != TokIdent {
			return nil, p.errAt(nameTok.Pos, "expected variable name after '$'")
		}
		if nameTok.PrecededBySpace || nameTok.PrecededByNewline {
			return nil, p.errAt(nameTok.Pos, "unexpected whitespace after '$'")
		}
		p.advance()
		return &VarRef{Name: nameTok.Text, TokPos: t.Pos}, nil
	case TokAt:
		p.advance()
		next := p.peek()
		if !next.PrecededBySpace && !next.PrecededByNewline {
			if next.Kind == TokIdent {
				p.advance()
				return &MetaRef{Name: next.Text, TokPos: t.Pos}, nil
			}
			if next.Kind == TokString {
				p.advance()
				return &MetaRef{Name: next.Text, Quoted: true, TokPos: t.Pos}, nil
			}
		}
		return &MetaRef{TokPos: t.Pos}, nil
	case TokIdent:
		return p.parseIdentPrimary()
	}
	return nil, p.errAt(t.Pos, "unexpected token %s (%q) in expression", t.Kind, t.Text)
}

func (p *parser) parseIdentPrimary() (Expr, error) {
	t := p.peek()
	switch t.Text {
	case "true":
		p.advance()
		return &Literal{Kind: LitBool, Raw: "true", Bool: true, TokPos: t.Pos}, nil
	case "false":
		p.advance()
		return &Literal{Kind: LitBool, Raw: "false", Bool: false, TokPos: t.Pos}, nil
	case "null":
		p.advance()
		return &Literal{Kind: LitNull, Raw: "null", TokPos: t.Pos}, nil
	case "this":
		p.advance()
		return &ThisExpr{TokPos: t.Pos}, nil
	case "root":
		p.advance()
		return &RootExpr{TokPos: t.Pos}, nil
	case "if":
		return p.parseIfExpr()
	case "match":
		return p.parseMatchExpr()
	case "meta":
		// Check for meta(<expr>) vs plain bare-ident meta.
		if p.peekAt(1).Kind == TokLParen && !p.peekAt(1).PrecededBySpace && !p.peekAt(1).PrecededByNewline {
			tok := p.advance() // 'meta'
			p.advance()        // '('
			p.skipInlineNewlines()
			key, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			p.skipInlineNewlines()
			if _, err := p.expect(TokRParen, "')'"); err != nil {
				return nil, err
			}
			return &MetaCall{Key: key, TokPos: tok.Pos}, nil
		}
	}

	// Check for lambda: `<ident> -> <body>`. `->` must be on the same line.
	if p.peekAt(1).Kind == TokArrow && !p.peekAt(1).PrecededByNewline {
		paramTok := p.advance()
		arrowTok := p.advance()
		// Body must be on the same line as ->.
		if p.peek().PrecededByNewline {
			return nil, p.errAt(p.peek().Pos, "lambda body must start on the same line as '->'")
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &Lambda{
			Param: paramTok.Text, Discard: paramTok.Text == "_", ParamPos: paramTok.Pos,
			Body: body, ArrowPos: arrowTok.Pos,
		}, nil
	}
	// Function call? `<ident>(` with no space.
	if p.peekAt(1).Kind == TokLParen && !p.peekAt(1).PrecededBySpace && !p.peekAt(1).PrecededByNewline {
		nameTok := p.advance()
		p.advance() // '('
		args, named, err := p.parseCallArgs()
		if err != nil {
			return nil, err
		}
		return &FunctionCall{Name: nameTok.Text, NamePos: nameTok.Pos, Args: args, Named: named}, nil
	}
	// Plain bare-ident — legacy `foo` = `this.foo` form.
	p.advance()
	return &Ident{Name: t.Text, TokPos: t.Pos}, nil
}

func (p *parser) parseArrayLit() (Expr, error) {
	open := p.advance() // '['
	arr := &ArrayLit{TokPos: open.Pos}
	p.skipInlineNewlines()
	if p.peek().Kind == TokRBracket {
		p.advance()
		return arr, nil
	}
	for {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		arr.Elems = append(arr.Elems, e)
		p.skipInlineNewlines()
		if p.peek().Kind == TokComma {
			p.advance()
			p.skipInlineNewlines()
			// trailing comma tolerance
			if p.peek().Kind == TokRBracket {
				p.advance()
				return arr, nil
			}
			continue
		}
		break
	}
	p.skipInlineNewlines()
	if _, err := p.expect(TokRBracket, "']'"); err != nil {
		return nil, err
	}
	return arr, nil
}

func (p *parser) parseObjectLit() (Expr, error) {
	open := p.advance() // '{'
	obj := &ObjectLit{TokPos: open.Pos}
	p.skipInlineNewlines()
	if p.peek().Kind == TokRBrace {
		p.advance()
		return obj, nil
	}
	for {
		entry, err := p.parseObjectEntry()
		if err != nil {
			return nil, err
		}
		obj.Entries = append(obj.Entries, entry)
		p.skipInlineNewlines()
		if p.peek().Kind == TokComma {
			p.advance()
			p.skipInlineNewlines()
			if p.peek().Kind == TokRBrace {
				p.advance()
				return obj, nil
			}
			continue
		}
		break
	}
	p.skipInlineNewlines()
	if _, err := p.expect(TokRBrace, "'}'"); err != nil {
		return nil, err
	}
	return obj, nil
}

func (p *parser) parseObjectEntry() (ObjectEntry, error) {
	// Key: quoted string OR any expression. Non-string literal keys are
	// rejected (§4.5). We accept the expression as-is and let downstream
	// tooling validate type.
	keyTok := p.peek()
	var key Expr
	if keyTok.Kind == TokString {
		p.advance()
		key = &Literal{Kind: LitString, Raw: keyTok.Text, Str: keyTok.Text, TokPos: keyTok.Pos}
	} else {
		e, err := p.parseExpr()
		if err != nil {
			return ObjectEntry{}, err
		}
		key = e
		// Reject non-string literal keys (bare int/float/bool/null
		// etc).
		if lit, ok := key.(*Literal); ok {
			switch lit.Kind {
			case LitInt, LitFloat, LitBool, LitNull, LitRawString:
				return ObjectEntry{}, &ParseError{
					Pos: lit.TokPos,
					Msg: fmt.Sprintf("object keys must be strings, got %s literal", litKindName(lit.Kind)),
				}
			}
		}
	}
	p.skipInlineNewlines()
	if _, err := p.expect(TokColon, "':'"); err != nil {
		return ObjectEntry{}, err
	}
	p.skipInlineNewlines()
	val, err := p.parseExpr()
	if err != nil {
		return ObjectEntry{}, err
	}
	return ObjectEntry{Key: key, Value: val}, nil
}

func litKindName(k LiteralKind) string {
	switch k {
	case LitNull:
		return "null"
	case LitBool:
		return "bool"
	case LitInt:
		return "int"
	case LitFloat:
		return "float"
	case LitString, LitRawString:
		return "string"
	}
	return "?"
}

func (p *parser) parseIfExpr() (Expr, error) {
	tok := p.advance() // 'if'
	ex := &IfExpr{TokPos: tok.Pos}
	br, err := p.parseIfExprBranch(tok.Pos)
	if err != nil {
		return nil, err
	}
	ex.Branches = append(ex.Branches, br)
	for p.peek().Kind == TokIdent && p.peek().Text == "else" {
		elseTok := p.advance()
		if p.peek().Kind == TokIdent && p.peek().Text == "if" {
			p.advance()
			nb, err := p.parseIfExprBranch(elseTok.Pos)
			if err != nil {
				return nil, err
			}
			ex.Branches = append(ex.Branches, nb)
			continue
		}
		if _, err := p.expect(TokLBrace, "'{'"); err != nil {
			return nil, err
		}
		p.skipInlineNewlines()
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipInlineNewlines()
		if _, err := p.expect(TokRBrace, "'}'"); err != nil {
			return nil, err
		}
		ex.Else = body
		break
	}
	return ex, nil
}

func (p *parser) parseIfExprBranch(pos Pos) (IfExprBranch, error) {
	cond, err := p.parseExpr()
	if err != nil {
		return IfExprBranch{}, err
	}
	if _, err := p.expect(TokLBrace, "'{'"); err != nil {
		return IfExprBranch{}, err
	}
	p.skipInlineNewlines()
	body, err := p.parseExpr()
	if err != nil {
		return IfExprBranch{}, err
	}
	p.skipInlineNewlines()
	if _, err := p.expect(TokRBrace, "'}'"); err != nil {
		return IfExprBranch{}, err
	}
	return IfExprBranch{Cond: cond, Body: body, Pos: pos}, nil
}

func (p *parser) parseMatchExpr() (Expr, error) {
	tok := p.advance() // 'match'
	ex := &MatchExpr{TokPos: tok.Pos}
	// Optional subject: any expression before '{'.
	if p.peek().Kind != TokLBrace {
		subj, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		ex.Subject = subj
	}
	if _, err := p.expect(TokLBrace, "'{'"); err != nil {
		return nil, err
	}
	p.skipInlineNewlines()
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		c, err := p.parseMatchCase()
		if err != nil {
			return nil, err
		}
		ex.Cases = append(ex.Cases, c)
		// Case separator: newline or comma.
		for p.peek().Kind == TokComma || p.peek().Kind == TokNewline {
			p.advance()
		}
	}
	if _, err := p.expect(TokRBrace, "'}'"); err != nil {
		return nil, err
	}
	return ex, nil
}

func (p *parser) parseMatchCase() (MatchCase, error) {
	t := p.peek()
	c := MatchCase{Pos: t.Pos}
	if t.Kind == TokIdent && t.Text == "_" && p.peekAt(1).Kind == TokFatArrow {
		p.advance()
		c.Wildcard = true
	} else {
		pat, err := p.parseExpr()
		if err != nil {
			return MatchCase{}, err
		}
		c.Pattern = pat
	}
	if _, err := p.expect(TokFatArrow, "'=>'"); err != nil {
		return MatchCase{}, err
	}
	p.skipInlineNewlines()
	body, err := p.parseExpr()
	if err != nil {
		return MatchCase{}, err
	}
	c.Body = body
	return c, nil
}
