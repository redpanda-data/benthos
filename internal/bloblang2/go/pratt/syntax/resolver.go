package syntax

import "fmt"

// ArgFolder performs parse-time evaluation of a stdlib call's arguments
// so the runtime can skip repeat work. The folder inspects the AST args
// (typically checking for string-literal shapes) and returns a
// same-length slice of folded values, using nil for argument positions
// that aren't eligible for folding. On success the resolver writes
// each non-nil entry onto the corresponding CallArg.Folded, and the
// interpreter substitutes the folded value for the arg at runtime.
//
// Returning a non-nil error surfaces as a resolver diagnostic anchored
// at the call site. That's the right behaviour for cases like an
// invalid regex pattern — the caller learns about the problem at
// parse time rather than on first call.
type ArgFolder func(args []CallArg) (folded []any, err error)

// CallFolder is the per-call-site analogue of ArgFolder. Where ArgFolder
// precomputes individual argument values, CallFolder precomputes a dispatch
// target for the call as a whole. The returned value (when non-nil) is
// written to the call node's Prebound slot and consulted by the interpreter
// before normal dispatch.
//
// The primary consumer is the public plugin surface: when every argument
// at a call site is a literal, the plugin's constructor can be invoked once
// at parse time and the resulting closure cached on the AST, eliminating
// per-call constructor overhead. Runs after ArgFolder so it can see already-
// folded values.
type CallFolder func(args []CallArg) (prebound any, err error)

// FunctionInfo carries compile-time metadata about a stdlib function.
type FunctionInfo struct {
	// Required is the number of required parameters.
	Required int
	// Total is the total number of parameters (required + optional).
	// -1 means no arity checking (variadic or handled at runtime).
	Total int
	// ArgFolder, if set, is invoked by the resolver to precompute
	// literal arguments (see ArgFolder docs).
	ArgFolder ArgFolder
	// CallFolder, if set, is invoked by the resolver to precompute a
	// call-site dispatch target (see CallFolder docs).
	CallFolder CallFolder
}

// MethodInfo carries compile-time metadata about a stdlib method.
type MethodInfo struct {
	// Required is the number of required parameters.
	Required int
	// Total is the total number of parameters (required + optional).
	// -1 means no arity checking (params not declared, validated at runtime).
	Total int
	// Params is per-parameter metadata, parallel to declared positions.
	// Empty when the method doesn't declare params (variadic — e.g. .sort);
	// in that case AcceptsLambda is the method-level fallback.
	Params []MethodParamInfo
	// AcceptsLambda is the method-level fallback used when Params is empty.
	AcceptsLambda bool
	// ArgFolder, if set, is invoked by the resolver to precompute
	// literal arguments (see ArgFolder docs).
	ArgFolder ArgFolder
	// CallFolder, if set, is invoked by the resolver to precompute a
	// call-site dispatch target (see CallFolder docs).
	CallFolder CallFolder
}

// MethodParamInfo carries compile-time metadata about one method parameter.
type MethodParamInfo struct {
	Name          string
	HasDefault    bool
	AcceptsLambda bool
}

// ParamAcceptsLambda reports whether a lambda is accepted at the given
// argument position. For named args, name selects the param; for positional
// args, position is used.
func (mi MethodInfo) ParamAcceptsLambda(position int, name string) bool {
	if len(mi.Params) == 0 {
		return mi.AcceptsLambda
	}
	if name != "" {
		for _, p := range mi.Params {
			if p.Name == name {
				return p.AcceptsLambda
			}
		}
		return false
	}
	if position < 0 || position >= len(mi.Params) {
		return false
	}
	return mi.Params[position].AcceptsLambda
}

