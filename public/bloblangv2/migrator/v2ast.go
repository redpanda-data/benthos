// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"strconv"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// V2Expr is the public-API marker interface for a Bloblang V2
// expression node constructed by a custom migration rule. The
// concrete shapes a rule will commonly construct (V2MethodCallExpr,
// V2CallExpr, V2LambdaExpr, V2FieldAccessExpr, V2IndexExpr,
// V2LiteralExpr, V2IdentExpr, V2VarExpr, V2InputExpr, V2OutputExpr,
// V2InputMetaExpr, V2OutputMetaExpr, V2ArrayLiteral, V2ObjectLiteral,
// V2BinaryExpr) all satisfy it. Less common shapes are returned by
// Context.Translate as opaque carriers that round-trip back through
// the internal translator.
//
// Implementations are migrator-internal — the unexported method
// prevents downstream code from satisfying the interface with custom
// types.
type V2Expr interface {
	unwrapV2() syntax.Expr
}

// V2CallArg is one argument in a V2 method or function call.
type V2CallArg struct {
	Name  string // empty for positional
	Value V2Expr
}

func (a V2CallArg) toInternal() syntax.CallArg {
	return syntax.CallArg{Name: a.Name, Value: unwrapV2OrNil(a.Value)}
}

// V2MethodCallExpr is a V2 method call (`receiver.method(args)`).
type V2MethodCallExpr struct {
	Receiver V2Expr
	Method   string
	Args     []V2CallArg
	Named    bool
	NullSafe bool
	// Pos is optional; when zero the translator fills in from the V1
	// node currently being processed.
	Pos Pos
}

func (m *V2MethodCallExpr) unwrapV2() syntax.Expr {
	args := make([]syntax.CallArg, len(m.Args))
	for i, a := range m.Args {
		args[i] = a.toInternal()
	}
	return &syntax.MethodCallExpr{
		Receiver:  unwrapV2OrNil(m.Receiver),
		Method:    m.Method,
		MethodPos: unwrapPosOrZero(m.Pos),
		Args:      args,
		Named:     m.Named,
		NullSafe:  m.NullSafe,
	}
}

// V2CallExpr is a V2 function call (`name(args)`) or namespaced call
// (`namespace::name(args)`).
type V2CallExpr struct {
	Name      string
	Namespace string
	Args      []V2CallArg
	Named     bool
	Pos       Pos
}

func (c *V2CallExpr) unwrapV2() syntax.Expr {
	args := make([]syntax.CallArg, len(c.Args))
	for i, a := range c.Args {
		args[i] = a.toInternal()
	}
	return &syntax.CallExpr{
		TokenPos:  unwrapPosOrZero(c.Pos),
		Name:      c.Name,
		Namespace: c.Namespace,
		Args:      args,
		Named:     c.Named,
	}
}

// V2LambdaParam is one parameter of a V2 lambda.
type V2LambdaParam struct {
	Name    string
	Discard bool
	Pos     Pos
}

// V2LambdaExpr is a V2 lambda expression (`(params) -> body` or
// `name -> body`).
type V2LambdaExpr struct {
	Params []V2LambdaParam
	Body   V2Expr
	Pos    Pos
}

func (l *V2LambdaExpr) unwrapV2() syntax.Expr {
	params := make([]syntax.Param, len(l.Params))
	for i, p := range l.Params {
		params[i] = syntax.Param{
			Name:      p.Name,
			Discard:   p.Discard,
			Pos:       unwrapPosOrZero(p.Pos),
			SlotIndex: -1,
		}
	}
	return &syntax.LambdaExpr{
		TokenPos: unwrapPosOrZero(l.Pos),
		Params:   params,
		Body:     &syntax.ExprBody{Result: unwrapV2OrNil(l.Body)},
	}
}

// V2FieldAccessExpr is a V2 field access (`receiver.field` or
// `receiver?.field`).
type V2FieldAccessExpr struct {
	Receiver V2Expr
	Field    string
	NullSafe bool
	Pos      Pos
}

