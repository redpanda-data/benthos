// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// ExampleMigrator_RegisterMethodRule demonstrates registering a custom
// method-rewrite rule. The fictional `widget_encode` plugin exists in
// V1 and has been ported to V2 under the new name `widget_encode_v2`.
// Downstream registers a rule so the migrator rewrites the V1
// callsite into the V2 form during translation.
func ExampleMigrator_RegisterMethodRule() {
	mig := migrator.New()
	mig.RegisterMethodRule("widget_encode", func(ctx *migrator.Context, m *migrator.V1MethodCall) migrator.Result {
		// V1 widget_encode took no arguments. V2 keeps that.
		if len(m.Args) != 0 {
			return ctx.Unsupported("widget_encode takes no arguments")
		}
		return ctx.Replace(&migrator.V2MethodCallExpr{
			Receiver: ctx.Translate(m.Receiver),
			Method:   "widget_encode_v2",
		})
	})

	report, err := mig.Migrate(`root.encoded = this.payload.widget_encode()`, migrator.Options{})
	if err != nil {
		fmt.Println("migrate failed:", err)
		return
	}
	fmt.Println(strings.TrimSpace(report.V2Mapping))

	// Output:
	// output.encoded = input?.payload.widget_encode_v2()
}

// ExampleMigrator_RegisterFunctionRule demonstrates a function-rule
// rewrite. The fictional V1 `widget_size()` function is replaced in
// V2 by an equivalent method on `input`.
func ExampleMigrator_RegisterFunctionRule() {
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

	report, err := mig.Migrate(`root.size = widget_size()`, migrator.Options{})
	if err != nil {
		fmt.Println("migrate failed:", err)
		return
	}
	fmt.Println(strings.TrimSpace(report.V2Mapping))

	// Output:
	// output.size = input.widget_size()
}
