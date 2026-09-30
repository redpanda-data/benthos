// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/fs"
	"maps"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	aFieldBasicAuth = "basic_auth"
	aFieldOAuth     = "oauth"
	aFieldJWT       = "jwt"
)

// NewHTTPRequestAuthSignerFields returns a list of config fields for adding
// authentication to HTTP requests. The options available with this field
// include OAuth (v1), basic authentication, and JWT as these are mechanisms
// that can be implemented by mutating a request object.
func NewHTTPRequestAuthSignerFields() []*ConfigField {
	return []*ConfigField{
		oAuthFieldSpec(),
		basicAuthField(),
		jwtFieldSpec(),
	}
}

// HTTPRequestAuthSignerFromParsed takes a parsed config which is expected to
// contain fields from NewHTTPRequestAuthSignerFields, and returns a func that
// applies those configured authentication mechanisms to a given HTTP request.
func (p *ParsedConfig) HTTPRequestAuthSignerFromParsed() (fn func(fs.FS, *http.Request) error, err error) {
	var oldConf authConfig
	if oldConf.OAuth, err = oauthFromParsed(p); err != nil {
		return
	}
	if oldConf.BasicAuth, err = basicAuthFromParsed(p); err != nil {
		return
	}
	if oldConf.JWT, err = jwtAuthFromParsed(p); err != nil {
		return
	}
	fn = oldConf.Sign
	return
}

type authConfig struct {
	OAuth     oauthConfig
	BasicAuth basicAuthConfig
	JWT       jwtConfig
}

// Sign method to sign an HTTP request for configured auth strategies.
func (c authConfig) Sign(f fs.FS, req *http.Request) error {
	if err := c.OAuth.Sign(req); err != nil {
		return err
	}
	if err := c.JWT.Sign(f, req); err != nil {
		return err
	}
	return c.BasicAuth.Sign(req)
}

//------------------------------------------------------------------------------

const (
	abFieldEnabled  = "enabled"
	abFieldUsername = "username"
	abFieldPassword = "password"
)

func basicAuthField() *ConfigField {
	return NewObjectField(aFieldBasicAuth,
		NewBoolField(abFieldEnabled).
			Description("Whether to use basic authentication in requests.").
			Default(false),

		NewStringField(abFieldUsername).
			Description("The username of the account credentials to authenticate as. Used together with `password` for basic authentication.").
			Default(""),

		NewStringField(abFieldPassword).
			Description("The password to use for authentication. Used together with `username` for basic authentication.").
			Default("").Secret(),
	).Description("Configure basic authentication for requests from this component.").
		Advanced().
		Optional()
}

func basicAuthFromParsed(conf *ParsedConfig) (res basicAuthConfig, err error) {
	if !conf.Contains(aFieldBasicAuth) {
		return
	}
	conf = conf.Namespace(aFieldBasicAuth)
	if res.Enabled, err = conf.FieldBool(abFieldEnabled); err != nil {
		return
	}
	if res.Username, err = conf.FieldString(abFieldUsername); err != nil {
		return
	}
	if res.Password, err = conf.FieldString(abFieldPassword); err != nil {
		return
	}
	return
}

type basicAuthConfig struct {
	Enabled  bool
	Username string
	Password string
}

// Sign method to sign an HTTP request for an OAuth exchange.
func (basic basicAuthConfig) Sign(req *http.Request) error {
	if basic.Enabled {
		req.SetBasicAuth(basic.Username, basic.Password)
	}
	return nil
}

//------------------------------------------------------------------------------

const (
	aoFieldEnabled           = "enabled"
	aoFieldConsumerKey       = "consumer_key"
	aoFieldConsumerSecret    = "consumer_secret"
	aoFieldAccessToken       = "access_token"
	aoFieldAccessTokenSecret = "access_token_secret"
)

func oAuthFieldSpec() *ConfigField {
	return NewObjectField(aFieldOAuth,
		NewBoolField(aoFieldEnabled).
			Description("Whether to enable OAuth version 1.0 authentication for requests.").
			Default(false),

		NewStringField(aoFieldConsumerKey).
			Description("The value used to identify this component or client to the service provider.").
			Default(""),

		NewStringField(aoFieldConsumerSecret).
			Description("The secret that establishes ownership of the consumer key in OAuth 1.0 authentication.").
			Default("").Secret(),

		NewStringField(aoFieldAccessToken).
			Description("The value used to gain access to the protected resources on behalf of the user.").
			Default(""),

		NewStringField(aoFieldAccessTokenSecret).
			Description("The secret that establishes ownership of the `access_token` in OAuth 1.0 authentication.").
			Default("").Secret(),
	).
		Description("Configure OAuth version 1.0 authentication for secure API access.").
		Advanced().
		Optional()
}