// ResolveOptions configures the semantic analysis pass.
type ResolveOptions struct {
	// Methods maps method names to their compile-time arity metadata.
	Methods map[string]MethodInfo
	// Functions maps function names to their compile-time arity metadata.
	Functions map[string]FunctionInfo
	// MethodOpcodes maps method names to opcode IDs for runtime dispatch.
	// Nil to skip opcode annotation (e.g. LSP diagnostics-only mode).
	MethodOpcodes map[string]uint16
	// FunctionOpcodes maps function names to opcode IDs for runtime dispatch.
	// Nil to skip opcode annotation.
	FunctionOpcodes map[string]uint16
}

// Resolve performs semantic analysis on a parsed program, checking for:
//   - Undeclared variable references
//   - Block-scoped variable visibility
//   - Map isolation (no input/output in map bodies)
//   - Lambda purity (no output assignments)
//   - Boolean literal cases in equality match
//   - Duplicate map names
//   - Function arity mismatches
//
// When opcode maps are provided in opts, AST nodes are annotated with
// compile-time opcode IDs for fast runtime dispatch.
func Resolve(prog *Program, opts ResolveOptions) []PosError {
	r := &resolver{
		prog:            prog,
		knownMethods:    opts.Methods,
		knownFunctions:  opts.Functions,
		methodOpcodes:   opts.MethodOpcodes,
		functionOpcodes: opts.FunctionOpcodes,
	}
	r.resolve()
	return r.errors
}

type resolver struct {
	prog            *Program
	knownMethods    map[string]MethodInfo
	knownFunctions  map[string]FunctionInfo
	methodOpcodes   map[string]uint16 // nil = skip annotation
	functionOpcodes map[string]uint16 // nil = skip annotation
	errors          []PosError
	scope           *resolveScope
	inMap           bool // true when inside a map body
	inMethodArg     bool // true when resolving a method argument
	maxSlots        int  // high-water mark for the current scope tree (program or map)
}

// trackSlots updates the high-water mark for the current scope.
// trackSlots updates the high-water mark for the current scope and
// propagates the child scope's slot usage back to the parent to prevent
// slot collisions between child expressions and subsequent parent allocations.
func (r *resolver) trackSlots() {
	if r.scope.nextSlot > r.maxSlots {
		r.maxSlots = r.scope.nextSlot
	}
	// Propagate child's nextSlot to parent so parent doesn't reuse
	// slots that were allocated inside the child scope.
	if r.scope.parent != nil && r.scope.nextSlot > r.scope.parent.nextSlot {
		r.scope.parent.nextSlot = r.scope.nextSlot
	}
}

// scopeMode determines how variable assignment interacts with outer scopes.
type resolveScopeMode int

const (
	// resolveScopeStatement: assigning to an existing outer variable targets
	// the ancestor's slot. New variables are block-scoped.
	resolveScopeStatement resolveScopeMode = iota
	// resolveScopeExpression: assignment always shadows (writes locally).
	resolveScopeExpression
)

type resolveScope struct {
	parent   *resolveScope
	vars     map[string]int // declared variables → slot index
	params   map[string]int // parameters → slot index
	nextSlot int            // next available slot in this scope tree
	mode     resolveScopeMode
}

func newResolveScope(parent *resolveScope, mode resolveScopeMode) *resolveScope {
	// Slot 0 is reserved (Go zero-value for int fields on AST nodes means
	// "unresolved"), so root scopes start allocating from slot 1.
	nextSlot := 1
	if parent != nil {
		nextSlot = parent.nextSlot
	}
	return &resolveScope{
		parent:   parent,
		vars:     make(map[string]int),
		params:   make(map[string]int),
		nextSlot: nextSlot,
		mode:     mode,
	}
}

func (s *resolveScope) isDeclared(name string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			return true
		}
		if _, ok := cur.params[name]; ok {
			return true
		}
	}
	return false
}

// lookupSlot finds the slot index for a variable/parameter by walking the scope chain.
func (s *resolveScope) lookupSlot(name string) (int, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if slot, ok := cur.vars[name]; ok {
			return slot, true
		}
		if slot, ok := cur.params[name]; ok {
			return slot, true
		}
	}
	return -1, false
}

