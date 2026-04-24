// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func parseBloblangV2Proc(t *testing.T, mapping string) *bloblangV2Proc {
	t.Helper()
	conf, err := bloblangV2ProcConfig().ParseYAML(mapping, nil)
	require.NoError(t, err)
	proc, err := newBloblangV2FromParsed(conf, service.MockResources())
	require.NoError(t, err)
	return proc
}

func TestBloblangV2ProcessorStructuredMapping(t *testing.T) {
	proc := parseBloblangV2Proc(t, `|
  output.id = input.id
  output.fans = input.fans.filter(fan -> fan.obsession > 0.5)
`)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage(nil)
	msg.SetStructured(map[string]any{
		"id":   "foo",
		"fans": []any{map[string]any{"obsession": 0.8}, map[string]any{"obsession": 0.2}},
	})

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)

	got, err := batches[0][0].AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"id":   "foo",
		"fans": []any{map[string]any{"obsession": 0.8}},
	}, got)
}

func TestBloblangV2ProcessorRootDeletionFilters(t *testing.T) {
	proc := parseBloblangV2Proc(t, `|
  output = if input.drop == true { deleted() } else { input }
`)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	keep := service.NewMessage(nil)
	keep.SetStructured(map[string]any{"drop": false, "id": "a"})

	drop := service.NewMessage(nil)
	drop.SetStructured(map[string]any{"drop": true, "id": "b"})

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{keep, drop})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1, "dropped message should be filtered out")

	got, err := batches[0][0].AsStructured()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"drop": false, "id": "a"}, got)
}

func TestBloblangV2ProcessorAllDeletedReturnsEmpty(t *testing.T) {
	proc := parseBloblangV2Proc(t, `output = deleted()`)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage([]byte(`{"x":1}`))
	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	assert.Empty(t, batches, "batch with every message deleted should collapse to nothing")
}

func TestBloblangV2ProcessorMetadataReplacement(t *testing.T) {
	// V2 semantics: output@ starts empty on each invocation. A mapping that
	// writes only one key should leave the produced message with only that
	// one key, regardless of what was on the incoming message.
	proc := parseBloblangV2Proc(t, `|
  output = input
  output@.kept = "yes"
`)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage(nil)
	msg.SetStructured(map[string]any{"v": 1})
	msg.MetaSetMut("will_be_dropped", "original")

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)

	out := batches[0][0]
	_, exists := out.MetaGetMut("will_be_dropped")
	assert.False(t, exists, "incoming metadata should not leak when mapping does not copy it")

	v, ok := out.MetaGetMut("kept")
	require.True(t, ok)
	assert.Equal(t, "yes", v)
}

func TestBloblangV2ProcessorMetadataCopyThrough(t *testing.T) {
	proc := parseBloblangV2Proc(t, `|
  output = input
  output@ = input@
  output@.added = "new"
`)
	t.Cleanup(func() { _ = proc.Close(t.Context()) })

	msg := service.NewMessage(nil)
	msg.SetStructured(map[string]any{"v": 1})
	msg.MetaSetMut("kept_from_input", "original")

	batches, err := proc.ProcessBatch(t.Context(), service.MessageBatch{msg})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)

	out := batches[0][0]
	v, ok := out.MetaGetMut("kept_from_input")
	require.True(t, ok)
	assert.Equal(t, "original", v)

	v, ok = out.MetaGetMut("added")
	require.True(t, ok)
	assert.Equal(t, "new", v)
}

func TestBloblangV2ProcessorParseErrorAtConstruction(t *testing.T) {
	conf, err := bloblangV2ProcConfig().ParseYAML(`output = nope(`, nil)
	require.NoError(t, err)
	_, err = newBloblangV2FromParsed(conf, service.MockResources())
	assert.Error(t, err)
}
