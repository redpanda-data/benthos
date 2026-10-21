// Copyright 2026 Redpanda Data, Inc.

package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func scrubTestEnv(t *testing.T) *service.Environment {
	t.Helper()

	env := service.NewEmptyEnvironment()
	spec := service.NewConfigSpec().
		Summary("SUMMARYDOC").
		Field(service.NewObjectField("obj",
			service.NewStringField("child").Description("CHILDDOC"),
		).Description("OBJDOC")).
		Field(service.NewStringAnnotatedEnumField("mode", map[string]string{"a": "OPTIONDOC"}).Description("MODEDOC"))
	require.NoError(t, env.RegisterInput("foo", spec,
		func(*service.ParsedConfig, *service.Resources) (service.Input, error) { return nil, nil }))

	bEnv := bloblang.NewEmptyEnvironment()
	require.NoError(t, bEnv.RegisterFunctionV2("probe",
		bloblang.NewPluginSpec().Description("FUNCDOC").Param(bloblang.NewStringParam("p").Description("PARAMDOC")),
		func(*bloblang.ParsedParams) (bloblang.Function, error) { return nil, nil }))
	env.UseBloblangEnvironment(bEnv)
	return env
}

// Minimise must scrub a copy of the docs. The specs it reads are shared with
// the environment's registry, so writing through them would blank the field,
// option and Bloblang parameter descriptions of every later schema built from
// the same environment.
func TestMinimiseDoesNotMutateEnvironment(t *testing.T) {
	env := scrubTestEnv(t)

	before, err := env.FullConfigSchema("v", "d").MarshalJSONV0()
	require.NoError(t, err)

	env.GenerateSchema("v", "d").Minimise()

	after, err := env.FullConfigSchema("v", "d").MarshalJSONV0()
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
	for _, doc := range []string{"SUMMARYDOC", "OBJDOC", "CHILDDOC", "MODEDOC", "OPTIONDOC", "FUNCDOC", "PARAMDOC"} {
		assert.Contains(t, string(after), doc)
	}
}

func TestMinimiseScrubsDocs(t *testing.T) {
	env := scrubTestEnv(t)

	cue, err := env.GenerateSchema("v", "d").Minimise().ToCUE()
	require.NoError(t, err)
	for _, doc := range []string{"SUMMARYDOC", "OBJDOC", "CHILDDOC", "MODEDOC", "OPTIONDOC"} {
		assert.NotContains(t, string(cue), doc)
	}

	full, err := env.GenerateSchema("v", "d").ToCUE()
	require.NoError(t, err)
	assert.Contains(t, string(full), "CHILDDOC", "the unminimised schema keeps its docs")
}