func oauthFromParsed(conf *ParsedConfig) (res oauthConfig, err error) {
	if !conf.Contains(aFieldOAuth) {
		return
	}
	conf = conf.Namespace(aFieldOAuth)
	if res.Enabled, err = conf.FieldBool(aoFieldEnabled); err != nil {
		return
	}
	if res.ConsumerKey, err = conf.FieldString(aoFieldConsumerKey); err != nil {
		return
	}
	if res.ConsumerSecret, err = conf.FieldString(aoFieldConsumerSecret); err != nil {
		return
	}
	if res.AccessToken, err = conf.FieldString(aoFieldAccessToken); err != nil {
		return
	}
	if res.AccessTokenSecret, err = conf.FieldString(aoFieldAccessTokenSecret); err != nil {
		return
	}
	return
}

type oauthConfig struct {
	Enabled           bool
	ConsumerKey       string
	ConsumerSecret    string
	AccessToken       string
	AccessTokenSecret string
}

// Sign method to sign an HTTP request for an OAuth exchange.
func (oauth oauthConfig) Sign(req *http.Request) error {
	if !oauth.Enabled {
		return nil
	}

	nonceGenerator := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonce := strconv.FormatInt(nonceGenerator.Int63(), 10)
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	params := &url.Values{}
	params.Add("oauth_consumer_key", oauth.ConsumerKey)
	params.Add("oauth_nonce", nonce)
	params.Add("oauth_signature_method", "HMAC-SHA1")
	params.Add("oauth_timestamp", ts)
	params.Add("oauth_token", oauth.AccessToken)
	params.Add("oauth_version", "1.0")

	sig, err := oauth.getSignature(req, params)
	if err != nil {
		return err
	}

	str := fmt.Sprintf(
		` oauth_consumer_key="%s", oauth_nonce="%s", oauth_signature="%s",`+
			` oauth_signature_method="%s", oauth_timestamp="%s",`+
			` oauth_token="%s", oauth_version="%s"`,
		url.QueryEscape(oauth.ConsumerKey),
		nonce,
		url.QueryEscape(sig),
		"HMAC-SHA1",
		ts,
		url.QueryEscape(oauth.AccessToken),
		"1.0",
	)
	req.Header.Add("Authorization", str)

	return nil
}

func (oauth oauthConfig) getSignature(
	req *http.Request,
	params *url.Values,
) (string, error) {
	baseSignatureString := req.Method + "&" +
		url.QueryEscape(req.URL.String()) + "&" +
		url.QueryEscape(params.Encode())

	signingKey := url.QueryEscape(oauth.ConsumerSecret) + "&" +
		url.QueryEscape(oauth.AccessTokenSecret)

	return oauth.computeHMAC(baseSignatureString, signingKey)
}

