// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// V1Expr is the public-API marker interface for a Bloblang V1
// expression node. The concrete shapes a custom rule will commonly
// inspect (V1MethodCall, V1FunctionCall, V1Lambda, V1Literal,
// V1Ident, V1ArrayLit, V1ObjectLit, V1ThisExpr, V1RootExpr,
// V1VarRef, V1FieldAccess, V1BinaryExpr) all satisfy it; less common
// shapes are exposed as an opaque carrier that is still translatable
// via Context.Translate.
//
// Implementations are migrator-internal only — the unexported method
// prevents external types from satisfying the interface, so the
// translator can rely on every V1Expr round-tripping through unwrap.
type V1Expr interface {
	unwrapV1() v1ast.Expr
}

// Pos is a public source position. Mirrors the internal v1ast.Pos /
// syntax.Pos pair (both use the same shape).
type Pos struct {
	Line   int
	Column int
}

func wrapPos(p v1ast.Pos) Pos { return Pos{Line: p.Line, Column: p.Column} }

// V1CallArg is one argument to a V1 method or function call.
type V1CallArg struct {
	// Name is empty for positional arguments.
	Name  string
	Value V1Expr
	Pos   Pos
}

// V1MethodCall is `recv.name(args)`.
type V1MethodCall struct {
	Receiver V1Expr
	Name     string
	NamePos  Pos
	Args     []V1CallArg
	// Named is true when every argument is named (name: value).
	Named bool

	inner *v1ast.MethodCall
}

func (m *V1MethodCall) unwrapV1() v1ast.Expr { return m.inner }

// V1FunctionCall is a top-level `name(args)`.
type V1FunctionCall struct {
	Name    string
	NamePos Pos
	Args    []V1CallArg
	Named   bool

	inner *v1ast.FunctionCall
}

func (f *V1FunctionCall) unwrapV1() v1ast.Expr { return f.inner }

// V1Lambda is `<name> -> <body>` or `_ -> <body>`.
type V1Lambda struct {
	Param   string
	Discard bool
	Body    V1Expr
	Pos     Pos

	inner *v1ast.Lambda
}

func (l *V1Lambda) unwrapV1() v1ast.Expr { return l.inner }

// V1LiteralKind classifies V1 literal nodes.
type V1LiteralKind int

// V1LiteralKind values mirror v1ast.LiteralKind.
const (
	V1LitNull V1LiteralKind = iota
	V1LitBool
	V1LitInt
	V1LitFloat
	V1LitString
	V1LitRawString
)

// V1Literal represents null, true/false, integers, floats, or strings.
type V1Literal struct {
	Kind  V1LiteralKind
	Raw   string
	Str   string
	Bool  bool
	Int   int64
	Float float64
	Pos   Pos

	inner *v1ast.Literal
}

func (l *V1Literal) unwrapV1() v1ast.Expr { return l.inner }

// V1Ident is a bare identifier at expression position (V1's legacy
// `foo` ≡ `this.foo` form). The translator's default rewrite turns
// these into V2 `input.foo`; a custom rule can opt out and emit
// something else.
type V1Ident struct {
	Name string
	Pos  Pos

	inner *v1ast.Ident
}

func (i *V1Ident) unwrapV1() v1ast.Expr { return i.inner }

// V1ThisExpr is the literal `this` keyword.
type V1ThisExpr struct {
	Pos Pos

	inner *v1ast.ThisExpr
}

func (t *V1ThisExpr) unwrapV1() v1ast.Expr { return t.inner }

// V1RootExpr is the literal `root` keyword at expression position.
type V1RootExpr struct {
	Pos Pos

	inner *v1ast.RootExpr
}

func (r *V1RootExpr) unwrapV1() v1ast.Expr { return r.inner }

// V1VarRef is `$name`.
type V1VarRef struct {
	Name string
	Pos  Pos

	inner *v1ast.VarRef
}

func (v *V1VarRef) unwrapV1() v1ast.Expr { return v.inner }

// V1ArrayLit is `[...]`.
type V1ArrayLit struct {
	Elems []V1Expr
	Pos   Pos

	inner *v1ast.ArrayLit
}

func (a *V1ArrayLit) unwrapV1() v1ast.Expr { return a.inner }

// V1ObjectEntry is one `key: value` member.
type V1ObjectEntry struct {
	Key   V1Expr
	Value V1Expr
}

// V1ObjectLit is `{...}`.
type V1ObjectLit struct {
	Entries []V1ObjectEntry
	Pos     Pos

	inner *v1ast.ObjectLit
}

func (o *V1ObjectLit) unwrapV1() v1ast.Expr { return o.inner }

