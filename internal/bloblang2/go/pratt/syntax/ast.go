package syntax

// Node is the interface implemented by all AST nodes.
type Node interface {
	nodePos() Pos
}

// Expr is the interface implemented by all expression nodes.
type Expr interface {
	Node
	exprNode()
}

// Stmt is the interface implemented by all statement nodes.
type Stmt interface {
	Node
	stmtNode()
}

//
// Top-level structures
//

// Program is the root AST node for a complete mapping.
type Program struct {
	Stmts       []Stmt                // top-level statements (assignments, if/match stmts)
	Maps        []*MapDecl            // map declarations (hoisted)
	Imports     []*ImportStmt         // import statements
	Namespaces  map[string][]*MapDecl // imported maps keyed by namespace
	MaxSlots    int                   // max variable stack slots needed (set by resolver)
	ReadsOutput bool                  // true if any expression reads output/output@ (set by resolver)
}

func (p *Program) nodePos() Pos {
	if len(p.Stmts) > 0 {
		return p.Stmts[0].nodePos()
	}
	return Pos{Line: 1, Column: 1}
}

// MapDecl is a user-defined function declaration.
type MapDecl struct {
	TokenPos   Pos
	Name       string
	Params     []Param
	Body       *ExprBody
	Namespaces map[string][]*MapDecl // namespaces available to this map (from its file's imports)
	MaxSlots   int                   // max variable stack slots needed for this map body (set by resolver)
}

func (m *MapDecl) nodePos() Pos { return m.TokenPos }

// Param is a parameter in a map or lambda declaration.
type Param struct {
	Name      string // empty for discard (_)
	Default   Expr   // nil if no default
	Discard   bool   // true for _ params
	Pos       Pos
	SlotIndex int // resolver-assigned stack slot (-1 = unassigned)
}

// ImportStmt is an import declaration.
type ImportStmt struct {
	TokenPos  Pos
	Path      string // the import path string
	Namespace string // the alias (as name)
}

func (i *ImportStmt) nodePos() Pos { return i.TokenPos }

//
// Expression body
//

// ExprBody is a sequence of variable assignments followed by a final
// expression. Used in map bodies, lambda blocks, and if/match expression arms.
type ExprBody struct {
	Assignments []*VarAssign // $var = expr (zero or more)
	Result      Expr         // the final expression (required)
}

// VarAssign is a variable assignment within an expression body.
type VarAssign struct {
	TokenPos  Pos
	Name      string        // variable name (without $)
	Path      []PathSegment // optional path components ($var.field[0] = ...)
	Value     Expr
	SlotIndex int // resolver-assigned stack slot (-1 = unassigned)
}

func (v *VarAssign) nodePos() Pos { return v.TokenPos }

//
// Statements
//

// Assignment is an assignment to output, output metadata, or a variable.
type Assignment struct {
	TokenPos Pos
	Target   AssignTarget
	Value    Expr
}

func (a *Assignment) nodePos() Pos { return a.TokenPos }
func (a *Assignment) stmtNode()    {}

// AssignTarget represents the left-hand side of an assignment.
type AssignTarget struct {
	Pos        Pos // position of the target root token
	Root       AssignTargetRoot
	VarName    string        // variable name (only for AssignVar root)
	MetaAccess bool          // true for output@ targets
	Path       []PathSegment // path components after root
	SlotIndex  int           // resolver-assigned stack slot for AssignVar (-1 = unassigned)
}

// AssignTargetRoot is the root of an assignment target.
type AssignTargetRoot int

const (
	// AssignOutput targets the output document.
	AssignOutput AssignTargetRoot = iota
	// AssignVar targets a variable.
	AssignVar
)

// IfStmt is a standalone if statement containing output assignments.
type IfStmt struct {
	TokenPos Pos
	Branches []IfBranch // first is the if, rest are else-if
	Else     []Stmt     // else body (nil if no else)
}

func (i *IfStmt) nodePos() Pos { return i.TokenPos }
func (i *IfStmt) stmtNode()    {}

// IfBranch is a single if or else-if branch in a statement.
type IfBranch struct {
	Cond Expr
	Body []Stmt
}

// MatchStmt is a standalone match statement containing output assignments.
type MatchStmt struct {
	TokenPos    Pos
	Subject     Expr        // nil for boolean match without expression
	Binding     string      // as-binding name (empty if no as)
	Cases       []MatchCase // match cases
	BindingSlot int         // resolver-assigned stack slot for as-binding (-1 = unassigned)
}

