package syntax

import (
	"fmt"
	"io"
	"strings"
)

// Print emits p as a formatted Bloblang V2 source string.
func Print(p *Program) string {
	var sb strings.Builder
	pr := newPrinter(&sb)
	pr.printProgram(p)
	return sb.String()
}

// PrintTo emits p to the given writer.
func PrintTo(w io.Writer, p *Program) error {
	var sb strings.Builder
	pr := newPrinter(&sb)
	pr.printProgram(p)
	_, err := io.WriteString(w, sb.String())
	return err
}

// -----------------------------------------------------------------------
// printer state
// -----------------------------------------------------------------------

type printer struct {
	w      *strings.Builder
	indent int
}

func newPrinter(w *strings.Builder) *printer {
	return &printer{w: w}
}

func (p *printer) write(s string) {
	p.w.WriteString(s)
}

func (p *printer) writeIndent() {
	for range p.indent {
		p.w.WriteString("  ")
	}
}

func (p *printer) newline() {
	p.w.WriteByte('\n')
}

// -----------------------------------------------------------------------
// Top-level
// -----------------------------------------------------------------------

func (p *printer) printProgram(prog *Program) {
	wrote := false

	// Imports first.
	for _, imp := range prog.Imports {
		if wrote {
			p.newline()
		}
		p.printLeadingTrivia(imp.Trivia().Leading)
		p.printImport(imp)
		p.printTrailingTrivia(imp.Trivia().Trailing)
		wrote = true
	}

	// Blank line before maps if we had imports — unless the first map
	// already carries a user-supplied blank line in its leading trivia.
	if len(prog.Maps) > 0 && wrote && !leadingStartsWithBlank(prog.Maps[0]) {
		p.newline()
		p.newline()
	} else if len(prog.Maps) > 0 && wrote {
		p.newline()
	}

	for i, m := range prog.Maps {
		if i > 0 {
			if leadingStartsWithBlank(m) {
				p.newline()
			} else {
				p.newline()
				p.newline()
			}
		}
		p.printLeadingTrivia(m.Trivia().Leading)
		p.printMapDecl(m)
		p.printTrailingTrivia(m.Trivia().Trailing)
		wrote = true
	}

	// Blank line before top-level statements.
	if len(prog.Stmts) > 0 && wrote && !leadingStartsWithBlank(prog.Stmts[0]) {
		p.newline()
		p.newline()
	} else if len(prog.Stmts) > 0 && wrote {
		p.newline()
	}

	for i, s := range prog.Stmts {
		if i > 0 {
			p.newline()
		}
		p.printLeadingTrivia(s.Trivia().Leading)
		p.writeIndent()
		p.printStmt(s)
		p.printTrailingTrivia(s.Trivia().Trailing)
	}

	if wrote || len(prog.Stmts) > 0 {
		p.newline()
	}
}

// printLeadingTrivia emits a node's leading trivia: blank-line markers
// become a bare newline, comments become `<indent># <text>\n`.
func (p *printer) printLeadingTrivia(tri []Trivia) {
	for _, t := range tri {
		switch t.Kind {
		case TriviaBlankLine:
			p.newline()
		case TriviaComment:
			p.writeIndent()
			p.write("#")
			p.write(t.Text)
			p.newline()
		}
	}
}

// printTrailingTrivia emits a node's trailing trivia — a same-line comment
// appended after the statement's last character (before the newline the
// caller adds).
func (p *printer) printTrailingTrivia(tri []Trivia) {
	for _, t := range tri {
		if t.Kind == TriviaComment {
			p.write("  #")
			p.write(t.Text)
		}
	}
}

// leadingStartsWithBlank reports whether a statement-like node's leading
// trivia starts with a blank-line marker, so we can suppress auto-inserted
// blank separators in favour of the user's version.
func leadingStartsWithBlank(s interface{ Trivia() *TriviaSet }) bool {
	tri := s.Trivia()
	if tri == nil || len(tri.Leading) == 0 {
		return false
	}
	return tri.Leading[0].Kind == TriviaBlankLine
}

func (p *printer) printImport(imp *ImportStmt) {
	p.write("import ")
	p.write(quoteString(imp.Path))
	p.write(" as ")
	p.write(imp.Namespace)
}

