package eval

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// maxRecursionDepth is the maximum allowed recursion depth for map calls.
const maxRecursionDepth = 10000

// Interpreter executes a parsed Bloblang V2 program.
type Interpreter struct {
	prog *syntax.Program

	// Runtime state.
	input      any
	inputMeta  map[string]any
	output     any
	outputMeta map[string]any
	deleted    bool

	// Map table: local maps + namespaced imports.
	maps       map[string]*syntax.MapDecl
	namespaces map[string]map[string]*syntax.MapDecl

	scope *scope
	depth int // recursion depth

	// Methods and functions. Static maps are shared across all interpreters
	// (built once at init time). Lambda methods need a per-interpreter map
	// because they close over the interpreter for callLambda dispatch.
	staticMethods   map[string]MethodSpec
	staticFunctions map[string]FunctionSpec
	lambdaMethods   map[string]MethodSpec

	// lambdaTable is the opcode-indexed dispatch table for lambda methods.
	// Indexed by (opcode - lambdaOpcodeBase).
	lambdaTable []MethodSpec
}

// MethodFunc is a stdlib method implementation.
// Receiver is the value the method is called on.
type MethodFunc func(receiver any, args []any) any

// MethodParam describes a method parameter for named argument support.
type MethodParam struct {
	Name       string
	Default    any // default value (nil means required)
	HasDefault bool
}

// MethodSpec bundles a method implementation with its behavioral metadata.
// Metadata is colocated with the method definition so the interpreter dispatch
// does not need hardcoded name lists.
type MethodSpec struct {
	Fn          MethodFunc       // regular method (mutually exclusive with LambdaFn)
	LambdaFn    lambdaMethodFunc // lambda method (mutually exclusive with Fn)
	Intrinsic   bool             // marks catch/or — dispatch handled inline, registered for name resolution only
	Params      []MethodParam    // nil for methods with no named-arg support
	AcceptsNull bool             // receiver can be nil (e.g., type, string, not_null)
}

// lambdaMethodFunc is a method that receives unevaluated AST args (for lambda/map-ref arguments).
type lambdaMethodFunc func(receiver any, args []syntax.CallArg) any

// FunctionFunc is a stdlib function implementation.
type FunctionFunc func(args []any) any

// FunctionSpec bundles a function implementation with its behavioral metadata.
type FunctionSpec struct {
	Fn     FunctionFunc
	Params []FunctionParam // for compile-time arity checking
}

// FunctionParam describes a function parameter for compile-time validation
// and named argument resolution.
type FunctionParam struct {
	Name       string
	Default    any // default value (used for named arg resolution)
	HasDefault bool
}

// New creates a new interpreter for the given program.
func New(prog *syntax.Program) *Interpreter {
	interp := &Interpreter{
		prog:            prog,
		maps:            make(map[string]*syntax.MapDecl),
		namespaces:      make(map[string]map[string]*syntax.MapDecl),
		staticMethods:   make(map[string]MethodSpec),
		staticFunctions: make(map[string]FunctionSpec),
		lambdaMethods:   make(map[string]MethodSpec, 16),
	}

	if prog != nil {
		// Hoist map declarations.
		for _, m := range prog.Maps {
			interp.maps[m.Name] = m
		}

		// Build namespace tables from imports.
		for ns, maps := range prog.Namespaces {
			table := make(map[string]*syntax.MapDecl, len(maps))
			for _, m := range maps {
				table[m.Name] = m
			}
			interp.namespaces[ns] = table
		}
	}

	return interp
}

// NewWithStdlib creates a new interpreter with the shared stdlib already
// wired in. This is the fast path for repeated execution of compiled
// mappings — the static method/function tables are shared (not copied)
// across all interpreters and only the lambda methods (which close over
// the interpreter) are allocated per-instance.
func NewWithStdlib(prog *syntax.Program) *Interpreter {
	interp := &Interpreter{
		prog:            prog,
		maps:            make(map[string]*syntax.MapDecl),
		namespaces:      make(map[string]map[string]*syntax.MapDecl),
		staticMethods:   sharedMethods,
		staticFunctions: sharedFunctions,
		lambdaMethods:   make(map[string]MethodSpec, 16),
		lambdaTable:     make([]MethodSpec, len(lambdaOpcodeOffsets)),
	}

	if prog != nil {
		for _, m := range prog.Maps {
			interp.maps[m.Name] = m
		}
		for ns, maps := range prog.Namespaces {
			table := make(map[string]*syntax.MapDecl, len(maps))
			for _, m := range maps {
				table[m.Name] = m
			}
			interp.namespaces[ns] = table
		}
	}

	interp.RegisterLambdaMethods()
	return interp
}

// RegisterMethod registers a stdlib method with its behavioral metadata.
func (interp *Interpreter) RegisterMethod(name string, spec MethodSpec) {
	interp.staticMethods[name] = spec
}

// RegisterFunction registers a stdlib function with its behavioral metadata.
func (interp *Interpreter) RegisterFunction(name string, spec FunctionSpec) {
	interp.staticFunctions[name] = spec
}

// RegisterLambdaMethod registers a method that needs the interpreter for
// lambda/map-ref dispatch. These are stored separately from static methods
// and checked first during dispatch. Also populates the opcode-indexed
// lambdaTable for fast dispatch.
func (interp *Interpreter) RegisterLambdaMethod(name string, spec MethodSpec) {
	interp.lambdaMethods[name] = spec
	if offset, ok := lambdaOpcodeOffsets[name]; ok {
		for int(offset) >= len(interp.lambdaTable) {
			interp.lambdaTable = append(interp.lambdaTable, MethodSpec{})
		}
		interp.lambdaTable[offset] = spec
	}
}

// lookupMethod resolves a method by name, checking lambda methods first
// (per-interpreter) then static methods (shared).
func (interp *Interpreter) lookupMethod(name string) (MethodSpec, bool) {
	if spec, ok := interp.lambdaMethods[name]; ok {
		return spec, true
	}
	spec, ok := interp.staticMethods[name]
	return spec, ok
}

