// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"os"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	spec := service.NewConfigSpec().
		Categories("Utility").
		Beta().
		Summary(`Exit the process with a code.`).
		Field(service.NewIntField("").Description("The exit code to use.").Default(0))
	service.MustRegisterProcessor(
		"exit", spec,
		func(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
			exitCode, err := conf.FieldInt()
			if err != nil {
				return nil, err
			}

			return &exitProcessor{exitCode}, nil
		})
}

type exitProcessor struct {
	exitCode int
}

func (l *exitProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	os.Exit(l.exitCode)

	return nil, nil
}

func (l *exitProcessor) Close(ctx context.Context) error {
	return nil
}