func (m *MatchStmt) nodePos() Pos { return m.TokenPos }
func (m *MatchStmt) stmtNode()    {}

// MatchCase is a single case in a match statement or expression.
type MatchCase struct {
	Pattern  Expr // nil for wildcard (_)
	Wildcard bool // true for _
	Body     any  // []Stmt (statement) or Expr (expression) or *ExprBody
}

//
// Expressions
//

// IfExpr is an if expression that returns a value.
type IfExpr struct {
	TokenPos Pos
	Branches []IfExprBranch // first is the if, rest are else-if
	Else     *ExprBody      // else body (nil = void when no branch matches)
}

func (i *IfExpr) nodePos() Pos { return i.TokenPos }
func (i *IfExpr) exprNode()    {}

// IfExprBranch is a single if or else-if branch in an expression.
type IfExprBranch struct {
	Cond Expr
	Body *ExprBody
}

// MatchExpr is a match expression that returns a value.
type MatchExpr struct {
	TokenPos    Pos
	Subject     Expr        // nil for boolean match without expression
	Binding     string      // as-binding name (empty if no as)
	Cases       []MatchCase // cases with Expr or *ExprBody bodies
	BindingSlot int         // resolver-assigned stack slot for as-binding (-1 = unassigned)
}

func (m *MatchExpr) nodePos() Pos { return m.TokenPos }
func (m *MatchExpr) exprNode()    {}

// BinaryExpr is a binary operation (a + b, a == b, etc.).
type BinaryExpr struct {
	Left  Expr
	Op    TokenType
	OpPos Pos
	Right Expr
}

func (b *BinaryExpr) nodePos() Pos { return b.Left.nodePos() }
func (b *BinaryExpr) exprNode()    {}

// UnaryExpr is a unary operation (!, -).
type UnaryExpr struct {
	Op      TokenType
	OpPos   Pos
	Operand Expr
}

func (u *UnaryExpr) nodePos() Pos { return u.OpPos }
func (u *UnaryExpr) exprNode()    {}

// CallExpr is a function call (name(args) or namespace::name(args)).
type CallExpr struct {
	TokenPos       Pos
	Name           string // function name
	Namespace      string // namespace (empty for unqualified calls)
	Args           []CallArg
	Named          bool   // true if using named arguments
	FunctionOpcode uint16 // resolver-assigned opcode for stdlib functions (0 = user map or unresolved)
}

func (c *CallExpr) nodePos() Pos { return c.TokenPos }
func (c *CallExpr) exprNode()    {}

// CallArg is a single argument in a function or method call.
type CallArg struct {
	Name  string // empty for positional args
	Value Expr
}

// MethodCallExpr is a method call on a receiver (receiver.method(args)).
type MethodCallExpr struct {
	Receiver     Expr
	Method       string
	MethodPos    Pos
	Args         []CallArg
	Named        bool   // true if using named arguments
	NullSafe     bool   // true for ?.method()
	MethodOpcode uint16 // resolver-assigned opcode for stdlib methods (0 = intrinsic or unresolved)
}

func (m *MethodCallExpr) nodePos() Pos { return m.Receiver.nodePos() }
func (m *MethodCallExpr) exprNode()    {}

// FieldAccessExpr is a field access on a receiver (receiver.field).
type FieldAccessExpr struct {
	Receiver Expr
	Field    string
	FieldPos Pos
	NullSafe bool // true for ?.field
}

func (f *FieldAccessExpr) nodePos() Pos { return f.Receiver.nodePos() }
func (f *FieldAccessExpr) exprNode()    {}

// IndexExpr is an index access on a receiver (receiver[index]).
type IndexExpr struct {
	Receiver    Expr
	Index       Expr
	LBracketPos Pos
	NullSafe    bool // true for ?[index]
}

func (i *IndexExpr) nodePos() Pos { return i.Receiver.nodePos() }
func (i *IndexExpr) exprNode()    {}

// LambdaExpr is a lambda expression (params -> body).
type LambdaExpr struct {
	TokenPos Pos
	Params   []Param
	Body     *ExprBody // single expression or block
}

func (l *LambdaExpr) nodePos() Pos { return l.TokenPos }
func (l *LambdaExpr) exprNode()    {}

// LiteralExpr is a literal value (int, float, string, bool, null).
type LiteralExpr struct {
	TokenPos  Pos
	TokenType TokenType // INT, FLOAT, STRING, RAW_STRING, TRUE, FALSE, NULL
	Value     string    // raw literal text (for INT/FLOAT) or processed string content
}

func (l *LiteralExpr) nodePos() Pos { return l.TokenPos }
func (l *LiteralExpr) exprNode()    {}

