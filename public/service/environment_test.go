// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func walkForSummaries(fn func(func(name string, config *service.ConfigView))) map[string]string {
	summaries := map[string]string{}
	fn(func(name string, config *service.ConfigView) {
		summaries[name] = config.Summary()
	})
	return summaries
}

func TestEnvironmentAdjustments(t *testing.T) {
	envOne := service.NewEnvironment()
	envTwo := envOne.Clone()

	assert.NoError(t, envOne.RegisterCache(
		"one_cache", service.NewConfigSpec().Summary("cache one"),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return nil, errors.New("cache one err")
		},
	))
	assert.NoError(t, envOne.RegisterInput(
		"one_input", service.NewConfigSpec().Summary("input one"),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Input, error) {
			return nil, errors.New("input one err")
		},
	))
	assert.NoError(t, envOne.RegisterOutput(
		"one_output", service.NewConfigSpec().Summary("output one"),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Output, int, error) {
			return nil, 0, errors.New("output one err")
		},
	))
	assert.NoError(t, envOne.RegisterProcessor(
		"one_processor", service.NewConfigSpec().Summary("processor one"),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			return nil, errors.New("processor one err")
		},
	))
	assert.NoError(t, envOne.RegisterRateLimit(
		"one_rate_limit", service.NewConfigSpec().Summary("rate limit one"),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.RateLimit, error) {
			return nil, errors.New("rate limit one err")
		},
	))

	assert.Equal(t, "cache one", walkForSummaries(envOne.WalkCaches)["one_cache"])
	assert.Equal(t, "input one", walkForSummaries(envOne.WalkInputs)["one_input"])
	assert.Equal(t, "output one", walkForSummaries(envOne.WalkOutputs)["one_output"])
	assert.Equal(t, "processor one", walkForSummaries(envOne.WalkProcessors)["one_processor"])
	assert.Equal(t, "rate limit one", walkForSummaries(envOne.WalkRateLimits)["one_rate_limit"])

	assert.NotContains(t, walkForSummaries(envTwo.WalkCaches), "one_cache")
	assert.NotContains(t, walkForSummaries(envTwo.WalkInputs), "one_input")
	assert.NotContains(t, walkForSummaries(envTwo.WalkOutputs), "one_output")
	assert.NotContains(t, walkForSummaries(envTwo.WalkProcessors), "one_processor")
	assert.NotContains(t, walkForSummaries(envTwo.WalkRateLimits), "one_rate_limit")

	testConfig := `
input:
  one_input: {}
pipeline:
  processors:
    - one_processor: {}
output:
  one_output: {}
cache_resources:
  - label: foocache
    one_cache: {}
rate_limit_resources:
  - label: foorl
    one_rate_limit: {}
`

	assert.NoError(t, envOne.NewStreamBuilder().SetYAML(testConfig))
	assert.Error(t, envTwo.NewStreamBuilder().SetYAML(testConfig))
}

func TestEnvironmentBloblangIsolation(t *testing.T) {
	bEnv := bloblang.NewEnvironment().WithoutFunctions("now")
	require.NoError(t, bEnv.RegisterFunctionV2("meow", bloblang.NewPluginSpec(), func(args *bloblang.ParsedParams) (bloblang.Function, error) {
		return func() (any, error) {
			return "meow", nil
		}, nil
	}))

	envOne := service.NewEnvironment()
	envOne.UseBloblangEnvironment(bEnv)

	badConfig := `
pipeline:
  processors:
    - bloblang: 'root = now()'
`

	goodConfig := `
pipeline:
  processors:
    - bloblang: 'root = meow()'

output:
  drop: {}

logger:
  level: OFF
`

	assert.Error(t, envOne.NewStreamBuilder().SetYAML(badConfig))

	strmBuilder := envOne.NewStreamBuilder()
	require.NoError(t, strmBuilder.SetYAML(goodConfig))

	var received []string
	require.NoError(t, strmBuilder.AddConsumerFunc(func(c context.Context, m *service.Message) error {
		b, err := m.AsBytes()
		if err != nil {
			return err
		}
		received = append(received, string(b))
		return nil
	}))

	pFn, err := strmBuilder.AddProducerFunc()
	require.NoError(t, err)

	strm, err := strmBuilder.Build()
	require.NoError(t, err)

	go func() {
		require.NoError(t, strm.Run(t.Context()))
	}()

	require.NoError(t, pFn(t.Context(), service.NewMessage([]byte("hello world"))))

	require.NoError(t, strm.StopWithin(time.Second))
	assert.Equal(t, []string{"meow"}, received)
}

