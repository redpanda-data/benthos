// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"encoding/json"
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
