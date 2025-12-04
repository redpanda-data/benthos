// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	maxUint := ^uint64(0)
	maxInt := maxUint >> 1

	bloblang.MustRegisterAdvancedFunction("counter",
		bloblang.NewPluginSpec().
			Category(query.FunctionCategoryGeneral).
			Experimental().
			Description("Generates an incrementing sequence of integers starting from a minimum value (default 1). Each counter instance maintains its own independent state across message processing. When the maximum value is reached, the counter automatically resets to the minimum.").
			Param(bloblang.NewQueryParam("min", true).
				Default(1).
				Description("The starting value of the counter. This is the first value yielded. Evaluated once when the mapping is initialized.")).
			Param(bloblang.NewQueryParam("max", true).
				Default(maxInt).
				Description("The maximum value before the counter resets to min. Evaluated once when the mapping is initialized.")).
			Param(bloblang.NewQueryParam("set", false).
				Optional().
				Description("An optional query that controls counter behavior: when it resolves to a non-negative integer, the counter is set to that value; when it resolves to `null`, the counter is read without incrementing; when it resolves to a deletion, the counter resets to min; otherwise the counter increments normally.")).
			Example("Generate sequential IDs for each message.", `root.id = counter()`,
				[2]string{
					`{}`,
					`{"id":1}`,
				},
				[2]string{
					`{}`,
					`{"id":2}`,
				},
			).
			Example("Use a custom range for the counter.",
				`root.batch_num = counter(min: 100, max: 200)`,
				[2]string{
					`{}`,
					`{"batch_num":100}`,
				},
				[2]string{
					`{}`,
					`{"batch_num":101}`,
				},
			).
			Example("Increment a counter multiple times within a single mapping using a named map.",
				`
map increment {
  root = counter()
}

root.first_id = null.apply("increment")
root.second_id = null.apply("increment")
`,
				[2]string{
					`{}`,
					`{"first_id":1,"second_id":2}`,
				},
				[2]string{
					`{}`,
					`{"first_id":3,"second_id":4}`,
				},
			).
			Example(
				"Conditionally reset a counter based on input data.",
				`root.streak = counter(set: if this.status != "success" { 0 })`,
				[2]string{
					`{"status":"success"}`,
					`{"streak":1}`,
				},
				[2]string{
					`{"status":"success"}`,
					`{"streak":2}`,
				},
				[2]string{
					`{"status":"failure"}`,
					`{"streak":0}`,
				},
				[2]string{
					`{"status":"success"}`,
					`{"streak":1}`,
				},
			).
			Example(
				"Peek at the current counter value without incrementing by using null in the set parameter.",
				`root.count = counter(set: if this.peek { null })`,
				[2]string{`{"peek":false}`, `{"count":1}`},
				[2]string{`{"peek":false}`, `{"count":2}`},
				[2]string{`{"peek":true}`, `{"count":2}`},
				[2]string{`{"peek":false}`, `{"count":3}`},
			),
		func(args *bloblang.ParsedParams) (bloblang.AdvancedFunction, error) {
			minFunc, err := args.GetQuery("min")
			if err != nil {
				return nil, err
			}

			maxFunc, err := args.GetOptionalQuery("max")
			if err != nil {
				return nil, err
			}

			setFunc, err := args.GetOptionalQuery("set")
			if err != nil {
				return nil, err
			}

			var minV, maxV int64
			var i *int64

			var mut sync.Mutex

			return func(ctx *bloblang.ExecContext) (any, error) {
				mut.Lock()
				defer mut.Unlock()

				if i == nil {
					var err error
					if minV, err = ctx.ExecToInt64(minFunc); err != nil {
						return nil, fmt.Errorf("failed to resolve min argument: %w", err)
					}
					if minV < 0 {
						return nil, fmt.Errorf("min argument must be >0, got %v", minV)
					}
					if maxV, err = ctx.ExecToInt64(maxFunc); err != nil {
						return nil, fmt.Errorf("failed to resolve max argument: %w", err)
					}
					if maxV < 0 || maxV <= minV {
						return nil, fmt.Errorf("max argument must be >0 and >min, got %v", maxV)
					}

					iV := minV - 1
					i = &iV
				}

				if setFunc != nil {
					setV, err := ctx.Exec(setFunc)
					if err != nil {
						return nil, fmt.Errorf("failed to resolve set argument: %w", err)
					}
					if setV == nil {
						return *i, nil
					}
					switch setV.(type) {
					case bloblang.ExecResultDelete:
						*i = minV - 1
					case bloblang.ExecResultNothing:
					default:
						iv, err := value.IGetInt(setV)
						if err != nil {
							return nil, fmt.Errorf("failed to resolve set argument: %w", err)
						}
						*i = iv
						return iv, nil
					}
				}

				*i++
				v := *i
				if v >= maxV {
					*i = minV - 1
				}
				return v, nil
			}, nil
		})
}
