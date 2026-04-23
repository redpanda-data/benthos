package v1ast

// Node is the interface implemented by every AST node.
type Node interface {
	// NodePos returns the source position of this node.
	NodePos() Pos
}

// Expr is implemented by every expression node.
type Expr interface {
	Node
	exprNode()
}

// Stmt is implemented by every statement node.
//
// Every Stmt carries a TriviaSet so that comments and blank lines collected
// by the parser survive the round trip to the V1→V2 translator. Use
// Leading() / Trailing() to read; the parser sets them via the embedded
// TriviaSet on each concrete type.
type Stmt interface {
	Node
	stmtNode()
	// Trivia returns the statement's leading+trailing trivia bucket.
	// The returned pointer is the statement's own storage — mutation sticks.
	Trivia() *TriviaSet
}

// TriviaKind identifies the kind of a trivia entry.
type TriviaKind int

// Trivia kinds.
const (
	// TriviaComment is a `# ...` line comment. Text excludes the leading `#`
	// and the trailing newline, verbatim otherwise.
	TriviaComment TriviaKind = iota
	// TriviaBlankLine marks a run of two or more consecutive newlines with
	// no content between them — i.e. an empty line in the source.
	TriviaBlankLine
)

// Trivia is a single entry in a TriviaSet.
type Trivia struct {
	Kind TriviaKind
	// Text is the comment text (without `#` or trailing newline). Empty for
	// blank-line trivia.
	Text string
	Pos  Pos
}

// TriviaSet groups leading and trailing trivia for a node.
//
// Leading trivia is everything between the previous statement's end and
// this statement's start (standalone comment lines, blank lines).
// Trailing trivia is a comment that appears on the same line as the
// statement's last significant token.
type TriviaSet struct {
	Leading  []Trivia
	Trailing []Trivia
}

// Trivia returns the set itself so *TriviaSet satisfies the Stmt contract
// when embedded.
func (t *TriviaSet) Trivia() *TriviaSet { return t }

// Program is the root of a V1 mapping AST. Maps and imports live alongside
// regular statements. The original source ordering is preserved on `Stmts`
// (maps and imports also appear in Stmts in order; convenience slices Maps /
// Imports are provided for quick access).
type Program struct {
	Stmts   []Stmt
	Maps    []*MapDecl
	Imports []*ImportStmt
	Pos     Pos
}

// NodePos returns the source position of this node.
func (p *Program) NodePos() Pos { return p.Pos }

//
// Statements
//

// Assignment is `<target> = <expr>` at statement position.
type Assignment struct {
	TriviaSet
	Target AssignTarget
	Value  Expr
	Pos    Pos
}

// NodePos returns the source position of this node.
func (a *Assignment) NodePos() Pos { return a.Pos }
func (a *Assignment) stmtNode()    {}

// AssignTargetKind enumerates the shapes of assignment targets. V1 is
// restrictive on the LHS (§6.4).
type AssignTargetKind int

const (
	// TargetRoot is `root` optionally followed by path segments.
	TargetRoot AssignTargetKind = iota
	// TargetThis is `this` optionally followed by path segments. Note: the V1
	// parser accepts this and produces literal top-level "this" key behaviour
	// (quirk 72); the AST preserves it verbatim.
	TargetThis
	// TargetBare is a bare-identifier first segment followed by more path
	// segments (legacy, equivalent to root.<ident>…).
	TargetBare
	// TargetMeta is `meta` with no key (wholesale replace), or `meta <ident>`
	// / `meta "key"` for a single entry.
	TargetMeta
)

// AssignTarget is the LHS of an `=` assignment.
type AssignTarget struct {
	Kind AssignTargetKind
	// Path is the list of segments after the root keyword. For TargetBare,
	// Path[0].Name is the bare identifier (and Quoted=false). For TargetMeta,
	// Path has at most one entry (the key); it is empty for wholesale meta.
	Path []PathSegment
	Pos  Pos
}

// PathSegment is a dotted path component.
type PathSegment struct {
	Name   string // the literal segment name (or unescaped quoted string)
	Quoted bool   // true if the segment was written in quoted form
	Pos    Pos
}

// LetStmt is `let <name> = <expr>` or `let "<name>" = <expr>`.
type LetStmt struct {
	TriviaSet
	Name       string
	NameQuoted bool
	NamePos    Pos
	Value      Expr
	Pos        Pos
}

// NodePos returns the source position of this node.
func (l *LetStmt) NodePos() Pos { return l.Pos }
func (l *LetStmt) stmtNode()    {}

// MapDecl is `map <name> { ... }`.
type MapDecl struct {
	TriviaSet
	Name    string
	NamePos Pos
	Body    []Stmt
	Pos     Pos
}