func TestEnvironmentBloblangV2SchemaIncludesPlugins(t *testing.T) {
	bEnv := bloblangv2.NewEmptyEnvironment()
	require.NoError(t, bEnv.RegisterFunction("hoot", bloblangv2.NewPluginSpec().Description("owl noise"),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) { return "hoot", nil }, nil
		},
	))
	require.NoError(t, bEnv.RegisterMethod("yell", bloblangv2.NewPluginSpec(),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) { return s + "!", nil }), nil
		},
	))

	env := service.NewEnvironment()
	env.UseBloblangV2Environment(bEnv)

	flat := env.GenerateSchema("test", "now").XFlattened()
	assert.Contains(t, flat["bloblang-v2-functions"], "hoot")
	assert.Contains(t, flat["bloblang-v2-methods"], "yell")
}

func TestConfigSchemaFromJSONV0RoundTripsBloblangV2(t *testing.T) {
	// Build a schema dump on a "remote" environment that has registered V2
	// plugins, then load it as JSON on a fresh "local" environment that has
	// no implementations of those plugins. Linting against the loaded
	// schema should accept mappings that reference the remote plugins, and
	// reject mappings that reference unknown ones — proving the stub
	// registrations carried through.
	remoteEnv := bloblangv2.NewEmptyEnvironment()
	require.NoError(t, remoteEnv.RegisterFunction("hoot",
		bloblangv2.NewPluginSpec().Description("owl noise"),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
			return func() (any, error) { return "hoot", nil }, nil
		},
	))
	require.NoError(t, remoteEnv.RegisterMethod("yell", bloblangv2.NewPluginSpec().
		Param(bloblangv2.NewStringParam("suffix").Default("!")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			suf, _ := args.GetString("suffix")
			return bloblangv2.StringMethod(func(s string) (any, error) { return s + suf, nil }), nil
		},
	))

	remoteSvcEnv := service.NewEmptyEnvironment()
	remoteSvcEnv.UseBloblangV2Environment(remoteEnv)
	dump, err := remoteSvcEnv.FullConfigSchema("v0", "now").MarshalJSONV0()
	require.NoError(t, err)

	localSchema, err := service.ConfigSchemaFromJSONV0(dump)
	require.NoError(t, err)

	// Re-marshal the loaded schema and confirm the V2 plugin descriptors
	// survived the decode → register-stub → enumerate cycle. If the decode
	// step had silently dropped them, MarshalJSONV0 on the loaded schema
	// would emit an empty set.
	rehydrated, err := localSchema.MarshalJSONV0()
	require.NoError(t, err)

	var redump struct {
		BloblangV2Functions []bloblangv2.PluginInfo `json:"bloblang-v2-functions"`
		BloblangV2Methods   []bloblangv2.PluginInfo `json:"bloblang-v2-methods"`
	}
	require.NoError(t, json.Unmarshal(rehydrated, &redump))

	require.Len(t, redump.BloblangV2Functions, 1)
	assert.Equal(t, "hoot", redump.BloblangV2Functions[0].Name)
	assert.Equal(t, "owl noise", redump.BloblangV2Functions[0].Description)

	require.Len(t, redump.BloblangV2Methods, 1)
	assert.Equal(t, "yell", redump.BloblangV2Methods[0].Name)
	require.Len(t, redump.BloblangV2Methods[0].Params, 1)
	assert.Equal(t, "suffix", redump.BloblangV2Methods[0].Params[0].Name)
	assert.Equal(t, "string", redump.BloblangV2Methods[0].Params[0].Kind)
	assert.True(t, redump.BloblangV2Methods[0].Params[0].HasDefault)
}

