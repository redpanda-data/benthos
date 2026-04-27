// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

func TestEnvironmentWalkFunctions(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()

	spec := bloblangv2.NewPluginSpec().
		Description("returns a constant greeting").
		Category("Test").
		Version("0.1.0").
		Param(bloblangv2.NewStringParam("name").Description("name to greet").Default("world"))

	if err := env.RegisterFunction("greet", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) { return "hello", nil }, nil
	}); err != nil {
		t.Fatal(err)
	}

	var seen []bloblangv2.PluginInfo
	env.WalkFunctions(func(_ string, view *bloblangv2.FunctionView) {
		seen = append(seen, view.Info())
	})

	if len(seen) != 1 {
		t.Fatalf("expected 1 function, got %d", len(seen))
	}
	got := seen[0]
	if got.Name != "greet" {
		t.Fatalf("name=%q", got.Name)
	}
	if got.Description != "returns a constant greeting" {
		t.Fatalf("description=%q", got.Description)
	}
	if got.Category != "Test" {
		t.Fatalf("category=%q", got.Category)
	}
	if got.Version != "0.1.0" {
		t.Fatalf("version=%q", got.Version)
	}
	if len(got.Params) != 1 {
		t.Fatalf("params=%v", got.Params)
	}
	if got.Params[0].Name != "name" {
		t.Fatalf("param name=%q", got.Params[0].Name)
	}
	if got.Params[0].Kind != "string" {
		t.Fatalf("param kind=%q", got.Params[0].Kind)
	}
	if !got.Params[0].HasDefault || got.Params[0].Default != "world" {
		t.Fatalf("param default not preserved: %+v", got.Params[0])
	}
}

func TestEnvironmentWalkMethodsExcludesStdlib(t *testing.T) {
	env := bloblangv2.NewEnvironment()

	if err := env.RegisterMethod("bang", bloblangv2.NewPluginSpec(), func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		return bloblangv2.StringMethod(func(s string) (any, error) { return s + "!", nil }), nil
	}); err != nil {
		t.Fatal(err)
	}

	var names []string
	env.WalkMethods(func(name string, _ *bloblangv2.MethodView) {
		names = append(names, name)
	})
	sort.Strings(names)

	// Stdlib methods like length / uppercase / contains must NOT show up; only
	// the user-registered "bang" method should be enumerated.
	if len(names) != 1 || names[0] != "bang" {
		t.Fatalf("walk should yield only user plugins, got %v", names)
	}
}

func TestPluginParamInfoUnmarshalNormalisesNumericDefaults(t *testing.T) {
	// JSON has no integer type — encoding/json decodes every number as
	// float64. The custom UnmarshalJSON on PluginParamInfo coerces Default
	// back to int64 / float64 according to the declared Kind so that
	// decoded specs match the Go types of the original registration.
	cases := []struct {
		raw      string
		wantKind string
		wantType string
		wantVal  any
	}{
		{`{"name":"a","kind":"int64","has_default":true,"default":5}`, "int64", "int64", int64(5)},
		{`{"name":"b","kind":"float64","has_default":true,"default":1.5}`, "float64", "float64", float64(1.5)},
		{`{"name":"c","kind":"string","has_default":true,"default":"hi"}`, "string", "string", "hi"},
		{`{"name":"d","kind":"bool","has_default":true,"default":true}`, "bool", "bool", true},
	}
	for _, tc := range cases {
		t.Run(tc.wantKind, func(t *testing.T) {
			var p bloblangv2.PluginParamInfo
			if err := json.Unmarshal([]byte(tc.raw), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if p.Kind != tc.wantKind {
				t.Fatalf("kind=%q", p.Kind)
			}
			if got := fmt.Sprintf("%T", p.Default); got != tc.wantType {
				t.Fatalf("default type = %s, want %s (value=%v)", got, tc.wantType, p.Default)
			}
			if p.Default != tc.wantVal {
				t.Fatalf("default value = %v, want %v", p.Default, tc.wantVal)
			}
		})
	}
}

func TestNewPluginSpecFromInfoRoundTripsRegistration(t *testing.T) {
	// Construct a PluginSpec via the public builders, dump → re-load, and
	// confirm we can register the reconstructed spec without losing param
	// metadata. This exercises every branch of the reverse builder
	// (status, params, kinds, defaults, optional) end-to-end.
	original := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Description("does a thing").
		Category("Tooling").
		Version("0.2.0").
		Beta().
		Param(bloblangv2.NewStringParam("name").Description("the name")).
		Param(bloblangv2.NewInt64Param("count").Default(int64(7))).
		Param(bloblangv2.NewBoolParam("loud").Optional())
	if err := original.RegisterFunction("things", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) { return nil, nil }, nil
	}); err != nil {
		t.Fatal(err)
	}

	var view *bloblangv2.FunctionView
	original.WalkFunctions(func(_ string, v *bloblangv2.FunctionView) { view = v })
	if view == nil {
		t.Fatal("no view")
	}

	// Dump → re-load.
	raw, err := json.Marshal(view.Info())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var info bloblangv2.PluginInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Reconstruct + register on a fresh env. No private-field access is
	// involved on the reverse path; if any builder validation rejected
	// the rebuilt spec it would surface here.
	rebuilt := bloblangv2.NewPluginSpecFromInfo(info)
	rebuiltEnv := bloblangv2.NewEmptyEnvironment()
	if err := rebuiltEnv.RegisterFunction("things", rebuilt, func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		// Confirm typed defaults are intact post round-trip — this is what
		// the per-param custom unmarshal exists to guarantee.
		count, err := args.GetInt64("count")
		if err != nil {
			return nil, err
		}
		if count != 7 {
			return nil, fmt.Errorf("count=%d, expected 7", count)
		}
		return func() (any, error) { return count, nil }, nil
	}); err != nil {
		t.Fatalf("register rebuilt: %v", err)
	}

	exec, err := rebuiltEnv.Parse(`output = things("hello")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := exec.Query(nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != int64(7) {
		t.Fatalf("got %#v, want int64(7)", got)
	}
}

func TestFunctionViewFormatJSON(t *testing.T) {
	env := bloblangv2.NewEmptyEnvironment()
	spec := bloblangv2.NewPluginSpec().
		Description("desc").
		Param(bloblangv2.NewInt64Param("count").Default(int64(3)))
	if err := env.RegisterFunction("counter", spec, func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) { return 0, nil }, nil
	}); err != nil {
		t.Fatal(err)
	}

	var view *bloblangv2.FunctionView
	env.WalkFunctions(func(_ string, v *bloblangv2.FunctionView) { view = v })
	if view == nil {
		t.Fatal("no view")
	}

	raw, err := view.FormatJSON()
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["name"] != "counter" {
		t.Fatalf("json name=%v", got["name"])
	}
	if got["description"] != "desc" {
		t.Fatalf("json description=%v", got["description"])
	}
}
