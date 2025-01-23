// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/template"
)

func TestMockWalkableError(t *testing.T) {
	batch := MessageBatch{
		NewMessage([]byte("a")),
		NewMessage([]byte("b")),
		NewMessage([]byte("c")),
	}

	batchError := errors.New("simulated error")
	err := NewBatchError(batch, batchError).
		Failed(0, errors.New("a error")).
		Failed(1, errors.New("b error")).
		Failed(2, errors.New("c error"))

	require.Len(t, batch, err.IndexedErrors(), "indexed errors did not match size of batch")
	require.ErrorIs(t, err, batchError, "headline error is not propagated")

	runs := 0
	err.WalkMessages(func(i int, m *Message, err error) bool {
		runs++

		bs, berr := m.AsBytes()
		require.NoErrorf(t, berr, "could not get bytes from message at %d", i)
		require.Equal(t, err.Error(), fmt.Sprintf("%s error", bs))
		return true
	})

	require.Equal(t, len(batch), runs, "WalkMessages did not iterate the whole batch")
}

func TestMockWalkableError_ExcessErrors(t *testing.T) {
	batch := MessageBatch{
		NewMessage([]byte("a")),
		NewMessage([]byte("b")),
		NewMessage([]byte("c")),
	}

	batchError := errors.New("simulated error")
	err := NewBatchError(batch, batchError).
		Failed(0, errors.New("a error")).
		Failed(1, errors.New("b error")).
		Failed(2, errors.New("c error")).
		Failed(3, errors.New("d error"))

	require.Equal(t, len(batch), err.IndexedErrors(), "indexed errors did not match size of batch")
}

func TestMockWalkableError_OmitSuccessfulMessages(t *testing.T) {
	batch := MessageBatch{
		NewMessage([]byte("a")),
		NewMessage([]byte("b")),
		NewMessage([]byte("c")),
	}

	batchError := errors.New("simulated error")
	err := NewBatchError(batch, batchError).
		Failed(0, errors.New("a error")).
		Failed(2, errors.New("c error"))

	require.Equal(t, 2, err.IndexedErrors(), "indexed errors did not match size of batch")
}

func TestBatchErrorIndexedBy(t *testing.T) {
	batch := MessageBatch{
		NewMessage([]byte("a")),
		NewMessage([]byte("b")),
		NewMessage([]byte("c")),
		NewMessage([]byte("d")),
	}

	indexer := batch.Index()

	// Scramble the batch
	batch[0], batch[1] = batch[1], batch[0]
	batch[1], batch[2] = batch[2], batch[1]
	batch[3] = NewMessage([]byte("e"))
	batch = append(batch, batch[2], batch[1], batch[0])

	batchError := errors.New("simulated error")
	err := NewBatchError(batch, batchError).
		Failed(0, errors.New("b error")).
		Failed(2, errors.New("a error")).
		Failed(3, errors.New("e error")).
		Failed(6, errors.New("b error"))

	type walkResult struct {
		i int
		c string
		e string
	}
	var results []walkResult
	err.WalkMessagesIndexedBy(indexer, func(i int, m *Message, err error) bool {
		bs, berr := m.AsBytes()
		require.NoErrorf(t, berr, "could not get bytes from message at %d", i)

		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		results = append(results, walkResult{
			i: i, c: string(bs), e: errStr,
		})
		return true
	})

	assert.Equal(t, []walkResult{
		{i: 1, c: "b", e: "b error"},
		{i: 2, c: "c", e: ""},
		{i: 0, c: "a", e: "a error"},
		{i: 0, c: "a", e: ""},
		{i: 2, c: "c", e: ""},
		{i: 1, c: "b", e: "b error"},
	}, results)
}

func TestBloblangErrorFuncs(t *testing.T) {
	resourceProcessorName := "foobar_resource"
	processorTemplateName := "foobar_template"
	tests := []struct {
		name      string
		label     string
		processor string
		expected  string
	}{
		{
			name:      "returns null when no error is set",
			label:     "foobar_label",
			processor: `mapping: root = this`,
			expected:  `{"error": null, "name": null, "label": null, "path": null}`,
		},
		{
			name:      "returns the label when set for a standard processor",
			label:     "foobar_label",
			processor: `mapping: root = throw("Kaboom!")`,
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "foobar_label", "path": "processors.0.try.1"}`,
		},
		{
			name:      "returns an empty label when not set for a standard processor",
			processor: `mapping: root = throw("Kaboom!")`,
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "", "path": "processors.0.try.1"}`,
		},
		{
			name:      "returns the label of a processor resource",
			label:     "foobar_label",
			processor: fmt.Sprintf("resource: %s", resourceProcessorName),
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "foobar_label", "path": "processor_resources.processors.1"}`,
		},
		{
			name:      "returns an empty label when not set for a processor resource",
			label:     "",
			processor: fmt.Sprintf("resource: %s", resourceProcessorName),
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "", "path": "processor_resources.processors.1"}`,
		},
		{
			name:      "returns the label set on a processor inside a processor template",
			label:     "foobar_label",
			processor: fmt.Sprintf(`%s: {}`, processorTemplateName),
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "foobar_label", "path": "processors.0.try.1.processors.1"}`,
		},
		{
			name:      "returns an empty label when not set for a processor template",
			label:     "",
			processor: fmt.Sprintf(`%s: {}`, processorTemplateName),
			expected:  `{"error":"failed assignment (line 1): Kaboom!", "name": "mapping", "label": "", "path": "processors.0.try.1.processors.1"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr, err := manager.New(manager.NewResourceConfig())
			require.NoError(t, err)

			// Register a resource processor
			resProcConf, err := docs.UnmarshalYAML([]byte(fmt.Sprintf(`
processors:
  - mapping: root.foo = "bar"
  - label: %s
    mapping: root = throw("Kaboom!")
`, test.label)))
			require.NoError(t, err)

			resProc, err := processor.FromAny(mgr, resProcConf)
			require.NoError(t, err)

			// TODO: Should `StoreProcessor()` use `resProc.Label` as the name instead of taking an extra parameter?
			err = mgr.StoreProcessor(context.Background(), resourceProcessorName, resProc)
			require.NoError(t, err)

			// Register a processor template
			err = template.RegisterTemplateYAML(mgr.Environment(), mgr.BloblEnvironment(), []byte(fmt.Sprintf(`
name: %s
type: processor

mapping: |
  root.processors = []
  root.processors."-".mapping = """root.foo = "bar" """
  root.processors."-" = {
      "label": @label,
      "mapping": """root = throw("Kaboom!")"""
    }
`, processorTemplateName)))
			require.NoError(t, err)

			// Configure a `processors` processor which exercises the `error_source_*` functions
			conf, err := docs.UnmarshalYAML([]byte(fmt.Sprintf(`
processors:
  # Use a try to make the path more interesting
  - try:
    - mapping: root.foo = "bar"
    - label: %s
      %s
  # Don't use a catch so we can exercise the functions even when there is no error
  - mapping: |
      root.error = error()
      root.name = error_source_name()
      root.label = error_source_label()
      root.path = error_source_path()
`, test.label, test.processor)))
			require.NoError(t, err)

			parsedConf, err := processor.FromAny(mgr, conf)
			require.NoError(t, err)

			proc, err := mgr.NewProcessor(parsedConf)
			require.NoError(t, err)

			outMsgs, err := proc.ProcessBatch(context.Background(), message.QuickBatch([][]byte{nil}))
			require.NoError(t, err)
			require.Len(t, outMsgs, 1)
			msg := outMsgs[0].Get(0)
			assert.JSONEq(t, test.expected, string(msg.AsBytes()))
		})
	}
}
