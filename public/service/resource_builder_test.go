// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func TestResourceBuilderResources(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "data_in.txt"), []byte("first\nsecond\nthird\n"), 0o777))

	b := service.NewResourceBuilder()
	b.SetEnvVarLookupFunc(func(_ context.Context, k string) (string, bool) {
		if k == "DIR" {
			return tmpDir, true
		}
		return "", false
	})

	require.NoError(t, b.AddCacheYAML(`
label: foocache
file:
  directory: ${DIR}
`))

	require.NoError(t, b.AddInputYAML(`
label: fooinput
file:
  paths: [ ${DIR}/data_in.txt ]
`))

	require.NoError(t, b.AddProcessorYAML(`
label: fooprocessor
mapping: 'root = file("${DIR}/" + content().string())'
`))

	require.NoError(t, b.AddOutputYAML(`
label: foooutput
file:
  path: ${DIR}/data_out.txt

`))

	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()

	res, stop, err := b.Build()
	require.NoError(t, err)
	defer func() {
		_ = stop(ctx)
	}()

	require.NoError(t, res.AccessInput(ctx, "fooinput", func(i *service.ResourceInput) {
		for _, exp := range []string{"first", "second", "third"} {
			b, aFn, err := i.ReadBatch(ctx)
			require.NoError(t, err)
			require.Len(t, b, 1)

			bBytes, err := b[0].AsBytes()
			require.NoError(t, err)

			assert.Equal(t, exp, string(bBytes))

			require.NoError(t, aFn(ctx, nil))
		}
	}))

	require.NoError(t, res.AccessCache(ctx, "foocache", func(c service.Cache) {
		require.NoError(t, c.Set(ctx, "cachea", []byte("foo"), nil))
		require.NoError(t, c.Set(ctx, "cacheb", []byte("bar"), nil))
	}))

	require.NoError(t, res.AccessOutput(ctx, "foooutput", func(o *service.ResourceOutput) {
		require.NoError(t, o.Write(ctx, service.NewMessage([]byte("out first"))))
		require.NoError(t, o.Write(ctx, service.NewMessage([]byte("out second"))))
	}))

	require.NoError(t, res.AccessProcessor(ctx, "fooprocessor", func(p *service.ResourceProcessor) {
		for k, v := range map[string]string{
			"cachea":       "foo",
			"cacheb":       "bar",
			"data_out.txt": "out first\nout second\n",
		} {
			res, err := p.Process(ctx, service.NewMessage([]byte(k)))
			require.NoError(t, err)
			require.Len(t, res, 1)

			rBytes, err := res[0].AsBytes()
			require.NoError(t, err)

			assert.Equal(t, string(rBytes), v)
		}
	}))
}

func TestResourceBuilderYAMLErrors(t *testing.T) {
	b := service.NewResourceBuilder()

	err := b.AddCacheYAML(`{ label: "", type: memory }`)
	require.Error(t, err)
	assert.EqualError(t, err, "a label must be specified")

	err = b.AddInputYAML(`not valid ! yaml 34324`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected object")

	err = b.AddInputYAML(`not_a_field: nah`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to infer")

	err = b.AddInputYAML(`generate: { not_a_field: nah }`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field not_a_field not recognised")

	err = b.AddRateLimitYAML(`{ label: "", local: {} }`)
	require.Error(t, err)
	assert.EqualError(t, err, "a label must be specified")
}

func TestResourceBuilderDisabledLinting(t *testing.T) {
	lintingErrorConfig := `
label: meow
generate:
  mapping: 'root = deleted()'
  meow: ignore this field
`
	b := service.NewResourceBuilder()
	require.Error(t, b.AddInputYAML(lintingErrorConfig))

	b = service.NewResourceBuilder()
	b.DisableLinting()
	require.NoError(t, b.AddInputYAML(lintingErrorConfig))
}
