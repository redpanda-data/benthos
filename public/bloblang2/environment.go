// Copyright 2026 Redpanda Data, Inc.

package bloblang2

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// pluginNameRegex matches snake-case identifiers, mirroring the V1 constraint.
var pluginNameRegex = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// Environment is a self-contained registry of methods and functions available
// to the mappings it parses. It always inherits the full Bloblang V2 standard
// library; plugins registered here extend (or, via WithoutMethods /
// WithoutFunctions, shadow) that baseline.
//
// Environments are safe for concurrent Parse; concurrent RegisterMethod /
// RegisterFunction calls are serialised internally but callers are expected
// to register all plugins before publishing the Environment for parsing.
type Environment struct {
	mu sync.RWMutex

	pluginMethods   map[string]pluginMethodReg
	pluginFunctions map[string]pluginFunctionReg

	// removedMethods / removedFunctions shadow the stdlib — names listed here
	// are hidden from the resolver, so mappings that reference them fail at
	// parse time with "unknown method" / "unknown function".
	removedMethods   map[string]struct{}
	removedFunctions map[string]struct{}

	onlyPure bool
}

type pluginMethodReg struct {
	spec     eval.MethodSpec
	impure   bool
	variadic bool
}

type pluginFunctionReg struct {
	spec     eval.FunctionSpec
	impure   bool
	variadic bool
}

// globalEnv is the default environment. Plugins registered via the
// package-level RegisterMethod / RegisterFunction functions land here.
var globalEnv = &Environment{
	pluginMethods:   map[string]pluginMethodReg{},
	pluginFunctions: map[string]pluginFunctionReg{},
}

// GlobalEnvironment returns the shared process-wide environment. Registering
// a plugin against this environment makes it available to any caller that
// uses the package-level Parse or omits an explicit environment.
func GlobalEnvironment() *Environment {
	return globalEnv
}

// NewEnvironment returns a fresh environment seeded with the plugins
// currently registered on the global environment. Further registrations on
// the returned environment do not affect the global one.
func NewEnvironment() *Environment {
	return globalEnv.Clone()
}

// NewEmptyEnvironment returns an environment with no plugins registered. It
// still inherits the standard library; only user-defined plugins are absent.
func NewEmptyEnvironment() *Environment {
	return &Environment{
		pluginMethods:   map[string]pluginMethodReg{},
		pluginFunctions: map[string]pluginFunctionReg{},
	}
}

// Clone returns a deep copy of the environment's plugin registry.
func (e *Environment) Clone() *Environment {
	e.mu.RLock()
	defer e.mu.RUnlock()

	clone := &Environment{
		pluginMethods:   make(map[string]pluginMethodReg, len(e.pluginMethods)),
		pluginFunctions: make(map[string]pluginFunctionReg, len(e.pluginFunctions)),
		onlyPure:        e.onlyPure,
	}
	for k, v := range e.pluginMethods {
		clone.pluginMethods[k] = v
	}
	for k, v := range e.pluginFunctions {
		clone.pluginFunctions[k] = v
	}
	if len(e.removedMethods) > 0 {
		clone.removedMethods = make(map[string]struct{}, len(e.removedMethods))
		for k := range e.removedMethods {
			clone.removedMethods[k] = struct{}{}
		}
	}
	if len(e.removedFunctions) > 0 {
		clone.removedFunctions = make(map[string]struct{}, len(e.removedFunctions))
		for k := range e.removedFunctions {
			clone.removedFunctions[k] = struct{}{}
		}
	}
	return clone
}

// WithoutMethods returns a clone with the named methods removed. Removing a
// method that does not exist is a no-op.
func (e *Environment) WithoutMethods(names ...string) *Environment {
	clone := e.Clone()
	if clone.removedMethods == nil {
		clone.removedMethods = make(map[string]struct{}, len(names))
	}
	for _, n := range names {
		clone.removedMethods[n] = struct{}{}
	}
	return clone
}

// WithoutFunctions returns a clone with the named functions removed.
func (e *Environment) WithoutFunctions(names ...string) *Environment {
	clone := e.Clone()
	if clone.removedFunctions == nil {
		clone.removedFunctions = make(map[string]struct{}, len(names))
	}
	for _, n := range names {
		clone.removedFunctions[n] = struct{}{}
	}
	return clone
}

// OnlyPure returns a clone with impure plugins stripped out.
func (e *Environment) OnlyPure() *Environment {
	clone := e.Clone()
	clone.onlyPure = true
	for name, reg := range clone.pluginMethods {
		if reg.impure {
			delete(clone.pluginMethods, name)
		}
	}
	for name, reg := range clone.pluginFunctions {
		if reg.impure {
			delete(clone.pluginFunctions, name)
		}
	}
	return clone
}

