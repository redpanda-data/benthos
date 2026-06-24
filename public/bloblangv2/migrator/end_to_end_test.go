// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestEndToEndCustomPlugin walks the full lifecycle a downstream
// repository would follow:
//
//  1. Register a fictional V1 plugin with the public bloblang
//     environment (mirroring how a real downstream library uses
//     RegisterMethodV2).
//  2. Register a matching V2 plugin so V2 mappings can call it.
//  3. Register a custom migrator rule that translates the V1 callsite
//     to the V2 callsite.
//  4. Run a V1 source through the migrator and confirm the output
//     compiles in V2 and produces the expected value end-to-end.
//
// The plugin is registered against fresh, isolated environments so
// the test doesn't pollute global state.
func TestEndToEndCustomPlugin(t *testing.T) {
	endToEndOnce.Do(registerEndToEndPlugins)

	mig := migrator.New()
	mig.RegisterMethodRule("widget_double", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		if len(m.Args) != 0 {
			return ctx.Unsupported("widget_double takes no arguments")
		}
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_double_v2",
		})
	})

	const v1Source = `root.doubled = this.value.widget_double()`
	rep, err := mig.Migrate(v1Source, migrator.Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(rep.V2Mapping, ".widget_double_v2()") {
		t.Fatalf("expected .widget_double_v2() in V2 output, got:\n%s", rep.V2Mapping)
	}

	v1Exec, err := bloblang.NewEnvironment().Parse(v1Source)
	if err != nil {
		t.Fatalf("v1 compile: %v", err)
	}
	v1Out, err := v1Exec.Query(map[string]any{"value": int64(3)})
	if err != nil {
		t.Fatalf("v1 exec: %v", err)
	}
	v1Map, ok := v1Out.(map[string]any)
	if !ok {
		t.Fatalf("expected v1 output to be an object, got %T", v1Out)
	}
	if v1Map["doubled"] != int64(6) {
		t.Fatalf("v1 widget_double(3) expected 6, got %v", v1Map["doubled"])
	}

	v2Exec, err := bloblangv2.GlobalEnvironment().Parse(rep.V2Mapping)
	if err != nil {
		t.Fatalf("v2 compile (translated):\n%s\nerr: %v", rep.V2Mapping, err)
	}
	v2Out, err := v2Exec.Query(map[string]any{"value": int64(3)})
	if err != nil {
		t.Fatalf("v2 exec: %v", err)
	}
	v2Map := v2Out.(map[string]any)
	if v2Map["doubled"] != int64(6) {
		t.Fatalf("v2 widget_double_v2(3) expected 6, got %v", v2Map["doubled"])
	}
}

var endToEndOnce sync.Once

func registerEndToEndPlugins() {
	// V1: register on the global bloblang environment.
	if err := bloblang.RegisterMethodV2("widget_double",
		bloblang.NewPluginSpec().Description("Doubles the receiver integer."),
		func(_ *bloblang.ParsedParams) (bloblang.Method, error) {
			return func(v any) (any, error) {
				switch n := v.(type) {
				case int64:
					return n * 2, nil
				case float64:
					return n * 2, nil
				}
				return nil, nil
			}, nil
		},
	); err != nil {
		panic("v1 widget_double registration: " + err.Error())
	}

	// V2: register on the global bloblangv2 environment under the new name.
	if err := bloblangv2.RegisterMethod("widget_double_v2",
		bloblangv2.NewPluginSpec().Description("Doubles the receiver integer."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) {
				switch n := v.(type) {
				case int64:
					return n * 2, nil
				case float64:
					return n * 2, nil
				}
				return nil, nil
			}, nil
		},
	); err != nil {
		panic("v2 widget_double_v2 registration: " + err.Error())
	}
}
