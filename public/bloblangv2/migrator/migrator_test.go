// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestDefaultMigrate exercises the package-level Migrate helper to
// confirm the public API behaves the same as the internal translator
// when no custom rules are registered.
func TestDefaultMigrate(t *testing.T) {
	rep, err := migrator.Migrate(`root.x = this.y`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "output.x") {
		t.Fatalf("expected output.x in V2 mapping, got:\n%s", rep.V2Mapping)
	}
	if !strings.Contains(rep.V2Mapping, "input") {
		t.Fatalf("expected input rewrite in V2 mapping, got:\n%s", rep.V2Mapping)
	}
}

// TestRegisterMethodRuleReplace registers a custom rule that maps a
// fictional V1 method `widget_encode` onto a V2 `widget_encode_v2`
// method call. The fictional plugin has no V1 stdlib counterpart —
// the rule fires solely because the migrator dispatched to a
// registered custom rule.
func TestRegisterMethodRuleReplace(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_encode", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		if len(m.Args) != 0 {
			return ctx.Unsupported("widget_encode takes no arguments in V1")
		}
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_encode_v2",
		})
	})

	rep, err := mig.Migrate(`root.encoded = this.payload.widget_encode()`, migrator.Options{Verbose: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, ".widget_encode_v2()") {
		t.Fatalf("expected .widget_encode_v2() in V2 mapping, got:\n%s", rep.V2Mapping)
	}
	if strings.Contains(rep.V2Mapping, "widget_encode(") {
		t.Fatalf("V1 method name leaked into V2 mapping:\n%s", rep.V2Mapping)
	}
}

// TestRegisterMethodRuleArgs exercises argument inspection: the rule
// pulls a literal, recursively translates a non-literal arg via
// ctx.Translate, and constructs a V2 method call with both.
func TestRegisterMethodRuleArgs(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_pack", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		if len(m.Args) != 2 {
			return ctx.Unsupported("widget_pack expects two arguments")
		}
		// First arg should be a string literal naming the format.
		fmtLit, ok := m.Args[0].Value.(*migrator.V1Literal)
		if !ok || fmtLit.Kind != migrator.V1LitString {
			return ctx.Unsupported("widget_pack: first argument must be a string literal")
		}
		// Second arg is opaque; recurse via Translate.
		payload := ctx.Translate(m.Args[1].Value)
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_pack_v2",
			Args: []migrator.V2CallArg{
				{Value: &migrator.V2LiteralExpr{Kind: migrator.V2LitString, Str: fmtLit.Str}},
				{Value: payload},
			},
		})
	})

	rep, err := mig.Migrate(`root.x = this.handle.widget_pack("brotli", this.payload)`, migrator.Options{Verbose: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "widget_pack_v2") {
		t.Fatalf("expected widget_pack_v2 in output:\n%s", rep.V2Mapping)
	}
	if !strings.Contains(rep.V2Mapping, `"brotli"`) {
		t.Fatalf("expected literal arg preserved in output:\n%s", rep.V2Mapping)
	}
}

// TestRegisterMethodRuleSkip exercises the Skip return path: the rule
// declines to transform on a shape it doesn't recognise, and the
// translator falls through to its default 1:1 translation.
func TestRegisterMethodRuleSkip(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_passthrough", func(ctx *migrator.Context, _ *migrator.V1MethodCall) migrator.Result {
		return ctx.Skip("widget_passthrough is identity in both versions")
	})

	rep, err := mig.Migrate(`root.x = this.handle.widget_passthrough()`, migrator.Options{Verbose: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Default translation preserves the method name.
	if !strings.Contains(rep.V2Mapping, ".widget_passthrough()") {
		t.Fatalf("expected method name preserved by default translation:\n%s", rep.V2Mapping)
	}
}

// TestRegisterMethodRuleUnsupported asserts the Unsupported path
// records an Error-severity Change.
func TestRegisterMethodRuleUnsupported(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_dynamic", func(ctx *migrator.Context, _ *migrator.V1MethodCall) migrator.Result {
		return ctx.Unsupported("dynamic dispatch has no V2 equivalent")
	})

	rep, err := mig.Migrate(`root.x = this.h.widget_dynamic()`, migrator.Options{Verbose: true, MinCoverage: 0.0001})
	// The Unsupported counts against coverage; below the default
	// threshold we'd see CoverageError. We set MinCoverage near zero
	// to surface the Report directly.
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var sawUnsupported bool
	for _, ch := range rep.Changes {
		if ch.Severity == migrator.SeverityError && strings.Contains(ch.Explanation, "dynamic dispatch") {
			sawUnsupported = true
			break
		}
	}
	if !sawUnsupported {
		t.Fatalf("expected an Error-severity Unsupported change with the rule's reason; got changes:\n%v", rep.Changes)
	}
}

// TestRegisterMethodRuleOverride registers a rule for a name that has
// a built-in translation (without). The custom rule must win,
// confirming the design P2 precedence model.
func TestRegisterMethodRuleOverride(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("without", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		// Custom override: rewrite into a fictional .strip() method.
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "strip",
		})
	})
	rep, err := mig.Migrate(`root = this.without("a", "b")`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, ".strip()") {
		t.Fatalf("custom rule should win over built-in .without rewrite, got:\n%s", rep.V2Mapping)
	}
	if strings.Contains(rep.V2Mapping, ".without(") {
		t.Fatalf("built-in .without rewrite leaked despite custom override:\n%s", rep.V2Mapping)
	}
}

