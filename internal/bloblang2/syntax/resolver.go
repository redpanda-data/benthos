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
	inMethodArg    bool // true when resolving a method argument
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
	// Validate parameter list.
	r.validateParams(m.Params, m.TokenPos)

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

func (r *resolver) validateParams(params []Param, _ Pos) {
	seenDefault := false
	for _, p := range params {
		if p.Discard {
			if p.Default != nil {
				r.error(p.Pos, "discard parameter _ cannot have a default value")
			}
			continue
		}
		if p.Default != nil {
			seenDefault = true
		} else if seenDefault {
			r.error(p.Pos, "required parameter after default parameter")
		}
	}
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
	// Lambdas cannot be stored in variables or assigned to output.
	if _, ok := a.Value.(*LambdaExpr); ok {
		r.error(a.TokenPos, "lambda expressions cannot be stored in a variable or assigned to output")
	}

	// Map/function names cannot be stored in variables.
	if ident, ok := a.Value.(*IdentExpr); ok {
		if a.Target.Root == AssignVar && (r.isKnownMap(ident.Name) || r.knownFunctions[ident.Name]) {
			r.error(a.TokenPos, fmt.Sprintf("cannot store %s in a variable (it is not a value)", ident.Name))
		}
	}

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
		if _, ok := va.Value.(*LambdaExpr); ok {
			r.error(va.TokenPos, "lambda expressions cannot be stored as values")
		}
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
		// Bare identifiers must resolve to a parameter or variable.
		if !r.scope.isDeclared(e.Name) {
			if r.isKnownMap(e.Name) || r.knownFunctions[e.Name] {
				// Map/function name in expression position — only valid as
				// method argument (higher-order). We check this in the
				// method arg context; in all other positions it's an error.
				if !r.inMethodArg {
					r.error(e.TokenPos, e.Name+" is not a valid expression (call it with parentheses or pass to a method)")
				}
			} else {
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
		saved := r.inMethodArg
		r.inMethodArg = true
		for _, arg := range e.Args {
			// Check map name references passed to higher-order methods.
			if ident, ok := arg.Value.(*IdentExpr); ok {
				if m := r.findMap(ident.Name); m != nil {
					required := 0
					for _, p := range m.Params {
						if p.Default == nil && !p.Discard {
							required++
						}
					}
					if required != 1 {
						r.error(ident.TokenPos, fmt.Sprintf("arity mismatch: %s() requires %d arguments, but higher-order methods pass 1", ident.Name, required))
					}
				}
			}
			r.resolveExpr(arg.Value)
		}
		r.inMethodArg = saved
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

	// Validate named arg consistency.
	if e.Named && len(e.Args) > 0 {
		seen := make(map[string]bool)
		for _, arg := range e.Args {
			if arg.Name == "" {
				r.error(e.TokenPos, "cannot mix positional and named arguments")
				break
			}
			if seen[arg.Name] {
				r.error(e.TokenPos, fmt.Sprintf("duplicate named argument %q", arg.Name))
			}
			seen[arg.Name] = true
		}
	}

	// Check that the function/map exists.
	if e.Namespace == "" && e.Name != "throw" && e.Name != "deleted" {
		m := r.findMap(e.Name)
		if m == nil && !r.knownFunctions[e.Name] {
			r.error(e.TokenPos, fmt.Sprintf("unknown function or map %q", e.Name))
		}
		if m != nil {
			r.checkMapArity(e, m)
		}
	}

	// Namespace-qualified call: check namespace and map exist.
	if e.Namespace != "" {
		maps, nsExists := r.prog.Namespaces[e.Namespace]
		if !nsExists {
			r.error(e.TokenPos, fmt.Sprintf("unknown namespace %q", e.Namespace))
		} else {
			found := false
			for _, m := range maps {
				if m.Name == e.Name {
					found = true
					break
				}
			}
			if !found {
				r.error(e.TokenPos, fmt.Sprintf("nonexistent map %s::%s()", e.Namespace, e.Name))
			}
		}
	}

	for _, arg := range e.Args {
		r.resolveExpr(arg.Value)
	}
}

func (r *resolver) checkMapArity(e *CallExpr, m *MapDecl) {
	required := 0
	total := 0
	hasDiscard := false
	for _, p := range m.Params {
		total++
		if p.Discard {
			hasDiscard = true
			required++ // discard params still need an argument
		} else if p.Default == nil {
			required++
		}
	}

	if e.Named && hasDiscard {
		r.error(e.TokenPos, "cannot use named arguments with discard parameters")
		return
	}

	if e.Named {
		// Named args: check for unknown arg names.
		paramNames := make(map[string]bool)
		for _, p := range m.Params {
			if !p.Discard {
				paramNames[p.Name] = true
			}
		}
		for _, arg := range e.Args {
			if !paramNames[arg.Name] {
				r.error(e.TokenPos, fmt.Sprintf("unknown named argument %q", arg.Name))
			}
		}
		// Check required params are provided.
		provided := make(map[string]bool)
		for _, arg := range e.Args {
			provided[arg.Name] = true
		}
		for _, p := range m.Params {
			if p.Discard {
				continue
			}
			if !provided[p.Name] && p.Default == nil {
				r.error(e.TokenPos, fmt.Sprintf("missing required named argument %q", p.Name))
			}
		}
	} else {
		// Positional args: check count.
		if len(e.Args) < required {
			r.error(e.TokenPos, fmt.Sprintf("arity mismatch: %s() requires at least %d arguments, got %d",
				e.Name, required, len(e.Args)))
		}
		if len(e.Args) > total {
			r.error(e.TokenPos, fmt.Sprintf("arity mismatch: %s() accepts at most %d arguments, got %d",
				e.Name, total, len(e.Args)))
		}
	}
}

func (r *resolver) findMap(name string) *MapDecl {
	for _, m := range r.prog.Maps {
		if m.Name == name {
			return m
		}
	}
	return nil
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
	r.validateParams(e.Params, e.TokenPos)

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
