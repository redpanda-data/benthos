// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"context"
	"errors"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	tcFieldProcessors  = "processors"
	tcFieldCatch       = "catch"
	tcFieldErrorMeta   = "error_metadata"
	tcDefaultErrorMeta = "error"
)

func init() {
	service.MustRegisterBatchProcessor("try_catch", service.NewConfigSpec().
		Beta().
		Categories("Composition").
		Summary("Executes a list of child `processors` on each message and, if any of them fail, executes a separate list of `catch` processors to recover from or react to the error.").
		Description(`
This processor combines the behaviour of the `+"xref:components:processors/try.adoc[`try`]"+` and xref:components:processors/catch.adoc[`+"`catch`"+`] processors into a single block with an explicit recovery path. Because it contains both the fallible step and its recovery within a single processor, it is the recommended way to handle expected errors when strict error handling (`+"`error_handling.strict`"+`) is enabled.

Each message of a batch is processed individually. The `+"`processors`"+` field is executed with "try" semantics: as soon as a processor fails for a given message the remaining `+"`processors`"+` are skipped for that message.

Any message that failed is then routed to the `+"`catch`"+` processors. Before they run, the failure is moved off the message: it is stored as a structured object in a metadata field (see `+"`error_metadata`"+`, `+"`error`"+` by default) and the message's failure flag is **cleared**. The error is therefore available to recovery logic as an ordinary variable rather than as a message property. The object contains:

- `+"`what`"+`: the error message.
- `+"`name`"+`: the name of the component that failed (when known).
- `+"`label`"+`: the label of the component that failed (when set).
- `+"`path`"+`: the dot-path of the component that failed (when known).

So a recovery mapping reads the failure with, for example, `+"`@error.what`"+` (equivalent to `+"`meta(\"error\").what`"+`). Because the flag is cleared, the `+"`catch`"+` processors run under the normal error semantics — including strict — so a _new_ failure raised while recovering is treated as a fresh error and is not silently tolerated.

Note that because the failure flag is cleared before the `+"`catch`"+` processors run, the xref:guides:bloblang/functions.adoc#error[`+"`error`"+`] and `+"`error_source_*`"+` functions do not report the original failure within the `+"`catch`"+` block; use the metadata object instead. An empty or omitted `+"`catch`"+` simply records the error in metadata and clears the flag (the failure is swallowed).

`+"```yaml"+`
pipeline:
  processors:
    - try_catch:
        processors:
          - resource: foo
          - resource: bar
        catch:
          - mutation: 'root = "failed to process: " + @error.what'
`+"```"+`

In the example above, if either `+"`foo` or `bar`"+` fails for a message then the `+"`mutation`"+` is applied to that message, replacing its contents with a description of the error (read from the metadata object), and the message continues downstream without a failure flag.

More information about error handling can be found in xref:configuration:error_handling.adoc[].`).
		Field(service.NewProcessorListField(tcFieldProcessors).
			Description("A list of processors to execute on each message. If a processor fails for a given message the remaining processors in this list are skipped for that message, and the message is routed to the `catch` processors.").
			Default([]any{})).
		Field(service.NewProcessorListField(tcFieldCatch).
			Description("A list of processors to execute on each message that failed one of the `processors` above. The message is no longer flagged as failed when these run; the error is available as an object in the metadata field named by `error_metadata` (e.g. `@error.what`). When omitted or empty the error is recorded in metadata and the flag is cleared (the failure is swallowed).").
			Default([]any{})).
		Field(service.NewStringField(tcFieldErrorMeta).
			Description("The metadata key under which the caught error is stored, as an object with a `what` field (the error message) plus `name`, `label` and `path` fields describing the component that failed, before the `catch` processors are executed.").
			Default(tcDefaultErrorMeta)),
		func(conf *service.ParsedConfig, res *service.Resources) (service.BatchProcessor, error) {
			mgr := interop.UnwrapManagement(res)

			pubProcs, err := conf.FieldProcessorList(tcFieldProcessors)
			if err != nil {
				return nil, err
			}
			pubCatch, err := conf.FieldProcessorList(tcFieldCatch)
			if err != nil {
				return nil, err
			}
			errorMeta, err := conf.FieldString(tcFieldErrorMeta)
			if err != nil {
				return nil, err
			}

			procs := make([]processor.V1, len(pubProcs))
			for i, p := range pubProcs {
				procs[i] = interop.UnwrapOwnedProcessor(p)
			}
			catch := make([]processor.V1, len(pubCatch))
			for i, p := range pubCatch {
				catch[i] = interop.UnwrapOwnedProcessor(p)
			}

			p := processor.NewAutoObservedBatchedProcessor("try_catch", newTryCatchProc(procs, catch, errorMeta), mgr)
			return interop.NewUnwrapInternalBatchProcessor(p), nil
		})
}

