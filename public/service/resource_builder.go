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
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/ratelimit"
	"github.com/redpanda-data/benthos/v4/internal/component/tracer"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
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
	metrics   metrics.Config
	tracer    tracer.Config

	apiMut       manager.APIReg
	customLogger log.Modular

	env             *Environment
	lintingDisabled bool
	envVarLookupFn  func(context.Context, string) (string, bool)

	onMgrInit []func(*Resources) error
}

// NewResourceBuilder creates a new ResourceBuilder.
func NewResourceBuilder() *ResourceBuilder {
	return &ResourceBuilder{
		apiMut:    mock.NewManager(),
		resources: manager.NewResourceConfig(),
		metrics:   metrics.NewConfig(),
		tracer:    tracer.NewConfig(),
		env:       globalEnvironment,
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

// SetHTTPMux sets an HTTP multiplexer to be used by resource components when
// registering endpoints.
func (r *ResourceBuilder) SetHTTPMux(m HTTPMultiplexer) {
	r.apiMut = &muxWrapper{m}
}

// SetEngineVersion sets the version string representing the Benthos engine that
// components will see. By default a best attempt will be made to determine a
// version either from the benthos module import or a build-time flag.
func (r *ResourceBuilder) SetEngineVersion(ev string) {
	r.engineVersion = ev
}

// DisableLinting configures the stream builder to no longer lint YAML configs,
// allowing you to add snippets of config to the builder without failing on
// linting rules.
func (r *ResourceBuilder) DisableLinting() {
	r.lintingDisabled = true
}

// SetEnvVarLookupFunc changes the behaviour of the resources builder so that
// the value of environment variable interpolations (of the form `${FOO}`) are
// obtained via a provided function rather than the default of os.LookupEnv.
func (r *ResourceBuilder) SetEnvVarLookupFunc(fn func(context.Context, string) (string, bool)) {
	r.envVarLookupFn = fn
}

// SetLogger sets a customer logger via Go's standard logging interface,
// allowing you to replace the default Benthos logger with your own.
func (r *ResourceBuilder) SetLogger(l *slog.Logger) {
	r.customLogger = log.NewBenthosLogAdapter(l)
}

// OnResourceInit adds a closure function to be called on built Resources once
// initialized but before any resources have been added, this allows you to
// access the Resources before any configured plugins have been added.
//
// WARNING: This is only for advanced use cases, and a provided closure can be
// called for multiple instances of a *Resources type during a single Build call
// due to internal mechanisms.
func (r *ResourceBuilder) OnResourceInit(fn func(*Resources) error) {
	r.onMgrInit = append(r.onMgrInit, fn)
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

// SetMetricsYAML parses a metrics YAML configuration and adds it to the builder
// such that all resource components emit metrics through it.
func (r *ResourceBuilder) SetMetricsYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeMetrics); err != nil {
		return err
	}

	mconf, err := metrics.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.metrics = mconf
	return nil
}

// SetTracerYAML parses a tracer YAML configuration and adds it to the builder
// such that all resource components emit tracing spans through it.
func (r *ResourceBuilder) SetTracerYAML(conf string) error {
	nconf, err := r.getYAMLNode([]byte(conf))
	if err != nil {
		return err
	}

	if err := r.lintYAMLComponent(nconf, docs.TypeTracer); err != nil {
		return err
	}

	tconf, err := tracer.FromAny(r.env.internal, nconf)
	if err != nil {
		return convertDocsLintErr(err)
	}

	r.tracer = tconf
	return nil
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
	if parsedConf.Label == "" {
		return errors.New("a label must be specified")
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
	if parsedConf.Label == "" {
		return errors.New("a label must be specified")
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
	if parsedConf.Label == "" {
		return errors.New("a label must be specified")
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
	if parsedConf.Label == "" {
		return errors.New("a label must be specified")
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
	if parsedConf.Label == "" {
		return errors.New("a label must be specified")
	}

	r.resources.ResourceRateLimits = append(r.resources.ResourceRateLimits, parsedConf)
	return nil
}

// Build attempts to create a collection of resources from the config provided,
// and also a function that closes and cleans up all resources once they're
// finished with.
func (r *ResourceBuilder) Build() (*Resources, func(context.Context) error, error) {
	engVer := r.engineVersion
	if engVer == "" {
		engVer = cli.Version
	}

	// This temporary manager is a very lazy way of instantiating a manager that
	// restricts the bloblang and component environments to custom plugins.
	// Ideally we would break out the constructor for our general purpose
	// manager to allow for a two-tier initialisation where we can defer
	// resource constructors until after this metrics exporter is initialised.
	tmpMgr, err := manager.New(
		manager.NewResourceConfig(),
		manager.OptSetEngineVersion(engVer),
		manager.OptSetEnvironment(r.env.internal),
		manager.OptSetBloblangEnvironment(r.env.getBloblangParserEnv()),
	)
	if err != nil {
		return nil, nil, err
	}

	for _, f := range r.onMgrInit {
		if err := f(newResourcesFromManager(tmpMgr)); err != nil {
			return nil, nil, err
		}
	}

	tracer, err := r.env.internal.TracersInit(r.tracer, tmpMgr)
	if err != nil {
		return nil, nil, err
	}

	stats, err := r.env.internal.MetricsInit(r.metrics, tmpMgr)
	if err != nil {
		return nil, nil, err
	}
	if hler := stats.HandlerFunc(); r.apiMut != nil && hler != nil {
		r.apiMut.RegisterEndpoint("/stats", "Exposes service-wide metrics in the format configured.", hler)
		r.apiMut.RegisterEndpoint("/metrics", "Exposes service-wide metrics in the format configured.", hler)
	}

	opts := []manager.OptFunc{
		manager.OptSetEngineVersion(engVer),
		manager.OptSetMetrics(stats),
		manager.OptSetTracer(tracer),
		manager.OptSetAPIReg(r.apiMut),
		manager.OptSetEnvironment(r.env.internal),
		manager.OptSetBloblangEnvironment(r.env.getBloblangParserEnv()),
		manager.OptSetFS(r.env.fs),
	}

	if r.customLogger != nil {
		opts = append(opts, manager.OptSetLogger(r.customLogger))
	}

	mgr, err := manager.New(r.resources, opts...)
	if err != nil {
		return nil, nil, err
	}

	res := newResourcesFromManager(mgr)
	for _, f := range r.onMgrInit {
		if err := f(res); err != nil {
			return nil, nil, err
		}
	}

	return res, func(ctx context.Context) error {
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