// V1FieldAccess is `recv.<name>`.
type V1FieldAccess struct {
	Receiver V1Expr
	Field    string
	// Quoted reports whether the path segment was a quoted string literal.
	Quoted bool
	Pos    Pos

	inner *v1ast.FieldAccess
}

func (f *V1FieldAccess) unwrapV1() v1ast.Expr { return f.inner }

// V1BinaryExpr is a binary-operator expression. Op is the original
// V1 operator token text (e.g. "+", "==", "&&").
type V1BinaryExpr struct {
	Op    string
	Left  V1Expr
	Right V1Expr
	Pos   Pos

	inner *v1ast.BinaryExpr
}

func (b *V1BinaryExpr) unwrapV1() v1ast.Expr { return b.inner }

// v1Opaque is the catch-all wrapper for V1 expression shapes that
// don't have a concrete public type. A rule cannot pattern-match on
// these but can pass them to Context.Translate to get the V2 form.
type v1Opaque struct {
	inner v1ast.Expr
}

func (o *v1Opaque) unwrapV1() v1ast.Expr { return o.inner }

// wrapV1 wraps an internal v1ast.Expr into the public V1Expr surface.
// Recursion is eager: the receiver / args / body of a wrapped node
// are themselves wrapped so a rule can switch on their shape.
func wrapV1(e v1ast.Expr) V1Expr {
	if e == nil {
		return nil
	}
	switch n := e.(type) {
	case *v1ast.MethodCall:
		return wrapV1MethodCall(n)
	case *v1ast.FunctionCall:
		return wrapV1FunctionCall(n)
	case *v1ast.Lambda:
		return wrapV1Lambda(n)
	case *v1ast.Literal:
		return &V1Literal{
			Kind:  V1LiteralKind(n.Kind),
			Raw:   n.Raw,
			Str:   n.Str,
			Bool:  n.Bool,
			Int:   n.Int,
			Float: n.Float,
			Pos:   wrapPos(n.TokPos),
			inner: n,
		}
	case *v1ast.Ident:
		return &V1Ident{Name: n.Name, Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.ThisExpr:
		return &V1ThisExpr{Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.RootExpr:
		return &V1RootExpr{Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.VarRef:
		return &V1VarRef{Name: n.Name, Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.ArrayLit:
		elems := make([]V1Expr, len(n.Elems))
		for i, e := range n.Elems {
			elems[i] = wrapV1(e)
		}
		return &V1ArrayLit{Elems: elems, Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.ObjectLit:
		entries := make([]V1ObjectEntry, len(n.Entries))
		for i, e := range n.Entries {
			entries[i] = V1ObjectEntry{Key: wrapV1(e.Key), Value: wrapV1(e.Value)}
		}
		return &V1ObjectLit{Entries: entries, Pos: wrapPos(n.TokPos), inner: n}
	case *v1ast.FieldAccess:
		return &V1FieldAccess{
			Receiver: wrapV1(n.Recv),
			Field:    n.Seg.Name,
			Quoted:   n.Seg.Quoted,
			Pos:      wrapPos(n.Seg.Pos),
			inner:    n,
		}
	case *v1ast.BinaryExpr:
		return &V1BinaryExpr{
			Op:    n.Op.String(),
			Left:  wrapV1(n.Left),
			Right: wrapV1(n.Right),
			Pos:   wrapPos(n.OpPos),
			inner: n,
		}
	}
	return &v1Opaque{inner: e}
}

func wrapV1MethodCall(m *v1ast.MethodCall) *V1MethodCall {
	args := make([]V1CallArg, len(m.Args))
	for i, a := range m.Args {
		args[i] = V1CallArg{Name: a.Name, Value: wrapV1(a.Value), Pos: wrapPos(a.Pos)}
	}
	return &V1MethodCall{
		Receiver: wrapV1(m.Recv),
		Name:     m.Name,
		NamePos:  wrapPos(m.NamePos),
		Args:     args,
		Named:    m.Named,
		inner:    m,
	}
}

func wrapV1FunctionCall(f *v1ast.FunctionCall) *V1FunctionCall {
	args := make([]V1CallArg, len(f.Args))
	for i, a := range f.Args {
		args[i] = V1CallArg{Name: a.Name, Value: wrapV1(a.Value), Pos: wrapPos(a.Pos)}
	}
	return &V1FunctionCall{
		Name:    f.Name,
		NamePos: wrapPos(f.NamePos),
		Args:    args,
		Named:   f.Named,
		inner:   f,
	}
}

func wrapV1Lambda(l *v1ast.Lambda) *V1Lambda {
	return &V1Lambda{
		Param:   l.Param,
		Discard: l.Discard,
		Body:    wrapV1(l.Body),
		Pos:     wrapPos(l.ParamPos),
		inner:   l,
	}
}