// RegisterMethod registers a method plugin against the environment. The
// PluginSpec declares the method's parameters and documentation; the
// constructor builds the runtime Method closure from those arguments.
//
// Plugin names must match the regular expression /^[a-z0-9]+(_[a-z0-9]+)*$/
// (snake case). The spec must be non-nil — pass NewPluginSpec() for a
// method that takes no arguments.
func (e *Environment) RegisterMethod(name string, spec *PluginSpec, ctor MethodConstructor) error {
	if spec == nil {
		return fmt.Errorf("method %q: spec must be non-nil (use NewPluginSpec() for no parameters)", name)
	}
	if err := validatePluginName(name); err != nil {
		return err
	}
	if spec.variadic && len(spec.params) > 0 {
		return fmt.Errorf("method %q: a spec cannot be both Variadic() and declare Param() entries", name)
	}
	if spec.impure && e.onlyPure {
		return fmt.Errorf("cannot register impure method %q in a pure-only environment", name)
	}

	methodSpec := buildMethodSpec(name, spec, ctor)

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.pluginMethods[name]; exists {
		return fmt.Errorf("method %q is already registered", name)
	}
	if isStdlibMethod(name) {
		return fmt.Errorf("method %q shadows a standard library method", name)
	}
	if e.pluginMethods == nil {
		e.pluginMethods = make(map[string]pluginMethodReg)
	}
	e.pluginMethods[name] = pluginMethodReg{spec: methodSpec, impure: spec.impure, variadic: spec.variadic}
	return nil
}

// RegisterFunction registers a function plugin against the environment. See
// RegisterMethod for the parameter-declaration rules; those apply identically
// to functions.
func (e *Environment) RegisterFunction(name string, spec *PluginSpec, ctor FunctionConstructor) error {
	if spec == nil {
		return fmt.Errorf("function %q: spec must be non-nil (use NewPluginSpec() for no parameters)", name)
	}
	if err := validatePluginName(name); err != nil {
		return err
	}
	if spec.variadic && len(spec.params) > 0 {
		return fmt.Errorf("function %q: a spec cannot be both Variadic() and declare Param() entries", name)
	}
	if spec.impure && e.onlyPure {
		return fmt.Errorf("cannot register impure function %q in a pure-only environment", name)
	}

	funcSpec := buildFunctionSpec(name, spec, ctor)

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.pluginFunctions[name]; exists {
		return fmt.Errorf("function %q is already registered", name)
	}
	if isStdlibFunction(name) {
		return fmt.Errorf("function %q shadows a standard library function", name)
	}
	if e.pluginFunctions == nil {
		e.pluginFunctions = make(map[string]pluginFunctionReg)
	}
	e.pluginFunctions[name] = pluginFunctionReg{spec: funcSpec, impure: spec.impure, variadic: spec.variadic}
	return nil
}

func validatePluginName(name string) error {
	if !pluginNameRegex.MatchString(name) {
		return fmt.Errorf("plugin name %q must be snake-case (matching %s)", name, pluginNameRegex.String())
	}
	return nil
}

// isStdlibMethod reports whether a name collides with a registered stdlib
// method opcode.
func isStdlibMethod(name string) bool {
	methods, _ := eval.StdlibOpcodes()
	_, ok := methods[name]
	return ok
}

// isStdlibFunction is the function analogue of isStdlibMethod.
func isStdlibFunction(name string) bool {
	_, functions := eval.StdlibOpcodes()
	_, ok := functions[name]
	return ok
}

// buildMethodSpec converts a PluginSpec + constructor pair into the internal
// eval.MethodSpec used by the interpreter. The returned spec carries both a
// per-call Fn (for dynamic arguments) and a CallFolder that pre-binds the
// constructor at parse time when every argument is a literal.
func buildMethodSpec(name string, spec *PluginSpec, ctor MethodConstructor) eval.MethodSpec {
	params := pluginParamsToMethodParams(spec)
	runMethod := func(receiver any, fn Method) any {
		out, err := fn(receiver)
		if err != nil {
			return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
		}
		return out
	}
	return eval.MethodSpec{
		Fn: func(receiver any, args []any) any {
			parsed, err := newParsedParams(spec, args)
			if err != nil {
				return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
			}
			fn, err := ctor(parsed)
			if err != nil {
				return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
			}
			return runMethod(receiver, fn)
		},
		Params: params,
		CallFolder: func(callArgs []syntax.CallArg) (any, error) {
			parsed, ok, err := foldLiteralArgs(name, spec, callArgs)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, nil
			}
			fn, err := ctor(parsed)
			if err != nil {
				return nil, fmt.Errorf("%s(): %s", name, err.Error())
			}
			return eval.PreboundMethod(func(receiver any) any {
				return runMethod(receiver, fn)
			}), nil
		},
	}
}

