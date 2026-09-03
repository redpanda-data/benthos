// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/public/service/migrator"
)

// TestRegisterRuleErrors covers the error-returning RegisterRule API and its
// Must* panic variant.
func TestRegisterRuleErrors(t *testing.T) {
	mig := migrator.New()
	target := migrator.Target{ComponentType: "processor", Name: "widget"}
	rule := func(*migrator.Context, *migrator.Component) migrator.Result { return migrator.Result{} }

	if err := mig.RegisterRule(target, nil); err == nil {
		t.Error("nil rule should error")
	}
	if err := mig.RegisterRule(target, rule); err != nil {
		t.Errorf("valid registration errored: %v", err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("MustRegisterRule should panic on a nil rule")
			}
		}()
		mig.MustRegisterRule(target, nil)
	}()
}
