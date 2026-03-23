package syntax

import "fmt"

// Resolve performs semantic analysis on a parsed program, checking for:
//   - Undeclared variable references
//   - Block-scoped variable visibility
//   - Map isolation (no input/output in map bodies)
//   - Lambda purity (no output assignments)
//   - Boolean literal cases in equality match
//   - Duplicate map names
//   - Arity mismatches for throw/deleted
//
// knownMethods and knownFunctions are sets of recognized names for
// compile-time validation.
func Resolve(prog *Program, knownMethods, knownFunctions map[string]bool) []PosError {
	r := &resolver{
		prog:           prog,
		knownMethods:   knownMethods,
		knownFunctions: knownFunctions,
	}
	r.resolve()
	return r.errors
}

type resolver struct {
	prog           *Program
	knownMethods   map[string]bool
	knownFunctions map[string]bool
	errors         []PosError
	scope          *resolveScope
	inMap          bool // true when inside a map body
}

type resolveScope struct {
	parent *resolveScope
	vars   map[string]bool // declared variables
	params map[string]bool // parameters (read-only)
}

func newResolveScope(parent *resolveScope) *resolveScope {
	return &resolveScope{
		parent: parent,
		vars:   make(map[string]bool),
		params: make(map[string]bool),
	}
}

func (s *resolveScope) isDeclared(name string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.vars[name] || cur.params[name] {
			return true
		}
	}
	return false
}

func (r *resolver) error(pos Pos, msg string) {
	r.errors = append(r.errors, PosError{Pos: pos, Msg: msg})
}

func (r *resolver) resolve() {
	// Check for duplicate map names.
	seen := make(map[string]Pos)
	for _, m := range r.prog.Maps {
		if prev, exists := seen[m.Name]; exists {
			r.error(m.TokenPos, fmt.Sprintf("duplicate map name %q (previously declared at %s)", m.Name, prev))
		}
		seen[m.Name] = m.TokenPos
	}

	// Build top-level scope.
	r.scope = newResolveScope(nil)

	// Resolve map bodies (isolated).
	for _, m := range r.prog.Maps {
		r.resolveMapDecl(m)
	}

	// Resolve top-level statements.
	for _, stmt := range r.prog.Stmts {
		r.resolveStmt(stmt)
	}
}

func (r *resolver) resolveMapDecl(m *MapDecl) {
	saved := r.scope
	savedInMap := r.inMap

	r.inMap = true
	mapScope := newResolveScope(nil) // isolated: no parent
	for _, p := range m.Params {
		if !p.Discard {
			mapScope.params[p.Name] = true
		}
	}
	r.scope = mapScope
	r.resolveExprBody(m.Body)

	r.scope = saved
	r.inMap = savedInMap
}

func (r *resolver) resolveStmt(stmt Stmt) {
	switch s := stmt.(type) {
	case *Assignment:
		r.resolveAssignment(s)
	case *IfStmt:
		r.resolveIfStmt(s)
	case *MatchStmt:
		r.resolveMatchStmt(s)
	}
}

func (r *resolver) resolveAssignment(a *Assignment) {
	r.resolveExpr(a.Value)

	// Track variable declarations.
	if a.Target.Root == AssignVar {
		if !r.scope.isDeclared(a.Target.VarName) {
			r.scope.vars[a.Target.VarName] = true
		}
	}
}

func (r *resolver) resolveIfStmt(s *IfStmt) {
	for _, branch := range s.Branches {
		r.resolveExpr(branch.Cond)
		child := newResolveScope(r.scope)
		saved := r.scope
		r.scope = child
		for _, stmt := range branch.Body {
			r.resolveStmt(stmt)
		}
		r.scope = saved
	}
	if s.Else != nil {
		child := newResolveScope(r.scope)
		saved := r.scope
		r.scope = child
		for _, stmt := range s.Else {
			r.resolveStmt(stmt)
		}
		r.scope = saved
	}
}

func (r *resolver) resolveMatchStmt(s *MatchStmt) {
	if s.Subject != nil {
		r.resolveExpr(s.Subject)
	}
	for _, c := range s.Cases {
		child := newResolveScope(r.scope)
		if s.Binding != "" {
			child.params[s.Binding] = true
		}
		saved := r.scope
		r.scope = child

		if c.Pattern != nil && !c.Wildcard {
			r.resolveExpr(c.Pattern)
		}
		if body, ok := c.Body.([]Stmt); ok {
			for _, stmt := range body {
				r.resolveStmt(stmt)
			}
		}
		r.scope = saved
	}
}

func (r *resolver) resolveExprBody(body *ExprBody) {
	if body == nil {
		return
	}
	for _, va := range body.Assignments {
		r.resolveExpr(va.Value)
		if !r.scope.isDeclared(va.Name) {
			r.scope.vars[va.Name] = true
		}
	}
	r.resolveExpr(body.Result)
}

