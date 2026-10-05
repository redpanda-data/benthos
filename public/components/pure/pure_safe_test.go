// Copyright 2025 Redpanda Data, Inc.

package pure_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// The components imported by this package are the ones that don't interact
// with the host or external systems, which makes them the base of every build,
// including sandboxed builds that only allow pure Bloblang functions and
// methods (bloblang.GlobalEnvironment().OnlyPure()), such as Redpanda Cloud.
//
// This test binary imports nothing but this package, so the registered
// components and Bloblang plugins are exactly that set and no list of names is
// needed. The tests below check that the documented defaults and examples of
// that set work in a pure-only environment, so a reader who copies them into a
// sandboxed build doesn't hit errors such as "unrecognised function 'count'".

func pureEnv() *bloblang.Environment {
	return bloblang.GlobalEnvironment().OnlyPure()
}

func allPureComponentSpecs() []docs.ComponentSpec {
	var specs []docs.ComponentSpec
	specs = append(specs, bundle.AllBuffers.Docs()...)
	specs = append(specs, bundle.AllCaches.Docs()...)
	specs = append(specs, bundle.AllInputs.Docs()...)
	specs = append(specs, bundle.AllMetrics.Docs()...)
	specs = append(specs, bundle.AllOutputs.Docs()...)
	specs = append(specs, bundle.AllProcessors.Docs()...)
	specs = append(specs, bundle.AllRateLimits.Docs()...)
	specs = append(specs, bundle.AllScanners.Docs()...)
	specs = append(specs, bundle.AllTracers.Docs()...)
	return specs
}

// checkPureField parses the default and example values of an interpolated or
// Bloblang field, and of all of its children, with the pure environment.
func checkPureField(t *testing.T, env *bloblang.Environment, path string, f docs.FieldSpec) {
	t.Helper()

	path += "." + f.Name
	check := func(what string, v any) {
		str, ok := v.(string)
		if !ok || str == "" {
			return
		}
		var err error
		switch {
		case f.Interpolated:
			err = env.CheckInterpolatedString(str)
		case f.Bloblang:
			_, err = env.Parse(str)
		default:
			return
		}
		assert.NoError(t, err, "%v %v: %q", path, what, str)
	}

	if f.Default != nil {
		check("default", *f.Default)
	}
	for i, e := range f.Examples {
		check(fmt.Sprintf("example %v", i), e)
	}
	for _, c := range f.Children {
		checkPureField(t, env, path, c)
	}
}

func TestPureComponentFieldsUsePureBloblang(t *testing.T) {
	env := pureEnv()

	specs := allPureComponentSpecs()
	require.NotEmpty(t, specs)

	for _, spec := range specs {
		t.Run(fmt.Sprintf("%v/%v", spec.Type, spec.Name), func(t *testing.T) {
			for _, f := range spec.Config.Children {
				checkPureField(t, env, spec.Name, f)
			}
		})
	}
}

func TestPureComponentExamplesUsePureBloblang(t *testing.T) {
	lConf := docs.NewLintConfig(bundle.GlobalEnvironment)
	lConf.BloblangEnv = pureEnv()

	for _, spec := range allPureComponentSpecs() {
		for i, e := range spec.Examples {
			t.Run(fmt.Sprintf("%v/%v/%v", spec.Type, spec.Name, i), func(t *testing.T) {
				lints, err := config.LintYAMLBytes(lConf, []byte(e.Config))
				require.NoError(t, err)

				// Examples can reference components that this build doesn't
				// include, so only Bloblang lints are relevant here.
				for _, l := range lints {
					if l.Type == docs.LintBadBloblang {
						t.Errorf("example %q: %v", e.Title, l.Error())
					}
				}
			})
		}
	}
}

