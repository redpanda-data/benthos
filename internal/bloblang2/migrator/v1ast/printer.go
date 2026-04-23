package v1ast

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Print writes the V1 source form of a *Program to w. The printed text
// round-trips via Parse back to a structurally equivalent AST (ignoring
// Pos values).
func Print(w io.Writer, p *Program) error {
	pr := &printer{w: w}
	for i, st := range p.Stmts {
		if i > 0 {
			pr.writeln("")
		}
		pr.printStmt(st, 0)
	}
	return pr.err
}

// PrintString is a convenience wrapper returning the printed output.
func PrintString(p *Program) string {
	var sb strings.Builder
	_ = Print(&sb, p)
	return sb.String()
}

type printer struct {
	w   io.Writer
	err error
}

func (pr *printer) write(s string) {
	if pr.err != nil {
		return
	}
	_, pr.err = io.WriteString(pr.w, s)
}

func (pr *printer) writeln(s string) {
	pr.write(s)
	pr.write("\n")
}

func (pr *printer) indent(level int) string {
	return strings.Repeat("  ", level)
}

func (pr *printer) printStmt(st Stmt, indent int) {
	ind := pr.indent(indent)
	pr.printLeadingTrivia(st.Trivia().Leading, ind)
	switch s := st.(type) {
	case *Assignment:
		pr.write(ind)
		pr.printTarget(s.Target)
		pr.write(" = ")
		pr.printExpr(s.Value)
	case *LetStmt:
		pr.write(ind)
		pr.write("let ")
		if s.NameQuoted {
			pr.write(strconv.Quote(s.Name))
		} else {
			pr.write(s.Name)
		}
		pr.write(" = ")
		pr.printExpr(s.Value)
	case *MapDecl:
		pr.write(ind)
		pr.write("map ")
		pr.write(s.Name)
		pr.write(" {\n")
		for _, inner := range s.Body {
			pr.printStmt(inner, indent+1)
			pr.write("\n")
		}
		pr.write(ind)
		pr.write("}")
	case *ImportStmt:
		pr.write(ind)
		pr.write("import ")
		pr.printExpr(s.Path)
	case *FromStmt:
		pr.write(ind)
		pr.write("from ")
		pr.printExpr(s.Path)
	case *IfStmt:
		pr.write(ind)
		for i, br := range s.Branches {
			if i == 0 {
				pr.write("if ")
			} else {
				pr.write(" else if ")
			}
			pr.printExpr(br.Cond)
			pr.write(" {\n")
			for _, inner := range br.Body {
				pr.printStmt(inner, indent+1)
				pr.write("\n")
			}
			pr.write(ind)
			pr.write("}")
		}
		if s.Else != nil {
			pr.write(" else {\n")
			for _, inner := range s.Else {
				pr.printStmt(inner, indent+1)
				pr.write("\n")
			}
			pr.write(ind)
			pr.write("}")
		}
	case *BareExprStmt:
		pr.write(ind)
		pr.printExpr(s.Expr)
	default:
		pr.err = fmt.Errorf("printer: unknown statement type %T", st)
	}
	for _, t := range st.Trivia().Trailing {
		if t.Kind == TriviaComment {
			pr.write("  #")
			pr.write(t.Text)
		}
	}
}

// printLeadingTrivia emits blank lines and standalone comments preceding a
// statement. Blank-line trivia becomes an empty line; comments render as
// `<indent># <text>` on their own lines.
func (pr *printer) printLeadingTrivia(tri []Trivia, ind string) {
	for _, t := range tri {
		switch t.Kind {
		case TriviaBlankLine:
			pr.write("\n")
		case TriviaComment:
			pr.write(ind)
			pr.write("#")
			pr.write(t.Text)
			pr.write("\n")
		}
	}
}