func (f *V2FieldAccessExpr) unwrapV2() syntax.Expr {
	return &syntax.FieldAccessExpr{
		Receiver: unwrapV2OrNil(f.Receiver),
		Field:    f.Field,
		FieldPos: unwrapPosOrZero(f.Pos),
		NullSafe: f.NullSafe,
	}
}

// V2IndexExpr is a V2 index access (`receiver[index]` or
// `receiver?[index]`).
type V2IndexExpr struct {
	Receiver V2Expr
	Index    V2Expr
	NullSafe bool
	Pos      Pos
}

func (i *V2IndexExpr) unwrapV2() syntax.Expr {
	return &syntax.IndexExpr{
		Receiver:    unwrapV2OrNil(i.Receiver),
		Index:       unwrapV2OrNil(i.Index),
		LBracketPos: unwrapPosOrZero(i.Pos),
		NullSafe:    i.NullSafe,
	}
}

// V2LiteralKind classifies V2 literal nodes. Mirrors the subset of
// syntax.TokenType that may appear in a literal expression.
type V2LiteralKind int

// V2LiteralKind values.
const (
	V2LitNull V2LiteralKind = iota
	V2LitBool
	V2LitInt
	V2LitFloat
	V2LitString
	V2LitRawString
)

// V2LiteralExpr is a V2 literal value. Only one of the typed fields
// is meaningful per Kind; the translator picks the right one.
type V2LiteralExpr struct {
	Kind  V2LiteralKind
	Bool  bool
	Int   int64
	Float float64
	Str   string
	Pos   Pos
}

func (l *V2LiteralExpr) unwrapV2() syntax.Expr {
	out := &syntax.LiteralExpr{TokenPos: unwrapPosOrZero(l.Pos)}
	switch l.Kind {
	case V2LitNull:
		out.TokenType = syntax.NULL
		out.Value = "null"
	case V2LitBool:
		if l.Bool {
			out.TokenType = syntax.TRUE
			out.Value = "true"
		} else {
			out.TokenType = syntax.FALSE
			out.Value = "false"
		}
	case V2LitInt:
		out.TokenType = syntax.INT
		out.Value = strconv.FormatInt(l.Int, 10)
	case V2LitFloat:
		out.TokenType = syntax.FLOAT
		out.Value = strconv.FormatFloat(l.Float, 'g', -1, 64)
	case V2LitString:
		out.TokenType = syntax.STRING
		out.Value = l.Str
	case V2LitRawString:
		out.TokenType = syntax.RAW_STRING
		out.Value = l.Str
	}
	return out
}

// V2IdentExpr is a bare identifier in expression position (e.g. a
// lambda parameter reference).
type V2IdentExpr struct {
	Name      string
	Namespace string
	Pos       Pos
}

func (i *V2IdentExpr) unwrapV2() syntax.Expr {
	return &syntax.IdentExpr{
		TokenPos:  unwrapPosOrZero(i.Pos),
		Namespace: i.Namespace,
		Name:      i.Name,
		SlotIndex: -1,
	}
}

// V2VarExpr is a variable reference (`$name`).
type V2VarExpr struct {
	Name string
	Pos  Pos
}

func (v *V2VarExpr) unwrapV2() syntax.Expr {
	return &syntax.VarExpr{TokenPos: unwrapPosOrZero(v.Pos), Name: v.Name, SlotIndex: -1}
}

// V2InputExpr is the `input` keyword.
type V2InputExpr struct{ Pos Pos }

func (i *V2InputExpr) unwrapV2() syntax.Expr {
	return &syntax.InputExpr{TokenPos: unwrapPosOrZero(i.Pos)}
}

// V2InputMetaExpr is `input@`.
type V2InputMetaExpr struct{ Pos Pos }

func (i *V2InputMetaExpr) unwrapV2() syntax.Expr {
	return &syntax.InputMetaExpr{TokenPos: unwrapPosOrZero(i.Pos)}
}

// V2OutputExpr is the `output` keyword.
type V2OutputExpr struct{ Pos Pos }