// impureCallPattern matches a call to any Bloblang function or method that the
// global environment has but the pure environment doesn't, so the set follows
// whatever is marked impure and no names are listed here.
func impureCallPattern(t *testing.T) *regexp.Regexp {
	t.Helper()

	pure := pureEnv()
	pureFns, pureMethods := map[string]bool{}, map[string]bool{}
	pure.WalkFunctions(func(name string, _ *bloblang.FunctionView) { pureFns[name] = true })
	pure.WalkMethods(func(name string, _ *bloblang.MethodView) { pureMethods[name] = true })

	var alts []string
	bloblang.GlobalEnvironment().WalkFunctions(func(name string, _ *bloblang.FunctionView) {
		if !pureFns[name] {
			alts = append(alts, `(?:^|[^\w.])`+regexp.QuoteMeta(name)+`\(`)
		}
	})
	bloblang.GlobalEnvironment().WalkMethods(func(name string, _ *bloblang.MethodView) {
		if !pureMethods[name] {
			alts = append(alts, `\.`+regexp.QuoteMeta(name)+`\(`)
		}
	})
	require.NotEmpty(t, alts, "expected at least one impure function, such as count")

	return regexp.MustCompile(strings.Join(alts, "|"))
}

func checkPureProse(t *testing.T, re *regexp.Regexp, where, text string) {
	t.Helper()
	if m := re.FindString(text); m != "" {
		t.Errorf("%v calls an impure Bloblang function or method: %q", where, m)
	}
}

func checkPureFieldProse(t *testing.T, re *regexp.Regexp, path string, f docs.FieldSpec) {
	t.Helper()
	path += "." + f.Name
	checkPureProse(t, re, path+" description", f.Description)
	for _, c := range f.Children {
		checkPureFieldProse(t, re, path, c)
	}
}

func TestPureComponentProseUsesPureBloblang(t *testing.T) {
	re := impureCallPattern(t)

	for _, spec := range allPureComponentSpecs() {
		t.Run(fmt.Sprintf("%v/%v", spec.Type, spec.Name), func(t *testing.T) {
			checkPureProse(t, re, "summary", spec.Summary)
			checkPureProse(t, re, "description", spec.Description)
			checkPureProse(t, re, "footnotes", spec.Footnotes)
			for _, e := range spec.Examples {
				checkPureProse(t, re, fmt.Sprintf("example %q summary", e.Title), e.Summary)
			}
			for _, f := range spec.Config.Children {
				checkPureFieldProse(t, re, spec.Name, f)
			}
		})
	}
}

func TestPureBloblangExamplesUsePureBloblang(t *testing.T) {
	env := pureEnv()

	env.WalkFunctions(func(name string, view *bloblang.FunctionView) {
		for i, e := range view.TemplateData().Examples {
			_, err := env.Parse(e.Mapping)
			assert.NoError(t, err, "function %v example %v: %q", name, i, e.Mapping)
		}
	})

	env.WalkMethods(func(name string, view *bloblang.MethodView) {
		data := view.TemplateData()
		for i, e := range data.Examples {
			_, err := env.Parse(e.Mapping)
			assert.NoError(t, err, "method %v example %v: %q", name, i, e.Mapping)
		}
		for _, c := range data.Categories {
			for i, e := range c.Examples {
				_, err := env.Parse(e.Mapping)
				assert.NoError(t, err, "method %v category %v example %v: %q", name, c.Category, i, e.Mapping)
			}
		}
	})
}

// The read_until example that consumes N messages uses counter() in place of
// the impure count(). This runs it in a pure-only environment to show that it
// still stops after N messages.
func TestReadUntilCounterExampleStopsAfterN(t *testing.T) {
	env := service.NewEnvironment()
	env.UseBloblangEnvironment(pureEnv())

	builder := env.NewStreamBuilder()
	require.NoError(t, builder.SetYAML(`
input:
  read_until:
    check: counter() >= 5
    input:
      generate:
        mapping: 'root = "hello"'
        interval: ""
output:
  drop: {}
logger:
  level: none
`))

	var mut sync.Mutex
	var received int
	require.NoError(t, builder.AddConsumerFunc(func(_ context.Context, _ *service.Message) error {
		mut.Lock()
		received++
		mut.Unlock()
		return nil
	}))

	strm, err := builder.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	require.NoError(t, strm.Run(ctx))

	mut.Lock()
	defer mut.Unlock()
	assert.Equal(t, 5, received)
}
