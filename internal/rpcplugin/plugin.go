// Copyright 2025 Redpanda Data, Inc.

package rpcplugin

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// InitPlugins parses and registers RPC plugins and returns any linting errors
// that occur.
func InitPlugins(env *bundle.Environment, pluginSpecPaths ...string) ([]string, error) {
	var lints []string
	for _, tPath := range pluginSpecPaths {
		tmplConf, tLints, err := ReadConfigFile(env, tPath)
		if err != nil {
			return nil, fmt.Errorf("rpc plugin %v: %w", tPath, err)
		}
		for _, l := range tLints {
			lints = append(lints, fmt.Sprintf("rpc plugin file %v: %v", tPath, l))
		}

		spec, err := tmplConf.ComponentSpec()
		if err != nil {
			return nil, fmt.Errorf("rpc plugin %v: %w", tPath, err)
		}

		if err := registerPlugin(env, parsedSpec{
			spec:            spec,
			pluginDirectory: tmplConf.Path,
		}); err != nil {
			return nil, fmt.Errorf("rpc plugin %v: %w", tPath, err)
		}
	}
	return lints, nil
}

//------------------------------------------------------------------------------

type parsedSpec struct {
	spec            docs.ComponentSpec
	pluginDirectory string
}

// registerPlugin attempts to add a template component to the global list of
// component types.
func registerPlugin(env *bundle.Environment, plugSpec parsedSpec) error {
	switch docs.Type(plugSpec.spec.Type) {
	/*
		case docs.TypeCache:
			return registerCachePlugin(plugSpec, env)
		case docs.TypeInput:
			return registerInputPlugin(plugSpec, env)
		case docs.TypeOutput:
			return registerOutputPlugin(plugSpec, env)
		case docs.TypeRateLimit:
			return registerRateLimitPlugin(plugSpec, env)
	*/
	case docs.TypeProcessor:
		return registerProcessorPlugin(plugSpec, env)
	}
	return fmt.Errorf("unable to register plugin for component type %v", plugSpec.spec.Type)
}

func registerProcessorPlugin(plugSpec parsedSpec, env *bundle.Environment) error {
	return env.ProcessorAdd(func(c processor.Config, nm bundle.NewManagement) (processor.V1, error) {
		newConf, err := tmpl.Render(c.Plugin, nm.Label())
		if err != nil {
			return nil, err
		}

		conf, err := processor.FromAny(env, newConf)
		if err != nil {
			return nil, err
		}

		return nm.NewProcessor(conf)
	}, plugSpec.spec)
}

/*
func registerCachePlugin(plugSpec parsedSpec, env *bundle.Environment) error {
	return env.CacheAdd(func(c cache.Config, nm bundle.NewManagement) (cache.V1, error) {
		// TODO
	}, plugSpec.spec)
}

func registerInputPlugin(plugSpec parsedSpec, env *bundle.Environment) error {
	return env.InputAdd(func(c input.Config, nm bundle.NewManagement) (input.Streamed, error) {
		// TODO
	}, plugSpec.spec)
}

func registerOutputPlugin(plugSpec parsedSpec, env *bundle.Environment) error {
	return env.OutputAdd(func(c output.Config, nm bundle.NewManagement, pcf ...processor.PipelineConstructorFunc) (output.Streamed, error) {
		// TODO
	}, plugSpec.spec)
}

func registerRateLimitPlugin(plugSpec parsedSpec, env *bundle.Environment) error {
	return env.RateLimitAdd(func(c ratelimit.Config, nm bundle.NewManagement) (ratelimit.V1, error) {
		// TODO
	}, plugSpec.spec)
}
*/
