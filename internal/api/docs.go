// Copyright 2025 Redpanda Data, Inc.

package api

import (
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/httpserver"
)

// Spec returns a field spec for the API configuration fields.
func Spec() docs.FieldSpecs {
	return docs.FieldSpecs{
		docs.FieldBool(fieldEnabled, "Whether to enable to HTTP server.").HasDefault(true),
		docs.FieldString(fieldAddress, "The address to bind to.").HasDefault("0.0.0.0:4195"),
		docs.FieldString(
			fieldRootPath, "Specifies a general prefix for all endpoints, this can help isolate the service endpoints when using a reverse proxy with other shared services. All endpoints will still be registered at the root as well as behind the prefix, e.g. with a root_path set to `/foo` the endpoint `/version` will be accessible from both `/version` and `/foo/version`.",
		).HasDefault("/benthos"),
		docs.FieldBool(
			fieldDebugEndpoints, "Whether to register a few extra endpoints that can be useful for debugging performance or behavioral problems.",
		).HasDefault(false),
		docs.FieldString(fieldCertFile, "An optional certificate file for enabling TLS.").Advanced().HasDefault(""),
		docs.FieldString(fieldKeyFile, "An optional key file for enabling TLS.").Advanced().HasDefault(""),
		httpserver.ServerCORSFieldSpec(),
		httpserver.BasicAuthFieldSpec(),
	}
}

// EndpointCaveats is a documentation section for HTTP components that explains
// some of the caveats in registering endpoints due to their non-deterministic
// ordering and lack of explicit path terminators.
func EndpointCaveats() string {
	return `
[CAUTION]
.Endpoint caveats
====
Components within a Redpanda Connect config will register their respective endpoints in a non-deterministic order. This means that establishing precedence of endpoints that are registered via multiple ` + "`http_server`" + ` inputs or outputs (either within brokers or from cohabiting streams) is not possible in a predictable way.

This ambiguity makes it difficult to ensure that paths which are both a subset of a path registered by a separate component, and end in a slash (` + "`/`" + `) and will therefore match against all extensions of that path, do not prevent the more specific path from matching against requests.

It is therefore recommended that you ensure paths of separate components do not collide unless they are explicitly non-competing.

For example, if you were to deploy two separate ` + "`http_server`" + ` inputs, one with a path ` + "`/foo/`" + ` and the other with a path ` + "`/foo/bar`" + `, it would not be possible to ensure that the path ` + "`/foo/`" + ` does not swallow requests made to ` + "`/foo/bar`" + `.
====`
}
