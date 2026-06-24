// Copyright 2026 Redpanda Data, Inc.

// Package migrator translates Bloblang V1 mappings into Bloblang V2.
//
// The package wraps the internal translator with a stable public API
// so downstream repositories that ship their own V1 plugins can
// register custom translation rules alongside the bundled built-ins.
//
// # Usage
//
// The simplest case mirrors the original
// internal/bloblang2/migrator/translator.Migrate signature: construct a
// Migrator, hand it a V1 source string, get a Report back.
//
//	mig := migrator.New()
//	report, err := mig.Migrate(v1Source, migrator.Options{})
//	if err != nil {
//	    return err
//	}
//	fmt.Println(report.V2Mapping)
//
// # Custom rules
//
// Register a method or function rule keyed by the V1 plugin name. The
// rule receives a Context and a wrapped V1 node and returns a Result
// describing the outcome. Custom rules win on name collision with the
// built-ins.
//
//	mig.RegisterMethodRule("widget_encode",
//	    func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
//	        return ctx.Replace(&migrator.V2MethodCallExpr{
//	            Receiver: ctx.Translate(m.Receiver),
//	            Method:   "widget_encode_v2",
//	        })
//	    })
//
// # Stability
//
// Public types and methods follow semantic versioning. The wrapped AST
// shapes intentionally mirror the internal AST 1:1 but evolve
// independently of the internal types — internal refactors never
// reach the public surface.
package migrator
