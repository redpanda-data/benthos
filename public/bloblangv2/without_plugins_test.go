// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"fmt"
	"strings"
	"testing"

	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// TestWithoutRemovesPlugins guards the sandbox-safety fix: WithoutMethods /
// WithoutFunctions must remove PLUGIN names too, not just shadow same-named
// stdlib entries. Previously resolverInputs deleted the name then
// unconditionally re-added the plugin (and snapshotPlugins ignored the removal
// set entirely), so a "removed" plugin survived both parse-time resolution and
// runtime dispatch — a hole in how distributions gate a sandboxed environment.
func TestWithoutRemovesPlugins(t *testing.T) {
	env := bloblangv2.NewEnvironment()
	if err := env.RegisterMethod("shout", bloblangv2.NewPluginSpec(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) { return fmt.Sprint(v) + "!", nil }, nil
		}); err != nil {
		t.Fatalf("RegisterMethod: %v", err)
	}
	if err := env.RegisterFunction("magic", bloblangv2.NewPluginSpec(),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) { return int64(42), nil }, nil
		}); err != nil {
		t.Fatalf("RegisterFunction: %v", err)
	}

	// Sanity: the plugins work in the un-stripped environment.
	if ex, err := env.Parse(`output = "hi".shout()`); err != nil {
		t.Fatalf("baseline method parse: %v", err)
	} else if out, err := ex.Query(map[string]any{}); err != nil || out != "hi!" {
		t.Fatalf("baseline method exec: out=%v err=%v", out, err)
	}

	// After removal, the plugin must be gone — rejected at parse, or (if it
	// somehow compiles) erroring at runtime. It must NOT execute successfully.
	assertGone := func(t *testing.T, e *bloblangv2.Environment, mapping, unwanted string) {
		t.Helper()
		ex, err := e.Parse(mapping)
		if err != nil {
			return // rejected at parse — good
		}
		out, err := ex.Query(map[string]any{})
		if err != nil {
			return // errored at runtime — acceptable
		}
		t.Fatalf("removed plugin %s still executed: %q -> %v", unwanted, mapping, out)
	}

	assertGone(t, env.WithoutMethods("shout"), `output = "hi".shout()`, ".shout()")
	assertGone(t, env.WithoutFunctions("magic"), `output = magic()`, "magic()")

	// Removing a plugin must not disturb the original environment or unrelated
	// stdlib.
	if ex, err := env.Parse(`output = "x".shout().uppercase()`); err != nil {
		t.Fatalf("original env method still expected: %v", err)
	} else if out, _ := ex.Query(map[string]any{}); !strings.Contains(fmt.Sprint(out), "X!") {
		t.Fatalf("original env should be unaffected, got %v", out)
	}
}
