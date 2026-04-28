// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// Migrator translates Bloblang V1 mappings into V2. Construct one with
// New, register any custom rules with RegisterMethodRule /
// RegisterFunctionRule, then call Migrate (any number of times). The
// Migrator is not safe for concurrent registration but is safe for
// concurrent Migrate calls once registration is complete.
type Migrator struct {
	methodRules   map[string]MethodRule
	functionRules map[string]FunctionRule
}

// New creates an empty Migrator with no custom rules registered. The
// built-in V1→V2 rules are always active; custom rules layer on top
// (and shadow built-ins on name collision per design P2).
func New() *Migrator {
	return &Migrator{
		methodRules:   map[string]MethodRule{},
		functionRules: map[string]FunctionRule{},
	}
}

// RegisterMethodRule registers a custom translation rule for a V1
// method named `name`. If a rule is already registered for the same
// name on this Migrator instance the new rule replaces it.
func (m *Migrator) RegisterMethodRule(name string, rule MethodRule) {
	m.methodRules[name] = rule
}

// RegisterFunctionRule registers a custom translation rule for a V1
// function (top-level call) named `name`.
func (m *Migrator) RegisterFunctionRule(name string, rule FunctionRule) {
	m.functionRules[name] = rule
}

// Migrate translates a V1 source mapping into V2. Per-call config —
// verbosity, coverage threshold, mode — is supplied via opts. The
// instance-owned rule registry is wired into the underlying
// translator for the duration of the call.
//
// Returns a *Report on success. Returns *CoverageError when the
// computed coverage falls below opts.MinCoverage; the Report is
// reachable via the error.
func (m *Migrator) Migrate(v1Source string, opts Options) (*Report, error) {
	internalOpts := translator.Options{
		MinCoverage:           opts.MinCoverage,
		Verbose:               opts.Verbose,
		TreatWarningsAsErrors: opts.TreatWarningsAsErrors,
		Files:                 opts.Files,
		Mode:                  opts.Mode,
	}
	if len(m.methodRules) > 0 {
		internalOpts.CustomMethodRules = make(map[string]translator.MethodRuleHook, len(m.methodRules))
		for name, rule := range m.methodRules {
			internalOpts.CustomMethodRules[name] = m.bridgeMethodRule(name, rule)
		}
	}
	if len(m.functionRules) > 0 {
		internalOpts.CustomFunctionRules = make(map[string]translator.FunctionRuleHook, len(m.functionRules))
		for name, rule := range m.functionRules {
			internalOpts.CustomFunctionRules[name] = m.bridgeFunctionRule(name, rule)
		}
	}
	return translator.Migrate(v1Source, internalOpts)
}

// bridgeMethodRule wraps a public MethodRule into the internal hook
// signature. Closure marshals V1 nodes from internal to public, runs
// the user rule, and translates the Result back to the (out,
// handled) tuple the internal translator expects.
func (m *Migrator) bridgeMethodRule(name string, rule MethodRule) translator.MethodRuleHook {
	return func(t translator.Translator, mc *v1ast.MethodCall, _ syntax.Expr) (syntax.Expr, bool) {
		ctx := &Context{t: t, defaultV1: v1Position{Line: mc.NamePos.Line, Column: mc.NamePos.Column}}
		res := rule(ctx, wrapV1MethodCall(mc))
		return resolveResult(t, mc.NamePos, "."+name+"()", res)
	}
}

func (m *Migrator) bridgeFunctionRule(name string, rule FunctionRule) translator.FunctionRuleHook {
	return func(t translator.Translator, fc *v1ast.FunctionCall) (syntax.Expr, bool) {
		ctx := &Context{t: t, defaultV1: v1Position{Line: fc.NamePos.Line, Column: fc.NamePos.Column}}
		res := rule(ctx, wrapV1FunctionCall(fc))
		return resolveResult(t, fc.NamePos, name+"()", res)
	}
}

// resolveResult interprets a public Result inside the internal hook
// contract. The recorder is updated through the public Translator
// interface so a custom rule's outcome moves coverage counters the
// same way a built-in rule's outcome does.
//
//   - Replace bumps Rewritten with a Change naming the V1 callsite,
//     and the supplied V2 expression replaces the V1 node.
//   - Unsupported bumps Unsupported with an Error-severity Change
//     carrying the rule's reason. The translator falls through to
//     its default 1:1 translation so the V2 source still parses
//     (the user is alerted via the Report).
//   - Skip falls through silently — the default 1:1 translation
//     fires and counts itself as Exact. The rule's reason, if any,
//     is logged as an Info Note.
func resolveResult(t translator.Translator, p v1ast.Pos, callsite string, res Result) (syntax.Expr, bool) {
	switch res.kind {
	case resultReplace:
		t.RecordRewritten(translator.Change{
			Line:        p.Line,
			Column:      p.Column,
			Severity:    translator.SeverityInfo,
			Category:    translator.CategoryIdiomRewrite,
			RuleID:      translator.RuleMethodDoesNotExist,
			Explanation: "custom migrator rule rewrote V1 " + callsite,
		})
		return res.expr, true
	case resultUnsupported:
		t.RecordUnsupported(translator.Change{
			Line:        p.Line,
			Column:      p.Column,
			RuleID:      translator.RuleUnsupportedConstruct,
			Explanation: "custom migrator rule could not translate V1 " + callsite + ": " + res.reason,
		})
		return nil, true
	case resultSkip:
		if res.reason != "" {
			t.EmitChange(translator.Change{
				Line:        p.Line,
				Column:      p.Column,
				Severity:    translator.SeverityInfo,
				Category:    translator.CategoryIdiomRewrite,
				Explanation: "custom migrator rule deferred V1 " + callsite + " to default translation: " + res.reason,
			})
		}
		return nil, false
	default:
		// resultUnset — defensive; treat as skip-without-reason.
		return nil, false
	}
}

// Migrate is a package-level convenience that builds a default
// Migrator (no custom rules) and runs it against the supplied source.
// Equivalent to `New().Migrate(src, opts)`. Use this when you only
// need built-in rules; create your own Migrator when you have rules
// to register.
func Migrate(v1Source string, opts Options) (*Report, error) {
	return New().Migrate(v1Source, opts)
}
