// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Jeffail/shutdown"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func resourceInputSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Stable().
		Categories("Utility").
		Summary(`Resource is an input type that channels messages from a resource input, identified by its name.`).
		Description(`Resources allow you to tidy up deeply nested configs. For example, the config:

` + "```yaml" + `
input:
  broker:
    inputs:
      - kafka:
          addresses: [ TODO ]
          topics: [ foo ]
          consumer_group: foogroup
      - gcp_pubsub:
          project: bar
          subscription: baz
` + "```" + `

Could also be expressed as:

` + "```yaml" + `
input:
  broker:
    inputs:
      - resource: foo
      - resource: bar

input_resources:
  - label: foo
    kafka:
      addresses: [ TODO ]
      topics: [ foo ]
      consumer_group: foogroup

  - label: bar
    gcp_pubsub:
      project: bar
      subscription: baz
` + "```" + `

Resources also allow you to reference a single input in multiple places, such as multiple streams mode configs, or multiple entries in a broker input. However, when a resource is referenced more than once the messages it produces are distributed across those references, so each message will only be directed to a single reference, not all of them.

You can find out more about resources in xref:configuration:resources.adoc[].`).
		Field(service.NewStringField("").Default(""))
}

func init() {
	service.MustRegisterBatchInput("resource", resourceInputSpec(), func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchInput, error) {
		name, err := conf.FieldString()
		if err != nil {
			return nil, err
		}
		if !mgr.HasInput(name) {
			return nil, fmt.Errorf("input resource '%v' was not found", name)
		}
		nm := interop.UnwrapManagement(mgr)
		ri := &resourceInput{
			mgr:     nm,
			name:    name,
			log:     nm.Logger(),
			tChan:   make(chan message.Transaction),
			shutSig: shutdown.NewSignaller(),
		}
		return interop.NewUnwrapInternalInput(ri), nil
	})
}

//------------------------------------------------------------------------------

type resourceInput struct {
	mgr       bundle.NewManagement
	tChan     chan message.Transaction
	name      string
	log       log.Modular
	shutSig   *shutdown.Signaller
	startOnce sync.Once
}

func (r *resourceInput) loop() {
	defer func() {
		close(r.tChan)
		r.shutSig.TriggerHasStopped()
	}()

	for {
		var resourceTChan <-chan message.Transaction
		if err := r.mgr.AccessInput(context.Background(), r.name, func(i input.Streamed) {
			resourceTChan = i.TransactionChan()
		}); err != nil {
			r.log.Error("Failed to obtain input resource '%v': %v", r.name, err)
			select {
			case <-r.shutSig.SoftStopChan():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		for {
			select {
			case <-r.shutSig.SoftStopChan():
				return
			case t, open := <-resourceTChan:
				if !open {
					return
				}
				select {
				case r.tChan <- t:
				case <-r.shutSig.HardStopChan():
					go func() {
						_ = t.Ack(context.Background(), component.ErrFailedSend)
					}()
					return
				}
			}
		}
	}
}

func (r *resourceInput) TransactionChan() (tChan <-chan message.Transaction) {
	return r.tChan
}

// ConnectionTest attempts to find the target resource and executes a connection
// test on it.
func (r *resourceInput) ConnectionTest(ctx context.Context) (res component.ConnectionTestResults) {
	if err := r.mgr.AccessInput(context.Background(), r.name, func(i input.Streamed) {
		res = i.ConnectionTest(ctx)
	}); err != nil {
		return component.ConnectionTestResults{
			component.ConnectionTestFailed(r.mgr, err),
		}
	}
	return
}

func (r *resourceInput) ConnectionStatus() (s component.ConnectionStatuses) {
	if err := r.mgr.AccessInput(context.Background(), r.name, func(i input.Streamed) {
		s = i.ConnectionStatus()
	}); err != nil {
		return component.ConnectionStatuses{
			component.ConnectionFailing(r.mgr, err),
		}
	}
	return
}

func (r *resourceInput) TriggerStartConsuming() {
	r.startOnce.Do(func() {
		go r.loop()
	})
}

func (r *resourceInput) TriggerStopConsuming() {
	r.shutSig.TriggerSoftStop()
}

func (r *resourceInput) TriggerCloseNow() {
	r.shutSig.TriggerHardStop()
}

func (r *resourceInput) WaitForClose(ctx context.Context) error {
	select {
	case <-r.shutSig.HasStoppedChan():
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