func (p *printer) printMapDecl(m *MapDecl) {
	p.writeIndent()
	p.write("map ")
	p.write(m.Name)
	p.write("(")
	for i, param := range m.Params {
		if i > 0 {
			p.write(", ")
		}
		p.printParam(param)
	}
	p.write(") {")
	p.newline()
	p.indent++
	p.printExprBody(m.Body)
	p.indent--
	p.newline()
	p.writeIndent()
	p.write("}")
}

func (p *printer) printParam(param Param) {
	if param.Discard {
		p.write("_")
		return
	}
	p.write(param.Name)
	if param.Default != nil {
		p.write(" = ")
		p.printExpr(param.Default, 0)
	}
}

// -----------------------------------------------------------------------
// Statements
// -----------------------------------------------------------------------

func (p *printer) printStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *Assignment:
		p.printAssignment(s)
	case *IfStmt:
		p.printIfStmt(s)
	case *MatchStmt:
		p.printMatchStmt(s)
	default:
		p.write(fmt.Sprintf("/* unknown stmt: %T */", stmt))
	}
}

func (p *printer) printAssignment(a *Assignment) {
	p.printAssignTarget(a.Target)
	p.write(" = ")
	p.printExpr(a.Value, 0)
}

func (p *printer) printAssignTarget(t AssignTarget) {
	switch t.Root {
	case AssignOutput:
		p.write("output")
		if t.MetaAccess {
			p.write("@")
		}
	case AssignVar:
		p.write("$")
		p.write(t.VarName)
	}
	for _, seg := range t.Path {
		p.printPathSegment(seg)
	}
}

func (p *printer) printIfStmt(s *IfStmt) {
	for i, b := range s.Branches {
		if i == 0 {
			p.write("if ")
		} else {
			p.write(" else if ")
		}
		p.printExpr(b.Cond, 0)
		p.write(" {")
		if len(b.Body) == 0 {
			p.write("}")
			continue
		}
		p.newline()
		p.indent++
		p.printNestedStmts(b.Body)
		p.indent--
		p.newline()
		p.writeIndent()
		p.write("}")
	}
	if s.Else != nil {
		p.write(" else {")
		if len(s.Else) == 0 {
			p.write("}")
			return
		}
		p.newline()
		p.indent++
		p.printNestedStmts(s.Else)
		p.indent--
		p.newline()
		p.writeIndent()
		p.write("}")
	}
}

// printNestedStmts emits a list of statements one per line, rendering the
// leading/trailing trivia on each.
func (p *printer) printNestedStmts(stmts []Stmt) {
	for j, st := range stmts {
		if j > 0 {
			p.newline()
		}
		p.printLeadingTrivia(st.Trivia().Leading)
		p.writeIndent()
		p.printStmt(st)
		p.printTrailingTrivia(st.Trivia().Trailing)
	}
}

func (p *printer) printMatchStmt(s *MatchStmt) {
	p.write("match")
	if s.Subject != nil {
		p.write(" ")
		p.printExpr(s.Subject, 0)
	}
	if s.Binding != "" {
		p.write(" as ")
		p.write(s.Binding)
	}
	p.write(" {")
	p.newline()
	p.indent++
	for i, c := range s.Cases {
		if i > 0 {
			p.newline()
		}
		p.writeIndent()
		p.printMatchCase(c, true)
	}
	p.indent--
	p.newline()
	p.writeIndent()
	p.write("}")
}