// lookupParamSlot finds the slot index for a parameter (not a variable) by
// walking the scope chain. Used for bare identifier resolution — bare
// identifiers must not resolve to $variables.
func (s *resolveScope) lookupParamSlot(name string) (int, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if slot, ok := cur.params[name]; ok {
			return slot, true
		}
	}
	return -1, false
}

// allocSlot assigns the next available slot index and returns it.
func (s *resolveScope) allocSlot() int {
	slot := s.nextSlot
	s.nextSlot++
	return slot
}

// declareVar declares a variable in this scope. In statement mode, if the
// variable exists in an ancestor, returns the ancestor's slot. Otherwise
// allocates a new slot.
func (s *resolveScope) declareVar(name string) int {
	if s.mode == resolveScopeStatement {
		// Check ancestors for existing declaration (write-through).
		for cur := s.parent; cur != nil; cur = cur.parent {
			if slot, ok := cur.vars[name]; ok {
				return slot
			}
		}
	}
	// Check if already declared in this scope.
	if slot, ok := s.vars[name]; ok {
		return slot
	}
	slot := s.allocSlot()
	s.vars[name] = slot
	return slot
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
	r.scope = newResolveScope(nil, resolveScopeStatement)
	r.maxSlots = 0

	// Resolve map bodies (isolated scope trees with independent slot spaces).
	for _, m := range r.prog.Maps {
		r.resolveMapDecl(m)
	}

	// Resolve top-level statements.
	for _, stmt := range r.prog.Stmts {
		r.resolveStmt(stmt)
	}
	r.trackSlots()
	r.prog.MaxSlots = r.maxSlots
}

func (r *resolver) resolveMapDecl(m *MapDecl) {
	// Validate parameter list.
	r.validateParams(m.Params, m.TokenPos)

	saved := r.scope
	savedInMap := r.inMap
	savedMaxSlots := r.maxSlots

	r.inMap = true
	r.maxSlots = 0
	mapScope := newResolveScope(nil, resolveScopeExpression) // isolated: no parent
	for i := range m.Params {
		p := &m.Params[i]
		if !p.Discard {
			p.SlotIndex = mapScope.allocSlot()
			mapScope.params[p.Name] = p.SlotIndex
		}
	}
	r.scope = mapScope
	r.resolveExprBody(m.Body)
	r.trackSlots()
	m.MaxSlots = r.maxSlots

	r.scope = saved
	r.inMap = savedInMap
	r.maxSlots = savedMaxSlots
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
	// Lambdas in any non-argument position are rejected by resolveExpr's
	// *LambdaExpr case (spec Section 3.4). No additional check needed here.

	// Map/function names cannot be stored in variables.
	if ident, ok := a.Value.(*IdentExpr); ok {
		_, isFn := r.knownFunctions[ident.Name]
		if a.Target.Root == AssignVar && (r.isKnownMap(ident.Name) || isFn) {
			r.error(a.TokenPos, fmt.Sprintf("cannot store %s in a variable (it is not a value)", ident.Name))
		}
	}

	r.resolveExpr(a.Value)

	// Resolve expressions inside assignment target path segments (e.g., output[$key]).
	for _, seg := range a.Target.Path {
		if seg.Index != nil {
			r.resolveExpr(seg.Index)
		}
	}

	// Track variable declarations.
	if a.Target.Root == AssignVar {
		a.Target.SlotIndex = r.scope.declareVar(a.Target.VarName)
	}
}

func (r *resolver) resolveIfStmt(s *IfStmt) {
	for _, branch := range s.Branches {
		r.resolveExpr(branch.Cond)
		child := newResolveScope(r.scope, resolveScopeStatement)
		saved := r.scope
		r.scope = child
		for _, stmt := range branch.Body {
			r.resolveStmt(stmt)
		}
		r.trackSlots()
		r.scope = saved
	}
	if s.Else != nil {
		child := newResolveScope(r.scope, resolveScopeStatement)
		saved := r.scope
		r.scope = child
		for _, stmt := range s.Else {
			r.resolveStmt(stmt)
		}
		r.trackSlots()
		r.scope = saved
	}
}