func (r *resolver) resolveExpr(expr Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *LiteralExpr:
		// no-op
	case *InputExpr:
		if r.inMap {
			r.error(e.TokenPos, "cannot access input inside a map body")
		}
	case *InputMetaExpr:
		if r.inMap {
			r.error(e.TokenPos, "cannot access input inside a map body")
		}
	case *OutputExpr:
		if r.inMap {
			r.error(e.TokenPos, "cannot access output inside a map body")
		}
	case *OutputMetaExpr:
		if r.inMap {
			r.error(e.TokenPos, "cannot access output inside a map body")
		}
	case *VarExpr:
		if !r.scope.isDeclared(e.Name) {
			r.error(e.TokenPos, "undeclared variable $"+e.Name)
		}
	case *IdentExpr:
		// Bare identifiers must resolve to a parameter, map, or function.
		if !r.scope.isDeclared(e.Name) {
			if !r.isKnownMap(e.Name) && !r.knownFunctions[e.Name] {
				r.error(e.TokenPos, fmt.Sprintf("undeclared identifier %q", e.Name))
			}
		}
	case *BinaryExpr:
		r.resolveExpr(e.Left)
		r.resolveExpr(e.Right)
	case *UnaryExpr:
		r.resolveExpr(e.Operand)
	case *CallExpr:
		r.resolveCall(e)
	case *MethodCallExpr:
		r.resolveExpr(e.Receiver)
		for _, arg := range e.Args {
			r.resolveExpr(arg.Value)
		}
	case *FieldAccessExpr:
		r.resolveExpr(e.Receiver)
	case *IndexExpr:
		r.resolveExpr(e.Receiver)
		r.resolveExpr(e.Index)
	case *ArrayLiteral:
		for _, elem := range e.Elements {
			r.resolveExpr(elem)
		}
	case *ObjectLiteral:
		for _, entry := range e.Entries {
			r.resolveExpr(entry.Key)
			r.resolveExpr(entry.Value)
		}
	case *IfExpr:
		r.resolveIfExpr(e)
	case *MatchExpr:
		r.resolveMatchExpr(e)
	case *LambdaExpr:
		r.resolveLambda(e)
	case *PathExpr:
		for _, seg := range e.Segments {
			if seg.Index != nil {
				r.resolveExpr(seg.Index)
			}
			for _, arg := range seg.Args {
				r.resolveExpr(arg.Value)
			}
		}
	}
}

func (r *resolver) resolveCall(e *CallExpr) {
	// Validate throw() arguments at compile time.
	if e.Name == "throw" && e.Namespace == "" {
		if len(e.Args) != 1 {
			r.error(e.TokenPos, "throw() requires exactly one string argument")
		} else if lit, ok := e.Args[0].Value.(*LiteralExpr); ok {
			if lit.TokenType != STRING && lit.TokenType != RAW_STRING {
				r.error(e.TokenPos, "throw() requires a string argument")
			}
		}
	}

	// Validate deleted() takes no args.
	if e.Name == "deleted" && e.Namespace == "" && len(e.Args) != 0 {
		r.error(e.TokenPos, "deleted() takes no arguments")
	}

	for _, arg := range e.Args {
		r.resolveExpr(arg.Value)
	}
}

func (r *resolver) resolveIfExpr(e *IfExpr) {
	for _, branch := range e.Branches {
		r.resolveExpr(branch.Cond)
		child := newResolveScope(r.scope)
		saved := r.scope
		r.scope = child
		r.resolveExprBody(branch.Body)
		r.scope = saved
	}
	if e.Else != nil {
		child := newResolveScope(r.scope)
		saved := r.scope
		r.scope = child
		r.resolveExprBody(e.Else)
		r.scope = saved
	}
}

func (r *resolver) resolveMatchExpr(e *MatchExpr) {
	if e.Subject != nil {
		r.resolveExpr(e.Subject)
	}

	// Check for boolean literal cases in equality match (no 'as', has subject).
	isEqualityMatch := e.Subject != nil && e.Binding == ""

	for _, c := range e.Cases {
		if c.Pattern != nil && !c.Wildcard {
			if isEqualityMatch {
				if lit, ok := c.Pattern.(*LiteralExpr); ok {
					if lit.TokenType == TRUE || lit.TokenType == FALSE {
						r.error(lit.TokenPos, "boolean literal as case value in equality match (use 'as' for boolean conditions)")
					}
				}
			}
			child := newResolveScope(r.scope)
			if e.Binding != "" {
				child.params[e.Binding] = true
			}
			saved := r.scope
			r.scope = child
			r.resolveExpr(c.Pattern)
			r.scope = saved
		}
		switch body := c.Body.(type) {
		case Expr:
			child := newResolveScope(r.scope)
			if e.Binding != "" {
				child.params[e.Binding] = true
			}
			saved := r.scope
			r.scope = child
			r.resolveExpr(body)
			r.scope = saved
		case *ExprBody:
			child := newResolveScope(r.scope)
			if e.Binding != "" {
				child.params[e.Binding] = true
			}
			saved := r.scope
			r.scope = child
			r.resolveExprBody(body)
			r.scope = saved
		}
	}
}

func (r *resolver) resolveLambda(e *LambdaExpr) {
	child := newResolveScope(r.scope)
	for _, p := range e.Params {
		if !p.Discard {
			child.params[p.Name] = true
		}
	}
	saved := r.scope
	r.scope = child
	r.resolveExprBody(e.Body)
	r.scope = saved
}

func (r *resolver) isKnownMap(name string) bool {
	for _, m := range r.prog.Maps {
		if m.Name == name {
			return true
		}
	}
	return false
}