// buildFunctionSpec mirrors buildMethodSpec for stdlib-style functions.
func buildFunctionSpec(name string, spec *PluginSpec, ctor FunctionConstructor) eval.FunctionSpec {
	params := pluginParamsToFunctionParams(spec)
	runFunction := func(fn Function) any {
		out, err := fn()
		if err != nil {
			return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
		}
		return out
	}
	return eval.FunctionSpec{
		Fn: func(args []any) any {
			parsed, err := newParsedParams(spec, args)
			if err != nil {
				return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
			}
			fn, err := ctor(parsed)
			if err != nil {
				return eval.NewError(fmt.Sprintf("%s(): %s", name, err.Error()))
			}
			return runFunction(fn)
		},
		Params: params,
		CallFolder: func(callArgs []syntax.CallArg) (any, error) {
			parsed, ok, err := foldLiteralArgs(name, spec, callArgs)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, nil
			}
			fn, err := ctor(parsed)
			if err != nil {
				return nil, fmt.Errorf("%s(): %s", name, err.Error())
			}
			return eval.PreboundFunction(func() any {
				return runFunction(fn)
			}), nil
		},
	}
}

// foldLiteralArgs walks a call's AST arguments and, if every argument is a
// literal, returns a ParsedParams suitable for invoking the plugin
// constructor at parse time. Returns (nil, false, nil) when any argument is
// dynamic or when the shape isn't eligible for folding (e.g. named args).
// Returns (nil, false, err) when the literal values don't satisfy the spec
// (missing required, wrong type) so the resolver can surface the problem.
func foldLiteralArgs(name string, spec *PluginSpec, args []syntax.CallArg) (*ParsedParams, bool, error) {
	for _, a := range args {
		if a.Name != "" {
			// Named arguments aren't folded in Phase 1.
			return nil, false, nil
		}
		if _, lit := a.Value.(*syntax.LiteralExpr); !lit {
			return nil, false, nil
		}
	}
	raw := make([]any, len(args))
	for i, a := range args {
		v, err := literalValue(a.Value.(*syntax.LiteralExpr))
		if err != nil {
			return nil, false, nil
		}
		raw[i] = v
	}
	parsed, err := newParsedParams(spec, raw)
	if err != nil {
		return nil, false, fmt.Errorf("%s(): %s", name, err.Error())
	}
	return parsed, true, nil
}

// literalValue converts a parsed LiteralExpr into the Go value the runtime
// would produce for it. Mirrors eval.evalLiteral; kept local to avoid a
// cross-package dependency.
func literalValue(lit *syntax.LiteralExpr) (any, error) {
	switch lit.TokenType {
	case syntax.STRING, syntax.RAW_STRING:
		return lit.Value, nil
	case syntax.INT:
		n, err := strconv.ParseInt(lit.Value, 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case syntax.FLOAT:
		f, err := strconv.ParseFloat(lit.Value, 64)
		if err != nil {
			return nil, err
		}
		return f, nil
	case syntax.TRUE:
		return true, nil
	case syntax.FALSE:
		return false, nil
	case syntax.NULL:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported literal token %v", lit.TokenType)
	}
}

// pluginParamsToMethodParams maps the public ParamDefinition slice onto the
// internal MethodParam slice used by the resolver for arity checking. A
// variadic spec yields nil params (equivalent to "no arity check").
func pluginParamsToMethodParams(spec *PluginSpec) []eval.MethodParam {
	if spec.variadic || len(spec.params) == 0 {
		return nil
	}
	out := make([]eval.MethodParam, len(spec.params))
	for i, p := range spec.params {
		out[i] = eval.MethodParam{
			Name:       p.name,
			Default:    p.defaultVal,
			HasDefault: p.hasDefault || p.optional,
		}
	}
	return out
}

// pluginParamsToFunctionParams is the function analogue of
// pluginParamsToMethodParams.
func pluginParamsToFunctionParams(spec *PluginSpec) []eval.FunctionParam {
	if spec.variadic || len(spec.params) == 0 {
		return nil
	}
	out := make([]eval.FunctionParam, len(spec.params))
	for i, p := range spec.params {
		out[i] = eval.FunctionParam{
			Name:       p.name,
			Default:    p.defaultVal,
			HasDefault: p.hasDefault || p.optional,
		}
	}
	return out
}

// Parse compiles a Bloblang V2 mapping against this environment. The
// returned Executor is safe for concurrent use.
func (e *Environment) Parse(src string) (*Executor, error) {
	prog, perrs := syntax.Parse(src, "", nil)
	if len(perrs) > 0 {
		return nil, parseErrorFromPosErrors(perrs)
	}
	syntax.Optimize(prog)

	methodInfos, functionInfos, methodOpcodes, functionOpcodes := e.resolverInputs()

	rerrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methodInfos,
		Functions:       functionInfos,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	})
	if len(rerrs) > 0 {
		return nil, parseErrorFromPosErrors(rerrs)
	}

	pluginMethods, pluginFunctions := e.snapshotPlugins()

	return newExecutor(prog, pluginMethods, pluginFunctions), nil
}

