// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestRegisterRuleErrors covers the error-returning registration API: invalid
// registrations report an error rather than silently no-op'ing, and the Must*
// variants panic. (The error return is what leaves room to add validation
// without a breaking signature change.)
func TestRegisterRuleErrors(t *testing.T) {
	mig := migrator.New()
	mrule := func(*migrator.Context, *migrator.V1MethodCall) migrator.Result { return migrator.Result{} }
	frule := func(*migrator.Context, *migrator.V1FunctionCall) migrator.Result { return migrator.Result{} }

	if err := mig.RegisterMethodRule("", mrule); err == nil {
		t.Error("empty method-rule name should error")
	}
	if err := mig.RegisterMethodRule("x", nil); err == nil {
		t.Error("nil method rule should error")
	}
	if err := mig.RegisterMethodRule("x", mrule); err != nil {
		t.Errorf("valid method-rule registration errored: %v", err)
	}
	if err := mig.RegisterFunctionRule("", frule); err == nil {
		t.Error("empty function-rule name should error")
	}
	if err := mig.RegisterFunctionRule("y", nil); err == nil {
		t.Error("nil function rule should error")
	}
	if err := mig.RegisterFunctionRule("y", frule); err != nil {
		t.Errorf("valid function-rule registration errored: %v", err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("MustRegisterMethodRule should panic on a nil rule")
			}
		}()
		mig.MustRegisterMethodRule("z", nil)
	}()
}