func (oauth oauthConfig) computeHMAC(
	message string,
	key string,
) (string, error) {
	h := hmac.New(sha1.New, []byte(key))
	if _, err := h.Write([]byte(message)); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

//------------------------------------------------------------------------------

const (
	ajFieldEnabled        = "enabled"
	ajFieldPrivateKeyFile = "private_key_file"
	ajFieldSigningMethod  = "signing_method"
	ajFieldClaims         = "claims"
	ajFieldHeaders        = "headers"
)

func jwtFieldSpec() *ConfigField {
	return NewObjectField(aFieldJWT,
		NewBoolField(ajFieldEnabled).
			Description("Whether to use JWT authentication in requests.").
			Default(false),

		NewStringField(ajFieldPrivateKeyFile).
			Description("Path to a file containing the PEM-encoded private key using PKCS#1 or PKCS#8 format. The private key must be compatible with the algorithm specified in the `signing_method` field.").
			Default(""),

		NewStringField(ajFieldSigningMethod).
			Description("The cryptographic algorithm used to sign the JWT. Supported algorithms are RS256, RS384, RS512, and EdDSA. This algorithm must be compatible with the private key specified in the `private_key_file` field.").
			Default(""),

		NewAnyMapField(ajFieldClaims).
			Description("A map of claims to include in the JWT. Claims pass the identity of the authenticated entity to the service provider.").
			Default(map[string]any{}).
			Advanced(),

		NewAnyMapField(ajFieldHeaders).
			Description("Additional key-value pairs to include in the JWT header (optional). These headers provide extra metadata for JWT processing.").
			Default(map[string]any{}).
			Advanced(),
	).
		Description("BETA: Configure JSON Web Token (JWT) authentication. This feature is in beta and may change in future releases. JWTs provide secure, stateless authentication between services.").
		Advanced()
}

func jwtAuthFromParsed(conf *ParsedConfig) (res jwtConfig, err error) {
	if !conf.Contains(aFieldJWT) {
		return
	}

	var key crypto.PrivateKey
	res.key = &key
	res.keyMx = &sync.Mutex{}

	conf = conf.Namespace(aFieldJWT)
	if res.Enabled, err = conf.FieldBool(ajFieldEnabled); err != nil {
		return
	}
	var claimsConfs map[string]*ParsedConfig
	if claimsConfs, err = conf.FieldAnyMap(ajFieldClaims); err != nil {
		return
	}
	res.Claims = jwt.MapClaims{}
	for k, v := range claimsConfs {
		if res.Claims[k], err = v.FieldAny(); err != nil {
			return
		}
	}
	var headersConfs map[string]*ParsedConfig
	if headersConfs, err = conf.FieldAnyMap(ajFieldHeaders); err != nil {
		return
	}
	res.Headers = map[string]any{}
	for k, v := range headersConfs {
		if res.Headers[k], err = v.FieldAny(); err != nil {
			return
		}
	}
	if res.SigningMethod, err = conf.FieldString(ajFieldSigningMethod); err != nil {
		return
	}
	if res.PrivateKeyFile, err = conf.FieldString(ajFieldPrivateKeyFile); err != nil {
		return
	}
	return
}

type jwtConfig struct {
	Enabled        bool
	Claims         jwt.MapClaims
	Headers        map[string]any
	SigningMethod  string
	PrivateKeyFile string

	// internal private fields
	keyMx *sync.Mutex
	key   *crypto.PrivateKey
}

// Sign method to sign an HTTP request for an JWT exchange.
func (j jwtConfig) Sign(f fs.FS, req *http.Request) error {
	if !j.Enabled {
		return nil
	}

	if err := j.parsePrivateKey(f); err != nil {
		return err
	}

	var token *jwt.Token
	switch j.SigningMethod {
	case "RS256":
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, j.Claims)
	case "RS384":
		token = jwt.NewWithClaims(jwt.SigningMethodRS384, j.Claims)
	case "RS512":
		token = jwt.NewWithClaims(jwt.SigningMethodRS512, j.Claims)
	case "EdDSA":
		token = jwt.NewWithClaims(jwt.SigningMethodEdDSA, j.Claims)
	default:
		return fmt.Errorf("jwt signing method %s not acepted. Try with RS256, RS384, RS512 or EdDSA", j.SigningMethod)
	}

	maps.Copy(token.Header, j.Headers)

	ss, err := token.SignedString(*j.key)
	if err != nil {
		return fmt.Errorf("failed to sign jwt: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+ss)
	return nil
}

// parsePrivateKey parses once the RSA private key.
// Needs mutex locking as Sign might be called by parallel threads.
func (j jwtConfig) parsePrivateKey(fs fs.FS) error {
	j.keyMx.Lock()
	defer j.keyMx.Unlock()

	if *j.key != nil {
		return nil
	}

	privateKey, err := ReadFile(fs, j.PrivateKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read private key: %v", err)
	}

	switch j.SigningMethod {
	case "RS256", "RS384", "RS512":
		*j.key, err = jwt.ParseRSAPrivateKeyFromPEM(privateKey)
	case "EdDSA":
		*j.key, err = jwt.ParseEdPrivateKeyFromPEM(privateKey)
	}
	if err != nil {
		return fmt.Errorf("failed to parse %s private key: %v", j.SigningMethod, err)
	}

	return nil
}