// resolverInputs builds the four maps the resolver consumes, merging the
// standard library with the environment's plugin registry and stripping any
// names removed via WithoutMethods / WithoutFunctions.
func (e *Environment) resolverInputs() (
	methods map[string]syntax.MethodInfo,
	functions map[string]syntax.FunctionInfo,
	methodOpcodes map[string]uint16,
	functionOpcodes map[string]uint16,
) {
	stdlibMethods, stdlibFunctions := eval.StdlibNames()
	stdlibMethodOpc, stdlibFunctionOpc := eval.StdlibOpcodes()

	methods = make(map[string]syntax.MethodInfo, len(stdlibMethods))
	for k, v := range stdlibMethods {
		methods[k] = v
	}
	functions = make(map[string]syntax.FunctionInfo, len(stdlibFunctions))
	for k, v := range stdlibFunctions {
		functions[k] = v
	}
	methodOpcodes = make(map[string]uint16, len(stdlibMethodOpc))
	for k, v := range stdlibMethodOpc {
		methodOpcodes[k] = v
	}
	functionOpcodes = make(map[string]uint16, len(stdlibFunctionOpc))
	for k, v := range stdlibFunctionOpc {
		functionOpcodes[k] = v
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for name := range e.removedMethods {
		delete(methods, name)
		delete(methodOpcodes, name)
	}
	for name := range e.removedFunctions {
		delete(functions, name)
		delete(functionOpcodes, name)
	}

	// Plugin names override any same-named stdlib entries. They have no
	// opcode — dispatch falls back to interp.lookupMethod by name.
	for name, reg := range e.pluginMethods {
		methods[name] = eval.MethodSpecToInfo(reg.spec)
		delete(methodOpcodes, name)
	}
	for name, reg := range e.pluginFunctions {
		info := eval.FunctionSpecToInfo(reg.spec)
		if reg.variadic {
			// FunctionInfo.Total == -1 disables resolver arity checking.
			info.Total = -1
			info.Required = 0
		}
		functions[name] = info
		delete(functionOpcodes, name)
	}

	return
}

// snapshotPlugins copies the current plugin registry so Executors remain
// independent of later registrations.
func (e *Environment) snapshotPlugins() (
	methods map[string]eval.MethodSpec,
	functions map[string]eval.FunctionSpec,
) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	methods = make(map[string]eval.MethodSpec, len(e.pluginMethods))
	for name, reg := range e.pluginMethods {
		methods[name] = reg.spec
	}
	functions = make(map[string]eval.FunctionSpec, len(e.pluginFunctions))
	for name, reg := range e.pluginFunctions {
		functions[name] = reg.spec
	}
	return
}

// ----------------------------------------------------------------------------
// Package-level conveniences that operate on the global environment.
// ----------------------------------------------------------------------------

// Parse compiles src against the global environment.
func Parse(src string) (*Executor, error) {
	return globalEnv.Parse(src)
}

// RegisterMethod registers a method plugin on the global environment.
func RegisterMethod(name string, spec *PluginSpec, ctor MethodConstructor) error {
	return globalEnv.RegisterMethod(name, spec, ctor)
}

// MustRegisterMethod is RegisterMethod but panics on failure.
func MustRegisterMethod(name string, spec *PluginSpec, ctor MethodConstructor) {
	if err := RegisterMethod(name, spec, ctor); err != nil {
		panic(err)
	}
}

// RegisterFunction registers a function plugin on the global environment.
func RegisterFunction(name string, spec *PluginSpec, ctor FunctionConstructor) error {
	return globalEnv.RegisterFunction(name, spec, ctor)
}

// MustRegisterFunction is RegisterFunction but panics on failure.
func MustRegisterFunction(name string, spec *PluginSpec, ctor FunctionConstructor) {
	if err := RegisterFunction(name, spec, ctor); err != nil {
		panic(err)
	}
}