// Exec runs the program against the given input and metadata.
func (interp *Interpreter) Exec(input any, metadata map[string]any) (output any, outputMeta map[string]any, deleted bool, err error) {
	interp.input = input
	interp.inputMeta = metadata
	interp.output = make(map[string]any)
	interp.outputMeta = make(map[string]any)
	interp.deleted = false
	interp.scope = newScope(nil, scopeStatement)
	interp.depth = 0

	for _, stmt := range interp.prog.Stmts {
		interp.execStmt(stmt)
		if interp.deleted {
			return nil, nil, true, nil
		}
	}

	return interp.output, interp.outputMeta, false, nil
}

// -----------------------------------------------------------------------
// Statement execution
// -----------------------------------------------------------------------

func (interp *Interpreter) execStmt(stmt syntax.Stmt) {
	switch s := stmt.(type) {
	case *syntax.Assignment:
		interp.execAssignment(s)
	case *syntax.IfStmt:
		interp.execIfStmt(s)
	case *syntax.MatchStmt:
		interp.execMatchStmt(s)
	}
}

func (interp *Interpreter) execAssignment(a *syntax.Assignment) {
	value := interp.evalExpr(a.Value)

	// Error propagation: if value is an error, it halts the mapping.
	if IsError(value) {
		panic(runtimeError{message: ErrorMessage(value)})
	}

	// Void handling.
	if IsVoid(value) {
		// For variable targets: declaration with void is an error,
		// reassignment with void skips the assignment.
		if a.Target.Root == syntax.AssignVar && len(a.Target.Path) == 0 {
			if _, exists := interp.scope.get(a.Target.VarName); !exists {
				panic(runtimeError{message: "void in variable declaration (use .or() to provide a default)"})
			}
		}
		return
	}

	switch a.Target.Root {
	case syntax.AssignOutput:
		if a.Target.MetaAccess {
			// Metadata root assignment validation (Section 7.4, 9.2).
			if len(a.Target.Path) == 0 {
				if IsDeleted(value) {
					panic(runtimeError{message: "cannot delete metadata object"})
				}
				obj, ok := value.(map[string]any)
				if !ok {
					panic(runtimeError{message: fmt.Sprintf("metadata must be an object, got %T", value)})
				}
				interp.outputMeta = DeepClone(obj).(map[string]any)
				return
			}
			var meta any = interp.outputMeta
			interp.assignPath(&meta, a.Target.Path, value)
			if m, ok := meta.(map[string]any); ok {
				interp.outputMeta = m
			}
		} else {
			// Message drop: output = deleted() — set flag and exit immediately
			// without storing the sentinel in interp.output.
			if len(a.Target.Path) == 0 && IsDeleted(value) {
				interp.deleted = true
				return
			}
			interp.assignPath(&interp.output, a.Target.Path, value)
		}
	case syntax.AssignVar:
		if IsDeleted(value) {
			if len(a.Target.Path) == 0 {
				panic(runtimeError{message: "cannot assign deleted() to a variable"})
			}
		}
		if len(a.Target.Path) == 0 {
			// No clone needed for simple assignment: the value is either a
			// freshly allocated result or immutable input data. The
			// path-assignment branch below clones before mutating, providing
			// copy-on-write semantics.
			interp.scope.set(a.Target.VarName, value)
		} else {
			existing, ok := interp.scope.get(a.Target.VarName)
			if !ok {
				panic(runtimeError{message: fmt.Sprintf("variable $%s not declared", a.Target.VarName)})
			}
			clone := DeepClone(existing)
			interp.setPath(&clone, a.Target.Path, value)
			interp.scope.set(a.Target.VarName, clone)
		}
	}
}

func (interp *Interpreter) execIfStmt(s *syntax.IfStmt) {
	for _, branch := range s.Branches {
		cond := interp.evalExpr(branch.Cond)
		if IsError(cond) {
			panic(runtimeError{message: ErrorMessage(cond)})
		}
		b, ok := cond.(bool)
		if !ok {
			panic(runtimeError{message: fmt.Sprintf("if condition must be boolean, got %T", cond)})
		}
		if b {
			childScope := newScope(interp.scope, scopeStatement)
			saved := interp.scope
			interp.scope = childScope
			for _, stmt := range branch.Body {
				interp.execStmt(stmt)
				if interp.deleted {
					interp.scope = saved
					return
				}
			}
			interp.scope = saved
			return
		}
	}

	if s.Else != nil {
		childScope := newScope(interp.scope, scopeStatement)
		saved := interp.scope
		interp.scope = childScope
		for _, stmt := range s.Else {
			interp.execStmt(stmt)
			if interp.deleted {
				interp.scope = saved
				return
			}
		}
		interp.scope = saved
	}
}

func (interp *Interpreter) execMatchStmt(s *syntax.MatchStmt) {
	var subject any
	if s.Subject != nil {
		subject = interp.evalExpr(s.Subject)
		if IsError(subject) {
			panic(runtimeError{message: ErrorMessage(subject)})
		}
	}

	for _, c := range s.Cases {
		matched, errVal := interp.matchCaseMatches(c, subject, s.Binding, s.Subject != nil)
		if errVal != nil {
			panic(runtimeError{message: ErrorMessage(errVal)})
		}
		if matched {
			body, ok := c.Body.([]syntax.Stmt)
			if !ok {
				return
			}
			childScope := newScope(interp.scope, scopeStatement)
			if s.Binding != "" {
				childScope.vars[s.Binding] = subject
			}
			saved := interp.scope
			interp.scope = childScope
			for _, stmt := range body {
				interp.execStmt(stmt)
				if interp.deleted {
					interp.scope = saved
					return
				}
			}
			interp.scope = saved
			return
		}
	}
}

// -----------------------------------------------------------------------
// Expression evaluation
// -----------------------------------------------------------------------

