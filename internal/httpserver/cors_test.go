// Copyright 2025 Redpanda Data, Inc.

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIEnableCORS(t *testing.T) {
	conf := NewServerCORSConfig()
	conf.Enabled = true
	conf.AllowedOrigins = []string{"*"}

	tmpHandler := http.NewServeMux()
	tmpHandler.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3"))
	})

	handler, err := conf.WrapHandler(tmpHandler)
	require.NoError(t, err)

	request, _ := http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Add("Origin", "meow")
	request.Header.Add("Access-Control-Request-Method", "POST")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
}

func TestAPIEnableCORSOrigins(t *testing.T) {
	conf := NewServerCORSConfig()
	conf.Enabled = true
	conf.AllowedOrigins = []string{"foo", "bar"}

	tmpHandler := http.NewServeMux()
	tmpHandler.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3"))
	})

	handler, err := conf.WrapHandler(tmpHandler)
	require.NoError(t, err)

	request, _ := http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Add("Origin", "foo")
	request.Header.Add("Access-Control-Request-Method", "POST")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "foo", response.Header().Get("Access-Control-Allow-Origin"))

	request, _ = http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Add("Origin", "bar")
	request.Header.Add("Access-Control-Request-Method", "POST")

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "bar", response.Header().Get("Access-Control-Allow-Origin"))

	request, _ = http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Add("Origin", "baz")
	request.Header.Add("Access-Control-Request-Method", "POST")

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
}

func TestAPIEnableCORSAllowedHeaders(t *testing.T) {
	conf := NewServerCORSConfig()
	conf.Enabled = true
	conf.AllowedOrigins = []string{"*"}
	conf.AllowedHeaders = []string{"Content-Type"}

	handler, err := conf.WrapHandler(http.NewServeMux())
	require.NoError(t, err)

	request, _ := http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Set("Origin", "meow")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "content-type")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "Content-Type", response.Header().Get("Access-Control-Allow-Headers"))
}

func TestAPIEnableCORSAllHeaders(t *testing.T) {
	conf := NewServerCORSConfig()
	conf.Enabled = true
	conf.AllowedOrigins = []string{"*"}
	conf.AllowedHeaders = []string{"*"}

	handler, err := conf.WrapHandler(http.NewServeMux())
	require.NoError(t, err)

	request, _ := http.NewRequest("OPTIONS", "/version", http.NoBody)
	request.Header.Set("Origin", "meow")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "content-type, x-request-id")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "content-type, x-request-id", response.Header().Get("Access-Control-Allow-Headers"))
}

func TestAPIEnableCORSNoHeaders(t *testing.T) {
	conf := NewServerCORSConfig()
	conf.Enabled = true

	_, err := conf.WrapHandler(http.NewServeMux())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must specify at least one allowed origin")
}
