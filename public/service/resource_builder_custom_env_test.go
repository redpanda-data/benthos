// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// mockCache is a simple in-memory cache for testing
type mockCache struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string][]byte),
	}
}

func (m *mockCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.data[key]; ok {
		return val, nil
	}
	return nil, service.ErrKeyNotFound
}

func (m *mockCache) Set(ctx context.Context, key string, value []byte, ttl *time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockCache) Add(ctx context.Context, key string, value []byte, ttl *time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[key]; exists {
		return service.ErrKeyAlreadyExists
	}
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCache) Close(ctx context.Context) error {
	return nil
}

// mockProcessor uppercases message content
type mockProcessor struct {
	prefix string
}

func (m *mockProcessor) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	content, err := msg.AsBytes()
	if err != nil {
		return nil, err
	}
	msg.SetBytes([]byte(m.prefix + string(content)))
	return service.MessageBatch{msg}, nil
}

func (m *mockProcessor) Close(ctx context.Context) error {
	return nil
}

// TestResourceBuilderCustomEnvironment tests creating a resource builder with a custom environment
func TestResourceBuilderCustomEnvironment(t *testing.T) {
	env := service.NewEnvironment()

	// Register a custom cache
	require.NoError(t, env.RegisterCache(
		"custom_cache",
		service.NewConfigSpec().
			Field(service.NewStringField("name")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return newMockCache(), nil
		},
	))

	// Register a custom processor
	require.NoError(t, env.RegisterProcessor(
		"custom_processor",
		service.NewConfigSpec().
			Field(service.NewStringField("prefix")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			prefix, err := conf.FieldString("prefix")
			if err != nil {
				return nil, err
			}
			return &mockProcessor{prefix: prefix}, nil
		},
	))

	// Create resource builder with custom environment
	b := env.NewResourceBuilder()

	// Add custom cache
	require.NoError(t, b.AddCacheYAML(`
label: test_cache
custom_cache:
  name: test
`))

	// Add custom processor
	require.NoError(t, b.AddProcessorYAML(`
label: test_processor
custom_processor:
  prefix: "PREFIXED: "
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	// Test custom cache
	require.NoError(t, res.AccessCache(ctx, "test_cache", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "key1", []byte("value1"), nil))

		val, err := c.Get(ctx, "key1")
		require.NoError(t, err)
		assert.Equal(t, "value1", string(val))
	}))

	// Test custom processor
	require.NoError(t, res.AccessProcessor(ctx, "test_processor", func(p *service.ResourceProcessor) {
		result, err := p.Process(ctx, service.NewMessage([]byte("test message")))
		require.NoError(t, err)
		require.Len(t, result, 1)

		resultBytes, err := result[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, "PREFIXED: test message", string(resultBytes))
	}))
}

// TestResourceBuilderCustomEnvironmentIsolation tests that custom environment is isolated
func TestResourceBuilderCustomEnvironmentIsolation(t *testing.T) {
	// Create a custom environment with a custom cache
	customEnv := service.NewEnvironment()
	require.NoError(t, customEnv.RegisterCache(
		"isolated_cache",
		service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return newMockCache(), nil
		},
	))

	// Try to use the custom cache with default environment (should fail)
	defaultBuilder := service.NewResourceBuilder()
	err := defaultBuilder.AddCacheYAML(`
label: test
isolated_cache: {}
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to infer")

	// Try to use the custom cache with custom environment (should succeed)
	customBuilder := customEnv.NewResourceBuilder()
	require.NoError(t, customBuilder.AddCacheYAML(`
label: test
isolated_cache: {}
`))
}

// TestResourceBuilderCustomEnvironmentWithMetrics tests setting metrics with custom environment
func TestResourceBuilderCustomEnvironmentWithMetrics(t *testing.T) {
	env := service.NewEnvironment()

	b := env.NewResourceBuilder()
	require.NoError(t, b.SetMetricsYAML(`
none: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NotNil(t, res)
}

// TestResourceBuilderCustomEnvironmentWithTracer tests setting tracer with custom environment
func TestResourceBuilderCustomEnvironmentWithTracer(t *testing.T) {
	env := service.NewEnvironment()

	b := env.NewResourceBuilder()
	require.NoError(t, b.SetTracerYAML(`
none: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NotNil(t, res)
}

// TestResourceBuilderCustomEnvironmentEnvVarLookup tests env var interpolation with custom environment
func TestResourceBuilderCustomEnvironmentEnvVarLookup(t *testing.T) {
	env := service.NewEnvironment()

	b := env.NewResourceBuilder()
	b.SetEnvVarLookupFunc(func(_ context.Context, k string) (string, bool) {
		if k == "CUSTOM_VALUE" {
			return "custom_replaced", true
		}
		return "", false
	})

	require.NoError(t, b.AddCacheYAML(`
label: ${CUSTOM_VALUE}_cache
memory: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	// Verify the cache was created with the interpolated name
	require.NoError(t, res.AccessCache(ctx, "custom_replaced_cache", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "test", []byte("value"), nil))
	}))
}

// TestResourceBuilderCustomEnvironmentDisabledLinting tests linting disabled with custom environment
func TestResourceBuilderCustomEnvironmentDisabledLinting(t *testing.T) {
	env := service.NewEnvironment()

	lintingErrorConfig := `
label: test
memory: {}
unknown_field: this should fail linting
`

	// First verify linting catches the error
	b1 := env.NewResourceBuilder()
	err := b1.AddCacheYAML(lintingErrorConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field unknown_field is invalid")

	// Now disable linting and verify it works
	b2 := env.NewResourceBuilder()
	b2.DisableLinting()
	require.NoError(t, b2.AddCacheYAML(lintingErrorConfig))
}

// TestResourceBuilderCustomEnvironmentOnResourceInit tests OnResourceInit callback with custom environment
func TestResourceBuilderCustomEnvironmentOnResourceInit(t *testing.T) {
	env := service.NewEnvironment()

	b := env.NewResourceBuilder()

	var initCallCount int
	b.OnResourceInit(func(r *service.Resources) error {
		initCallCount++
		require.NotNil(t, r)
		return nil
	})

	require.NoError(t, b.AddCacheYAML(`
label: test_cache
memory: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NotNil(t, res)
	assert.Greater(t, initCallCount, 0, "OnResourceInit should have been called")
}

// TestResourceBuilderCustomEnvironmentBuildSuspended tests BuildSuspended with custom environment
func TestResourceBuilderCustomEnvironmentBuildSuspended(t *testing.T) {
	env := service.NewEnvironment()

	b := env.NewResourceBuilder()
	require.NoError(t, b.AddCacheYAML(`
label: test_cache
memory: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.BuildSuspended()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NotNil(t, res)

	// Verify we can access the cache even though resources are suspended
	require.NoError(t, res.AccessCache(ctx, "test_cache", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "key", []byte("value"), nil))

		val, err := c.Get(ctx, "key")
		require.NoError(t, err)
		assert.Equal(t, "value", string(val))
	}))
}

// TestResourceBuilderCustomEnvironmentMultipleResources tests adding multiple resource types
func TestResourceBuilderCustomEnvironmentMultipleResources(t *testing.T) {
	env := service.NewEnvironment()

	// Register custom components
	require.NoError(t, env.RegisterCache(
		"custom_cache",
		service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return newMockCache(), nil
		},
	))

	require.NoError(t, env.RegisterProcessor(
		"custom_processor",
		service.NewConfigSpec().
			Field(service.NewStringField("prefix").Default("PREFIX: ")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			prefix, _ := conf.FieldString("prefix")
			return &mockProcessor{prefix: prefix}, nil
		},
	))

	b := env.NewResourceBuilder()

	// Add multiple resources
	require.NoError(t, b.AddCacheYAML(`
label: cache1
custom_cache: {}
`))

	require.NoError(t, b.AddCacheYAML(`
label: cache2
memory: {}
`))

	require.NoError(t, b.AddProcessorYAML(`
label: proc1
custom_processor:
  prefix: "PROC1: "
`))

	require.NoError(t, b.AddProcessorYAML(`
label: proc2
custom_processor:
  prefix: "PROC2: "
`))

	require.NoError(t, b.AddRateLimitYAML(`
label: rl1
local:
  count: 10
  interval: 1s
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	// Verify all resources are accessible
	require.NoError(t, res.AccessCache(ctx, "cache1", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "k1", []byte("v1"), nil))
	}))

	require.NoError(t, res.AccessCache(ctx, "cache2", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "k2", []byte("v2"), nil))
	}))

	require.NoError(t, res.AccessProcessor(ctx, "proc1", func(p *service.ResourceProcessor) {
		result, err := p.Process(ctx, service.NewMessage([]byte("msg")))
		require.NoError(t, err)
		require.Len(t, result, 1)
		resultBytes, err := result[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, "PROC1: msg", string(resultBytes))
	}))

	require.NoError(t, res.AccessProcessor(ctx, "proc2", func(p *service.ResourceProcessor) {
		result, err := p.Process(ctx, service.NewMessage([]byte("msg")))
		require.NoError(t, err)
		require.Len(t, result, 1)
		resultBytes, err := result[0].AsBytes()
		require.NoError(t, err)
		assert.Equal(t, "PROC2: msg", string(resultBytes))
	}))
}

// TestResourceBuilderCustomEnvironmentClone tests that cloned environments work independently
func TestResourceBuilderCustomEnvironmentClone(t *testing.T) {
	// Create base environment with one component
	baseEnv := service.NewEnvironment()
	require.NoError(t, baseEnv.RegisterCache(
		"base_cache",
		service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return newMockCache(), nil
		},
	))

	// Clone and add another component
	clonedEnv := baseEnv.Clone()
	require.NoError(t, clonedEnv.RegisterCache(
		"cloned_cache",
		service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Cache, error) {
			return newMockCache(), nil
		},
	))

	// Base environment should have both caches available
	baseBuilder := baseEnv.NewResourceBuilder()
	require.NoError(t, baseBuilder.AddCacheYAML(`
label: test1
base_cache: {}
`))

	// Cloned environment should have both caches available
	clonedBuilder := clonedEnv.NewResourceBuilder()
	require.NoError(t, clonedBuilder.AddCacheYAML(`
label: test2
base_cache: {}
`))
	require.NoError(t, clonedBuilder.AddCacheYAML(`
label: test3
cloned_cache: {}
`))

	// Original base environment should still work
	baseBuilder2 := baseEnv.NewResourceBuilder()
	require.NoError(t, baseBuilder2.AddCacheYAML(`
label: test4
base_cache: {}
`))
}