// printMatchCase prints a match case. isStmt indicates whether this is a
// statement-context match (always braced body) or an expression-context
// match (body may be bare Expr or braced ExprBody).
func (p *printer) printMatchCase(c MatchCase, isStmt bool) {
	if c.Wildcard {
		p.write("_")
	} else {
		p.printExpr(c.Pattern, 0)
	}
	p.write(" => ")

	switch body := c.Body.(type) {
	case []Stmt:
		// Statement match case.
		p.write("{")
		if len(body) == 0 {
			p.write("}")
			return
		}
		p.newline()
		p.indent++
		p.printNestedStmts(body)
		p.indent--
		p.newline()
		p.writeIndent()
		p.write("}")

	case *ExprBody:
		// Expression match case with braced body. Always emit braces — the
		// parser distinguishes between a bare-expression case and a braced
		// expr-body case, so we must preserve the AST shape.
		if len(body.Assignments) == 0 && isSimpleExpr(body.Result) {
			p.write("{ ")
			p.printExpr(body.Result, 0)
			p.write(" }")
			return
		}
		p.write("{")
		p.newline()
		p.indent++
		p.printExprBody(body)
		p.indent--
		p.newline()
		p.writeIndent()
		p.write("}")

	case Expr:
		// Bare expression body. Object literals must be wrapped in parens
		// because `{` would be parsed as a braced body introducer.
		if _, isObj := body.(*ObjectLiteral); isObj {
			p.write("(")
			p.printExpr(body, 0)
			p.write(")")
			return
		}
		p.printExpr(body, 0)

	case nil:
		// empty
	default:
		if isStmt {
			p.write(fmt.Sprintf("/* unknown stmt body: %T */", body))
		} else {
			p.write(fmt.Sprintf("/* unknown expr body: %T */", body))
		}
	}
}

// -----------------------------------------------------------------------
// Expression bodies
// -----------------------------------------------------------------------

// printExprBody prints an ExprBody indented to the current level.
// Each assignment and the result live on their own line.
func (p *printer) printExprBody(body *ExprBody) {
	for i, va := range body.Assignments {
		if i > 0 {
			p.newline()
		}
		p.printLeadingTrivia(va.Trivia().Leading)
		p.writeIndent()
		p.printVarAssign(va)
		p.printTrailingTrivia(va.Trivia().Trailing)
	}
	if len(body.Assignments) > 0 {
		p.newline()
	}
	p.writeIndent()
	p.printExpr(body.Result, 0)
}

func (p *printer) printVarAssign(va *VarAssign) {
	p.write("$")
	p.write(va.Name)
	for _, seg := range va.Path {
		p.printPathSegment(seg)
	}
	p.write(" = ")
	p.printExpr(va.Value, 0)
}

// -----------------------------------------------------------------------
// Expressions — precedence-aware
// -----------------------------------------------------------------------

// Precedence levels used to determine whether parens are needed.
// Higher = tighter binding.
const (
	precLowest     = 0
	precOr         = 1
	precAnd        = 2
	precEquality   = 3
	precComparison = 4
	precAdditive   = 5
	precMultiply   = 6
	precUnary      = 7
	precPostfix    = 8
	precAtom       = 9
)

func binaryPrec(op TokenType) int {
	switch op {
	case OR:
		return precOr
	case AND:
		return precAnd
	case EQ, NE:
		return precEquality
	case GT, GE, LT, LE:
		return precComparison
	case PLUS, MINUS:
		return precAdditive
	case STAR, SLASH, PERCENT:
		return precMultiply
	default:
		return precLowest
	}
}

// exprPrec returns the precedence of the outermost operator of expr.
// Atoms (literals, idents, input/output, var, paren-wrappable things)
// and postfix-ish nodes return precAtom.
func exprPrec(expr Expr) int {
	switch e := expr.(type) {
	case *BinaryExpr:
		return binaryPrec(e.Op)
	case *UnaryExpr:
		return precUnary
	case *IfExpr, *MatchExpr, *LambdaExpr:
		// These need parens when used inside binary/unary/postfix chains,
		// treat as low precedence.
		return precLowest
	default:
		return precAtom
	}
}

// printExpr prints expr, wrapping in parens if its precedence is lower
// than minPrec (i.e. the context binds tighter).
func (p *printer) printExpr(expr Expr, minPrec int) {
	ep := exprPrec(expr)
	needParens := ep < minPrec
	if needParens {
		p.write("(")
	}
	p.printExprRaw(expr)
	if needParens {
		p.write(")")
	}
}