func TestEnvironmentBloblangV2ClonePropagation(t *testing.T) {
	customEnv := bloblangv2.NewEmptyEnvironment()
	require.NoError(t, customEnv.RegisterFunction("oink", bloblangv2.NewPluginSpec(), func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) { return "oink", nil }, nil
	}))

	procSpec := service.NewConfigSpec().Field(service.NewBloblangV2Field("mapping"))
	procCtor := func(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
		exec, err := conf.FieldBloblangV2("mapping")
		if err != nil {
			return nil, err
		}
		return &v2MappingProc{exec: exec}, nil
	}

	base := service.NewEnvironment()
	base.UseBloblangV2Environment(customEnv)
	require.NoError(t, base.RegisterProcessor("v2_clone_map", procSpec, procCtor))

	// A clone must inherit the custom V2 env and resolve the plugin.
	cloned := base.Clone()
	assertEnvResolves(t, cloned, "v2_clone_map", "output = oink()", "oink")

	// The With* variants also propagate the V2 env (exercise Without as the
	// most restrictive one — others share the same Clone-based machinery).
	without := base.Without("nonexistent_plugin")
	assertEnvResolves(t, without, "v2_clone_map", "output = oink()", "oink")
}

// assertEnvResolves runs a minimal stream that uses the v2_clone_map processor
// with the given mapping and asserts the consumer receives the expected output.
func assertEnvResolves(t *testing.T, env *service.Environment, procName, mapping, expected string) {
	t.Helper()

	yamlConf := fmt.Sprintf(`
pipeline:
  processors:
    - %s:
        mapping: '%s'

input:
  generate:
    count: 1
    mapping: 'root = "hello"'

output:
  drop: {}

logger:
  level: OFF
`, procName, mapping)

	builder := env.NewStreamBuilder()
	require.NoError(t, builder.SetYAML(yamlConf))

	var received []string
	require.NoError(t, builder.AddConsumerFunc(func(_ context.Context, m *service.Message) error {
		b, err := m.AsBytes()
		if err != nil {
			return err
		}
		received = append(received, string(b))
		return nil
	}))

	strm, err := builder.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, strm.Run(ctx))

	assert.Equal(t, []string{expected}, received)
}

func TestEnvironmentBloblangV2LintFailsForBadMapping(t *testing.T) {
	env := service.NewEnvironment()
	require.NoError(t, env.RegisterProcessor(
		"v2_lint_check",
		service.NewConfigSpec().Field(service.NewBloblangV2Field("mapping")),
		func(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
			_, err := conf.FieldBloblangV2("mapping")
			return nil, err
		},
	))

	badConfig := `
pipeline:
  processors:
    - v2_lint_check:
        mapping: 'output = nope('

output:
  drop: {}

logger:
  level: OFF
`
	err := env.NewStreamBuilder().SetYAML(badConfig)
	require.Error(t, err, "lint pass should reject malformed V2 mappings at SetYAML time")
	assert.Contains(t, err.Error(), "expected")
}

func TestEnvironmentBloblangV2LintRespectsCustomEnv(t *testing.T) {
	bEnv := bloblangv2.NewEmptyEnvironment()
	require.NoError(t, bEnv.RegisterFunction("squeak", bloblangv2.NewPluginSpec(), func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) { return "squeak", nil }, nil
	}))

	env := service.NewEnvironment()
	env.UseBloblangV2Environment(bEnv)
	require.NoError(t, env.RegisterProcessor(
		"v2_lint_check",
		service.NewConfigSpec().Field(service.NewBloblangV2Field("mapping")),
		func(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
			_, err := conf.FieldBloblangV2("mapping")
			return nil, err
		},
	))

	goodConfig := `
pipeline:
  processors:
    - v2_lint_check:
        mapping: 'output = squeak()'

output:
  drop: {}

logger:
  level: OFF
`
	// The custom env knows squeak so SetYAML should accept the mapping. A
	// fresh environment with the global V2 env would reject it during lint.
	require.NoError(t, env.NewStreamBuilder().SetYAML(goodConfig))

	plainEnv := service.NewEnvironment()
	require.NoError(t, plainEnv.RegisterProcessor(
		"v2_lint_check",
		service.NewConfigSpec().Field(service.NewBloblangV2Field("mapping")),
		func(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
			_, err := conf.FieldBloblangV2("mapping")
			return nil, err
		},
	))
	err := plainEnv.NewStreamBuilder().SetYAML(goodConfig)
	require.Error(t, err, "lint should reject squeak() against the default V2 env")
}

