// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	parProcFieldCap        = "cap"
	parProcFieldProcessors = "processors"
)

func init() {
	service.MustRegisterBatchProcessor(
		"parallel", service.NewConfigSpec().
			Categories("Composition").
			Stable().
			Summary(`A processor that applies a list of child processors to messages of a batch as though they were each a batch of one message (similar to the `+"xref:components:processors/for_each.adoc[`for_each`]"+` processor), but where each message is processed in parallel.`).
			Description(`
The field `+"`cap`"+`, if greater than zero, caps the maximum number of parallel processing threads.

The functionality of this processor depends on being applied across messages that are batched. You can find out more about batching in xref:configuration:batching.adoc[].`).
			Fields(
				service.NewIntField(parProcFieldCap).
					Description("The maximum number of messages to have processing at a given time.").
					Default(0),
				service.NewProcessorListField(parProcFieldProcessors).
					Description("A list of child processors to apply."),
			),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			var p parallelProc
			var err error

			if p.cap, err = conf.FieldInt(parProcFieldCap); err != nil {
				return nil, err
			}

			var pChildren []*service.OwnedProcessor
			if pChildren, err = conf.FieldProcessorList(parProcFieldProcessors); err != nil {
				return nil, err
			}
			p.children = make([]processor.V1, len(pChildren))
			for i, c := range pChildren {
				p.children[i] = interop.UnwrapOwnedProcessor(c)
			}

			return interop.NewUnwrapInternalBatchProcessor(processor.NewAutoObservedBatchedProcessor("parallel", &p, interop.UnwrapManagement(mgr))), nil
		})
}

type parallelProc struct {
	children []processor.V1
	cap      int
}

func (p *parallelProc) ProcessBatch(ctx *processor.BatchProcContext, msg message.Batch) ([]message.Batch, error) {
	resultMsgs := make([]message.Batch, msg.Len())
	_ = msg.Iter(func(i int, p *message.Part) error {
		resultMsgs[i] = message.Batch{p}
		return nil
	})

	maxV := p.cap
	if maxV == 0 || msg.Len() < maxV {
		maxV = msg.Len()
	}

	reqChan := make(chan int)
	wg := sync.WaitGroup{}
	wg.Add(maxV)

	for i := 0; i < maxV; i++ {
		go func() {
			defer wg.Done()

			for index := range reqChan {
				resMsgs, err := processor.ExecuteAll(ctx.Context(), p.children, resultMsgs[index])
				if err != nil {
					return
				}
				resultParts := []*message.Part{}
				for _, m := range resMsgs {
					_ = m.Iter(func(i int, p *message.Part) error {
						resultParts = append(resultParts, p)
						return nil
					})
				}
				resultMsgs[index] = resultParts
			}
		}()
	}
	for i := 0; i < msg.Len(); i++ {
		reqChan <- i
	}
	close(reqChan)
	wg.Wait()

	if err := ctx.Context().Err(); err != nil {
		return nil, err
	}

	resMsg := message.QuickBatch(nil)
	for _, m := range resultMsgs {
		_ = m.Iter(func(i int, p *message.Part) error {
			resMsg = append(resMsg, p)
			return nil
		})
	}

	return []message.Batch{resMsg}, nil
}

func (p *parallelProc) Close(ctx context.Context) error {
	for _, c := range p.children {
		if err := c.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}