func (interp *Interpreter) evalExpr(expr syntax.Expr) any {
	switch e := expr.(type) {
	case *syntax.LiteralExpr:
		return interp.evalLiteral(e)
	case *syntax.BinaryExpr:
		return interp.evalBinary(e)
	case *syntax.UnaryExpr:
		return interp.evalUnary(e)
	case *syntax.InputExpr:
		return interp.input // immutable, no clone needed
	case *syntax.InputMetaExpr:
		return interp.inputMeta // immutable, no clone needed
	case *syntax.OutputExpr:
		return DeepClone(interp.output) // mutable; must snapshot for COW semantics
	case *syntax.OutputMetaExpr:
		return DeepClone(interp.outputMeta) // mutable; must snapshot for COW semantics
	case *syntax.VarExpr:
		v, ok := interp.scope.get(e.Name)
		if !ok {
			panic(runtimeError{message: "undefined variable $" + e.Name})
		}
		return v
	case *syntax.IdentExpr:
		return interp.evalIdent(e)
	case *syntax.CallExpr:
		return interp.evalCall(e)
	case *syntax.FieldAccessExpr:
		return interp.evalFieldAccess(e)
	case *syntax.MethodCallExpr:
		return interp.evalMethodCall(e)
	case *syntax.IndexExpr:
		return interp.evalIndex(e)
	case *syntax.IfExpr:
		return interp.evalIfExpr(e)
	case *syntax.MatchExpr:
		return interp.evalMatchExpr(e)
	case *syntax.ArrayLiteral:
		return interp.evalArrayLiteral(e)
	case *syntax.ObjectLiteral:
		return interp.evalObjectLiteral(e)
	case *syntax.LambdaExpr:
		// Lambdas in expression position shouldn't be evaluated directly.
		// They're handled by the method that receives them.
		panic(runtimeError{message: "lambda expression cannot be used as a value"})
	case *syntax.PathExpr:
		return interp.evalPathExpr(e)
	default:
		panic(runtimeError{message: fmt.Sprintf("unknown expression type %T", expr)})
	}
}

func (interp *Interpreter) evalLiteral(e *syntax.LiteralExpr) any {
	switch e.TokenType {
	case syntax.INT:
		n, _ := strconv.ParseInt(e.Value, 10, 64)
		return n
	case syntax.FLOAT:
		f, _ := strconv.ParseFloat(e.Value, 64)
		return f
	case syntax.STRING, syntax.RAW_STRING:
		return e.Value
	case syntax.TRUE:
		return true
	case syntax.FALSE:
		return false
	case syntax.NULL:
		return nil
	default:
		return nil
	}
}

func (interp *Interpreter) evalBinary(e *syntax.BinaryExpr) any {
	left := interp.evalExpr(e.Left)
	if IsError(left) {
		return left
	}
	if IsVoid(left) {
		return NewError("void in expression")
	}
	if IsDeleted(left) {
		return NewError("deleted value in expression")
	}

	// Short-circuit for logical operators.
	if e.Op == syntax.AND {
		b, ok := left.(bool)
		if !ok {
			return NewError(fmt.Sprintf("&& requires boolean operands, got %T", left))
		}
		if !b {
			return false
		}
		right := interp.evalExpr(e.Right)
		if IsError(right) {
			return right
		}
		rb, ok := right.(bool)
		if !ok {
			return NewError(fmt.Sprintf("&& requires boolean operands, got %T", right))
		}
		return rb
	}
	if e.Op == syntax.OR {
		b, ok := left.(bool)
		if !ok {
			return NewError(fmt.Sprintf("|| requires boolean operands, got %T", left))
		}
		if b {
			return true
		}
		right := interp.evalExpr(e.Right)
		if IsError(right) {
			return right
		}
		rb, ok := right.(bool)
		if !ok {
			return NewError(fmt.Sprintf("|| requires boolean operands, got %T", right))
		}
		return rb
	}

	right := interp.evalExpr(e.Right)
	if IsError(right) {
		return right
	}
	if IsVoid(right) {
		return NewError("void in expression")
	}
	if IsDeleted(right) {
		return NewError("deleted value in expression")
	}

	return interp.evalBinaryOp(e.Op, left, right)
}

func (interp *Interpreter) evalUnary(e *syntax.UnaryExpr) any {
	operand := interp.evalExpr(e.Operand)
	if IsError(operand) {
		return operand
	}
	if IsVoid(operand) {
		return NewError("void in expression")
	}
	if IsDeleted(operand) {
		return NewError("deleted value in expression")
	}

	switch e.Op {
	case syntax.MINUS:
		return numericNegate(operand)
	case syntax.BANG:
		b, ok := operand.(bool)
		if !ok {
			return NewError(fmt.Sprintf("! requires boolean operand, got %T", operand))
		}
		return !b
	default:
		return NewError(fmt.Sprintf("unknown unary operator %s", e.Op))
	}
}

func (interp *Interpreter) evalFieldAccess(e *syntax.FieldAccessExpr) any {
	receiver := interp.evalExpr(e.Receiver)
	if IsError(receiver) {
		return receiver
	}
	if e.NullSafe && receiver == nil {
		return nil
	}
	if receiver == nil {
		return NewError(fmt.Sprintf("cannot access field %q on null", e.Field))
	}
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("cannot access field %q on %T", e.Field, receiver))
	}
	return obj[e.Field]
}

func (interp *Interpreter) evalIndex(e *syntax.IndexExpr) any {
	receiver := interp.evalExpr(e.Receiver)
	if IsError(receiver) {
		return receiver
	}
	if e.NullSafe && receiver == nil {
		return nil
	}

	index := interp.evalExpr(e.Index)
	if IsError(index) {
		return index
	}

	return interp.indexValue(receiver, index)
}

