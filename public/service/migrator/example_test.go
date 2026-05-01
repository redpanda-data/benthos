// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/public/service/migrator"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// ExampleMigrate demonstrates the package-level Migrate helper
// rewriting a stream config's `bloblang` processor as `bloblang_v2`,
// with the embedded mapping translated through the bundled Bloblang
// V1->V2 migrator.
func ExampleMigrate() {
	in := `
pipeline:
  processors:
    - bloblang: |
        root.id = this.id
`
	rep, err := migrator.Migrate([]byte(in), migrator.Options{})
	if err != nil {
		fmt.Println("migrate:", err)
		return
	}
	fmt.Println(strings.TrimSpace(rep.OutputYAML))

	// Output:
	// pipeline:
	//   processors:
	//     - bloblang_v2: |
	//         output = input
	//         output.id = input?.id
}

// ExampleMigrator_RegisterRule demonstrates a downstream registering
// a rule for a fictional `old_widget` processor that has been ported
// to a new `new_widget` plugin in V2.
func ExampleMigrator_RegisterRule() {
	mig := migrator.New()
	mig.RegisterRule(
		migrator.Target{ComponentType: "processor", Name: "old_widget"},
		func(ctx *migrator.Context, c *migrator.Component) migrator.Result {
			body, ok := c.BodyString()
			if !ok {
				return ctx.Unsupported("expected scalar body")
			}
			return ctx.Replace("new_widget", body)
		},
	)
	_ = mig
}
