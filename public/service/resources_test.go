// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

type fooReader struct {
	mgr *service.Resources
}

func (r fooReader) Connect(context.Context) error {
	return nil
}

func (r fooReader) ReadBatch(ctx context.Context) (b service.MessageBatch, aFn service.AckFunc, err error) {
	if accessErr := r.mgr.AccessInput(ctx, "foo", func(i *service.ResourceInput) {
		b, aFn, err = i.ReadBatch(ctx)
	}); accessErr != nil {
		err = accessErr
	}
	return
}

func (r fooReader) Close(ctx context.Context) error {
	return nil
}

func TestResourceInput(t *testing.T) {
	env := service.NewEnvironment()
	require.NoError(t, env.RegisterBatchInput(
		"foo_reader", service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchInput, error) {
			return fooReader{mgr: mgr}, nil
		}))

	b := env.NewStreamBuilder()

	require.NoError(t, b.SetYAML(`
input:
  foo_reader: {}

input_resources:
  - label: foo
    generate:
      count: 3
      mapping: |
        root.id = count("public/service/resources_test.TestResourceInput")
        root.purpose = "test resource inputs"

output:
  drop: {}
`))

	var consumedMessages []string
	var consumedMut sync.Mutex
	require.NoError(t, b.AddConsumerFunc(func(ctx context.Context, m *service.Message) error {
		consumedMut.Lock()
		mBytes, _ := m.AsBytes()
		consumedMessages = append(consumedMessages, string(mBytes))
		consumedMut.Unlock()
		return nil
	}))

	strm, err := b.Build()
	require.NoError(t, err)

	require.NoError(t, strm.Run(t.Context()))
	assert.Equal(t, []string{
		`{"id":1,"purpose":"test resource inputs"}`,
		`{"id":2,"purpose":"test resource inputs"}`,
		`{"id":3,"purpose":"test resource inputs"}`,
	}, consumedMessages)
}

type fooWriter struct {
	mgr *service.Resources
}

func (r fooWriter) Connect(context.Context) error {
	return nil
}

func (r fooWriter) Write(ctx context.Context, msg *service.Message) (err error) {
	if accessErr := r.mgr.AccessOutput(ctx, "foo", func(o *service.ResourceOutput) {
		err = o.Write(ctx, msg)
	}); accessErr != nil {
		err = accessErr
	}
	return
}

func (r fooWriter) Close(ctx context.Context) error {
	return nil
}

func TestResourceOutput(t *testing.T) {
	tmpDir := t.TempDir()

	outFilePath := filepath.Join(tmpDir, "out.txt")

	env := service.NewEnvironment()
	require.NoError(t, env.RegisterOutput(
		"foo_writer", service.NewConfigSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Output, int, error) {
			return fooWriter{mgr: mgr}, 1, nil
		}))

	b := env.NewStreamBuilder()

	require.NoError(t, b.SetYAML(fmt.Sprintf(`
input:
  generate:
    count: 3
    mapping: |
      root.id = count("public/service/resources_test.TestResourceOutput")
      root.purpose = "test resource outputs"

output_resources:
  - label: foo
    file:
      codec: lines
      path: %v

output:
  foo_writer: {}
`, outFilePath)))

	strm, err := b.Build()
	require.NoError(t, err)

	require.NoError(t, strm.Run(t.Context()))

	outBytes, err := os.ReadFile(outFilePath)
	require.NoError(t, err)

	assert.Equal(t, `{"id":1,"purpose":"test resource outputs"}
{"id":2,"purpose":"test resource outputs"}
{"id":3,"purpose":"test resource outputs"}
`, string(outBytes))
}

type (
	testKeyTypeA int
	testKeyTypeB int
)

const (
	testKeyA testKeyTypeA = iota
	testKeyB testKeyTypeB = iota
)

func TestResourcesGenericValues(t *testing.T) {
	res := service.MockResources()

	res.SetGeneric(testKeyA, "foo")
	res.SetGeneric(testKeyB, "bar")

	_, exists := res.GetGeneric("not a key")
	assert.False(t, exists)

	v, exists := res.GetGeneric(testKeyA)
	assert.True(t, exists)
	assert.Equal(t, "foo", v)

	v, exists = res.GetGeneric(testKeyB)
	assert.True(t, exists)
	assert.Equal(t, "bar", v)
}

func TestResourcesGenericGetOrSet(t *testing.T) {
	res := service.MockResources()

	v, loaded := res.GetOrSetGeneric(testKeyA, "foo")
	assert.False(t, loaded)
	assert.Equal(t, "foo", v)

	v, loaded = res.GetOrSetGeneric(testKeyA, "bar")
	assert.True(t, loaded)
	assert.Equal(t, "foo", v)
}