func (interp *Interpreter) evalMethodCall(e *syntax.MethodCallExpr) any {
	// Intrinsic: .catch() — intercepts errors, passes void/deleted through.
	if e.Method == "catch" {
		return interp.evalCatch(e)
	}

	// Intrinsic: .or() — rescues null/void/deleted with short-circuit evaluation.
	if e.Method == "or" {
		return interp.evalOr(e)
	}

	receiver := interp.evalExpr(e.Receiver)

	// Error propagation: errors skip method calls (except .catch handled above).
	if IsError(receiver) {
		return receiver
	}

	// Null-safe: ?.method() returns nil if receiver is null.
	if e.NullSafe && receiver == nil {
		return nil
	}

	// Look up the method via opcode (fast path) or name (fallback).
	var spec MethodSpec
	if opc := e.MethodOpcode; opc != 0 {
		if opc >= lambdaOpcodeBase {
			spec = interp.lambdaTable[opc-lambdaOpcodeBase]
		} else {
			spec = methodTable[opc]
		}
	} else {
		var ok bool
		spec, ok = interp.lookupMethod(e.Method)
		if !ok {
			if receiver == nil {
				return NewError(fmt.Sprintf(".%s() does not support null", e.Method))
			}
			return NewError(fmt.Sprintf("unknown method .%s()", e.Method))
		}
	}

	// Null check using spec metadata.
	if receiver == nil && !e.NullSafe && !spec.AcceptsNull {
		return NewError(fmt.Sprintf(".%s() does not support null", e.Method))
	}

	// Void and deleted in method calls (except .or handled above) are errors.
	if IsVoid(receiver) {
		return NewError("cannot call method on void")
	}
	if IsDeleted(receiver) {
		return NewError("cannot call method on deleted value")
	}

	// Lambda methods: receive unevaluated AST args (for lambdas/map-refs).
	if spec.LambdaFn != nil {
		args := e.Args
		if e.Named && spec.Params != nil {
			args = reorderNamedCallArgs(args, spec.Params)
		}
		return spec.LambdaFn(receiver, args)
	}

	// Evaluate arguments, resolving named args to positional if needed.
	var args []any
	if e.Named {
		resolved := interp.resolveNamedMethodArgs(e)
		if IsError(resolved) {
			return resolved
		}
		args = resolved.([]any)
	} else {
		args = interp.evalArgs(e.Args)
	}
	for _, a := range args {
		if IsError(a) {
			return a
		}
	}

	return spec.Fn(receiver, args)
}

func (interp *Interpreter) evalCatch(e *syntax.MethodCallExpr) any {
	receiver := interp.evalExpr(e.Receiver)

	// .catch() passes non-errors (including void and deleted) through unchanged.
	if !IsError(receiver) {
		return receiver
	}

	// Error: invoke the catch handler lambda.
	if len(e.Args) != 1 {
		return NewError(".catch() requires exactly one argument")
	}
	lambda, ok := e.Args[0].Value.(*syntax.LambdaExpr)
	if !ok {
		return NewError(".catch() argument must be a lambda")
	}

	// Build the error object: {"what": "error message"}.
	errObj := map[string]any{"what": ErrorMessage(receiver)}

	return interp.callLambda(lambda, []any{errObj})
}

func (interp *Interpreter) evalOr(e *syntax.MethodCallExpr) any {
	receiver := interp.evalExpr(e.Receiver)

	// .or() rescues null, void, and deleted — returns the argument.
	// For all other values (including errors), returns the receiver unchanged.
	if receiver != nil && !IsVoid(receiver) && !IsDeleted(receiver) {
		return receiver
	}

	// Short-circuit: only evaluate the argument when rescuing.
	if len(e.Args) != 1 {
		return NewError(".or() requires exactly one argument")
	}
	return interp.evalExpr(e.Args[0].Value)
}

func (interp *Interpreter) callLambda(lambda *syntax.LambdaExpr, args []any) any {
	lambdaScope := newScope(interp.scope, scopeExpression)
	return interp.callLambdaWithScope(lambda, args, lambdaScope)
}

// callLambdaWithScope executes a lambda using a pre-allocated scope. The scope's
// vars map is cleared and repopulated with the lambda parameters. This allows
// iterator methods (map, filter, etc.) to reuse a single scope across iterations.
func (interp *Interpreter) callLambdaWithScope(lambda *syntax.LambdaExpr, args []any, lambdaScope *scope) any {
	// Clear any leftover vars from previous iteration.
	clear(lambdaScope.vars)

	for i, p := range lambda.Params {
		if p.Discard {
			continue
		}
		if i < len(args) {
			lambdaScope.vars[p.Name] = args[i]
		} else if p.Default != nil {
			lambdaScope.vars[p.Name] = interp.evalExpr(p.Default)
		}
	}

	saved := interp.scope
	interp.scope = lambdaScope
	result := interp.evalExprBody(lambda.Body)
	interp.scope = saved

	return result
}

func (interp *Interpreter) evalCall(e *syntax.CallExpr) any {
	// Check for namespace-qualified call.
	if e.Namespace != "" {
		return interp.callNamespaced(e)
	}

	// Check for user-defined map.
	if m, ok := interp.maps[e.Name]; ok {
		return interp.callMap(m, e)
	}

	// Check stdlib functions via opcode (fast path) or name (fallback).
	var spec FunctionSpec
	var isStdlib bool
	if opc := e.FunctionOpcode; opc != 0 {
		spec = functionTable[opc]
		isStdlib = true
	} else if s, ok := interp.staticFunctions[e.Name]; ok {
		spec = s
		isStdlib = true
	}
	if isStdlib {
		var args []any
		if e.Named {
			resolved := interp.resolveNamedFuncArgs(e, spec)
			if IsError(resolved) {
				return resolved
			}
			args = resolved.([]any)
		} else {
			args = interp.evalArgs(e.Args)
		}
		for _, a := range args {
			if IsError(a) {
				return a
			}
		}
		return spec.Fn(args)
	}

	return NewError(fmt.Sprintf("unknown function %s()", e.Name))
}