func (o *V2OutputExpr) unwrapV2() syntax.Expr {
	return &syntax.OutputExpr{TokenPos: unwrapPosOrZero(o.Pos)}
}

// V2OutputMetaExpr is `output@`.
type V2OutputMetaExpr struct{ Pos Pos }

func (o *V2OutputMetaExpr) unwrapV2() syntax.Expr {
	return &syntax.OutputMetaExpr{TokenPos: unwrapPosOrZero(o.Pos)}
}

// V2ArrayLiteral is a `[...]` literal.
type V2ArrayLiteral struct {
	Elements []V2Expr
	Pos      Pos
}

func (a *V2ArrayLiteral) unwrapV2() syntax.Expr {
	out := make([]syntax.Expr, len(a.Elements))
	for i, e := range a.Elements {
		out[i] = unwrapV2OrNil(e)
	}
	return &syntax.ArrayLiteral{LBracketPos: unwrapPosOrZero(a.Pos), Elements: out}
}

// V2ObjectEntry is one entry in an object literal.
type V2ObjectEntry struct {
	Key   V2Expr
	Value V2Expr
}

// V2ObjectLiteral is a `{...}` literal.
type V2ObjectLiteral struct {
	Entries []V2ObjectEntry
	Pos     Pos
}

func (o *V2ObjectLiteral) unwrapV2() syntax.Expr {
	out := make([]syntax.ObjectEntry, len(o.Entries))
	for i, e := range o.Entries {
		out[i] = syntax.ObjectEntry{Key: unwrapV2OrNil(e.Key), Value: unwrapV2OrNil(e.Value)}
	}
	return &syntax.ObjectLiteral{LBracePos: unwrapPosOrZero(o.Pos), Entries: out}
}

// V2BinaryOp identifies a V2 binary operator. The string form mirrors
// the source-syntax token: "+", "-", "==", "!=", "&&", "||", etc.
type V2BinaryOp string

// V2BinaryExpr is a binary expression.
type V2BinaryExpr struct {
	Op    V2BinaryOp
	Left  V2Expr
	Right V2Expr
	Pos   Pos
}

func (b *V2BinaryExpr) unwrapV2() syntax.Expr {
	return &syntax.BinaryExpr{
		Left:  unwrapV2OrNil(b.Left),
		Op:    binaryOpFromString(string(b.Op)),
		OpPos: unwrapPosOrZero(b.Pos),
		Right: unwrapV2OrNil(b.Right),
	}
}

// v2Opaque carries an internal V2 expression returned from
// Context.Translate when the result doesn't map to a concrete public
// shape. Rules can pass it back as the receiver / args of constructed
// V2 nodes; the migrator unwraps it transparently.
type v2Opaque struct {
	inner syntax.Expr
}

func (o *v2Opaque) unwrapV2() syntax.Expr { return o.inner }

func unwrapV2OrNil(e V2Expr) syntax.Expr {
	if e == nil {
		return nil
	}
	return e.unwrapV2()
}

func unwrapPosOrZero(p Pos) syntax.Pos {
	return syntax.Pos{Line: p.Line, Column: p.Column}
}

func wrapV2(e syntax.Expr) V2Expr {
	if e == nil {
		return nil
	}
	return &v2Opaque{inner: e}
}

// binaryOpFromString maps a source-syntax operator string to the
// internal TokenType. Used by V2BinaryExpr.unwrapV2.
func binaryOpFromString(op string) syntax.TokenType {
	switch op {
	case "+":
		return syntax.PLUS
	case "-":
		return syntax.MINUS
	case "*":
		return syntax.STAR
	case "/":
		return syntax.SLASH
	case "%":
		return syntax.PERCENT
	case "==":
		return syntax.EQ
	case "!=":
		return syntax.NE
	case "<":
		return syntax.LT
	case "<=":
		return syntax.LE
	case ">":
		return syntax.GT
	case ">=":
		return syntax.GE
	case "&&":
		return syntax.AND
	case "||":
		return syntax.OR
	}
	return 0
}