// NodePos returns the source position of this node.
func (m *MapDecl) NodePos() Pos { return m.Pos }
func (m *MapDecl) stmtNode()    {}

// ImportStmt is `import "path"`.
type ImportStmt struct {
	TriviaSet
	Path Expr // string literal
	Pos  Pos
}

// NodePos returns the source position of this node.
func (i *ImportStmt) NodePos() Pos { return i.Pos }
func (i *ImportStmt) stmtNode()    {}

// FromStmt is `from "path"`.
type FromStmt struct {
	TriviaSet
	Path Expr
	Pos  Pos
}

// NodePos returns the source position of this node.
func (f *FromStmt) NodePos() Pos { return f.Pos }
func (f *FromStmt) stmtNode()    {}

// IfStmt is the statement form of `if / else if / else { ... }`.
type IfStmt struct {
	TriviaSet
	Branches []IfBranch // first is the if, rest are else-if
	Else     []Stmt     // may be nil if no else clause
	Pos      Pos
}

// NodePos returns the source position of this node.
func (i *IfStmt) NodePos() Pos { return i.Pos }
func (i *IfStmt) stmtNode()    {}

// IfBranch is one `(if|else if) cond { body }` branch.
type IfBranch struct {
	Cond Expr
	Body []Stmt
	Pos  Pos
}

// BareExprStmt is a lone expression acting as the whole mapping (shorthand
// for `root = expr`). Legal only when it is the sole statement.
type BareExprStmt struct {
	TriviaSet
	Expr Expr
	Pos  Pos
}

// NodePos returns the source position of this node.
func (b *BareExprStmt) NodePos() Pos { return b.Pos }
func (b *BareExprStmt) stmtNode()    {}

//
// Expressions
//

// LiteralKind identifies the kind of a Literal.
type LiteralKind int

// Literal kinds.
const (
	LitNull LiteralKind = iota
	LitBool
	LitInt
	LitFloat
	LitString
	LitRawString
)

// Literal represents null, true, false, integers, floats, strings.
type Literal struct {
	Kind LiteralKind
	// Raw is the original source text (for INT/FLOAT preserved as-is; for
	// strings it is the raw text of the token — quoted or triple-quoted). May
	// be empty if synthesised.
	Raw string
	// Str is the decoded string value for LitString / LitRawString. Bool/Int
	// readers can consult Raw.
	Str    string
	Bool   bool
	Int    int64
	Float  float64
	TokPos Pos
}

// NodePos returns the source position of this node.
func (l *Literal) NodePos() Pos { return l.TokPos }
func (l *Literal) exprNode()    {}

// Ident is a bare identifier at expression position (the legacy `foo` =
// `this.foo` form). The parser intentionally does NOT rewrite this; the
// migrator is free to decide.
type Ident struct {
	Name   string
	TokPos Pos
}

// NodePos returns the source position of this node.
func (i *Ident) NodePos() Pos { return i.TokPos }
func (i *Ident) exprNode()    {}

// ThisExpr is the literal `this` keyword.
type ThisExpr struct{ TokPos Pos }

// NodePos returns the source position of this node.
func (t *ThisExpr) NodePos() Pos { return t.TokPos }
func (t *ThisExpr) exprNode()    {}

// RootExpr is the literal `root` keyword at expression position.
type RootExpr struct{ TokPos Pos }

// NodePos returns the source position of this node.
func (r *RootExpr) NodePos() Pos { return r.TokPos }
func (r *RootExpr) exprNode()    {}

// VarRef is `$name`.
type VarRef struct {
	Name   string
	TokPos Pos
}

// NodePos returns the source position of this node.
func (v *VarRef) NodePos() Pos { return v.TokPos }
func (v *VarRef) exprNode()    {}

// MetaRef is `@` (whole metadata, Name empty) or `@name` / `@"name"`.
type MetaRef struct {
	Name   string // empty for bare `@`
	Quoted bool
	TokPos Pos
}

// NodePos returns the source position of this node.
func (m *MetaRef) NodePos() Pos { return m.TokPos }
func (m *MetaRef) exprNode()    {}

// BinaryExpr is a binary-operator expression.
type BinaryExpr struct {
	Left, Right Expr
	Op          TokenKind
	OpPos       Pos
}

// NodePos returns the source position of this node.
func (b *BinaryExpr) NodePos() Pos { return b.Left.NodePos() }
func (b *BinaryExpr) exprNode()    {}

// UnaryExpr is `!x` or `-x`.
type UnaryExpr struct {
	Op      TokenKind
	Operand Expr
	OpPos   Pos
}