// TestResourceBuilderCustomEnvironmentWithRestrictedComponents tests using With() to restrict components
func TestResourceBuilderCustomEnvironmentWithRestrictedComponents(t *testing.T) {
	// Create environment with only specific components
	restrictedEnv := service.NewEnvironment().With("memory", "generate", "drop")

	b := restrictedEnv.NewResourceBuilder()

	// These should work (memory, generate, drop are included)
	require.NoError(t, b.AddCacheYAML(`
label: allowed_cache
memory: {}
`))

	// This should fail (file is not included)
	err := b.AddCacheYAML(`
label: disallowed_cache
file:
  directory: /tmp
`)
	require.Error(t, err)
}

// TestResourceBuilderCustomEnvironmentYAMLErrors tests error handling with custom environment
func TestResourceBuilderCustomEnvironmentYAMLErrors(t *testing.T) {
	env := service.NewEnvironment()
	b := env.NewResourceBuilder()

	// Test missing label error
	err := b.AddCacheYAML(`{ label: "", memory: {} }`)
	require.Error(t, err)
	assert.EqualError(t, err, "a label must be specified")

	// Test invalid YAML
	err = b.AddInputYAML(`not valid ! yaml 34324`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected object")

	// Test unknown field
	err = b.AddInputYAML(`not_a_field: nah`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to infer")

	// Test missing label for rate limit
	err = b.AddRateLimitYAML(`{ label: "", local: {} }`)
	require.Error(t, err)
	assert.EqualError(t, err, "a label must be specified")
}

// TestResourceBuilderCustomEnvironmentSetEngineVersion tests setting engine version
func TestResourceBuilderCustomEnvironmentSetEngineVersion(t *testing.T) {
	env := service.NewEnvironment()
	b := env.NewResourceBuilder()

	b.SetEngineVersion("test-version-1.0.0")

	require.NoError(t, b.AddCacheYAML(`
label: test_cache
memory: {}
`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NotNil(t, res)
}