// TestRegisterFunctionRule exercises the function-rule path with a
// fictional V1 function that V2 turns into a method call on `input`.
func TestRegisterFunctionRule(t *testing.T) {
	mig := migrator.New()
	mig.RegisterFunctionRule("widget_size", func(ctx *migrator.Context, f *migrator.V1FunctionCall) migrator.Result {
		if len(f.Args) != 0 {
			return ctx.Unsupported("widget_size takes no arguments")
		}
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: &migrator.V2InputExpr{},
			Method:   "widget_size",
		})
	})
	rep, err := mig.Migrate(`root.size = widget_size()`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, "input.widget_size()") {
		t.Fatalf("expected input.widget_size() in output:\n%s", rep.V2Mapping)
	}
}

// TestNoteEmitsExtraDiagnostic ensures ctx.Note records a Change
// alongside the Result without breaking coverage.
func TestNoteEmitsExtraDiagnostic(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_encode", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		ctx.Note(migrator.Change{
			Severity:    migrator.SeverityWarning,
			Category:    migrator.CategorySemanticChange,
			Explanation: "widget_encode now defaults to UTF-8 in V2",
		})
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_encode_v2",
		})
	})
	rep, err := mig.Migrate(`root.x = this.h.widget_encode()`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var sawNote bool
	for _, ch := range rep.Changes {
		if ch.Severity == migrator.SeverityWarning && strings.Contains(ch.Explanation, "UTF-8") {
			sawNote = true
			break
		}
	}
	if !sawNote {
		t.Fatalf("expected the rule's Note in the change list, got:\n%v", rep.Changes)
	}
}

// TestPushScopeAndThisRebind walks the translator scope APIs through
// a synthetic lambda body the rule constructs from scratch.
func TestPushScopeAndThisRebind(t *testing.T) {
	mig := migrator.New()
	// Fake V1 method: turn `recv.where(predicate)` into a V2
	// `recv.find_by(__v -> <predicate>)` lambda where the predicate
	// has its `this` references rebound to __v.
	mig.RegisterMethodRule("where", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		if len(m.Args) != 1 {
			return ctx.Unsupported("where: need exactly one predicate argument")
		}
		const param = "__v"
		ctx.PushScope(param)
		ctx.PushThisRebind(param)
		body := ctx.Translate(m.Args[0].Value)
		ctx.PopThisRebind()
		ctx.PopScope()
		if body == nil {
			return ctx.Unsupported("where: predicate failed to translate")
		}
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "find_by",
			Args: []migrator.V2CallArg{{
				Value: &migrator.V2LambdaExpr{
					Params: []migrator.V2LambdaParam{{Name: param}},
					Body:   body,
				},
			}},
		})
	})
	rep, err := mig.Migrate(`root.match = this.items.where(this.id == 5)`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, ".find_by(__v ->") {
		t.Fatalf("expected synthesized lambda in output:\n%s", rep.V2Mapping)
	}
	if !strings.Contains(rep.V2Mapping, "__v") || !strings.Contains(rep.V2Mapping, "id == 5") {
		t.Fatalf("rebinding did not replace `this` in predicate body:\n%s", rep.V2Mapping)
	}
}

// TestCoverageReflectsReplace asserts a custom Replace bumps the
// Rewritten counter so coverage stats stay honest. Without proper
// recorder hooks the counter would silently stay at zero and the
// coverage gate would lie.
func TestCoverageReflectsReplace(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_encode", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_encode_v2",
		})
	})
	rep, err := mig.Migrate(`root.x = this.h.widget_encode()`, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if rep.Coverage.Rewritten == 0 {
		t.Fatalf("expected Rewritten counter to bump on a Replace; got coverage %+v", rep.Coverage)
	}
}

// TestCoverageReflectsUnsupported asserts a custom Unsupported bumps
// the Unsupported counter (mirroring how built-in Unsupported flows
// through recorder.Unsupported).
func TestCoverageReflectsUnsupported(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_dynamic", func(ctx *migrator.Context, _ *migrator.V1MethodCall) migrator.Result {
		return ctx.Unsupported("dynamic dispatch has no V2 equivalent")
	})
	rep, err := mig.Migrate(`root.x = this.h.widget_dynamic()`, migrator.Options{MinCoverage: 0.0001})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if rep.Coverage.Unsupported == 0 {
		t.Fatalf("expected Unsupported counter to bump on an Unsupported result; got coverage %+v", rep.Coverage)
	}
}

// TestMigrateConcurrentSafe asserts the Migrator is safe for
// concurrent Migrate calls once registration completes.
func TestMigrateConcurrentSafe(t *testing.T) {
	mig := migrator.New()
	mig.RegisterMethodRule("widget", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_v2",
		})
	})
	const goroutines = 8
	done := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := mig.Migrate(`root.x = this.h.widget()`, migrator.Options{})
			done <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent migrate failed: %v", err)
		}
	}
}
