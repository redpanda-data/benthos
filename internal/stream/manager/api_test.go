// Copyright 2025 Redpanda Data, Inc.

package manager_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	bmanager "github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/internal/stream/manager"

	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

func router(m *manager.Type) *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/ready", m.HandleStreamReady)
	router.HandleFunc("/streams", m.HandleStreamsCRUD)
	router.HandleFunc("/streams/{id}", m.HandleStreamCRUD)
	router.HandleFunc("/streams/{id}/stats", m.HandleStreamStats)
	router.HandleFunc("/resources/{type}/{id}", m.HandleResourceCRUD)
	return router
}

func genRequest(verb, url string, payload any) *http.Request {
	var body io.Reader

	if payload != nil {
		if s, ok := payload.(string); ok {
			body = strings.NewReader(s)
		} else {
			bodyBytes, err := json.Marshal(payload)
			if err != nil {
				panic(err)
			}
			body = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequest(verb, url, body)
	if err != nil {
		panic(err)
	}

	return req
}

func genYAMLRequest(verb, url string, payload any) *http.Request {
	var body io.Reader

	if payload != nil {
		if s, ok := payload.(string); ok {
			body = bytes.NewReader([]byte(s))
		} else {
			bodyBytes, err := yaml.Marshal(payload)
			if err != nil {
				panic(err)
			}
			body = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequest(verb, url, body)
	if err != nil {
		panic(err)
	}

	return req
}

type listItemBody struct {
	Active    bool    `json:"active"`
	Uptime    float64 `json:"uptime"`
	UptimeStr string  `json:"uptime_str"`
}

type listBody map[string]listItemBody

func parseListBody(data *bytes.Buffer) listBody {
	result := listBody{}
	if err := json.Unmarshal(data.Bytes(), &result); err != nil {
		panic(err)
	}
	return result
}

type getBody struct {
	Active    bool    `json:"active"`
	Uptime    float64 `json:"uptime"`
	UptimeStr string  `json:"uptime_str"`
	Config    any     `json:"config"`
}

func parseGetBody(t *testing.T, data *bytes.Buffer) getBody {
	t.Helper()
	result := getBody{}
	if err := yaml.Unmarshal(data.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

type endpointReg struct {
	endpoints map[string]http.HandlerFunc
}

func (f *endpointReg) RegisterEndpoint(path, desc string, h http.HandlerFunc) {
	f.endpoints[path] = h
}

func TestTypeAPIDisabled(t *testing.T) {
	r := &endpointReg{endpoints: map[string]http.HandlerFunc{}}
	rMgr, err := bmanager.New(bmanager.NewResourceConfig(), bmanager.OptSetAPIReg(r))
	require.NoError(t, err)

	_ = manager.New(rMgr,
		manager.OptAPIEnabled(true),
	)
	assert.Greater(t, len(r.endpoints), 1)

	r = &endpointReg{endpoints: map[string]http.HandlerFunc{}}
	rMgr, err = bmanager.New(bmanager.NewResourceConfig(), bmanager.OptSetAPIReg(r))
	require.NoError(t, err)

	_ = manager.New(rMgr,
		manager.OptAPIEnabled(false),
	)
	assert.Len(t, r.endpoints, 1)
	assert.Contains(t, r.endpoints, "/ready")
}

func TestTypeAPIBadMethods(t *testing.T) {
	mgr := manager.New(mock.NewManager())

	r := router(mgr)

	request := genRequest("DELETE", "/streams", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusBadRequest, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v", act, exp)
	}

	request = genRequest("DERP", "/streams/foo", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusBadRequest, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v", act, exp)
	}
}

func harmlessConf() any {
	return map[string]any{
		"input": map[string]any{
			"generate": map[string]any{
				"mapping": "root = deleted()",
			},
		},
		"output": map[string]any{
			"drop": map[string]any{},
		},
	}
}

func TestTypeAPIBasicOperations(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)
	conf := harmlessConf()

	request := genRequest("PUT", "/streams/foo", conf)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())

	request = genRequest("GET", "/streams/foo", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genRequest("POST", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	request = genRequest("POST", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)

	assert.Eventually(t, func() bool {
		request = genRequest("GET", "/ready", nil)
		response = httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response.Code == http.StatusOK
	}, time.Second*10, time.Millisecond*50)

	request = genRequest("GET", "/streams/bar", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genRequest("GET", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	info := parseGetBody(t, response.Body)
	assert.True(t, info.Active)

	assert.Equal(t, "root = deleted()", gabs.Wrap(info.Config).S("input", "generate", "mapping").Data())

	newConf := harmlessConf()
	_, _ = gabs.Wrap(newConf).Set("memory", "buffer", "type")

	request = genRequest("PUT", "/streams/foo", newConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	request = genRequest("GET", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	info = parseGetBody(t, response.Body)
	assert.True(t, info.Active)

	assert.Equal(t, "memory", gabs.Wrap(info.Config).S("buffer", "type").Data())

	request = genRequest("DELETE", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	request = genRequest("DELETE", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())

	testVar := "__TEST_INPUT_MAPPING"

	t.Setenv(testVar, `root.meow = 5`)

	request = genRequest("POST", "/streams/fooEnv?chilled=true", map[string]any{
		"input": map[string]any{
			"generate": map[string]any{
				"mapping": "${__TEST_INPUT_MAPPING}",
			},
		},
		"output": map[string]any{
			"type": "drop",
		},
	})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	request = genRequest("GET", "/streams/fooEnv", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	info = parseGetBody(t, response.Body)

	assert.True(t, info.Active)
	assert.Equal(t, `root.meow = 5`, gabs.Wrap(info.Config).S("input", "generate", "mapping").Data())

	request = genRequest("DELETE", "/streams/fooEnv", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

// TestTypeAPIStreamEnvOverrides covers the optional per-request "env" field
// accepted by POST/PUT /streams/{id}, which supplies template values for
// `${FOO}`-style placeholders in the rest of the config body.
// streamTemplate renders a minimal stream config as the string that travels
// inside an envelope body's `template` field. JSON is used because it is valid
// YAML, and because it saves hand-quoting mappings into a YAML scalar.
func streamTemplate(mapping string) string {
	b, err := json.Marshal(map[string]any{
		"input":  map[string]any{"generate": map[string]any{"mapping": mapping}},
		"output": map[string]any{"drop": map[string]any{}},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// envelopeRequest builds a request in the envelope form, carrying per-request
// env overrides alongside the config template. A nil env omits the field.
func envelopeRequest(verb, url string, env map[string]string, template string) *http.Request {
	body := map[string]any{"template": template}
	if env != nil {
		body["env"] = env
	}
	return genRequest(verb, url, body)
}

// TestTypeAPIStreamEnvOverrides covers the envelope request form accepted by
// POST/PUT /streams/{id}, which supplies template values for `${FOO}`-style
// placeholders in the config.
func TestTypeAPIStreamEnvOverrides(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)
	r := router(mgr)

	readMapping := func(t *testing.T, id string) any {
		t.Helper()
		request := genRequest("GET", "/streams/"+id, nil)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return gabs.Wrap(parseGetBody(t, response.Body).Config).S("input", "generate", "mapping").Data()
	}

	t.Run("the envelope alone supplies a var with no OS env var set", func(t *testing.T) {
		request := envelopeRequest("POST", "/streams/envoverride?chilled=true",
			map[string]string{"FOO": "root.meow = 5"}, streamTemplate("${FOO}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 5", readMapping(t, "envoverride"))
	})

	t.Run("an override takes precedence over an OS env var", func(t *testing.T) {
		t.Setenv("FOO", "root.meow = 99")

		request := envelopeRequest("POST", "/streams/envprecedence?chilled=true",
			map[string]string{"FOO": "root.meow = 5"}, streamTemplate("${FOO}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 5", readMapping(t, "envprecedence"))
	})

	// The original request form, unchanged: no envelope, so the body is the
	// config itself and every variable resolves from the OS as it always has.
	t.Run("a raw body falls back to the OS env var", func(t *testing.T) {
		t.Setenv("FOO", "root.meow = 99")

		request := genRequest("POST", "/streams/envfallback?chilled=true", map[string]any{
			"input":  map[string]any{"generate": map[string]any{"mapping": "${FOO}"}},
			"output": map[string]any{"drop": map[string]any{}},
		})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 99", readMapping(t, "envfallback"))
	})

	// A single config references two variables, one supplied by the envelope
	// and the other left to resolve from the real OS environment.
	t.Run("overrides and OS vars resolve together in one config", func(t *testing.T) {
		t.Setenv("MIXED_OS_ONLY", "root.woof = 7")

		request := envelopeRequest("POST", "/streams/envmixed?chilled=true",
			map[string]string{"MIXED_OVERRIDE": "root.meow = 5"},
			streamTemplate("${MIXED_OVERRIDE}\n${MIXED_OS_ONLY}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 5\nroot.woof = 7", readMapping(t, "envmixed"))
	})

	// An override present in the request must not disturb the
	// `${FOO:default}` fallback syntax for a different, entirely unset var.
	t.Run("a default still applies to an unrelated unset var", func(t *testing.T) {
		request := envelopeRequest("POST", "/streams/envmixeddefault?chilled=true",
			map[string]string{"MIXED_OVERRIDE": "root.meow = 5"},
			streamTemplate("${MIXED_OVERRIDE}\n${THIS_VAR_IS_ALSO_NOT_SET_24680:root.woof = 9}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 5\nroot.woof = 9", readMapping(t, "envmixeddefault"))
	})

	// A `${{FOO}}` escape must not be unescaped *and then* interpolated, which
	// would leak the real OS value of a variable the caller explicitly asked to
	// be left as literal text.
	t.Run("an escape is not unescaped and then interpolated", func(t *testing.T) {
		t.Setenv("MIXED_ESCAPED", "leaked-os-value")

		request := envelopeRequest("POST", "/streams/envmixedescape?chilled=true",
			map[string]string{"MIXED_OVERRIDE": "root.meow = 5"},
			streamTemplate(`root.v = "${{MIXED_ESCAPED}}"`))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, `root.v = "${MIXED_ESCAPED}"`, readMapping(t, "envmixedescape"))
	})

	// An override elsewhere in the same config doesn't grant a free pass for an
	// unrelated missing var.
	t.Run("an unrelated missing var still errors", func(t *testing.T) {
		request := envelopeRequest("POST", "/streams/envmixedmissing",
			map[string]string{"MIXED_OVERRIDE": "root.meow = 5"},
			streamTemplate("${MIXED_OVERRIDE}\n${THIS_VAR_IS_DEFINITELY_NOT_SET_67890}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "required environment variables were not set")
	})

	t.Run("a missing var in a raw body errors as it always did", func(t *testing.T) {
		request := genRequest("POST", "/streams/envmissing", map[string]any{
			"input": map[string]any{
				"generate": map[string]any{"mapping": "${THIS_VAR_IS_DEFINITELY_NOT_SET_12345}"},
			},
			"output": map[string]any{"drop": map[string]any{}},
		})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "required environment variables were not set")
	})

	// Mirrors the os.LookupEnv contract of returning only strings.
	t.Run("a non-string override is a 400", func(t *testing.T) {
		request := genRequest("POST", "/streams/envbadtype", map[string]any{
			"env":      map[string]any{"FOO": 3},
			"template": streamTemplate("${FOO}"),
		})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "value of 'FOO' must be a string")
	})

	t.Run("an env value that is not an object is a 400", func(t *testing.T) {
		request := genRequest("POST", "/streams/envbadshape", map[string]any{
			"env":      []string{"FOO=bar"},
			"template": streamTemplate("${FOO}"),
		})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "must be an object of string values")
	})

	// `env` is no longer a recognised sibling of the config fields: alongside
	// real config it is an unknown field like any other, and the name is free
	// for a genuine config section later.
	t.Run("an env field beside config fields is not an envelope", func(t *testing.T) {
		request := genRequest("POST", "/streams/envnotenvelope", map[string]any{
			"env":    map[string]any{"FOO": "root.meow = 5"},
			"input":  map[string]any{"generate": map[string]any{"mapping": "root.id = uuid_v4()"}},
			"output": map[string]any{"drop": map[string]any{}},
		})
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), "env")
	})

	t.Run("PUT accepts the envelope too", func(t *testing.T) {
		request := envelopeRequest("POST", "/streams/envput?chilled=true",
			map[string]string{"FOO": "root.meow = 1"}, streamTemplate("${FOO}"))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		request = envelopeRequest("PUT", "/streams/envput?chilled=true",
			map[string]string{"FOO": "root.meow = 2"}, streamTemplate("${FOO}"))
		response = httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, "root.meow = 2", readMapping(t, "envput"))
	})
}

// The streams API's request bodies are config *templates*: they are not
// necessarily valid YAML until after env var substitution has run. Carrying the
// document as a string means it is never parsed before substitution, so a
// template works with and without overrides alike.
func TestTypeAPIStreamEnvOverridesPreserveDocument(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)
	r := router(mgr)

	readMapping := func(t *testing.T, id string) any {
		t.Helper()
		request := genRequest("GET", "/streams/"+id, nil)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return gabs.Wrap(parseGetBody(t, response.Body).Config).S("input", "generate", "mapping").Data()
	}

	// The documented `${VAR: default}` form puts a `: ` inside what is
	// otherwise a plain scalar, so this template only parses as YAML once
	// substitution has happened. A block scalar carries it literally, while the
	// envelope supplies a *different* variable in the same request — the case
	// the body-field design could not serve.
	t.Run("accepts a template only valid after substitution", func(t *testing.T) {
		request := genYAMLRequest("POST", "/streams/envtemplateonly?chilled=true", `
env:
  STREAM_ENV_MAPPING: root.id = "x"
template: |
  input:
    generate:
      interval: 1h
      mapping: ${STREAM_ENV_MAPPING}
  output:
    file:
      path: ${STREAM_ENV_UNSET_PATH: ./fallback.jsonl}
`)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		request = genRequest("GET", "/streams/envtemplateonly", nil)
		response = httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		info := parseGetBody(t, response.Body)

		assert.Equal(t, `root.id = "x"`, gabs.Wrap(info.Config).S("input", "generate", "mapping").Data())
		assert.Equal(t, "./fallback.jsonl", gabs.Wrap(info.Config).S("output", "file", "path").Data())
	})

	// Quoting is a property of the YAML node, not of the decoded value, so
	// re-serialising the document from decoded values would drop the caller's
	// own quotes around an interpolation. Here the override value contains a
	// `: `, which parses as a nested mapping the moment those quotes are lost.
	t.Run("preserves quoting around interpolations", func(t *testing.T) {
		request := genYAMLRequest("POST", "/streams/envquoted?chilled=true", `
env:
  ENV_QUOTED_VALUE: 'a: b'
template: |
  input:
    generate:
      interval: 1h
      mapping: 'root.v = "${ENV_QUOTED_VALUE}"'
  output:
    drop: {}
`)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())

		assert.Equal(t, `root.v = "a: b"`, readMapping(t, "envquoted"))
	})
}

// A scalar the caller never referenced must not be re-resolved by YAML's
// implicit typing just because the request carries overrides.
func TestTypeAPIStreamEnvOverridesPreserveScalars(t *testing.T) {
	t.Chdir(t.TempDir())

	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)
	r := router(mgr)

	// `2024-01-02` is a valid YAML timestamp, so a decode/re-encode round trip
	// rewrites it as `2024-01-02T00:00:00Z` and the output writes to a
	// different file than the one the caller asked for.
	request := genYAMLRequest("POST", "/streams/envscalars?chilled=true", `
env:
  ENV_SCALAR_MAPPING: root.id = "x"
template: |
  input:
    generate:
      count: 1
      interval: 1ms
      mapping: ${ENV_SCALAR_MAPPING}
  output:
    file:
      path: 2024-01-02
`)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var names []string
	assert.Eventually(t, func() bool {
		entries, err := os.ReadDir(".")
		if err != nil {
			return false
		}
		names = names[:0]
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return len(names) > 0
	}, time.Second*5, time.Millisecond*10)
	assert.Equal(t, []string{"2024-01-02"}, names)
}

func TestTypeAPIPatch(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)
	conf := harmlessConf()

	request := genRequest("PATCH", "/streams/foo", conf)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusNotFound, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v", act, exp)
	}

	request = genRequest("POST", "/streams/foo?chilled=true", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	patchConf := map[string]any{
		"input": map[string]any{
			"generate": map[string]any{
				"interval": "2s",
			},
		},
	}
	request = genRequest("PATCH", "/streams/foo", patchConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusOK, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v: %v", act, exp, response.Body.String())
	}

	request = genRequest("GET", "/streams/foo", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusOK, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v: %v", act, exp, response.Body.String())
	}
	info := parseGetBody(t, response.Body)
	if !info.Active {
		t.Fatal("Stream not active")
	}

	assert.Equal(t, "2s", gabs.Wrap(info.Config).S("input", "generate", "interval").Data())
}

func TestTypeAPIBasicOperationsYAML(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)
	conf := harmlessConf()

	request := genYAMLRequest("PUT", "/streams/foo?chilled=true", conf)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genYAMLRequest("GET", "/streams/foo", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genYAMLRequest("POST", "/streams/foo?chilled=true", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	request = genYAMLRequest("POST", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)

	request = genYAMLRequest("GET", "/streams/bar", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genYAMLRequest("GET", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	info := parseGetBody(t, response.Body)
	require.True(t, info.Active)
	assert.Equal(t, "root = deleted()", gabs.Wrap(info.Config).S("input", "generate", "mapping").Data())

	newConf := harmlessConf()
	_, _ = gabs.Wrap(newConf).Set("memory", "buffer", "type")

	request = genYAMLRequest("PUT", "/streams/foo?chilled=true", newConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	request = genYAMLRequest("GET", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	info = parseGetBody(t, response.Body)
	require.True(t, info.Active)
	assert.Equal(t, "memory", gabs.Wrap(info.Config).S("buffer", "type").Data())

	request = genYAMLRequest("DELETE", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	request = genYAMLRequest("DELETE", "/streams/foo", conf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestTypeAPIList(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	request := genRequest("GET", "/streams", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusOK, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v", act, exp)
	}
	info := parseListBody(response.Body)
	if exp, act := (listBody{}), info; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong list response: %v != %v", act, exp)
	}

	conf, err := testutil.StreamFromYAML(`
input:
  generate:
    mapping: 'root = deleted()'
output:
  drop: {}
`)
	require.NoError(t, err)

	if err := mgr.Create("foo", conf); err != nil {
		t.Fatal(err)
	}

	request = genRequest("GET", "/streams", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if exp, act := http.StatusOK, response.Code; exp != act {
		t.Errorf("Unexpected result: %v != %v", act, exp)
	}
	info = parseListBody(response.Body)
	if exp, act := true, info["foo"].Active; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong list response: %v != %v", act, exp)
	}
}

func TestTypeAPISetStreams(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	origConf, err := testutil.StreamFromYAML(`
input:
  generate:
    mapping: 'root = deleted()'
output:
  drop: {}
`)
	require.NoError(t, err)

	require.NoError(t, mgr.Create("foo", origConf))
	require.NoError(t, mgr.Create("bar", origConf))

	request := genRequest("GET", "/streams", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	info := parseListBody(response.Body)
	assert.True(t, info["foo"].Active)
	assert.True(t, info["bar"].Active)

	barConf := harmlessConf()
	_, _ = gabs.Wrap(barConf).Set("root = this.BAR_ONE", "input", "generate", "mapping")
	bar2Conf := harmlessConf()
	_, _ = gabs.Wrap(bar2Conf).Set("root = this.BAR_TWO", "input", "generate", "mapping")
	bazConf := harmlessConf()
	_, _ = gabs.Wrap(bazConf).Set("root = this.BAZ_ONE", "input", "generate", "mapping")

	streamsBody := map[string]any{}
	streamsBody["bar"] = barConf
	streamsBody["bar2"] = bar2Conf
	streamsBody["baz"] = bazConf

	request = genRequest("POST", "/streams", streamsBody)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	request = genRequest("GET", "/streams", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	info = parseListBody(response.Body)
	assert.NotContains(t, info, "foo")
	assert.Contains(t, info, "bar")
	assert.Contains(t, info, "baz")

	request = genRequest("GET", "/streams/bar", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	conf := parseGetBody(t, response.Body)
	assert.Equal(t, "root = this.BAR_ONE", gabs.Wrap(conf.Config).S("input", "generate", "mapping").Data())

	request = genRequest("GET", "/streams/bar2", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	conf = parseGetBody(t, response.Body)
	assert.Equal(t, "root = this.BAR_TWO", gabs.Wrap(conf.Config).S("input", "generate", "mapping").Data())

	request = genRequest("GET", "/streams/baz", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())

	conf = parseGetBody(t, response.Body)
	assert.Equal(t, "root = this.BAZ_ONE", gabs.Wrap(conf.Config).S("input", "generate", "mapping").Data())
}

func testConfToAny(t testing.TB, conf any) any {
	var node yaml.Node
	err := node.Encode(conf)
	require.NoError(t, err)

	sanitConf := docs.NewSanitiseConfig(bundle.GlobalEnvironment)
	sanitConf.RemoveTypeField = true
	sanitConf.ScrubSecrets = true
	err = config.Spec().SanitiseYAML(&node, sanitConf)
	require.NoError(t, err)

	var v any
	require.NoError(t, node.Decode(&v))
	return v
}

func TestTypeAPIStreamsDefaultConf(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	body := []byte(`{
	"foo": {
		"input": {
			"generate": {
				"mapping": "root = deleted()"
			}
		},
		"output": {
			"drop": {}
		}
	}
}`)

	request, err := http.NewRequest("POST", "/streams", bytes.NewReader(body))
	require.NoError(t, err)

	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	status, err := mgr.Read("foo")
	require.NoError(t, err)

	v := testConfToAny(t, status.Config())

	assert.Nil(t, gabs.Wrap(v).S("input", "generate", "interval").Data())
}

func TestTypeAPIStreamsLinting(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	body := []byte(`{
	"foo": {
		"input": {
			"generate": {
				"mapping": "root = deleted()"
			}
		},
		"output": {
			"type":"drop",
			"inproc": "meow"
		}
	},
	"bar": {
		"input": {
			"generate": {
				"mapping": "root = deleted()"
			},
			"type": "inproc"
		},
		"output": {
			"drop": {}
		}
	}
}`)

	request, err := http.NewRequest("POST", "/streams", bytes.NewReader(body))
	require.NoError(t, err)

	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "application/json", response.Result().Header.Get("Content-Type"))

	expLints := []string{
		"stream 'foo': (10,1) field inproc is invalid when the component type is drop (output)",
		"stream 'bar': (15,1) field generate is invalid when the component type is inproc (input)",
	}
	var actLints struct {
		LintErrors []string `json:"lint_errors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &actLints))
	assert.ElementsMatch(t, expLints, actLints.LintErrors)

	request, err = http.NewRequest("POST", "/streams?chilled=true", bytes.NewReader(body))
	require.NoError(t, err)

	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestTypeAPIDefaultConf(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	body := []byte(`{
	"input": {
		"generate": {
			"mapping": "root = deleted()"
		}
	},
	"output": {
		"drop": {}
	}
}`)

	request, err := http.NewRequest("POST", "/streams/foo", bytes.NewReader(body))
	require.NoError(t, err)

	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	status, err := mgr.Read("foo")
	require.NoError(t, err)

	v := testConfToAny(t, status.Config())
	assert.Nil(t, gabs.Wrap(v).S("input", "generate", "interval").Data())
}

func TestTypeAPILinting(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	body := []byte(`{
	"input": {
		"generate": {
			"mapping": "root = deleted()"
		}
	},
	"output": {
		"type":"drop",
		"inproc": "meow"
	},
	"cache_resources": [
		{"label":"not_interested","memory":{}}
	]
}`)

	request, err := http.NewRequest("POST", "/streams/foo", bytes.NewReader(body))
	require.NoError(t, err)

	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "application/json", response.Result().Header.Get("Content-Type"))

	expLints := `{"lint_errors":["(9,1) field inproc is invalid when the component type is drop (output)","(11,1) field cache_resources not recognised"]}`
	assert.Equal(t, expLints, response.Body.String())

	request, err = http.NewRequest("POST", "/streams/foo?chilled=true", bytes.NewReader(body))
	require.NoError(t, err)

	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestResourceAPILinting(t *testing.T) {
	tests := []struct {
		name   string
		ctype  string
		config string
		lints  []string
	}{
		{
			name:  "cache bad",
			ctype: "cache",
			config: `memory:
  default_ttl: 123s
  nope: nah
  compaction_interval: 1s`,
			lints: []string{
				"(3,1) field nope not recognised",
			},
		},
		{
			name:  "input bad",
			ctype: "input",
			config: `generate:
  mapping: root = deleted()
  nope: nah`,
			lints: []string{
				"(3,1) field nope not recognised",
			},
		},
		{
			name:  "output bad",
			ctype: "output",
			config: `retry:
  output:
    drop: {}
  nope: nah`,
			lints: []string{
				"(4,1) field nope not recognised",
			},
		},
		{
			name:  "processor bad",
			ctype: "processor",
			config: `split:
  size: 10
  nope: nah`,
			lints: []string{
				"(3,1) field nope not recognised",
			},
		},
		{
			name:  "rate limit bad",
			ctype: "rate_limit",
			config: `local:
  count: 10
  nope: nah`,
			lints: []string{
				"(3,1) field nope not recognised",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bmgr, err := bmanager.New(bmanager.NewResourceConfig())
			require.NoError(t, err)

			mgr := manager.New(bmgr)

			r := router(mgr)

			url := fmt.Sprintf("/resources/%v/foo", test.ctype)
			body := []byte(test.config)

			request, err := http.NewRequest("POST", url, bytes.NewReader(body))
			require.NoError(t, err)

			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, "application/json", response.Result().Header.Get("Content-Type"))

			expLints, err := json.Marshal(struct {
				LintErrors []string `json:"lint_errors"`
			}{
				LintErrors: test.lints,
			})
			require.NoError(t, err)

			assert.Equal(t, string(expLints), response.Body.String())

			request, err = http.NewRequest("POST", url+"?chilled=true", bytes.NewReader(body))
			require.NoError(t, err)

			response = httptest.NewRecorder()
			r.ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		})
	}
}

func TestTypeAPIGetStats(t *testing.T) {
	mgr, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	smgr := manager.New(mgr)

	r := router(smgr)

	origConf, err := testutil.StreamFromYAML(`
input:
  generate:
    mapping: 'root = deleted()'
output:
  drop: {}
`)
	require.NoError(t, err)

	err = smgr.Create("foo", origConf)
	require.NoError(t, err)

	<-time.After(time.Millisecond * 100)

	request := genRequest("GET", "/streams/not_exist/stats", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)

	request = genRequest("POST", "/streams/foo/stats", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)

	request = genRequest("GET", "/streams/foo/stats", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	stats, err := gabs.ParseJSON(response.Body.Bytes())
	require.NoError(t, err)

	assert.NotEmpty(t, stats.ChildrenMap(), response.Body.String())
}

func TestTypeAPISetResources(t *testing.T) {
	bmgr, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	tChan := make(chan message.Transaction)
	bmgr.SetPipe("feed_in", tChan)

	mgr := manager.New(bmgr)

	tmpDir := t.TempDir()

	dir1 := filepath.Join(tmpDir, "dir1")
	require.NoError(t, os.MkdirAll(dir1, 0o750))

	dir2 := filepath.Join(tmpDir, "dir2")
	require.NoError(t, os.MkdirAll(dir2, 0o750))

	r := router(mgr)

	request := genYAMLRequest("POST", "/resources/cache/foocache?chilled=true", fmt.Sprintf(`
file:
  directory: %v
`, dir1))
	hResponse := httptest.NewRecorder()
	r.ServeHTTP(hResponse, request)
	assert.Equal(t, http.StatusOK, hResponse.Code, hResponse.Body.String())

	streamConf, err := testutil.StreamFromYAML(`
input:
  inproc: feed_in
output:
  cache:
    key: '${! json("id") }'
    target: foocache
`)
	require.NoError(t, err)

	request = genYAMLRequest("POST", "/streams/foo?chilled=true", streamConf)
	hResponse = httptest.NewRecorder()
	r.ServeHTTP(hResponse, request)
	assert.Equal(t, http.StatusOK, hResponse.Code, hResponse.Body.String())

	resChan := make(chan error)
	select {
	case tChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(`{"id":"first","content":"hello world"}`)}), resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}
	select {
	case <-resChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	request = genYAMLRequest("POST", "/resources/cache/foocache?chilled=true", fmt.Sprintf(`
file:
  directory: %v
`, dir2))
	hResponse = httptest.NewRecorder()
	r.ServeHTTP(hResponse, request)
	assert.Equal(t, http.StatusOK, hResponse.Code, hResponse.Body.String())

	select {
	case tChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(`{"id":"second","content":"hello world 2"}`)}), resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}
	select {
	case <-resChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	files, err := os.ReadDir(dir1)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	file1Bytes, err := os.ReadFile(filepath.Join(dir1, "first"))
	require.NoError(t, err)
	assert.Equal(t, `{"id":"first","content":"hello world"}`, string(file1Bytes))

	files, err = os.ReadDir(dir2)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	file2Bytes, err := os.ReadFile(filepath.Join(dir2, "second"))
	require.NoError(t, err)
	assert.Equal(t, `{"id":"second","content":"hello world 2"}`, string(file2Bytes))
}

// TestResourceAPIEnvOverrides covers the same per-request "env" field for
// POST /resources/{type}/{id}, reusing the same extraction/lookup seam as
// the streams endpoint.
func TestResourceAPIEnvOverrides(t *testing.T) {
	bmgr, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	tChan := make(chan message.Transaction)
	bmgr.SetPipe("feed_in_env", tChan)

	mgr := manager.New(bmgr)
	r := router(mgr)

	tmpDir := t.TempDir()
	overrideDir := filepath.Join(tmpDir, "override")
	require.NoError(t, os.MkdirAll(overrideDir, 0o750))
	osDir := filepath.Join(tmpDir, "os")
	require.NoError(t, os.MkdirAll(osDir, 0o750))

	// Precedence: an OS env var of the same name points elsewhere, the
	// request-supplied "env" override must be the one that's used.
	t.Setenv("ENV_OVERRIDE_CACHE_DIR", osDir)

	request := genRequest("POST", "/resources/cache/envcache?chilled=true", map[string]any{
		"env": map[string]any{
			"ENV_OVERRIDE_CACHE_DIR": overrideDir,
		},
		"file": map[string]any{
			"directory": "${ENV_OVERRIDE_CACHE_DIR}",
		},
	})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	streamConf, err := testutil.StreamFromYAML(`
input:
  inproc: feed_in_env
output:
  cache:
    key: '${! json("id") }'
    target: envcache
`)
	require.NoError(t, err)

	request = genYAMLRequest("POST", "/streams/envcacheuser?chilled=true", streamConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	resChan := make(chan error)
	select {
	case tChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(`{"id":"first","content":"hello world"}`)}), resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}
	select {
	case <-resChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	files, err := os.ReadDir(overrideDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	files, err = os.ReadDir(osDir)
	require.NoError(t, err)
	assert.Empty(t, files)

	// Mixed: a single field template references two variables, one supplied
	// by the request "env" override and the other left to resolve from the
	// real OS environment, in the same request — both must resolve and
	// combine correctly.
	mixedSubDir := "mixedsub"
	mixedDir := filepath.Join(overrideDir, mixedSubDir)
	require.NoError(t, os.MkdirAll(mixedDir, 0o750))

	t.Setenv("ENV_MIXED_SUB_DIR", mixedSubDir)

	request = genRequest("POST", "/resources/cache/envmixedcache?chilled=true", map[string]any{
		"env": map[string]any{
			"ENV_MIXED_BASE_DIR": overrideDir,
		},
		"file": map[string]any{
			"directory": "${ENV_MIXED_BASE_DIR}/${ENV_MIXED_SUB_DIR}",
		},
	})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	// A dedicated pipe/channel, distinct from feed_in_env above: the
	// envcacheuser stream created earlier is still running and would race
	// this one for messages if they shared a pipe.
	mixedTChan := make(chan message.Transaction)
	bmgr.SetPipe("feed_in_env_mixed", mixedTChan)

	streamConf, err = testutil.StreamFromYAML(`
input:
  inproc: feed_in_env_mixed
output:
  cache:
    key: '${! json("id") }'
    target: envmixedcache
`)
	require.NoError(t, err)

	request = genYAMLRequest("POST", "/streams/envmixedcacheuser?chilled=true", streamConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	select {
	case mixedTChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(`{"id":"mixed","content":"hello world"}`)}), resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}
	select {
	case <-resChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	files, err = os.ReadDir(mixedDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	// Fallback: no "env" field in the request at all, the referenced variable
	// resolves from the real OS environment exactly as it did before "env"
	// existed.
	fallbackDir := filepath.Join(tmpDir, "fallback")
	require.NoError(t, os.MkdirAll(fallbackDir, 0o750))

	t.Setenv("ENV_FALLBACK_CACHE_DIR", fallbackDir)

	request = genRequest("POST", "/resources/cache/envfallbackcache?chilled=true", map[string]any{
		"file": map[string]any{
			"directory": "${ENV_FALLBACK_CACHE_DIR}",
		},
	})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	fallbackTChan := make(chan message.Transaction)
	bmgr.SetPipe("feed_in_env_fallback", fallbackTChan)

	streamConf, err = testutil.StreamFromYAML(`
input:
  inproc: feed_in_env_fallback
output:
  cache:
    key: '${! json("id") }'
    target: envfallbackcache
`)
	require.NoError(t, err)

	request = genYAMLRequest("POST", "/streams/envfallbackcacheuser?chilled=true", streamConf)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	select {
	case fallbackTChan <- message.NewTransaction(message.QuickBatch([][]byte{[]byte(`{"id":"fallback","content":"hello world"}`)}), resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}
	select {
	case <-resChan:
	case <-time.After(time.Second * 5):
		t.Fatal("timed out")
	}

	files, err = os.ReadDir(fallbackDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)

	// Missing var: neither the request "env" nor the OS environment provides
	// the referenced variable, this still errors exactly as it did before
	// "env" existed.
	request = genRequest("POST", "/resources/cache/envcachemissing", map[string]any{
		"env": map[string]any{
			"ENV_UNRELATED_VAR": "unused",
		},
		"file": map[string]any{
			"directory": "${ENV_MISSING_CACHE_DIR}",
		},
	})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "required environment variables were not set")

	// Regression: a body with no "env" field that is only valid YAML once
	// substitution has run must be accepted, as it was before "env" existed.
	request = genYAMLRequest("POST", "/resources/cache/envcachetemplateonly?chilled=true", `
file:
  directory: ${ENV_UNSET_CACHE_DIR: `+fallbackDir+`}
`)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	// Non-string "env" value: 400, same contract as the streams endpoint.
	request = genRequest("POST", "/resources/cache/envcachebad", map[string]any{
		"env": map[string]any{
			"ENV_OVERRIDE_CACHE_DIR": 3,
		},
		"file": map[string]any{
			"directory": "${ENV_OVERRIDE_CACHE_DIR}",
		},
	})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

func TestAPIReady(t *testing.T) {
	res, err := bmanager.New(bmanager.NewResourceConfig())
	require.NoError(t, err)

	mgr := manager.New(res)

	r := router(mgr)

	request := genRequest("POST", "/streams/foo", `
input:
  generate:
    count: 1
    mapping: 'root = {}'
    interval: ""

output:
  drop: {}
`)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	assert.Eventually(t, func() bool {
		request = genRequest("GET", "/ready", nil)
		response = httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response.Code == http.StatusOK
	}, time.Second*10, time.Millisecond*50)

	request = genRequest("POST", "/streams/bar", `
input:
  generate:
    count: 1
    mapping: 'root = {}'
    interval: ""

output:
  websocket:
    url: not**a**valid**url
`)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)

	assert.Eventually(t, func() bool {
		request = genRequest("GET", "/ready", nil)
		response = httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response.Code == http.StatusServiceUnavailable
	}, time.Second*10, time.Millisecond*50)
}
