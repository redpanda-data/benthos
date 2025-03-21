// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/component/cache"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/ratelimit"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager"
)

// ResourceBuilder provides methods for building a Benthos stream configuration.
// When parsing Benthos configs this builder follows the schema and field
// defaults of a standard Benthos configuration. Environment variable
// interpolations are also parsed and resolved the same as regular configs.
//
// Streams built with a stream builder have the HTTP server for exposing metrics
// and ready checks disabled by default, which is the only deviation away from a
// standard Benthos default configuration. In order to enable the server set the
// configuration field `http.enabled` to `true` explicitly, or use `SetHTTPMux`
// in order to provide an explicit HTTP multiplexer for registering those
// endpoints.
type ResourceBuilder struct {
	engineVersion string

	resources manager.ResourceConfig
	logger    log.Config

	customLogger log.Modular

	configSpec      docs.FieldSpecs
	env             *Environment
	lintingDisabled bool
	envVarLookupFn  func(context.Context, string) (string, bool)
}

// NewResourceBuilder creates a new ResourceBuilder.
func NewResourceBuilder() *ResourceBuilder {
	tmpSpec := config.Spec()
	tmpSpec.SetDefault(false, "http", "enabled")

	return &ResourceBuilder{
		resources:  manager.NewResourceConfig(),
		logger:     log.NewConfig(),
		configSpec: tmpSpec,
		env:        globalEnvironment,
		envVarLookupFn: func(_ context.Context, k string) (string, bool) {
			return os.LookupEnv(k)
		},
	}
}

func (r *ResourceBuilder) getLintContext() docs.LintContext {
	conf := docs.NewLintConfig(r.env.internal)
	conf.DocsProvider = r.env.internal
	conf.BloblangEnv = r.env.bloblangEnv.Deactivated()
	return docs.NewLintContext(conf)
}

//------------------------------------------------------------------------------

// SetEngineVersion sets the version string representing the Benthos engine that
// components will see. By default a best attempt will be made to determine a
// version either from the benthos module import or a build-time flag.
func (r *ResourceBuilder) SetEngineVersion(ev string) {
	r.engineVersion = ev
}

// SetSchema overrides the default config schema used when linting and parsing
// full configs with the SetYAML method. Other XYAML methods will not use this
// schema as they parse individual component configs rather than a larger
// configuration.
//
// This method is useful as a mechanism for modifying the default top-level
// settings, such as metrics types and so on.
func (r *ResourceBuilder) SetSchema(schema *ConfigSchema) {
	if r.engineVersion == "" {
		r.engineVersion = schema.version
	}
	r.configSpec = schema.fields
}

// DisableLinting configures the stream builder to no longer lint YAML configs,
// allowing you to add snippets of config to the builder without failing on
// linting rules.
func (r *ResourceBuilder) DisableLinting() {
	r.lintingDisabled = true
}

// SetEnvVarLookupFunc changes the behaviour of the stream builder so that the
// value of environment variable interpolations (of the form `${FOO}`) are
// obtained via a provided function rather than the default of os.LookupEnv.
func (r *ResourceBuilder) SetEnvVarLookupFunc(fn func(context.Context, string) (string, bool)) {
	r.envVarLookupFn = fn
}

// SetLogger sets a customer logger via Go's standard logging interface,
// allowing you to replace the default Benthos logger with your own.
func (r *ResourceBuilder) SetLogger(l *slog.Logger) {
	r.customLogger = log.NewBenthosLogAdapter(l)
}

//------------------------------------------------------------------------------

// AddYAML parses resource configurations and adds them to the config.
func (r *ResourceBuilder) AddYAML(conf string) error {
	node, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	spec := manager.Spec()
	if err := r.lintYAMLSpec(spec, node); err != nil {
		return err
	}

	pConf, err := spec.ParsedConfigFromAny(node)
	if err != nil {
		return convertDocsLintErr(err)
	}

	rconf, err := manager.FromParsed(r.env.internal, pConf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	return r.resources.AddFrom(&rconf)
}

// AddCacheYAML parses a cache configuration and adds it to the config.
func (r *ResourceBuilder) AddCacheYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeCache); err != nil {
		return err
	}

	parsedConf, err := cache.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.resources.ResourceCaches = append(r.resources.ResourceCaches, parsedConf)
	return nil
}

