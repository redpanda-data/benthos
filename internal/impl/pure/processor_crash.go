// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"

	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	spec := service.NewConfigSpec().
		Categories("Utility").
		Beta().
		Summary(`Crashes the process using a fatal log message. The log message can be set using function interpolations described in  xref:configuration:interpolation.adoc#bloblang-queries[Bloblang queries] which allows you to log the contents and metadata of messages.`).
		Field(service.NewInterpolatedStringField(""))
	service.MustRegisterProcessor(
		"crash", spec,
		func(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
			messageStr, err := conf.FieldInterpolatedString()
			if err != nil {
				return nil, err
			}
			mgr := interop.UnwrapManagement(res)
			return &crashProcessor{mgr.Logger(), messageStr}, nil
		})
}

type crashProcessor struct {
	logger  log.Modular
	message *service.InterpolatedString
}

func (l *crashProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	m, err := l.message.TryString(msg)
	if err != nil {
		l.logger.Fatal("failed to interpolate crash message: %v", err)
	} else {
		l.logger.Fatal("%s", m)
	}
	return nil, nil
}

func (l *crashProcessor) Close(ctx context.Context) error {
	return nil
}