func (interp *Interpreter) callNamespaced(e *syntax.CallExpr) any {
	ns, ok := interp.namespaces[e.Namespace]
	if !ok {
		return NewError(fmt.Sprintf("unknown namespace %q", e.Namespace))
	}
	m, ok := ns[e.Name]
	if !ok {
		return NewError(fmt.Sprintf("unknown function %s::%s()", e.Namespace, e.Name))
	}
	return interp.callMap(m, e)
}

func (interp *Interpreter) callMap(m *syntax.MapDecl, e *syntax.CallExpr) any {
	interp.depth++
	if interp.depth > maxRecursionDepth {
		panic(recursionError{})
	}
	defer func() { interp.depth-- }()

	// Evaluate and bind parameters into an isolated scope.
	mapScope := newScope(nil, scopeExpression) // isolated: no parent
	if e.Named {
		// Named args: resolve to positional via shared helper, then bind.
		if err := interp.bindNamedMapParams(mapScope, m, e); err != "" {
			return NewError(err)
		}
	} else {
		args := interp.evalArgs(e.Args)
		for _, a := range args {
			if IsError(a) {
				return a
			}
		}
		if err := interp.bindPositionalParams(mapScope, m.Params, args); err != "" {
			return NewError(err)
		}
	}

	// Evaluate the map body. If the map has its own namespace context
	// (from an imported file), temporarily switch to it so that
	// qualified calls within the map resolve correctly.
	savedScope := interp.scope
	savedNamespaces := interp.namespaces
	savedMaps := interp.maps

	interp.scope = mapScope
	if m.Namespaces != nil {
		// Build namespace tables for this map's context.
		nsTable := make(map[string]map[string]*syntax.MapDecl, len(m.Namespaces))
		for ns, maps := range m.Namespaces {
			table := make(map[string]*syntax.MapDecl, len(maps))
			for _, md := range maps {
				table[md.Name] = md
			}
			nsTable[ns] = table
		}
		interp.namespaces = nsTable
	}

	result := interp.evalExprBody(m.Body)

	interp.scope = savedScope
	interp.namespaces = savedNamespaces
	interp.maps = savedMaps

	return result
}

func (interp *Interpreter) evalIdent(e *syntax.IdentExpr) any {
	// Qualified reference — only valid in higher-order method args
	// (handled by extractLambdaOrMapRef). If we reach here, it's misused.
	if e.Namespace != "" {
		return NewError(e.Namespace + "::" + e.Name + " cannot be used as a value (pass to a higher-order method or call with parentheses)")
	}
	// Check scope (parameters, variables).
	if v, ok := interp.scope.get(e.Name); ok {
		return v
	}
	// Bare map name without call — error per spec.
	if _, ok := interp.maps[e.Name]; ok {
		return NewError("map " + e.Name + " cannot be used as a value (call it with parentheses)")
	}
	return NewError("undefined identifier " + e.Name)
}

func (interp *Interpreter) evalIfExpr(e *syntax.IfExpr) any {
	for _, branch := range e.Branches {
		cond := interp.evalExpr(branch.Cond)
		if IsError(cond) {
			return cond
		}
		b, ok := cond.(bool)
		if !ok {
			return NewError(fmt.Sprintf("if condition must be boolean, got %T", cond))
		}
		if b {
			childScope := newScope(interp.scope, scopeExpression)
			saved := interp.scope
			interp.scope = childScope
			result := interp.evalExprBody(branch.Body)
			interp.scope = saved
			return result
		}
	}

	if e.Else != nil {
		childScope := newScope(interp.scope, scopeExpression)
		saved := interp.scope
		interp.scope = childScope
		result := interp.evalExprBody(e.Else)
		interp.scope = saved
		return result
	}

	return Void
}

func (interp *Interpreter) evalMatchExpr(e *syntax.MatchExpr) any {
	var subject any
	if e.Subject != nil {
		subject = interp.evalExpr(e.Subject)
		if IsError(subject) {
			return subject
		}
	}

	for _, c := range e.Cases {
		matched, errVal := interp.matchCaseMatches(c, subject, e.Binding, e.Subject != nil)
		if errVal != nil {
			return errVal
		}
		if matched {
			childScope := newScope(interp.scope, scopeExpression)
			if e.Binding != "" {
				childScope.vars[e.Binding] = subject
			}
			saved := interp.scope
			interp.scope = childScope

			var result any
			switch body := c.Body.(type) {
			case syntax.Expr:
				result = interp.evalExpr(body)
			case *syntax.ExprBody:
				result = interp.evalExprBody(body)
			}

			interp.scope = saved
			return result
		}
	}

	return Void
}

// matchCaseMatches returns (matched, errorValue). If errorValue is non-nil,
// the case expression produced an error that should be propagated.
func (interp *Interpreter) matchCaseMatches(c syntax.MatchCase, subject any, binding string, hasSubject bool) (bool, any) {
	if c.Wildcard {
		return true, nil
	}

	if hasSubject && binding == "" {
		// Equality match: compare pattern against subject.
		patternVal := interp.evalExpr(c.Pattern)
		if IsError(patternVal) {
			return false, patternVal
		}
		// Boolean case values are a runtime error in equality match.
		if _, ok := patternVal.(bool); ok {
			return false, NewError("boolean case value in equality match (use 'as' for boolean conditions)")
		}
		return valuesEqual(subject, patternVal), nil
	}

	// Boolean match (with or without 'as'): case must evaluate to bool.
	if binding != "" {
		// Evaluate pattern in a child scope with the binding.
		childScope := newScope(interp.scope, interp.scope.mode)
		childScope.vars[binding] = subject
		saved := interp.scope
		interp.scope = childScope
		patternVal := interp.evalExpr(c.Pattern)
		interp.scope = saved
		if IsError(patternVal) {
			return false, patternVal
		}
		b, ok := patternVal.(bool)
		if !ok {
			return false, NewError(fmt.Sprintf("boolean match case must evaluate to bool, got %T", patternVal))
		}
		return b, nil
	}
	patternVal := interp.evalExpr(c.Pattern)
	if IsError(patternVal) {
		return false, patternVal
	}
	b, ok := patternVal.(bool)
	if !ok {
		return false, NewError(fmt.Sprintf("boolean match case must evaluate to bool, got %T", patternVal))
	}
	return b, nil
}