// ArrayLiteral is an array literal [elem, ...].
type ArrayLiteral struct {
	LBracketPos Pos
	Elements    []Expr
}

func (a *ArrayLiteral) nodePos() Pos { return a.LBracketPos }
func (a *ArrayLiteral) exprNode()    {}

// ObjectLiteral is an object literal {key: value, ...}.
type ObjectLiteral struct {
	LBracePos Pos
	Entries   []ObjectEntry
}

func (o *ObjectLiteral) nodePos() Pos { return o.LBracePos }
func (o *ObjectLiteral) exprNode()    {}

// ObjectEntry is a single key-value pair in an object literal.
type ObjectEntry struct {
	Key   Expr
	Value Expr
}

// InputExpr is the "input" keyword as an expression atom.
type InputExpr struct {
	TokenPos Pos
}

func (i *InputExpr) nodePos() Pos { return i.TokenPos }
func (i *InputExpr) exprNode()    {}

// InputMetaExpr is "input@" as an expression atom.
type InputMetaExpr struct {
	TokenPos Pos
}

func (i *InputMetaExpr) nodePos() Pos { return i.TokenPos }
func (i *InputMetaExpr) exprNode()    {}

// OutputExpr is the "output" keyword as an expression atom (read context).
type OutputExpr struct {
	TokenPos Pos
}

func (o *OutputExpr) nodePos() Pos { return o.TokenPos }
func (o *OutputExpr) exprNode()    {}

// OutputMetaExpr is "output@" as an expression atom (read context).
type OutputMetaExpr struct {
	TokenPos Pos
}

func (o *OutputMetaExpr) nodePos() Pos { return o.TokenPos }
func (o *OutputMetaExpr) exprNode()    {}

// VarExpr is a variable reference ($name) as an expression atom.
type VarExpr struct {
	TokenPos  Pos
	Name      string // without the $
	SlotIndex int    // resolver-assigned stack slot (-1 = unassigned)
}

func (v *VarExpr) nodePos() Pos { return v.TokenPos }
func (v *VarExpr) exprNode()    {}

// IdentExpr is a bare identifier in expression position. Resolved by the
// name resolution pass to a parameter, map name, or stdlib function.
type IdentExpr struct {
	TokenPos  Pos
	Namespace string // non-empty for qualified references (e.g., math::double)
	Name      string
	SlotIndex int // resolver-assigned stack slot when identifier is a variable/parameter (-1 = not a variable)
}

func (i *IdentExpr) nodePos() Pos { return i.TokenPos }
func (i *IdentExpr) exprNode()    {}

//
// Path expressions (produced by post-parse collapse pass)
//

// PathRoot is the root of a collapsed path expression.
type PathRoot int

const (
	// PathRootInput is the "input" root.
	PathRootInput PathRoot = iota
	// PathRootInputMeta is the "input@" root.
	PathRootInputMeta
	// PathRootOutput is the "output" root.
	PathRootOutput
	// PathRootOutputMeta is the "output@" root.
	PathRootOutputMeta
	// PathRootVar is a "$variable" root.
	PathRootVar
)

// PathExpr is a collapsed path expression: root + segments.
// Produced by the post-parse optimization pass from chains like
// FieldAccess(FieldAccess(InputExpr, "user"), "name") → PathExpr(input, ["user", "name"]).
type PathExpr struct {
	TokenPos     Pos
	Root         PathRoot
	VarName      string // only set when Root == PathRootVar
	Segments     []PathSegment
	VarSlotIndex int // resolver-assigned stack slot for PathRootVar (-1 = unassigned)
}

func (p *PathExpr) nodePos() Pos { return p.TokenPos }
func (p *PathExpr) exprNode()    {}

// PathSegment is a single segment in a path expression.
type PathSegment struct {
	Kind         PathSegmentKind
	Name         string    // for FieldAccess and MethodCall
	Index        Expr      // for Index
	Args         []CallArg // for MethodCall
	Named        bool      // for MethodCall: named arguments
	NullSafe     bool
	Pos          Pos
	MethodOpcode uint16 // resolver-assigned opcode for method segments (0 = unresolved)
}

// PathSegmentKind is the type of a path segment.
type PathSegmentKind int

const (
	// PathSegField is a field access (.name or ?.name).
	PathSegField PathSegmentKind = iota
	// PathSegIndex is an index access ([expr] or ?[expr]).
	PathSegIndex
	// PathSegMethod is a method call (.name(args) or ?.name(args)).
	PathSegMethod
)