func (p *printer) printExprRaw(expr Expr) {
	switch e := expr.(type) {
	case *LiteralExpr:
		p.printLiteral(e)
	case *InputExpr:
		p.write("input")
	case *InputMetaExpr:
		p.write("input@")
	case *OutputExpr:
		p.write("output")
	case *OutputMetaExpr:
		p.write("output@")
	case *VarExpr:
		p.write("$")
		p.write(e.Name)
	case *IdentExpr:
		if e.Namespace != "" {
			p.write(e.Namespace)
			p.write("::")
		}
		p.write(e.Name)
	case *BinaryExpr:
		p.printBinary(e)
	case *UnaryExpr:
		p.printUnary(e)
	case *CallExpr:
		p.printCall(e)
	case *MethodCallExpr:
		p.printMethodCall(e)
	case *FieldAccessExpr:
		p.printFieldAccess(e)
	case *IndexExpr:
		p.printIndex(e)
	case *LambdaExpr:
		p.printLambda(e)
	case *ArrayLiteral:
		p.printArray(e)
	case *ObjectLiteral:
		p.printObject(e)
	case *IfExpr:
		p.printIfExpr(e)
	case *MatchExpr:
		p.printMatchExpr(e)
	case *PathExpr:
		p.printPath(e)
	default:
		p.write(fmt.Sprintf("/* unknown expr: %T */", expr))
	}
}

// -----------------------------------------------------------------------
// Atoms / postfix
// -----------------------------------------------------------------------

func (p *printer) printLiteral(l *LiteralExpr) {
	switch l.TokenType {
	case INT, FLOAT:
		p.write(l.Value)
	case STRING:
		p.write(quoteString(l.Value))
	case RAW_STRING:
		// Prefer raw if no backticks present; otherwise fall back to quoted.
		if !strings.Contains(l.Value, "`") {
			p.write("`")
			p.write(l.Value)
			p.write("`")
		} else {
			p.write(quoteString(l.Value))
		}
	case TRUE:
		p.write("true")
	case FALSE:
		p.write("false")
	case NULL:
		p.write("null")
	default:
		p.write(l.Value)
	}
}

func (p *printer) printBinary(b *BinaryExpr) {
	myPrec := binaryPrec(b.Op)

	// Left side: left-associative, so same prec is OK on the left.
	p.printExpr(b.Left, myPrec)
	p.write(" ")
	p.write(b.Op.String())
	p.write(" ")
	// Right side needs tighter binding; use myPrec+1 to force parens around
	// a right-hand child with equal or lower precedence. (For non-associative
	// operators this also protects against parsing errors.)
	p.printExpr(b.Right, myPrec+1)
}

func (p *printer) printUnary(u *UnaryExpr) {
	p.write(u.Op.String())
	p.printExpr(u.Operand, precUnary)
}

func (p *printer) printCall(c *CallExpr) {
	if c.Namespace != "" {
		p.write(c.Namespace)
		p.write("::")
	}
	p.write(c.Name)
	p.write("(")
	p.printArgs(c.Args, c.Named)
	p.write(")")
}

func (p *printer) printArgs(args []CallArg, named bool) {
	for i, a := range args {
		if i > 0 {
			p.write(", ")
		}
		if named {
			p.write(a.Name)
			p.write(": ")
		}
		p.printExpr(a.Value, 0)
	}
}

func (p *printer) printMethodCall(m *MethodCallExpr) {
	// Receiver needs postfix-level binding.
	p.printExpr(m.Receiver, precPostfix)
	if m.NullSafe {
		p.write("?.")
	} else {
		p.write(".")
	}
	p.write(m.Method)
	p.write("(")
	p.printArgs(m.Args, m.Named)
	p.write(")")
}

func (p *printer) printFieldAccess(f *FieldAccessExpr) {
	p.printExpr(f.Receiver, precPostfix)
	if f.NullSafe {
		p.write("?.")
	} else {
		p.write(".")
	}
	p.write(fieldName(f.Field))
}

func (p *printer) printIndex(i *IndexExpr) {
	p.printExpr(i.Receiver, precPostfix)
	if i.NullSafe {
		p.write("?[")
	} else {
		p.write("[")
	}
	p.printExpr(i.Index, 0)
	p.write("]")
}

func (p *printer) printPath(e *PathExpr) {
	switch e.Root {
	case PathRootInput:
		p.write("input")
	case PathRootInputMeta:
		p.write("input@")
	case PathRootOutput:
		p.write("output")
	case PathRootOutputMeta:
		p.write("output@")
	case PathRootVar:
		p.write("$")
		p.write(e.VarName)
	}
	for _, seg := range e.Segments {
		p.printPathSegment(seg)
	}
}

