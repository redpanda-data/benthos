// Copyright 2026 Redpanda Data, Inc.

package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// Rendering changes doc text, not the JSON shape: an absent list stays nil.
func TestRenderMarkdownKeepsNilLists(t *testing.T) {
	full := schema.Full{
		Inputs: []docs.ComponentSpec{{Name: "foo", Description: "A.", Config: docs.FieldComponent()}},
		BloblangMethods: []query.MethodSpec{{
			Name:       "m",
			Categories: []query.MethodCatSpec{{Category: "c", Description: "D."}},
		}},
		BloblangFunctions: []query.FunctionSpec{{Name: "f"}},
	}
	assert.Empty(t, full.RenderMarkdown(docs.MarkdownOptions{}))
	assert.Nil(t, full.Inputs[0].Examples)
	assert.Nil(t, full.Inputs[0].Config.Children)
	assert.Nil(t, full.BloblangMethods[0].Categories[0].Examples)
	assert.Nil(t, full.BloblangFunctions[0].Examples)
	assert.Nil(t, full.BloblangFunctions[0].Params.Definitions)
}
