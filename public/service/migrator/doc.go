// Copyright 2026 Redpanda Data, Inc.

// Package migrator rewrites Benthos stream configs by replacing one
// plugin instance with another, optionally translating any embedded
// configuration (e.g. Bloblang V1 mappings) along the way.
//
// The package ships with built-in rules that rewrite the V1
// `bloblang`, `mapping` and `mutation` processors to the V2
// `bloblang_v2` processor, threading each mapping body through
// public/bloblangv2/migrator. Downstream repositories with their own
// plugins can register additional rules keyed by (component type,
// plugin name).
//
// # Usage
//
//	mig := migrator.New()
//	report, err := mig.Migrate(streamYAML, migrator.Options{})
//	if err != nil {
//	    return err
//	}
//	fmt.Println(report.OutputYAML)
//
// # Custom rules
//
// Register a rule keyed by the (ComponentType, Name) of the plugin to
// replace. The rule receives a Context and a Component describing the
// matched node, and returns a Result describing the outcome. Custom
// rules win on collision with the built-ins (the downstream rule fully
// replaces the built-in for that key).
//
//	mig.RegisterRule(migrator.Target{ComponentType: "processor", Name: "old_widget"},
//	    func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
//	        body, _ := c.BodyString()
//	        return ctx.Replace("new_widget", body)
//	    })
//
// # Stability
//
// Public types and methods follow semantic versioning. The walker
// implementation is private to the package and may evolve
// independently of the public surface.
package migrator
