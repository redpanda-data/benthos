// Copyright 2025 Redpanda Data, Inc.

package httpclient

import (
	"context"
	"crypto/tls"
	"io/fs"
	"net/http"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	hcFieldURL                 = "url"
	hcFieldVerb                = "verb"
	hcFieldHeaders             = "headers"
	hcFieldMetadata            = "metadata"
	hcFieldExtractHeaders      = "extract_headers"
	hcFieldRateLimit           = "rate_limit"
	hcFieldTimeout             = "timeout"
	hcFieldRetryPeriod         = "retry_period"
	hcFieldMaxRetryBackoff     = "max_retry_backoff"
	hcFieldRetries             = "retries"
	hcFieldFollowRedirects     = "follow_redirects"
	hcFieldBackoffOn           = "backoff_on"
	hcFieldDropOn              = "drop_on"
	hcFieldSuccessfulOn        = "successful_on"
	hcFieldDumpRequestLogLevel = "dump_request_log_level"
	hcFieldTLS                 = "tls"
	hcFieldProxyURL            = "proxy_url"
	hcFieldDisableHTTP2        = "disable_http2"
)

// ConfigField returns a public API config field spec for an HTTP component,
// with optional extra fields added to the end.
func ConfigField(defaultVerb string, forOutput bool, extraChildren ...*service.ConfigField) *service.ConfigField {
	innerFields := []*service.ConfigField{
		service.NewInterpolatedStringField(hcFieldURL).
			Description("The URL to connect to."),
		service.NewStringField(hcFieldVerb).
			Description("A verb to connect with.").
			Examples("POST", "GET", "DELETE").
			Default(defaultVerb),
		service.NewInterpolatedStringMapField(hcFieldHeaders).
			Description("A map of headers to add to the request.").
			Example(map[string]any{
				"Content-Type": "application/octet-stream",
				"traceparent":  `${! tracing_span().traceparent }`,
			}).
			Default(map[string]any{}),
		service.NewMetadataFilterField(hcFieldMetadata).
			Description("Specify matching rules that determine which metadata keys should be added to the HTTP request as headers.").
			Advanced().
			Optional(),
		service.NewStringEnumField(hcFieldDumpRequestLogLevel, "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "").
			Description("EXPERIMENTAL: Set the logging level for the request and response payloads of each HTTP request.").
			Advanced().
			Default("").
			Version("4.12.0"),
	}
	innerFields = append(innerFields, AuthFieldSpecsExpanded()...)

	extractHeadersDesc := "Specify which response headers to add to the resulting messages as metadata. Header keys are automatically converted to lowercase before matching, so make sure that your patterns target the lowercase versions of the expected header keys."
	if forOutput {
		extractHeadersDesc = `Specify which response headers to add to the resulting synchronous response messages as metadata. Header keys are automatically converted to lowercase before matching, so make sure that your patterns target the lowercase versions of the expected header keys.

This field is only applicable when ` + "`" + `propagate_response` + "`" + ` is set to ` + "`" + `true` + "`" + `.`
	}
	innerFields = append(innerFields,
		service.NewTLSToggledField(hcFieldTLS),
		service.NewMetadataFilterField(hcFieldExtractHeaders).
			Description(extractHeadersDesc).
			Advanced(),
		service.NewStringField(hcFieldRateLimit).
			Description("A xref:components:rate_limits/about.adoc[rate limit] to throttle requests by (optional).").
			Optional(),
		service.NewDurationField(hcFieldTimeout).
			Description("A static timeout to apply to requests.").
			Default("5s"),
		service.NewDurationField(hcFieldRetryPeriod).
			Description("The initial period to wait between failed requests before retrying.").
			Advanced().
			Default("1s"),
		service.NewDurationField(hcFieldMaxRetryBackoff).
			Description("The maximum period to wait between failed requests.").
			Advanced().
			Default("300s"),
		service.NewIntField(hcFieldRetries).
			Description("The maximum number of retry attempts to make.").
			Advanced().
			Default(3),
		service.NewBoolField(hcFieldFollowRedirects).
			Description("Whether to transparently follow redirects, that is, responses with HTTP status codes in the 300-399 range. If set to `false`, the response message includes the body, status, and headers from the redirect response, and the component does not make a request to the URL specified in the `Location` header.").Version("4.39.0").
			Advanced().
			Default(true),
		service.NewIntListField(hcFieldBackoffOn).
			Description("A list of status codes that indicate a request failure and trigger retries with an increasing backoff period between attempts.").
			Advanced().
			Default([]any{429}),
		service.NewIntListField(hcFieldDropOn).
			Description(`A list of status codes that indicate a request failure where retries should not be attempted. This helps avoid unnecessary retries for requests that are unlikely to succeed.

NOTE: In these cases, the _request_ is dropped, but the _message_ that triggered the request is retained.`).
			Advanced().
			Default([]any{}),
		service.NewIntListField(hcFieldSuccessfulOn).
			Description(`A list of HTTP status codes that should be considered successful, even if they are not 2XX codes. This is useful for handling cases where non-2XX codes indicate that the request was processed successfully, such as `+"`"+`303 See Other`+"`"+` or `+"`"+`409 Conflict`+"`"+`.

By default, all 2XX codes are considered successful unless they are specified in the `+"`"+`backoff_on`+"`"+` or `+"`"+`drop_on`+"`"+` fields, regardless of this field.`).
			Advanced().
			Default([]any{}),
		service.NewStringField(hcFieldProxyURL).
			Description("An HTTP proxy URL (optional).").
			Advanced().
			Optional(),
		service.NewBoolField(hcFieldDisableHTTP2).
			Description("Whether to disable HTTP/2. By default, HTTP/2 is enabled.").
			Advanced().
			Default(false).
			Version("4.47.0"),
	)

	innerFields = append(innerFields, extraChildren...)
	return service.NewObjectField("", innerFields...)
}

//------------------------------------------------------------------------------

// ConfigFromParsed attempts to parse an http client config struct from a parsed
// plugin config.
func ConfigFromParsed(pConf *service.ParsedConfig) (conf OldConfig, err error) {
	if conf.URL, err = pConf.FieldInterpolatedString(hcFieldURL); err != nil {
		return
	}
	if conf.Verb, err = pConf.FieldString(hcFieldVerb); err != nil {
		return
	}
	if conf.Headers, err = pConf.FieldInterpolatedStringMap(hcFieldHeaders); err != nil {
		return
	}
	if conf.Metadata, err = pConf.FieldMetadataFilter(hcFieldMetadata); err != nil {
		return
	}
	if conf.ExtractMetadata, err = pConf.FieldMetadataFilter(hcFieldExtractHeaders); err != nil {
		return
	}
	conf.RateLimit, _ = pConf.FieldString(hcFieldRateLimit)
	if conf.Timeout, err = pConf.FieldDuration(hcFieldTimeout); err != nil {
		return
	}
	if conf.Retry, err = pConf.FieldDuration(hcFieldRetryPeriod); err != nil {
		return
	}
	if conf.MaxBackoff, err = pConf.FieldDuration(hcFieldMaxRetryBackoff); err != nil {
		return
	}
	if conf.NumRetries, err = pConf.FieldInt(hcFieldRetries); err != nil {
		return
	}
	if conf.FollowRedirects, err = pConf.FieldBool(hcFieldFollowRedirects); err != nil {
		return
	}
	if conf.BackoffOn, err = pConf.FieldIntList(hcFieldBackoffOn); err != nil {
		return
	}
	if conf.DropOn, err = pConf.FieldIntList(hcFieldDropOn); err != nil {
		return
	}
	if conf.SuccessfulOn, err = pConf.FieldIntList(hcFieldSuccessfulOn); err != nil {
		return
	}
	conf.DumpRequestLogLevel, _ = pConf.FieldString(hcFieldDumpRequestLogLevel)
	if conf.TLSConf, conf.TLSEnabled, err = pConf.FieldTLSToggled(hcFieldTLS); err != nil {
		return
	}
	conf.ProxyURL, _ = pConf.FieldString(hcFieldProxyURL)
	if conf.authSigner, err = pConf.HTTPRequestAuthSignerFromParsed(); err != nil {
		return
	}
	if conf.DisableHTTP2, err = pConf.FieldBool(hcFieldDisableHTTP2); err != nil {
		return
	}
	if conf.clientCtor, err = oauth2ClientCtorFromParsed(pConf); err != nil {
		return
	}
	return
}

// OldConfig is a configuration struct for an HTTP client.
type OldConfig struct {
	URL                 *service.InterpolatedString
	Verb                string
	Headers             map[string]*service.InterpolatedString
	Metadata            *service.MetadataFilter
	ExtractMetadata     *service.MetadataFilter
	RateLimit           string
	Timeout             time.Duration
	Retry               time.Duration
	MaxBackoff          time.Duration
	NumRetries          int
	FollowRedirects     bool
	BackoffOn           []int
	DropOn              []int
	SuccessfulOn        []int
	DumpRequestLogLevel string
	TLSEnabled          bool
	TLSConf             *tls.Config
	ProxyURL            string
	DisableHTTP2        bool
	authSigner          func(f fs.FS, req *http.Request) error
	clientCtor          func(context.Context, *http.Client) *http.Client
}
