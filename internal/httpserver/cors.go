// Copyright 2025 Redpanda Data, Inc.

package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/handlers"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

const (
	fieldCORS               = "cors"
	fieldCORSEnabled        = "enabled"
	fieldCORSAllowedOrigins = "allowed_origins"
	fieldCORSAllowedHeaders = "allowed_headers"
)

// CORSConfig contains struct configuration for allowing CORS headers.
type CORSConfig struct {
	Enabled        bool     `json:"enabled" yaml:"enabled"`
	AllowedOrigins []string `json:"allowed_origins" yaml:"allowed_origins"`
	AllowedHeaders []string `json:"allowed_headers" yaml:"allowed_headers"`
}

// NewServerCORSConfig returns a new server CORS config with default fields.
func NewServerCORSConfig() CORSConfig {
	return CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{},
		AllowedHeaders: []string{},
	}
}

// WrapHandler wraps a provided HTTP handler with middleware that enables CORS
// requests (when configured).
func (conf CORSConfig) WrapHandler(handler http.Handler) (http.Handler, error) {
	if !conf.Enabled {
		return handler, nil
	}
	if len(conf.AllowedOrigins) == 0 {
		return nil, errors.New("must specify at least one allowed origin")
	}
	corsHandler := handlers.CORS(
		handlers.AllowedOrigins(conf.AllowedOrigins),
		handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}),
		handlers.AllowedHeaders(conf.AllowedHeaders),
	)(handler)

	if !conf.allowsAllHeaders() {
		return corsHandler, nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedHeaders := r.Header.Get("Access-Control-Request-Headers")
		if r.Method != http.MethodOptions || requestedHeaders == "" {
			corsHandler.ServeHTTP(w, r)
			return
		}

		request := r.Clone(r.Context())
		request.Header = r.Header.Clone()
		request.Header.Del("Access-Control-Request-Headers")
		w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
		corsHandler.ServeHTTP(w, request)
	}), nil
}

func (conf CORSConfig) allowsAllHeaders() bool {
	for _, header := range conf.AllowedHeaders {
		if strings.TrimSpace(header) == "*" {
			return true
		}
	}
	return false
}

// ServerCORSFieldSpec returns a field spec for an http server CORS component.
func ServerCORSFieldSpec() docs.FieldSpec {
	return docs.FieldObject(fieldCORS, "Adds Cross-Origin Resource Sharing headers.").WithChildren(
		docs.FieldBool(fieldCORSEnabled, "Whether to allow CORS requests.").HasDefault(false),
		docs.FieldString(fieldCORSAllowedOrigins, "An explicit list of origins that are allowed for CORS requests.").Array().HasDefault([]any{}),
		docs.FieldString(fieldCORSAllowedHeaders, "An explicit list of headers allowed in CORS requests. Specify `*` to allow any requested header.").Array().HasDefault([]any{}),
	).AtVersion("3.63.0").Advanced()
}

// CORSConfigFromParsed extracts the CORS fields from the parsed config and returns a CORS config.
func CORSConfigFromParsed(pConf *docs.ParsedConfig) (conf CORSConfig, err error) {
	pConf = pConf.Namespace(fieldCORS)
	if conf.Enabled, err = pConf.FieldBool(fieldCORSEnabled); err != nil {
		return
	}
	if conf.AllowedOrigins, err = pConf.FieldStringList(fieldCORSAllowedOrigins); err != nil {
		return
	}
	if conf.AllowedHeaders, err = pConf.FieldStringList(fieldCORSAllowedHeaders); err != nil {
		return
	}
	return
}
