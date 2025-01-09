// Copyright 2025 Redpanda Data, Inc.

package testutil

import (
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/buffer"
	"github.com/redpanda-data/benthos/v4/internal/component/cache"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/ratelimit"
	"github.com/redpanda-data/benthos/v4/internal/component/tracer"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/stream"
)

// BufferFromYAML attempts to parse a config string and returns a buffer config
// if successful or an error otherwise.
func BufferFromYAML(confStr string) (buffer.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return buffer.Config{}, err
	}
	return buffer.FromAny(bundle.GlobalEnvironment, node)
}

// CacheFromYAML attempts to parse a config string and returns a cache config
// if successful or an error otherwise.
func CacheFromYAML(confStr string) (cache.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return cache.Config{}, err
	}
	return cache.FromAny(bundle.GlobalEnvironment, node)
}

// InputFromYAML attempts to parse a config string and returns an input config
// if successful or an error otherwise.
func InputFromYAML(confStr string) (input.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return input.Config{}, err
	}
	return input.FromAny(bundle.GlobalEnvironment, node)
}

// MetricsFromYAML attempts to parse a config string and returns a metrics
// config if successful or an error otherwise.
func MetricsFromYAML(confStr string) (metrics.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return metrics.Config{}, err
	}
	return metrics.FromAny(bundle.GlobalEnvironment, node)
}

// OutputFromYAML attempts to parse a config string and returns an output config
// if successful or an error otherwise.
func OutputFromYAML(confStr string) (output.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return output.Config{}, err
	}
	return output.FromAny(bundle.GlobalEnvironment, node)
}

// ProcessorFromYAML attempts to parse a config string and returns a processor
// config if successful or an error otherwise.
func ProcessorFromYAML(confStr string) (processor.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return processor.Config{}, err
	}
	return processor.FromAny(bundle.GlobalEnvironment, node)
}

// RateLimitFromYAML attempts to parse a config string and returns a ratelimit
// config if successful or an error otherwise.
func RateLimitFromYAML(confStr string) (ratelimit.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return ratelimit.Config{}, err
	}
	return ratelimit.FromAny(bundle.GlobalEnvironment, node)
}

// TracerFromYAML attempts to parse a config string and returns a tracer config
// if successful or an error otherwise.
func TracerFromYAML(confStr string) (tracer.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return tracer.Config{}, err
	}
	return tracer.FromAny(bundle.GlobalEnvironment, node)
}

// ManagerFromYAML attempts to parse a config string and returns a manager
// config if successful or an error otherwise.
func ManagerFromYAML(confStr string) (manager.ResourceConfig, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return manager.ResourceConfig{}, err
	}
	return manager.FromAny(bundle.GlobalEnvironment, node)
}

// StreamFromYAML attempts to parse a config string and returns a stream config
// if successful or an error otherwise.
func StreamFromYAML(confStr string) (stream.Config, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return stream.Config{}, err
	}

	var rawSource any
	_ = node.Decode(&rawSource)

	pConf, err := stream.Spec().ParsedConfigFromAny(node)
	if err != nil {
		return stream.Config{}, err
	}
	return stream.FromParsed(bundle.GlobalEnvironment, pConf, rawSource)
}

// ConfigFromYAML attempts to parse a config string and returns a Benthos
// service config if successful or an error otherwise.
func ConfigFromYAML(confStr string) (config.Type, error) {
	node, err := docs.UnmarshalYAML([]byte(confStr))
	if err != nil {
		return config.Type{}, err
	}

	var rawSource any
	_ = node.Decode(&rawSource)

	pConf, err := config.Spec().ParsedConfigFromAny(node)
	if err != nil {
		return config.Type{}, err
	}
	return config.FromParsed(bundle.GlobalEnvironment, pConf, rawSource)
}