func (pr *printer) printTarget(t AssignTarget) {
	switch t.Kind {
	case TargetRoot:
		pr.write("root")
		pr.printPathSegments(t.Path)
	case TargetThis:
		pr.write("this")
		pr.printPathSegments(t.Path)
	case TargetBare:
		for i, seg := range t.Path {
			if i > 0 {
				pr.write(".")
			}
			pr.printSegment(seg)
		}
	case TargetMeta:
		pr.write("meta")
		if len(t.Path) == 1 {
			pr.write(" ")
			if t.Path[0].Quoted {
				pr.write(strconv.Quote(t.Path[0].Name))
			} else {
				pr.write(t.Path[0].Name)
			}
		}
	}
}

func (pr *printer) printPathSegments(segs []PathSegment) {
	for _, s := range segs {
		pr.write(".")
		pr.printSegment(s)
	}
}

func (pr *printer) printSegment(s PathSegment) {
	if s.Quoted {
		pr.write(strconv.Quote(s.Name))
	} else {
		pr.write(s.Name)
	}
}

func (pr *printer) printExpr(e Expr) {
	switch n := e.(type) {
	case *Literal:
		pr.printLiteral(n)
	case *Ident:
		pr.write(n.Name)
	case *ThisExpr:
		pr.write("this")
	case *RootExpr:
		pr.write("root")
	case *VarRef:
		pr.write("$")
		pr.write(n.Name)
	case *MetaRef:
		pr.write("@")
		if n.Name != "" {
			if n.Quoted {
				pr.write(strconv.Quote(n.Name))
			} else {
				pr.write(n.Name)
			}
		}
	case *MetaCall:
		pr.write("meta(")
		pr.printExpr(n.Key)
		pr.write(")")
	case *ParenExpr:
		pr.write("(")
		pr.printExpr(n.Inner)
		pr.write(")")
	case *UnaryExpr:
		switch n.Op {
		case TokBang:
			pr.write("!")
		case TokMinus:
			pr.write("-")
		}
		pr.printExpr(n.Operand)
	case *BinaryExpr:
		pr.printExpr(n.Left)
		pr.write(" ")
		pr.write(opSymbol(n.Op))
		pr.write(" ")
		pr.printExpr(n.Right)
	case *FieldAccess:
		pr.printExpr(n.Recv)
		pr.write(".")
		pr.printSegment(n.Seg)
	case *MethodCall:
		pr.printExpr(n.Recv)
		pr.write(".")
		pr.write(n.Name)
		pr.write("(")
		pr.printArgs(n.Args)
		pr.write(")")
	case *FunctionCall:
		pr.write(n.Name)
		pr.write("(")
		pr.printArgs(n.Args)
		pr.write(")")
	case *MapExpr:
		pr.printExpr(n.Recv)
		pr.write(".(")
		pr.printExpr(n.Body)
		pr.write(")")
	case *Lambda:
		if n.Discard {
			pr.write("_")
		} else {
			pr.write(n.Param)
		}
		pr.write(" -> ")
		pr.printExpr(n.Body)
	case *ArrayLit:
		pr.write("[")
		for i, el := range n.Elems {
			if i > 0 {
				pr.write(", ")
			}
			pr.printExpr(el)
		}
		pr.write("]")
	case *ObjectLit:
		pr.write("{")
		for i, ent := range n.Entries {
			if i > 0 {
				pr.write(", ")
			}
			pr.printObjectKey(ent.Key)
			pr.write(": ")
			pr.printExpr(ent.Value)
		}
		pr.write("}")
	case *IfExpr:
		for i, br := range n.Branches {
			if i == 0 {
				pr.write("if ")
			} else {
				pr.write(" else if ")
			}
			pr.printExpr(br.Cond)
			pr.write(" { ")
			pr.printExpr(br.Body)
			pr.write(" }")
		}
		if n.Else != nil {
			pr.write(" else { ")
			pr.printExpr(n.Else)
			pr.write(" }")
		}
	case *MatchExpr:
		pr.write("match")
		if n.Subject != nil {
			pr.write(" ")
			pr.printExpr(n.Subject)
		}
		pr.write(" {\n")
		for _, c := range n.Cases {
			pr.write("  ")
			if c.Wildcard {
				pr.write("_")
			} else {
				pr.printExpr(c.Pattern)
			}
			pr.write(" => ")
			pr.printExpr(c.Body)
			pr.write("\n")
		}
		pr.write("}")
	default:
		pr.err = fmt.Errorf("printer: unknown expression type %T", e)
	}
}

