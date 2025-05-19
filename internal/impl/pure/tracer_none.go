// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterOtelTracerProvider(
		"none", service.NewConfigSpec().
			Stable().
			Summary(`Do not send tracing events anywhere.`).
			Field(
				service.NewObjectField("").Default(map[string]any{}),
			),
		func(conf *service.ParsedConfig) (trace.TracerProvider, error) {
			return noop.NewTracerProvider(), nil
		})
}