func TestEnvironmentBloblangV2Isolation(t *testing.T) {
	bEnv := bloblangv2.NewEmptyEnvironment()
	require.NoError(t, bEnv.RegisterFunction("woof", bloblangv2.NewPluginSpec(), func(args *bloblangv2.ParsedParams) (bloblangv2.Function, error) {
		return func() (any, error) {
			return "woof", nil
		}, nil
	}))

	// Register a processor plugin on the global environment that extracts a V2
	// mapping at construction time. Both environments below share this plugin,
	// but each environment has its own Bloblang V2 registry — so the "woof"
	// function only resolves on the environment that has the custom env set.
	procSpec := service.NewConfigSpec().Field(service.NewBloblangV2Field("mapping"))
	procCtor := func(conf *service.ParsedConfig, _ *service.Resources) (service.Processor, error) {
		exec, err := conf.FieldBloblangV2("mapping")
		if err != nil {
			return nil, err
		}
		return &v2MappingProc{exec: exec}, nil
	}

	envOne := service.NewEnvironment()
	envOne.UseBloblangV2Environment(bEnv)
	require.NoError(t, envOne.RegisterProcessor("v2_test_map", procSpec, procCtor))

	envTwo := service.NewEnvironment()
	require.NoError(t, envTwo.RegisterProcessor("v2_test_map", procSpec, procCtor))

	mappingConfig := `
pipeline:
  processors:
    - v2_test_map:
        mapping: 'output = woof()'

input:
  generate:
    count: 1
    mapping: 'root = "hello"'

output:
  drop: {}

logger:
  level: OFF
`

	// envTwo has the default global V2 environment, which does not have
	// "woof". The lint pass at SetYAML time should reject the mapping.
	builderTwo := envTwo.NewStreamBuilder()
	err := builderTwo.SetYAML(mappingConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "woof")

	// envOne has a custom V2 env containing "woof". Run must succeed and the
	// processor must rewrite messages to "woof".
	builderOne := envOne.NewStreamBuilder()
	require.NoError(t, builderOne.SetYAML(mappingConfig))

	var received []string
	require.NoError(t, builderOne.AddConsumerFunc(func(_ context.Context, m *service.Message) error {
		b, err := m.AsBytes()
		if err != nil {
			return err
		}
		received = append(received, string(b))
		return nil
	}))

	strm, err := builderOne.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, strm.Run(ctx))

	assert.Equal(t, []string{"woof"}, received)
}

type v2MappingProc struct {
	exec *bloblangv2.Executor
}

func (p *v2MappingProc) Process(_ context.Context, m *service.Message) (service.MessageBatch, error) {
	in, err := m.AsStructured()
	if err != nil {
		b, _ := m.AsBytes()
		in = b
	}
	out, err := p.exec.Query(in)
	if err != nil {
		return nil, err
	}
	nm := m.Copy()
	if s, ok := out.(string); ok {
		nm.SetBytes([]byte(s))
	} else {
		nm.SetStructured(out)
	}
	return service.MessageBatch{nm}, nil
}

func (p *v2MappingProc) Close(context.Context) error { return nil }

type testFS struct {
	ifs.FS
	override fstest.MapFS
}

func (fs testFS) Open(name string) (fs.File, error) {
	if f, err := fs.override.Open(name); err == nil {
		return f, nil
	}

	return fs.FS.Open(name)
}

func (fs testFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	if f, err := fs.override.Open(name); err == nil {
		return f, nil
	}

	return fs.FS.OpenFile(name, flag, perm)
}

func (fs testFS) Stat(name string) (fs.FileInfo, error) {
	if f, err := fs.override.Stat(name); err == nil {
		return f, nil
	}

	return fs.FS.Stat(name)
}

func TestEnvironmentUseFS(t *testing.T) {
	tmpDir := t.TempDir()
	outFilePath := filepath.Join(tmpDir, "out.txt")

	env := service.NewEnvironment()
	env.UseFS(service.NewFS(testFS{ifs.OS(), fstest.MapFS{
		"hello.txt": {
			Data: []byte("hello\nworld"),
		},
	}}))

	b := env.NewStreamBuilder()

	require.NoError(t, b.SetYAML(fmt.Sprintf(`
input:
  file:
    paths: [hello.txt]

output:
  label: foo
  file:
    codec: lines
    path: %v
`, outFilePath)))

	strm, err := b.Build()
	require.NoError(t, err)

	require.NoError(t, strm.Run(t.Context()))

	outBytes, err := os.ReadFile(outFilePath)
	require.NoError(t, err)

	assert.Equal(t, `hello
world
`, string(outBytes))
}