func (interp *Interpreter) evalArrayLiteral(e *syntax.ArrayLiteral) any {
	result := make([]any, 0, len(e.Elements))
	for _, elem := range e.Elements {
		val := interp.evalExpr(elem)
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("void in array literal (use deleted() to omit elements, or add an else branch)")
		}
		if IsDeleted(val) {
			continue // deleted elements are removed
		}
		result = append(result, val)
	}
	return result
}

func (interp *Interpreter) evalObjectLiteral(e *syntax.ObjectLiteral) any {
	result := make(map[string]any, len(e.Entries))
	for _, entry := range e.Entries {
		key := interp.evalExpr(entry.Key)
		if IsError(key) {
			return key
		}
		keyStr, ok := key.(string)
		if !ok {
			return NewError(fmt.Sprintf("object key must be string, got %T", key))
		}
		val := interp.evalExpr(entry.Value)
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("void in object literal (use deleted() to omit fields, or add an else branch)")
		}
		if IsDeleted(val) {
			continue // deleted fields are removed
		}
		result[keyStr] = val
	}
	return result
}

func (interp *Interpreter) evalExprBody(body *syntax.ExprBody) any {
	for _, va := range body.Assignments {
		val := interp.evalExpr(va.Value)
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			// Void in variable declaration is an error.
			// Void in reassignment (variable exists in any reachable scope) skips.
			if _, exists := interp.scope.get(va.Name); exists {
				continue
			}
			return NewError("void in variable declaration (use .or() to provide a default)")
		}
		if IsDeleted(val) {
			if len(va.Path) == 0 {
				return NewError("cannot assign deleted() to a variable")
			}
		}
		if len(va.Path) == 0 {
			interp.scope.set(va.Name, val)
		} else {
			existing, ok := interp.scope.get(va.Name)
			if !ok {
				return NewError(fmt.Sprintf("variable $%s not declared", va.Name))
			}
			clone := DeepClone(existing)
			interp.setPath(&clone, va.Path, val)
			interp.scope.set(va.Name, clone)
		}
	}
	return interp.evalExpr(body.Result)
}