// AddInputYAML parses an input configuration and adds it to the config.
func (r *ResourceBuilder) AddInputYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeInput); err != nil {
		return err
	}

	parsedConf, err := input.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.resources.ResourceInputs = append(r.resources.ResourceInputs, parsedConf)
	return nil
}

// AddOutputYAML parses an output configuration and adds it to the config.
func (r *ResourceBuilder) AddOutputYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeOutput); err != nil {
		return err
	}

	parsedConf, err := output.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.resources.ResourceOutputs = append(r.resources.ResourceOutputs, parsedConf)
	return nil
}

// AddProcessorYAML parses a processor configuration and adds it to the config.
func (r *ResourceBuilder) AddProcessorYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeProcessor); err != nil {
		return err
	}

	parsedConf, err := processor.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.resources.ResourceProcessors = append(r.resources.ResourceProcessors, parsedConf)
	return nil
}

// AddRateLimitYAML parses a rate limit configuration and adds it to the config.
func (r *ResourceBuilder) AddRateLimitYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeRateLimit); err != nil {
		return err
	}

	parsedConf, err := ratelimit.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.resources.ResourceRateLimits = append(r.resources.ResourceRateLimits, parsedConf)
	return nil
}

// Build attempts to create a collection of resources from the config provided,
// and also a function that closes and cleans up all resources once they're
// finished with.
func (r *ResourceBuilder) Build() (*Resources, func(context.Context) error, error) {
	logger := r.customLogger
	if logger == nil {
		var err error
		if logger, err = log.New(os.Stdout, r.env.fs, r.logger); err != nil {
			return nil, nil, err
		}
	}

	engVer := r.engineVersion
	if engVer == "" {
		engVer = cli.Version
	}

	mgr, err := manager.New(
		r.resources,
		manager.OptSetEngineVersion(engVer),
		manager.OptSetLogger(logger),
		manager.OptSetEnvironment(r.env.internal),
		manager.OptSetBloblangEnvironment(r.env.getBloblangParserEnv()),
		manager.OptSetFS(r.env.fs),
	)
	if err != nil {
		return nil, nil, err
	}

	return newResourcesFromManager(mgr), func(ctx context.Context) error {
		mgr.TriggerStopConsuming()
		if err := mgr.WaitForClose(ctx); err != nil {
			mgr.TriggerCloseNow()
			return err
		}
		if err := mgr.CloseObservability(ctx); err != nil {
			return err
		}
		return nil
	}, nil
}

//------------------------------------------------------------------------------

func (r *ResourceBuilder) getYAMLNode(b []byte) (*yaml.Node, error) {
	var err error
	if b, err = config.NewReader("", nil, config.OptUseEnvLookupFunc(r.envVarLookupFn)).ReplaceEnvVariables(context.TODO(), b); err != nil {
		// TODO: Allow users to specify whether they care about env variables
		// missing, in which case we error or not based on that.
		var errEnvMissing *config.ErrMissingEnvVars
		if errors.As(err, &errEnvMissing) {
			b = errEnvMissing.BestAttempt
		} else {
			return nil, err
		}
	}
	return docs.UnmarshalYAML(b)
}

func (r *ResourceBuilder) lintYAMLSpec(spec docs.FieldSpecs, node *yaml.Node) error {
	if r.lintingDisabled {
		return nil
	}
	return lintsToErr(spec.LintYAML(r.getLintContext(), node))
}

func (r *ResourceBuilder) lintYAMLComponent(node *yaml.Node, ctype docs.Type) error {
	if r.lintingDisabled {
		return nil
	}
	return lintsToErr(docs.LintYAML(r.getLintContext(), ctype, node))
}