// NodePos returns the source position of this node.
func (u *UnaryExpr) NodePos() Pos { return u.OpPos }
func (u *UnaryExpr) exprNode()    {}

// ParenExpr wraps an expression in parentheses. Preserved in the AST so the
// printer can round-trip.
type ParenExpr struct {
	Inner  Expr
	TokPos Pos
}

// NodePos returns the source position of this node.
func (p *ParenExpr) NodePos() Pos { return p.TokPos }
func (p *ParenExpr) exprNode()    {}

// FieldAccess is `recv.<name>` where Name is an identifier-class or quoted
// path segment.
type FieldAccess struct {
	Recv Expr
	Seg  PathSegment
}

// NodePos returns the source position of this node.
func (f *FieldAccess) NodePos() Pos { return f.Recv.NodePos() }
func (f *FieldAccess) exprNode()    {}

// MethodCall is `recv.name(args)`.
type MethodCall struct {
	Recv    Expr
	Name    string
	NamePos Pos
	Args    []CallArg
	Named   bool // all arguments are named (name: value)
}

// NodePos returns the source position of this node.
func (m *MethodCall) NodePos() Pos { return m.Recv.NodePos() }
func (m *MethodCall) exprNode()    {}

// FunctionCall is a top-level call `name(args)`.
type FunctionCall struct {
	Name    string
	NamePos Pos
	Args    []CallArg
	Named   bool
}

// NodePos returns the source position of this node.
func (f *FunctionCall) NodePos() Pos { return f.NamePos }
func (f *FunctionCall) exprNode()    {}

// MetaCall is `meta(<expr>)` used as an expression (read form).
type MetaCall struct {
	Key    Expr
	TokPos Pos
}

// NodePos returns the source position of this node.
func (m *MetaCall) NodePos() Pos { return m.TokPos }
func (m *MetaCall) exprNode()    {}

// CallArg is one argument, optionally named.
type CallArg struct {
	Name  string // empty for positional
	Value Expr
	Pos   Pos
}

// MapExpr is `recv.(body)` — a path-scoped subexpression that rebinds
// `this`. For the named-capture variant `recv.(name -> body)` Body is a
// Lambda.
type MapExpr struct {
	Recv   Expr
	Body   Expr
	TokPos Pos // position of the '.' before '('
}

// NodePos returns the source position of this node.
func (m *MapExpr) NodePos() Pos { return m.Recv.NodePos() }
func (m *MapExpr) exprNode()    {}

// Lambda is `<name> -> <body>` or `_ -> <body>`.
type Lambda struct {
	Param    string
	Discard  bool // true if param is `_`
	ParamPos Pos
	Body     Expr
	ArrowPos Pos
}

// NodePos returns the source position of this node.
func (l *Lambda) NodePos() Pos { return l.ParamPos }
func (l *Lambda) exprNode()    {}

// ArrayLit is `[...]`.
type ArrayLit struct {
	Elems  []Expr
	TokPos Pos // '['
}

// NodePos returns the source position of this node.
func (a *ArrayLit) NodePos() Pos { return a.TokPos }
func (a *ArrayLit) exprNode()    {}

// ObjectLit is `{...}`.
type ObjectLit struct {
	Entries []ObjectEntry
	TokPos  Pos // '{'
}

// NodePos returns the source position of this node.
func (o *ObjectLit) NodePos() Pos { return o.TokPos }
func (o *ObjectLit) exprNode()    {}

// ObjectEntry is one `key: value` member.
type ObjectEntry struct {
	Key   Expr // may be *Literal (QuotedString) or any other expression (dynamic)
	Value Expr
}

// IfExpr is the expression form of if/else if/else, where each branch body
// is a single expression.
type IfExpr struct {
	Branches []IfExprBranch
	Else     Expr // nil if no else
	TokPos   Pos
}

// NodePos returns the source position of this node.
func (i *IfExpr) NodePos() Pos { return i.TokPos }
func (i *IfExpr) exprNode()    {}

// IfExprBranch is one arm of an IfExpr.
type IfExprBranch struct {
	Cond Expr
	Body Expr
	Pos  Pos
}

// MatchExpr is `match [subject] { cases }`.
type MatchExpr struct {
	Subject Expr // nil for subject-less match
	Cases   []MatchCase
	TokPos  Pos
}

// NodePos returns the source position of this node.
func (m *MatchExpr) NodePos() Pos { return m.TokPos }
func (m *MatchExpr) exprNode()    {}

// MatchCase is one `pattern => body` arm.
type MatchCase struct {
	Pattern  Expr // nil for wildcard `_`
	Wildcard bool
	Body     Expr
	Pos      Pos
}