func (interp *Interpreter) evalPathExpr(e *syntax.PathExpr) any {
	var root any
	switch e.Root {
	case syntax.PathRootInput:
		root = interp.input
	case syntax.PathRootInputMeta:
		root = interp.inputMeta
	case syntax.PathRootOutput:
		root = DeepClone(interp.output)
	case syntax.PathRootOutputMeta:
		root = DeepClone(interp.outputMeta)
	case syntax.PathRootVar:
		v, ok := interp.scope.get(e.VarName)
		if !ok {
			return NewError("undefined variable $" + e.VarName)
		}
		root = v
	}

	current := root
	for _, seg := range e.Segments {
		if IsError(current) {
			return current
		}
		switch seg.Kind {
		case syntax.PathSegField:
			if seg.NullSafe && current == nil {
				return nil
			}
			obj, ok := current.(map[string]any)
			if !ok {
				return NewError(fmt.Sprintf("cannot access field %q on %T", seg.Name, current))
			}
			current = obj[seg.Name]
		case syntax.PathSegIndex:
			if seg.NullSafe && current == nil {
				return nil
			}
			idx := interp.evalExpr(seg.Index)
			if IsError(idx) {
				return idx
			}
			current = interp.indexValue(current, idx)
			if IsError(current) {
				return current
			}
		case syntax.PathSegMethod:
			if seg.NullSafe && current == nil {
				return nil
			}
			var spec MethodSpec
			if opc := seg.MethodOpcode; opc != 0 {
				if opc >= lambdaOpcodeBase {
					spec = interp.lambdaTable[opc-lambdaOpcodeBase]
				} else {
					spec = methodTable[opc]
				}
			} else {
				var ok bool
				spec, ok = interp.lookupMethod(seg.Name)
				if !ok {
					return NewError(fmt.Sprintf("unknown method .%s()", seg.Name))
				}
			}
			// Intrinsic methods (catch/or) cannot appear in path expressions
			// because they require control over receiver evaluation.
			if spec.Intrinsic {
				return NewError(fmt.Sprintf(".%s() cannot be used in path expressions", seg.Name))
			}
			if current == nil && !seg.NullSafe && !spec.AcceptsNull {
				return NewError(fmt.Sprintf(".%s() does not support null", seg.Name))
			}
			if IsVoid(current) {
				return NewError("cannot call method on void")
			}
			if IsDeleted(current) {
				return NewError("cannot call method on deleted value")
			}
			if spec.LambdaFn != nil {
				lambdaArgs := seg.Args
				if seg.Named && spec.Params != nil {
					lambdaArgs = reorderNamedCallArgs(lambdaArgs, spec.Params)
				}
				current = spec.LambdaFn(current, lambdaArgs)
			} else {
				args := interp.evalArgs(seg.Args)
				for _, a := range args {
					if IsError(a) {
						return a
					}
				}
				current = spec.Fn(current, args)
			}
		}
	}
	return current
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// namedArgParam is a unified parameter descriptor for named argument resolution,
// used by both methods and functions.
type namedArgParam struct {
	Name       string
	Default    any
	HasDefault bool
}

// resolveNamedArgs maps named call arguments to positional order using parameter
// metadata. context is used in error messages (e.g., ".replace_all()", "random_int()").
// Returns []any or an errorVal.
func (interp *Interpreter) resolveNamedArgs(callArgs []syntax.CallArg, params []namedArgParam, context string) any {
	if len(params) == 0 {
		// No parameter metadata — evaluate named args by name order.
		args := make([]any, 0, len(callArgs))
		for _, arg := range callArgs {
			v := interp.evalExpr(arg.Value)
			if IsError(v) {
				return v
			}
			args = append(args, v)
		}
		return args
	}

	// Build named arg map.
	named := make(map[string]any, len(callArgs))
	for _, arg := range callArgs {
		v := interp.evalExpr(arg.Value)
		if IsError(v) {
			return v
		}
		named[arg.Name] = v
	}

	// Map to positional based on parameter metadata.
	args := make([]any, len(params))
	for i, p := range params {
		if v, ok := named[p.Name]; ok {
			args[i] = v
		} else if p.HasDefault {
			args[i] = p.Default
		} else {
			return NewError(fmt.Sprintf("%s: missing required argument %q", context, p.Name))
		}
	}
	return args
}

// resolveNamedMethodArgs maps named arguments to positional using the method's
// parameter metadata. Returns []any or an errorVal.
func (interp *Interpreter) resolveNamedMethodArgs(e *syntax.MethodCallExpr) any {
	var spec MethodSpec
	var specOK bool
	if opc := e.MethodOpcode; opc != 0 {
		if opc >= lambdaOpcodeBase {
			spec = interp.lambdaTable[opc-lambdaOpcodeBase]
		} else {
			spec = methodTable[opc]
		}
		specOK = true
	} else {
		spec, specOK = interp.lookupMethod(e.Method)
	}
	var params []namedArgParam
	if specOK && spec.Params != nil {
		params = make([]namedArgParam, len(spec.Params))
		for i, p := range spec.Params {
			params[i] = namedArgParam(p)
		}
	}
	return interp.resolveNamedArgs(e.Args, params, "."+e.Method+"()")
}

// resolveNamedFuncArgs maps named arguments to positional using the function's
// parameter metadata. Trailing unspecified optional args are truncated so that
// functions using len(args) for optional parameter detection continue to work.
// Returns []any or an errorVal.
func (interp *Interpreter) resolveNamedFuncArgs(e *syntax.CallExpr, spec FunctionSpec) any {
	params := make([]namedArgParam, len(spec.Params))
	for i, p := range spec.Params {
		params[i] = namedArgParam(p)
	}
	resolved := interp.resolveNamedArgs(e.Args, params, e.Name+"()")
	if IsError(resolved) {
		return resolved
	}
	args := resolved.([]any)

	// Truncate trailing default-filled args: find the last parameter position
	// that was explicitly provided and trim the slice there.
	provided := make(map[string]bool, len(e.Args))
	for _, arg := range e.Args {
		provided[arg.Name] = true
	}
	lastExplicit := -1
	for i, p := range spec.Params {
		if provided[p.Name] {
			lastExplicit = i
		}
	}
	if lastExplicit >= 0 && lastExplicit < len(args)-1 {
		args = args[:lastExplicit+1]
	}
	return args
}

// reorderNamedCallArgs reorders named CallArgs to positional order based on
// parameter metadata. Missing optional args are omitted (the method handles
// missing trailing args via len(args) checks internally).
func reorderNamedCallArgs(args []syntax.CallArg, params []MethodParam) []syntax.CallArg {
	byName := make(map[string]syntax.CallArg, len(args))
	for _, arg := range args {
		byName[arg.Name] = arg
	}
	result := make([]syntax.CallArg, 0, len(params))
	for _, p := range params {
		if arg, ok := byName[p.Name]; ok {
			result = append(result, arg)
		} else if !p.HasDefault {
			// Required param missing — append a placeholder that will trigger
			// an error when the method tries to use it. But in practice, the
			// resolver catches arity mismatches at compile time.
			result = append(result, syntax.CallArg{})
		}
		// Optional param missing: omit — method handles via len(args).
	}
	return result
}

func (interp *Interpreter) evalArgs(args []syntax.CallArg) []any {
	result := make([]any, len(args))
	for i, a := range args {
		v := interp.evalExpr(a.Value)
		if IsVoid(v) {
			result[i] = NewError("void passed as argument (use .or() to provide a default)")
		} else if IsDeleted(v) {
			result[i] = NewError("deleted() passed as argument")
		} else {
			result[i] = v
		}
	}
	return result
}

// bindPositionalParams binds evaluated positional args to map parameters,
// handling discard params and AST-expression defaults.
func (interp *Interpreter) bindPositionalParams(s *scope, params []syntax.Param, args []any) string {
	argIdx := 0
	for _, p := range params {
		if p.Discard {
			if argIdx < len(args) {
				argIdx++
			}
			continue
		}
		if argIdx < len(args) {
			s.vars[p.Name] = args[argIdx]
			argIdx++
		} else if p.Default != nil {
			s.vars[p.Name] = interp.evalExpr(p.Default)
		} else {
			return fmt.Sprintf("missing argument for parameter %q", p.Name)
		}
	}
	return ""
}

// bindNamedMapParams resolves named call args to positional order using the
// shared resolveNamedArgs helper, then binds them into the scope. This
// evaluates each argument exactly once.
func (interp *Interpreter) bindNamedMapParams(s *scope, m *syntax.MapDecl, e *syntax.CallExpr) string {
	// Build namedArgParam descriptors, evaluating AST defaults.
	params := make([]namedArgParam, 0, len(m.Params))
	for _, p := range m.Params {
		if p.Discard {
			continue
		}
		nap := namedArgParam{Name: p.Name}
		if p.Default != nil {
			nap.HasDefault = true
			nap.Default = interp.evalExpr(p.Default)
		}
		params = append(params, nap)
	}

	resolved := interp.resolveNamedArgs(e.Args, params, e.Name+"()")
	if IsError(resolved) {
		return ErrorMessage(resolved)
	}
	args := resolved.([]any)

	// Bind into scope (positional order, discard params already excluded).
	for i, p := range params {
		if i < len(args) {
			s.vars[p.Name] = args[i]
		}
	}
	return ""
}

// assignPath sets a value at a path within a root value, auto-creating
// intermediate objects and arrays as needed.
func (interp *Interpreter) assignPath(root *any, path []syntax.PathSegment, value any) {
	if len(path) == 0 {
		*root = DeepClone(value)
		return
	}

	interp.assignPathRecursive(root, path, value)
}

func (interp *Interpreter) assignPathRecursive(current *any, path []syntax.PathSegment, value any) {
	seg := path[0]
	isLast := len(path) == 1

	switch seg.Kind {
	case syntax.PathSegField:
		// Ensure current is an object. Auto-create only from nil.
		obj, ok := (*current).(map[string]any)
		if !ok {
			if *current != nil {
				panic(runtimeError{message: fmt.Sprintf(
					"cannot access field %q on %T (expected object)", seg.Name, *current)})
			}
			obj = make(map[string]any)
			*current = obj
		}

		if isLast {
			if IsDeleted(value) {
				delete(obj, seg.Name)
			} else {
				obj[seg.Name] = value
			}
			return
		}

		child, exists := obj[seg.Name]
		if !exists {
			child = nil // will be auto-created by recursive call
		}
		interp.assignPathRecursive(&child, path[1:], value)
		obj[seg.Name] = child

	case syntax.PathSegIndex:
		idx := interp.evalExpr(seg.Index)
		if IsError(idx) {
			return
		}

		// String index → object field.
		if key, ok := idx.(string); ok {
			obj, ok := (*current).(map[string]any)
			if !ok {
				obj = make(map[string]any)
				*current = obj
			}
			if isLast {
				if IsDeleted(value) {
					delete(obj, key)
				} else {
					obj[key] = value
				}
				return
			}
			child, exists := obj[key]
			if !exists {
				child = make(map[string]any)
			}
			interp.assignPathRecursive(&child, path[1:], value)
			obj[key] = child
			return
		}

		// Integer index → array element.
		i, ok := toInt64(idx)
		if !ok {
			return
		}

		arr, isArr := (*current).([]any)
		if !isArr {
			if *current != nil {
				panic(runtimeError{message: fmt.Sprintf(
					"cannot index into %T (expected array)", *current)})
			}
			// Auto-create array from nil.
			arr = make([]any, 0)
		}

		// Handle negative indexing.
		if i < 0 {
			i += int64(len(arr))
		}

		if isLast && IsDeleted(value) {
			// Delete array element: remove and shift.
			if i < 0 || i >= int64(len(arr)) {
				panic(runtimeError{message: "array index deletion: index out of bounds"})
			}
			arr = append(arr[:i], arr[i+1:]...)
			*current = arr
			return
		}

		// Grow array with null gaps if needed.
		for int64(len(arr)) <= i {
			arr = append(arr, nil)
		}
		*current = arr

		if isLast {
			arr[i] = value
			return
		}

		child := arr[i]
		if child == nil {
			child = make(map[string]any)
		}
		interp.assignPathRecursive(&child, path[1:], value)
		arr[i] = child
	}
}

func (interp *Interpreter) setPath(root *any, path []syntax.PathSegment, value any) {
	interp.assignPath(root, path, value)
}

func (interp *Interpreter) indexValue(receiver, index any) any {
	switch r := receiver.(type) {
	case map[string]any:
		key, ok := index.(string)
		if !ok {
			return NewError(fmt.Sprintf("non-string index on object: got %T", index))
		}
		return r[key]
	case []any:
		return indexSequence(index, int64(len(r)), func(i int64) any { return r[i] })
	case string:
		runes := []rune(r)
		return indexSequence(index, int64(len(runes)), func(i int64) any { return int64(runes[i]) })
	case []byte:
		return indexSequence(index, int64(len(r)), func(i int64) any { return int64(r[i]) })
	case nil:
		return NewError("cannot index null value")
	default:
		return NewError(fmt.Sprintf("cannot index %T", receiver))
	}
}

func indexSequence(index any, length int64, get func(int64) any) any {
	idx, ok := toInt64(index)
	if !ok {
		// Distinguish non-numeric from non-whole-number float.
		if f, isFloat := index.(float64); isFloat {
			if f != math.Trunc(f) {
				return NewError("index must be a whole number, got float with fractional part")
			}
		}
		if f, isFloat := index.(float32); isFloat {
			if float64(f) != math.Trunc(float64(f)) {
				return NewError("index must be a whole number, got float with fractional part")
			}
		}
		return NewError(fmt.Sprintf("non-numeric index: got %T", index))
	}
	if idx < 0 {
		idx += length
	}
	if idx < 0 || idx >= length {
		return NewError("index out of bounds")
	}
	return get(idx)
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case float64:
		if n != math.Trunc(n) || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		if n > math.MaxInt64 || n < math.MinInt64 {
			return 0, false
		}
		return int64(n), true
	case float32:
		f := float64(n)
		if f != math.Trunc(f) || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		if f > math.MaxInt64 || f < math.MinInt64 {
			return 0, false
		}
		return int64(f), true
	default:
		return 0, false
	}
}

func numericNegate(v any) any {
	switch n := v.(type) {
	case int64:
		if n == math.MinInt64 {
			return NewError("int64 overflow")
		}
		return -n
	case int32:
		if n == math.MinInt32 {
			return NewError("int32 overflow")
		}
		return -n
	case float64:
		return -n
	case float32:
		return -n
	case uint32:
		return -int64(n)
	case uint64:
		if n > math.MaxInt64 {
			return NewError("cannot negate uint64 value exceeding int64 range")
		}
		return -int64(n)
	default:
		return NewError(fmt.Sprintf("cannot negate %T", v))
	}
}

// -----------------------------------------------------------------------
// Error handling via panic/recover
// -----------------------------------------------------------------------

type runtimeError struct {
	message string
}

type recursionError struct{}

// Run executes the program with panic recovery, converting runtime panics
// to error returns.
func (interp *Interpreter) Run(input any, metadata map[string]any) (output any, outputMeta map[string]any, deleted bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case runtimeError:
				err = fmt.Errorf("%s", e.message)
			case recursionError:
				err = errors.New("maximum recursion depth exceeded")
			default:
				panic(r) // re-panic for unexpected errors
			}
		}
	}()
	return interp.Exec(input, metadata)
}
