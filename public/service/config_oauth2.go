// Copyright 2026 Redpanda Data, Inc.

package service

// NewOAuth2Field defines a new object type config field that configures OAuth
// 2.0 authentication with the client credentials token flow, as used by the
// HTTP components of this module. The object is optional and advanced, and
// contains the fields `enabled` (default `false`), `client_key`,
// `client_secret`, `token_url`, `scopes` and `endpoint_params`.
//
// This package does not parse the field into a client, so plugins read its
// child fields by those names.
func NewOAuth2Field(name string) *ConfigField {
	return NewObjectField(name,
		NewBoolField("enabled").
			Description("Whether to use OAuth version 2 in requests.").
			Default(false),

		NewStringField("client_key").
			Description("A value used to identify the client to the token provider.").
			Default(""),

		NewStringField("client_secret").
			Description("The secret used to establish ownership of the client key.").
			Default("").Secret(),

		NewURLField("token_url").
			Description("The URL of the token provider.").
			Default(""),

		NewStringListField("scopes").
			Description("A list of requested permissions (optional).").
			Default([]any{}).
			Advanced().
			Version("3.45.0"),

		NewAnyMapField("endpoint_params").
			Description("A map of optional parameters to send to the token provider with client credentials token requests. Each value must be an array of strings.").
			Advanced().
			Example(map[string]any{
				"audience": []string{"https://example.com"},
				"resource": []string{"https://api.example.com"},
			}).
			Default(map[string]any{}).
			Version("4.21.0").
			Optional().
			LintRule(`
root = if this.type() == "object" {
  this.values().map_each(ele -> if ele.type() != "array" {
    "field must be an object containing arrays of strings, got %s (%v)".format(ele.format_json(no_indent: true), ele.type())
  } else {
    ele.map_each(str -> if str.type() != "string" {
      "field values must be strings, got %s (%v)".format(str.format_json(no_indent: true), str.type())
    } else { deleted() })
  }).
    flatten()
}
`),
	).
		Description("Configures authentication with OAuth version 2 using the client credentials token flow. To exchange a refresh token instead, add a `grant_type` parameter with the value `refresh_token` and a `refresh_token` parameter with the token to `endpoint_params`.").
		Optional().Advanced()
}