func (p *printer) printPathSegment(seg PathSegment) {
	switch seg.Kind {
	case PathSegField:
		if seg.NullSafe {
			p.write("?.")
		} else {
			p.write(".")
		}
		p.write(fieldName(seg.Name))
	case PathSegIndex:
		if seg.NullSafe {
			p.write("?[")
		} else {
			p.write("[")
		}
		p.printExpr(seg.Index, 0)
		p.write("]")
	case PathSegMethod:
		if seg.NullSafe {
			p.write("?.")
		} else {
			p.write(".")
		}
		p.write(seg.Name)
		p.write("(")
		p.printArgs(seg.Args, seg.Named)
		p.write(")")
	}
}

// fieldName returns the textual form of a field name — bare word if it is
// a valid identifier or keyword word, otherwise a quoted string.
func fieldName(name string) string {
	if isWord(name) {
		return name
	}
	return quoteString(name)
}

func isWord(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		ch := s[i]
		isStart := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
		if i == 0 {
			if !isStart {
				return false
			}
			continue
		}
		isCont := isStart || (ch >= '0' && ch <= '9')
		if !isCont {
			return false
		}
	}
	return true
}

// -----------------------------------------------------------------------
// Literals
// -----------------------------------------------------------------------

// quoteString produces a Go-style double-quoted string literal whose
// contents round-trip through Bloblang's string scanner.
func quoteString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			sb.WriteString(`\\`)
		case '"':
			sb.WriteString(`\"`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04X`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// -----------------------------------------------------------------------
// Array / object
// -----------------------------------------------------------------------

func (p *printer) printArray(a *ArrayLiteral) {
	if shouldWrapArray(a) {
		p.write("[")
		p.newline()
		p.indent++
		for i, el := range a.Elements {
			p.writeIndent()
			p.printExpr(el, 0)
			if i < len(a.Elements)-1 {
				p.write(",")
			}
			p.newline()
		}
		p.indent--
		p.writeIndent()
		p.write("]")
		return
	}
	p.write("[")
	for i, el := range a.Elements {
		if i > 0 {
			p.write(", ")
		}
		p.printExpr(el, 0)
	}
	p.write("]")
}

func (p *printer) printObject(o *ObjectLiteral) {
	if len(o.Entries) == 0 {
		p.write("{}")
		return
	}
	if shouldWrapObject(o) {
		p.write("{")
		p.newline()
		p.indent++
		for _, entry := range o.Entries {
			p.writeIndent()
			p.printExpr(entry.Key, 0)
			p.write(": ")
			p.printExpr(entry.Value, 0)
			// Always emit a trailing comma so newlines between entries and the
			// closing brace are handled uniformly by the parser.
			p.write(",")
			p.newline()
		}
		p.indent--
		p.writeIndent()
		p.write("}")
		return
	}
	p.write("{")
	for i, entry := range o.Entries {
		if i > 0 {
			p.write(", ")
		}
		p.printExpr(entry.Key, 0)
		p.write(": ")
		p.printExpr(entry.Value, 0)
	}
	p.write("}")
}

// shouldWrapArray returns true if the array should be emitted multi-line.
func shouldWrapArray(a *ArrayLiteral) bool {
	if len(a.Elements) >= 3 {
		for _, el := range a.Elements {
			if hasStructured(el) {
				return true
			}
		}
	}
	for _, el := range a.Elements {
		if hasStructured(el) {
			return true
		}
	}
	return false
}

// shouldWrapObject returns true if the object should be emitted multi-line.
func shouldWrapObject(o *ObjectLiteral) bool {
	if len(o.Entries) >= 3 {
		return true
	}
	for _, entry := range o.Entries {
		if hasStructured(entry.Value) || hasStructured(entry.Key) {
			return true
		}
	}
	return false
}

// hasStructured reports whether e contains (or is) a nested array/object.
func hasStructured(e Expr) bool {
	switch v := e.(type) {
	case *ArrayLiteral:
		return len(v.Elements) > 0
	case *ObjectLiteral:
		return len(v.Entries) > 0
	}
	return false
}

// -----------------------------------------------------------------------
// Lambda / control flow
// -----------------------------------------------------------------------

func (p *printer) printLambda(l *LambdaExpr) {
	// Single-param form: name -> body (only when one non-default non-discard
	// param without parens ambiguity).
	if len(l.Params) == 1 && !l.Params[0].Discard && l.Params[0].Default == nil {
		p.write(l.Params[0].Name)
	} else if len(l.Params) == 1 && l.Params[0].Discard {
		p.write("_")
	} else {
		p.write("(")
		for i, param := range l.Params {
			if i > 0 {
				p.write(", ")
			}
			p.printParam(param)
		}
		p.write(")")
	}
	p.write(" -> ")
	// If body has variable assignments, use a block. Otherwise, bare expression.
	if len(l.Body.Assignments) == 0 {
		// If result is an object literal, we must wrap in parens to avoid
		// object-literal vs block ambiguity? Actually the parser peeks inside
		// to disambiguate — a bare { without a leading $var/output is parsed
		// as an object literal expression. So bare { key: value } is fine as
		// a lambda body.
		p.printExpr(l.Body.Result, 0)
		return
	}
	p.write("{")
	p.newline()
	p.indent++
	p.printExprBody(l.Body)
	p.indent--
	p.newline()
	p.writeIndent()
	p.write("}")
}

func (p *printer) printIfExpr(e *IfExpr) {
	for i, b := range e.Branches {
		if i == 0 {
			p.write("if ")
		} else {
			p.write(" else if ")
		}
		p.printExpr(b.Cond, 0)
		p.write(" {")
		p.printBracedExprBody(b.Body)
		p.write("}")
	}
	if e.Else != nil {
		p.write(" else {")
		p.printBracedExprBody(e.Else)
		p.write("}")
	}
}

// printBracedExprBody emits an expression body inside braces. If the body is
// a single simple result, it is printed inline with spaces. Otherwise, it is
// printed on multiple indented lines.
func (p *printer) printBracedExprBody(body *ExprBody) {
	if body == nil {
		return
	}
	if len(body.Assignments) == 0 && isSimpleExpr(body.Result) {
		p.write(" ")
		p.printExpr(body.Result, 0)
		p.write(" ")
		return
	}
	p.newline()
	p.indent++
	p.printExprBody(body)
	p.indent--
	p.newline()
	p.writeIndent()
}

// isSimpleExpr reports whether expr is simple enough to fit inline in a
// braced body.
func isSimpleExpr(expr Expr) bool {
	switch v := expr.(type) {
	case *LiteralExpr, *InputExpr, *InputMetaExpr, *OutputExpr, *OutputMetaExpr,
		*VarExpr, *IdentExpr:
		return true
	case *ArrayLiteral:
		if len(v.Elements) == 0 {
			return true
		}
		return false
	case *ObjectLiteral:
		return len(v.Entries) == 0
	case *PathExpr:
		// Simple if all segments are simple field/index/method segs.
		for _, s := range v.Segments {
			if s.Kind == PathSegMethod && len(s.Args) > 1 {
				return false
			}
		}
		return true
	case *FieldAccessExpr, *IndexExpr:
		return true
	case *MethodCallExpr:
		return len(v.Args) <= 1
	case *CallExpr:
		return len(v.Args) <= 2
	case *UnaryExpr:
		return isSimpleExpr(v.Operand)
	case *BinaryExpr:
		return isSimpleExpr(v.Left) && isSimpleExpr(v.Right)
	}
	return false
}

func (p *printer) printMatchExpr(m *MatchExpr) {
	p.write("match")
	if m.Subject != nil {
		p.write(" ")
		p.printExpr(m.Subject, 0)
	}
	if m.Binding != "" {
		p.write(" as ")
		p.write(m.Binding)
	}
	p.write(" {")
	p.newline()
	p.indent++
	for i, c := range m.Cases {
		if i > 0 {
			p.write(",")
			p.newline()
		}
		p.writeIndent()
		p.printMatchCase(c, false)
	}
	p.write(",")
	p.newline()
	p.indent--
	p.writeIndent()
	p.write("}")
}
