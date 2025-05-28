// Copyright 2025 Redpanda Data, Inc.

package manager

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/buffer"
	"github.com/redpanda-data/benthos/v4/internal/component/cache"
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/component/ratelimit"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

func TestInitialization(t *testing.T) {
	env := bundle.NewEnvironment()

	require.NoError(t, env.BufferAdd(func(c buffer.Config, mgr bundle.NewManagement) (buffer.Streamed, error) {
		return nil, errors.New("not this buffer")
	}, docs.ComponentSpec{
		Name: "testbuffer",
	}))

	require.NoError(t, env.CacheAdd(func(c cache.Config, mgr bundle.NewManagement) (cache.V1, error) {
		return nil, errors.New("not this cache")
	}, docs.ComponentSpec{
		Name: "testcache",
	}))

	require.NoError(t, env.InputAdd(func(c input.Config, mgr bundle.NewManagement) (input.Streamed, error) {
		return nil, errors.New("not this input")
	}, docs.ComponentSpec{
		Name: "testinput",
	}))

	lenOutputProcs := 0
	require.NoError(t, env.OutputAdd(func(c output.Config, mgr bundle.NewManagement, p ...processor.PipelineConstructorFunc) (output.Streamed, error) {
		lenOutputProcs = len(p)
		return nil, errors.New("not this output")
	}, docs.ComponentSpec{
		Name: "testoutput",
	}))

	require.NoError(t, env.ProcessorAdd(func(c processor.Config, mgr bundle.NewManagement) (processor.V1, error) {
		return nil, errors.New("not this processor")
	}, docs.ComponentSpec{
		Name: "testprocessor",
	}))

	require.NoError(t, env.RateLimitAdd(func(c ratelimit.Config, mgr bundle.NewManagement) (ratelimit.V1, error) {
		return nil, errors.New("not this rate limit")
	}, docs.ComponentSpec{
		Name: "testratelimit",
	}))

	mgr, err := New(NewResourceConfig(), OptSetEnvironment(env))
	require.NoError(t, err)

	bConf := buffer.NewConfig()
	bConf.Type = "testbuffer"
	_, err = mgr.NewBuffer(bConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this buffer")

	cConf := cache.NewConfig()
	cConf.Type = "testcache"
	_, err = mgr.NewCache(cConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this cache")

	iConf := input.NewConfig()
	iConf.Type = "testinput"
	_, err = mgr.NewInput(iConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this input")

	oConf := output.NewConfig()
	oConf.Type = "testoutput"
	_, err = mgr.NewOutput(oConf, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this output")
	assert.Equal(t, 3, lenOutputProcs)

	pConf := processor.NewConfig()
	pConf.Type = "testprocessor"
	_, err = mgr.NewProcessor(pConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this processor")

	rConf := ratelimit.NewConfig()
	rConf.Type = "testratelimit"
	_, err = mgr.NewRateLimit(rConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not this rate limit")
}