type tryCatchProc struct {
	processors []processor.V1
	catch      []processor.V1
	errorMeta  string
}

func newTryCatchProc(processors, catch []processor.V1, errorMeta string) *tryCatchProc {
	return &tryCatchProc{
		processors: processors,
		catch:      catch,
		errorMeta:  errorMeta,
	}
}

// errorToObject builds the structured metadata value describing a caught
// failure. The `what` field always holds the error message. When the failure
// originated from a component (a *query.ComponentError, the usual case) the
// source `name`, `label` and `path` are also included, mirroring the
// error_source_name/label/path Bloblang functions.
func errorToObject(err error) map[string]any {
	obj := map[string]any{"what": err.Error()}
	var cErr *query.ComponentError
	if errors.As(err, &cErr) {
		obj["name"] = cErr.Name
		obj["label"] = cErr.Label
		obj["path"] = query.SliceToDotPath(cErr.Path...)
	}
	return obj
}

func (p *tryCatchProc) ProcessBatch(ctx *processor.BatchProcContext, msg message.Batch) ([]message.Batch, error) {
	// Operate on each message individually so that a failure in one does not
	// short-circuit processing of the others.
	resultMsgs := make([]message.Batch, msg.Len())
	_ = msg.Iter(func(i int, part *message.Part) error {
		resultMsgs[i] = message.Batch{part}
		return nil
	})

	// Execute the primary processors with "try" semantics: once a message fails
	// a step the remaining steps are skipped for that message.
	var err error
	if resultMsgs, err = processor.ExecuteTryAll(ctx.Context(), p.processors, resultMsgs...); err != nil {
		return nil, err
	}

	// For each message that failed, move the error into metadata and clear the
	// failure flag, then run the catch processors on it as an ordinary message.
	// Because the message is no longer flagged, the catch processors execute
	// under the ambient (including strict) error semantics: a new failure raised
	// during recovery is treated as a fresh error rather than being tolerated.
	var nextResultMsgs []message.Batch
	for _, m := range resultMsgs {
		if len(m) == 0 {
			continue
		}
		// Matches the try/catch convention of treating the first message of a
		// resulting batch as representative of its failure state.
		if m.Get(0).ErrorGet() == nil {
			nextResultMsgs = append(nextResultMsgs, m)
			continue
		}

		_ = m.Iter(func(_ int, part *message.Part) error {
			if e := part.ErrorGet(); e != nil {
				part.MetaSetMut(p.errorMeta, errorToObject(e))
				part.ErrorSet(nil)
			}
			return nil
		})

		if len(p.catch) == 0 {
			nextResultMsgs = append(nextResultMsgs, m)
			continue
		}

		caughtMsgs, cerr := processor.ExecuteAll(ctx.Context(), p.catch, m)
		if cerr != nil {
			return nil, cerr
		}
		nextResultMsgs = append(nextResultMsgs, caughtMsgs...)
	}

	resMsg := message.QuickBatch(nil)
	for _, m := range nextResultMsgs {
		_ = m.Iter(func(i int, part *message.Part) error {
			resMsg = append(resMsg, part)
			return nil
		})
	}
	if resMsg.Len() == 0 {
		return nil, nil
	}

	return []message.Batch{resMsg}, nil
}

func (p *tryCatchProc) Close(ctx context.Context) error {
	for _, c := range p.processors {
		if err := c.Close(ctx); err != nil {
			return err
		}
	}
	for _, c := range p.catch {
		if err := c.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}
