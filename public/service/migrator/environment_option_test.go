// Copyright 2026 Redpanda Data, Inc.

package migrator

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/public/service"
)

// TestResolveProviderEnvironment covers Options.Environment plumbing: a nil
// environment resolves to the global component registry, and a custom
// *service.Environment is threaded through to the config walker (so a
// distribution can migrate configs against a non-global bundle).
func TestResolveProviderEnvironment(t *testing.T) {
	if got := resolveProvider(nil); got != bundle.GlobalEnvironment {
		t.Errorf("nil environment should resolve to the global environment, got %v", got)
	}
	custom := service.NewEmptyEnvironment()
	if got := resolveProvider(custom); got == bundle.GlobalEnvironment {
		t.Error("a custom environment should resolve to its own registry, not the global one")
	}

	// A zero-value &service.Environment{} unwraps to a nil *bundle.Environment;
	// resolveProvider must fall back to the global rather than returning a
	// non-nil docs.Provider wrapping a nil pointer (which panics on use).
	if got := resolveProvider(&service.Environment{}); got != bundle.GlobalEnvironment {
		t.Errorf("zero-value environment should fall back to the global, got %v", got)
	}
}