func (r *resolver) resolveMatchStmt(s *MatchStmt) {
	if s.Subject != nil {
		r.resolveExpr(s.Subject)
	}
	// Allocate binding slot once so all cases share it.
	if s.Binding != "" {
		s.BindingSlot = r.scope.allocSlot()
	}
	for _, c := range s.Cases {
		child := newResolveScope(r.scope, resolveScopeStatement)
		if s.Binding != "" {
			child.params[s.Binding] = s.BindingSlot
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
		r.trackSlots()
		r.scope = saved
	}
}

func (r *resolver) resolveExprBody(body *ExprBody) {
	if body == nil {
		return
	}
	for _, va := range body.Assignments {
		// Lambdas in non-argument positions are caught by resolveExpr's
		// *LambdaExpr case (spec Section 3.4).
		r.resolveExpr(va.Value)
		// Resolve expressions inside path segments (e.g., $acc[item.k] = ...).
		for _, seg := range va.Path {
			if seg.Index != nil {
				r.resolveExpr(seg.Index)
			}
		}
		if len(va.Path) > 0 {
			// Path assignment: mutate the existing variable if it's declared
			// anywhere in scope, otherwise declare it in the current scope
			// (Section 3.7: path assignment to undeclared is a declaration).
			if slot, ok := r.scope.lookupSlot(va.Name); ok {
				va.SlotIndex = slot
			} else {
				va.SlotIndex = r.scope.declareVar(va.Name)
			}
		} else {
			va.SlotIndex = r.scope.declareVar(va.Name)
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
		r.prog.ReadsOutput = true
	case *OutputMetaExpr:
		if r.inMap {
			r.error(e.TokenPos, "cannot access output inside a map body")
		}
		r.prog.ReadsOutput = true
	case *VarExpr:
		if !r.scope.isDeclared(e.Name) {
			r.error(e.TokenPos, "undeclared variable $"+e.Name)
		} else if slot, ok := r.scope.lookupSlot(e.Name); ok {
			e.SlotIndex = slot
		}
	case *IdentExpr:
		if e.Namespace != "" {
			// Qualified reference (e.g., math::double) — only valid as
			// a method argument to higher-order methods.
			if !r.inMethodArg {
				r.error(e.TokenPos, e.Namespace+"::"+e.Name+" is not a valid expression (call it with parentheses or pass to a method)")
			}
			r.resolveQualifiedIdent(e)
		} else if slot, ok := r.scope.lookupParamSlot(e.Name); ok {
			// Resolves to a parameter (map param, lambda param, match-as
			// binding) — annotate with slot. Bare identifiers must NOT
			// resolve to $variables (those require the $ prefix via VarExpr).
			e.SlotIndex = slot
		} else {
			// Not a variable/parameter — check if it's a map or function name.
			_, isFn := r.knownFunctions[e.Name]
			if r.isKnownMap(e.Name) || isFn {
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
		mi, miKnown := r.knownMethods[e.Method]
		if miKnown {
			r.checkMethodArity(e, mi)
			r.applyArgFolder(mi.ArgFolder, e.Args, e.MethodPos, "."+e.Method+"()")
		}
		if r.methodOpcodes != nil {
			e.MethodOpcode = r.methodOpcodes[e.Method]
		}
		saved := r.inMethodArg
		r.inMethodArg = true
		for i, arg := range e.Args {
			// Check map name references passed to higher-order methods.
			if ident, ok := arg.Value.(*IdentExpr); ok {
				if ident.Namespace != "" {
					// Qualified reference: check arity in namespace.
					if m := r.findNamespacedMap(ident.Namespace, ident.Name); m != nil {
						r.checkMapRefArity(ident.TokenPos, ident.Namespace+"::"+ident.Name, m)
					}
				} else if m := r.findMap(ident.Name); m != nil {
					r.checkMapRefArity(ident.TokenPos, ident.Name, m)
				}
			}
			acceptsLambda := !miKnown || mi.ParamAcceptsLambda(i, arg.Name)
			r.resolveArgValue(arg.Value, acceptsLambda, e.Method)
		}
		r.inMethodArg = saved
		if miKnown {
			e.Prebound = r.applyCallFolder(mi.CallFolder, e.Args, e.MethodPos, "."+e.Method+"()")
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
		r.error(e.TokenPos, "lambda is only valid as a call argument (spec Section 3.4)")
		// Still resolve the body so downstream passes don't see unresolved
		// parameter slots. Errors already emitted will surface the issue.
		r.resolveLambda(e)
	case *PathExpr:
		// Check map isolation for the path root.
		if r.inMap {
			switch e.Root {
			case PathRootInput, PathRootInputMeta:
				r.error(e.TokenPos, "cannot access input inside a map body")
			case PathRootOutput, PathRootOutputMeta:
				r.error(e.TokenPos, "cannot access output inside a map body")
			}
		}
		if e.Root == PathRootOutput || e.Root == PathRootOutputMeta {
			r.prog.ReadsOutput = true
		}
		if e.Root == PathRootVar {
			if !r.scope.isDeclared(e.VarName) {
				r.error(e.TokenPos, "undeclared variable $"+e.VarName)
			} else if slot, ok := r.scope.lookupSlot(e.VarName); ok {
				e.VarSlotIndex = slot
			}
		}
		for i := range e.Segments {
			seg := &e.Segments[i]
			if seg.Index != nil {
				r.resolveExpr(seg.Index)
			}
			if seg.Kind == PathSegMethod {
				if mi, ok := r.knownMethods[seg.Name]; ok {
					r.checkMethodArityAt(seg.Pos, seg.Name, len(seg.Args), mi)
					r.applyArgFolder(mi.ArgFolder, seg.Args, seg.Pos, "."+seg.Name+"()")
				}
				if r.methodOpcodes != nil {
					seg.MethodOpcode = r.methodOpcodes[seg.Name]
				}
			}
			if len(seg.Args) > 0 {
				saved := r.inMethodArg
				r.inMethodArg = true
				var segMi MethodInfo
				segMiKnown := false
				if seg.Kind == PathSegMethod {
					segMi, segMiKnown = r.knownMethods[seg.Name]
				}
				for i, arg := range seg.Args {
					if ident, ok := arg.Value.(*IdentExpr); ok {
						if ident.Namespace != "" {
							if m := r.findNamespacedMap(ident.Namespace, ident.Name); m != nil {
								r.checkMapRefArity(ident.TokenPos, ident.Namespace+"::"+ident.Name, m)
							}
						} else if m := r.findMap(ident.Name); m != nil {
							r.checkMapRefArity(ident.TokenPos, ident.Name, m)
						}
					}
					acceptsLambda := !segMiKnown || segMi.ParamAcceptsLambda(i, arg.Name)
					r.resolveArgValue(arg.Value, acceptsLambda, seg.Name)
				}
				r.inMethodArg = saved
			}
			if seg.Kind == PathSegMethod {
				if mi, ok := r.knownMethods[seg.Name]; ok {
					seg.Prebound = r.applyCallFolder(mi.CallFolder, seg.Args, seg.Pos, "."+seg.Name+"()")
				}
			}
		}
	}
}

func (r *resolver) resolveCall(e *CallExpr) {
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

	// Check that the function/map exists and validate arity.
	// User maps take priority over stdlib functions (maps shadow stdlib).
	if e.Namespace == "" {
		m := r.findMap(e.Name)
		if m != nil {
			r.checkMapArity(e, m)
		} else if fi, ok := r.knownFunctions[e.Name]; ok {
			r.checkFunctionArity(e, fi)
			r.applyArgFolder(fi.ArgFolder, e.Args, e.TokenPos, e.Name+"()")
			if r.functionOpcodes != nil {
				e.FunctionOpcode = r.functionOpcodes[e.Name]
			}
			e.Prebound = r.applyCallFolder(fi.CallFolder, e.Args, e.TokenPos, e.Name+"()")
		} else {
			r.error(e.TokenPos, fmt.Sprintf("unknown function or map %q", e.Name))
		}

		// Special compile-time check: throw() literal arg must be a string.
		if e.Name == "throw" && len(e.Args) == 1 {
			if lit, ok := e.Args[0].Value.(*LiteralExpr); ok {
				if lit.TokenType != STRING && lit.TokenType != RAW_STRING {
					r.error(e.TokenPos, "throw() requires a string argument")
				}
			}
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
		// No function or user map accepts a lambda argument.
		r.resolveArgValue(arg.Value, false, e.Name)
	}
}

// applyCallFolder runs a CallFolder against the call site's args and
// returns the Prebound value (or nil if the folder declined to fold).
// Folder errors are recorded as resolver diagnostics anchored at pos.
func (r *resolver) applyCallFolder(folder CallFolder, args []CallArg, pos Pos, calleeLabel string) any {
	if folder == nil {
		return nil
	}
	prebound, err := folder(args)
	if err != nil {
		r.error(pos, calleeLabel+": "+err.Error())
		return nil
	}
	return prebound
}

// applyArgFolder runs folder against args and, on success, attaches
// non-nil folded values to the matching CallArg.Folded field. A folder
// error is recorded as a resolver diagnostic anchored at pos. Silently
// tolerates folder-returned slices of the wrong length (a contract
// violation we don't want to block compilation for).
func (r *resolver) applyArgFolder(folder ArgFolder, args []CallArg, pos Pos, calleeLabel string) {
	if folder == nil || len(args) == 0 {
		return
	}
	folded, err := folder(args)
	if err != nil {
		r.error(pos, calleeLabel+": "+err.Error())
		return
	}
	if len(folded) != len(args) {
		return
	}
	for i := range args {
		if folded[i] != nil {
			args[i].Folded = folded[i]
		}
	}
}

// resolveArgValue resolves a call argument's value. Lambdas are only legal
// in this position (spec Section 3.4); they're rejected everywhere else by
// resolveExpr's *LambdaExpr case. When acceptsLambda is false, a lambda
// argument is rejected with a compile error that names the callee.
func (r *resolver) resolveArgValue(value Expr, acceptsLambda bool, calleeName string) {
	if lam, ok := value.(*LambdaExpr); ok {
		if !acceptsLambda {
			r.error(lam.TokenPos, calleeName+"() does not accept a lambda argument")
		}
		r.resolveLambda(lam)
		return
	}
	r.resolveExpr(value)
}

// findNamespacedMap looks up a map by namespace and name.
func (r *resolver) findNamespacedMap(namespace, name string) *MapDecl {
	maps, ok := r.prog.Namespaces[namespace]
	if !ok {
		return nil
	}
	for _, m := range maps {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// checkMapRefArity verifies a map reference passed to a higher-order method
// has exactly 1 required parameter.
func (r *resolver) checkMapRefArity(pos Pos, displayName string, m *MapDecl) {
	required := 0
	for _, p := range m.Params {
		if p.Default == nil && !p.Discard {
			required++
		}
	}
	if required != 1 {
		r.error(pos, fmt.Sprintf("arity mismatch: %s() requires %d arguments, but higher-order methods pass 1", displayName, required))
	}
}

// resolveQualifiedIdent checks that a qualified identifier (namespace::name)
// refers to a valid namespace and map.
func (r *resolver) resolveQualifiedIdent(e *IdentExpr) {
	maps, nsExists := r.prog.Namespaces[e.Namespace]
	if !nsExists {
		r.error(e.TokenPos, fmt.Sprintf("unknown namespace %q", e.Namespace))
		return
	}
	found := false
	for _, m := range maps {
		if m.Name == e.Name {
			found = true
			break
		}
	}
	if !found {
		r.error(e.TokenPos, fmt.Sprintf("nonexistent map %s::%s", e.Namespace, e.Name))
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
				r.error(e.TokenPos, fmt.Sprintf("arity mismatch: missing required named argument %q", p.Name))
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

func (r *resolver) checkFunctionArity(e *CallExpr, fi FunctionInfo) {
	if fi.Total < 0 {
		return // no arity checking
	}
	nArgs := len(e.Args)
	if nArgs < fi.Required {
		r.error(e.TokenPos, fmt.Sprintf("%s() requires at least %d arguments, got %d",
			e.Name, fi.Required, nArgs))
	}
	if nArgs > fi.Total {
		r.error(e.TokenPos, fmt.Sprintf("%s() accepts at most %d arguments, got %d",
			e.Name, fi.Total, nArgs))
	}
}

func (r *resolver) checkMethodArity(e *MethodCallExpr, mi MethodInfo) {
	r.checkMethodArityAt(e.MethodPos, e.Method, len(e.Args), mi)
}

func (r *resolver) checkMethodArityAt(pos Pos, name string, nArgs int, mi MethodInfo) {
	if mi.Total < 0 {
		return // no arity checking
	}
	if nArgs < mi.Required {
		r.error(pos, fmt.Sprintf("%s() requires at least %d arguments, got %d",
			name, mi.Required, nArgs))
	}
	if nArgs > mi.Total {
		r.error(pos, fmt.Sprintf("%s() accepts at most %d arguments, got %d",
			name, mi.Total, nArgs))
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
		child := newResolveScope(r.scope, resolveScopeExpression)
		saved := r.scope
		r.scope = child
		r.resolveExprBody(branch.Body)
		r.trackSlots()
		r.scope = saved
	}
	if e.Else != nil {
		child := newResolveScope(r.scope, resolveScopeExpression)
		saved := r.scope
		r.scope = child
		r.resolveExprBody(e.Else)
		r.trackSlots()
		r.scope = saved
	}
}

func (r *resolver) resolveMatchExpr(e *MatchExpr) {
	if e.Subject != nil {
		r.resolveExpr(e.Subject)
	}

	// Allocate the as-binding slot ONCE in the parent scope so all cases
	// share the same slot.
	if e.Binding != "" {
		e.BindingSlot = r.scope.allocSlot()
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
			child := newResolveScope(r.scope, resolveScopeExpression)
			if e.Binding != "" {
				child.params[e.Binding] = e.BindingSlot
			}
			saved := r.scope
			r.scope = child
			r.resolveExpr(c.Pattern)
			r.trackSlots()
			r.scope = saved
		}
		switch body := c.Body.(type) {
		case Expr:
			child := newResolveScope(r.scope, resolveScopeExpression)
			if e.Binding != "" {
				child.params[e.Binding] = e.BindingSlot
			}
			saved := r.scope
			r.scope = child
			r.resolveExpr(body)
			r.trackSlots()
			r.scope = saved
		case *ExprBody:
			child := newResolveScope(r.scope, resolveScopeExpression)
			if e.Binding != "" {
				child.params[e.Binding] = e.BindingSlot
			}
			saved := r.scope
			r.scope = child
			r.resolveExprBody(body)
			r.trackSlots()
			r.scope = saved
		}
	}
}

func (r *resolver) resolveLambda(e *LambdaExpr) {
	r.validateParams(e.Params, e.TokenPos)

	child := newResolveScope(r.scope, resolveScopeExpression)
	for i := range e.Params {
		if !e.Params[i].Discard {
			e.Params[i].SlotIndex = child.allocSlot()
			child.params[e.Params[i].Name] = e.Params[i].SlotIndex
		}
	}
	saved := r.scope
	r.scope = child
	r.resolveExprBody(e.Body)
	r.trackSlots()
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