func (pr *printer) printLiteral(l *Literal) {
	switch l.Kind {
	case LitNull:
		pr.write("null")
	case LitBool:
		if l.Bool {
			pr.write("true")
		} else {
			pr.write("false")
		}
	case LitInt:
		// Prefer the original raw text if available so oversize literals (e.g.
		// uint64 max) round-trip without int64 overflow.
		if l.Raw != "" {
			pr.write(l.Raw)
		} else {
			pr.write(strconv.FormatInt(l.Int, 10))
		}
	case LitFloat:
		if l.Raw != "" {
			pr.write(l.Raw)
		} else {
			s := strconv.FormatFloat(l.Float, 'f', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			pr.write(s)
		}
	case LitString:
		pr.write(strconv.Quote(l.Str))
	case LitRawString:
		// Raw strings may contain characters that strconv.Quote can't
		// reproduce byte-for-byte (literal newlines). Preserve via """..."""
		// form.
		pr.write(`"""`)
		pr.write(l.Str)
		pr.write(`"""`)
	}
}

func (pr *printer) printObjectKey(k Expr) {
	// Quoted-string keys are emitted as string literals. For expressions
	// that can appear as an object key without ambiguity (anything that
	// does not start with a quoted string literal), emit verbatim. Only
	// add parens when needed.
	if lit, ok := k.(*Literal); ok && (lit.Kind == LitString || lit.Kind == LitRawString) {
		pr.write(strconv.Quote(lit.Str))
		return
	}
	if _, ok := k.(*ParenExpr); ok {
		pr.printExpr(k)
		return
	}
	if keyNeedsParens(k) {
		pr.write("(")
		pr.printExpr(k)
		pr.write(")")
		return
	}
	pr.printExpr(k)
}

// keyNeedsParens reports whether an object-literal key expression must be
// wrapped in parens. V1's object-key parser is `OneOf(QuotedString,
// queryParser)`: if the key starts with a quoted string literal token, it
// will be committed as the key and any following `.method()` / `+ x` tails
// would fail. So we only need parens when the expression's head token is a
// quoted string literal that has something after it (tails or a binary
// operator).
func keyNeedsParens(e Expr) bool {
	// Atoms without tails are safe.
	switch e.(type) {
	case *Literal, *Ident, *ThisExpr, *RootExpr, *VarRef, *MetaRef,
		*MetaCall, *FunctionCall, *ArrayLit, *ObjectLit,
		*IfExpr, *MatchExpr, *Lambda, *ParenExpr:
		return false
	}
	// Anything with a receiver that begins with a quoted string literal
	// needs parens.
	return startsWithStringLit(e)
}

func startsWithStringLit(e Expr) bool {
	switch n := e.(type) {
	case *Literal:
		return n.Kind == LitString || n.Kind == LitRawString
	case *FieldAccess:
		return startsWithStringLit(n.Recv)
	case *MethodCall:
		return startsWithStringLit(n.Recv)
	case *MapExpr:
		return startsWithStringLit(n.Recv)
	case *BinaryExpr:
		return startsWithStringLit(n.Left)
	case *UnaryExpr:
		return false
	}
	return false
}

func (pr *printer) printArgs(args []CallArg) {
	for i, a := range args {
		if i > 0 {
			pr.write(", ")
		}
		if a.Name != "" {
			pr.write(a.Name)
			pr.write(": ")
		}
		pr.printExpr(a.Value)
	}
}

func opSymbol(k TokenKind) string {
	if n, ok := tokenNames[k]; ok {
		return n
	}
	return "?"
}
