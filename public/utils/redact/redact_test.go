// Copyright 2026 Redpanda Data, Inc.

package redact

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "S3cretPass"

// badHostURL fails to parse because of the space in the host. It is a
// variable so that linters do not reject the deliberately invalid URL.
var badHostURL = "amqp://admin:" + secret + "@bad host"

func TestURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no credentials", "amqp://127.0.0.1:5672/", "amqp://127.0.0.1:5672/"},
		{"username only", "amqp://guest@127.0.0.1:5672/", "amqp://guest@127.0.0.1:5672/"},
		{"userinfo password", "amqp://guest:" + secret + "@127.0.0.1:5672/", "amqp://guest:xxxxx@127.0.0.1:5672/"},
		{"empty password", "redis://:" + secret + "@localhost:6379", "redis://:xxxxx@localhost:6379"},
		{"query password", "tcp://host:9000?username=u&password=" + secret + "&database=db", "tcp://host:9000?username=xxxxx&password=xxxxx&database=xxxxx"},
		{"query token", "https://host/path?access_token=" + secret, "https://host/path?access_token=xxxxx"},
		{"sas signature", "https://acct.blob.core.windows.net/c?sv=2020&sig=" + secret, "https://acct.blob.core.windows.net/c?sv=xxxxx&sig=xxxxx"},
		{"influxdb password parameter", "http://host:8086/write?db=x&u=user&p=" + secret, "http://host:8086/write?db=xxxxx&u=xxxxx&p=xxxxx"},
		{"every query value redacted in order", "https://host/path?b=2&a=1", "https://host/path?b=xxxxx&a=xxxxx"},
		{"unknown credential key", "https://host/path?pw=" + secret, "https://host/path?pw=xxxxx"},
		{"bare query token", "https://host/hook?" + secret, "https://host/hook?xxxxx"},
		{"fragment", "https://host/cb#access_token=" + secret, "https://host/cb#xxxxx"},
		{"unparseable with query", "amqp://u@bad host:5672?pw=" + secret, "amqp://xxxxx"},
		{"libpq question mark in value", "host=h password='a?" + secret + "' dbname=db", "host=xxxxx password=xxxxx dbname=xxxxx"},
		{"ado question mark in value", "Server=h;Password=ab?" + secret + ";Database=d", "Server=xxxxx;Password=xxxxx;Database=xxxxx"},
		{"odbc brace quoted value", "Server=h;Password={SEC;" + secret + "}};x};Database=d", "Server=xxxxx;Password=xxxxx;Database=xxxxx"},
		{"mysql query value with at sign", "user:" + secret + "@tcp(db:3306)/app?tls=@" + secret, "user:xxxxx@xxxxx"},
		{"mysql password with space", "user:SEC " + secret + "@tcp(h:3306)/db", "user:xxxxx@tcp(h:3306)/db"},
		{"bad host", "amqp://admin:" + secret + "@bad host:5672", "amqp://xxxxx"},
		{"control character", "mongodb://admin:" + secret + "@host:27017/db\x7f", "mongodb://xxxxx"},
		{"password containing at sign", "amqp://admin:p@ss" + secret + "@host:5672", "amqp://admin:xxxxx@host:5672"},
		{"mysql dsn", "root:" + secret + "@tcp(localhost:3306)/db?parseTime=true", "root:xxxxx@tcp(localhost:3306)/db?parseTime=xxxxx"},
		{"libpq keyword dsn", "host=localhost user=u password=" + secret + " dbname=db", "host=xxxxx user=xxxxx password=xxxxx dbname=xxxxx"},
		{"libpq spaces around equals", "host = h password = " + secret, "host = xxxxx password = xxxxx"},
		{"libpq quoted password", "host=localhost password='" + secret + " x' dbname=db", "host=xxxxx password=xxxxx dbname=xxxxx"},
		{"ado connection string", "Server=h;User Id=u;Password=" + secret + ";Database=db", "Server=xxxxx;User Id=xxxxx;Password=xxxxx;Database=xxxxx"},
		{"url list", "amqp://u:" + secret + "@h1:5672,amqp://u:" + secret + "2@h2:5672", "amqp://u:xxxxx@h1:5672,amqp://u:xxxxx@h2:5672"},
		{"url list with spaces", "nats://u:" + secret + "@h1:4222, nats://u:" + secret + "2@h2:4222", "nats://u:xxxxx@h1:4222, nats://u:xxxxx@h2:4222"},
		{"comma in a single url", "https://h/p?ids=1,2", "https://h/p?ids=xxxxx"},
		{"leading whitespace url", "  postgres://u:" + secret + "@h/db", "  postgres://u:xxxxx@h/db"},
		{"leading whitespace mysql dsn", " root:" + secret + "@tcp(h:3306)/db", " root:xxxxx@tcp(h:3306)/db"},
		{"host port only", "localhost:9092", "localhost:9092"},
		{"empty", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := String(test.in)
			assert.Equal(t, test.want, got)
			assert.NotContains(t, got, secret)
		})
	}
}

func TestURLValue(t *testing.T) {
	u, err := url.Parse("nats://u:" + secret + "@host:4222?token=" + secret)
	require.NoError(t, err)

	assert.Equal(t, "nats://u:xxxxx@host:4222?token=xxxxx", URL(u))
	assert.Equal(t, "nats://u:"+secret+"@host:4222?token="+secret, u.String(), "input must not be modified")
	assert.Empty(t, URL(nil))
}

func TestParseURL(t *testing.T) {
	_, err := ParseURL("amqp://admin:" + secret + "@bad host:5672")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	assert.Equal(t, `parse "amqp://xxxxx": invalid character in host name`, err.Error())

	var uErr *url.Error
	require.ErrorAs(t, err, &uErr)
	assert.Equal(t, "parse", uErr.Op)

	u, err := ParseURL("amqp://admin:" + secret + "@host:5672")
	require.NoError(t, err)
	p, _ := u.User.Password()
	assert.Equal(t, secret, p, "a successful parse must keep the password")
}

func TestParseRequestURI(t *testing.T) {
	_, err := ParseRequestURI("http://u:" + secret + "@bad host/")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

var errSentinel = errors.New("sentinel")

func TestError(t *testing.T) {
	assert.NoError(t, Error(nil, Conns("x")))

	dsn := "postgres://u:" + secret + "@host/db"
	err := Error(fmt.Errorf("cannot connect to %s: %w", dsn, errSentinel), Conns(dsn))
	assert.Equal(t, "cannot connect to postgres://u:xxxxx@host/db: sentinel", err.Error())
	assert.ErrorIs(t, err, errSentinel)

	_, pErr := url.Parse(badHostURL)
	err = Error(fmt.Errorf("invalid AMQP URL: %w", pErr), Conns(badHostURL))
	assert.NotContains(t, err.Error(), secret)
	var uErr *url.Error
	assert.ErrorAs(t, err, &uErr)

	plain := errors.New("connection refused")
	assert.Same(t, plain, Error(plain, Conns(dsn)))
}

func TestErrorRedactsWrappedURLErrors(t *testing.T) {
	_, pErr := url.Parse(badHostURL)
	wrapped := fmt.Errorf("dial: %w", errors.Join(errSentinel, pErr))

	err := Error(wrapped)
	assert.NotContains(t, err.Error(), secret)

	var uErr *url.Error
	require.ErrorAs(t, err, &uErr)
	assert.NotContains(t, uErr.URL, secret, "errors.As must not recover the credentials")
	assert.ErrorIs(t, err, errSentinel)
}

func TestErrorRedactsQuotedAndDerivedForms(t *testing.T) {
	dsn := "mongodb://admin:" + secret + "@host:27017/db\x7f"
	err := Error(fmt.Errorf("parse %q: bad", dsn), Conns(dsn))
	assert.NotContains(t, err.Error(), secret)

	// A driver that prints its own rewrite of the DSN, such as pgx, which
	// redacts the userinfo password but not a password query parameter.
	pgDSN := "postgres://u@host/db?password=" + secret + "&sslmode=bogus"
	err = Error(errors.New("cannot parse `postgres://u@host/db?sslmode=bogus&password="+secret+"`: invalid sslmode"), Conns(pgDSN))
	assert.NotContains(t, err.Error(), secret)

	// A derived URL, such as a base URL joined with a request path.
	base := "https://u:" + secret + "@registry:8081"
	err = Error(fmt.Errorf("unable to GET %q: timeout", base+"/schemas/ids/1"), Conns(base))
	assert.NotContains(t, err.Error(), secret)

	// Short credentials are not removed from the rest of the message.
	err = Error(errors.New("pq: password authentication failed for user abc"), Conns("postgres://u:abc@host/db"))
	assert.Equal(t, "pq: password authentication failed for user abc", err.Error())
}

func TestParseURLPasswordsThatBreakParsing(t *testing.T) {
	for _, in := range []string{
		"postgres://u:Zk9Qm" + secret + "/xY+w@h:5432/db",
		"postgres://u:Zk9Qm" + secret + "#rest@h:5432/db",
		"postgres://u:Zk9Qm" + secret + "?rest@h:5432/db",
	} {
		_, err := ParseURL(in)
		require.Error(t, err, in)
		assert.NotContains(t, err.Error(), secret, in)

		// The same error from a library that calls url.Parse itself.
		_, rawErr := url.Parse(in)
		require.Error(t, rawErr)
		assert.NotContains(t, Error(fmt.Errorf("connecting: %w", rawErr), Conns(in)).Error(), secret, in)
		assert.NotContains(t, Error(fmt.Errorf("connecting: %w", rawErr)).Error(), secret, in)
	}
}

func TestErrorDoesNotExposeTheOriginal(t *testing.T) {
	dsn := "postgres://u:" + secret + "@host/db"
	_, pErr := url.Parse(badHostURL)
	err := Error(fmt.Errorf("dial %s: %w", dsn, errors.Join(errSentinel, pErr)), Conns(dsn))

	for e := err; e != nil; e = errors.Unwrap(e) {
		assert.NotContains(t, e.Error(), secret)
	}
	assert.ErrorIs(t, err, errSentinel)

	var uErr *url.Error
	require.ErrorAs(t, err, &uErr)
	assert.NotContains(t, uErr.Error(), secret)
	assert.Contains(t, pErr.Error(), secret, "the original error must not be modified")
}

func TestErrorConcurrentUse(t *testing.T) {
	_, pErr := url.Parse(badHostURL)
	shared := fmt.Errorf("dial: %w", pErr)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			assert.NotContains(t, Error(shared).Error(), secret)
			_ = shared.Error()
		})
	}
	wg.Wait()
}

func TestErrorRedactsURLsInMessage(t *testing.T) {
	// No connection string is given, and the key is not a known credential
	// key, so only the URL in the message reveals the credential.
	err := Error(fmt.Errorf("push to %q failed: 500", "http://gw:9091/metrics?pw="+secret))
	assert.NotContains(t, err.Error(), secret)

	err = Error(errors.New("cannot parse `postgres://u:" + secret + "@host/db?sslmode=bogus`: invalid sslmode"))
	assert.NotContains(t, err.Error(), secret)

	plain := errors.New("dial tcp 10.0.0.1:5432: connection refused (see https://docs.example.com/errors)")
	assert.Same(t, plain, Error(plain))

	plain = errors.New("connecting to http://h:9000: connection refused")
	assert.Same(t, plain, Error(plain))

	for _, msg := range []string{
		"dial postgres://u:it's" + secret + "@h/db failed",
		"dial postgres://u:SEC " + secret + "@h/db failed",
		`dial postgres://u:SEC"` + secret + "@h/db failed",
		"bad dsn postgres://u:" + secret + "trunc...",
	} {
		assert.NotContains(t, Error(errors.New(msg)).Error(), secret, msg)
	}
}

func TestErrorKeepsShortQueryValues(t *testing.T) {
	err := Error(errors.New("tls is true but the server refused"), Conns("postgres://h/db?ssl=true"))
	assert.Equal(t, "tls is true but the server refused", err.Error())
}

func TestErrorRedactsCommaSeparatedLists(t *testing.T) {
	list := "nats://a:" + secret + "1@good:4222,nats://b:" + secret + "2@bad host:4222"
	err := Error(fmt.Errorf("parse %q: invalid character", "nats://b:"+secret+"2@bad host:4222"), Conns(list))
	assert.NotContains(t, err.Error(), secret)
}

func TestParseURLRedactsEscapeErrors(t *testing.T) {
	_, err := ParseURL("amqp://admin:S3c%ZZret@host:5672")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "%ZZ")
	assert.NotContains(t, err.Error(), "S3c")
}

func BenchmarkURLValue(b *testing.B) {
	for _, raw := range []string{
		"https://collector.example.com:4318/v1/traces",
		"https://u:p@collector.example.com:4318/v1/traces?tenant=a&region=b",
		"https://acct.blob.core.windows.net/c/blob?sv=2020&se=2026&sig=abc",
	} {
		u, err := url.Parse(raw)
		require.NoError(b, err)
		b.Run("stdlib/"+u.Host, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = u.Redacted()
			}
		})
		b.Run("redact/"+u.Host, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = URL(u)
			}
		})
	}
}

// minFuzzValueLen keeps fuzzed values from occurring by chance in fixed text.
const minFuzzValueLen = 8

// FuzzStringAndError checks that credentials in common connection strings
// never survive String or Error, skipping inputs that hit documented limits.

func FuzzStringAndError(f *testing.F) {
	for _, seed := range [][2]string{
		{"S3cretPass", "S3cretToken"},
		{"p@ss/w0rd#x", "query@value1"},
		{"a b'c\"d", "spaced value"},
		{"%ZZpass", "%ZZescaped"},
		{"000000", "@0000000"},
		{"0000", "0000\x89000"}, // invalid UTF-8, which once panicked when building a regexp
	} {
		f.Add(seed[0], seed[1])
	}
	templates := []struct {
		format   string
		host     string
		keywords bool
	}{
		{format: "postgres://user:%s@db.example.com:5432/app?sslmode=%s", host: "db.example.com"},
		{format: "amqp://user:%s@bad host:5672/?token=%s", host: "bad host"},
		{format: "user:%s@tcp(db:3306)/app?tls=%s"},
		{format: "host=db user=u password=%s sslmode=%s", keywords: true},
		{format: "Server=db;User Id=u;Password=%s;Database=%s", keywords: true},
	}
	f.Fuzz(func(t *testing.T, password, value string) {
		if len(password) < minSecretLen || len(value) < minFuzzValueLen ||
			strings.ContainsAny(password+value, "\n\r\x01") || strings.Contains(value, "&") {
			return
		}
		for _, tmpl := range templates {
			if tmpl.keywords && strings.ContainsAny(password+value, " \t;'\"{}=") {
				continue // such values must be quoted in keyword/value strings
			}
			s := fmt.Sprintf(tmpl.format, password, value)
			if tmpl.host != "" {
				if u, err := url.Parse(s); err == nil && u.Hostname() != tmpl.host {
					continue // split in the wrong place, a documented limit
				}
			} else if strings.HasPrefix(password, "//") {
				continue // reads as a URL, a documented limit
			}
			fixed := strings.ReplaceAll(tmpl.format, "%s", "")
			check := func(where, out string) {
				out = strings.ReplaceAll(out, Marker, "\x01")
				for _, cred := range []string{password, value} {
					if strings.Contains(out, cred) && !strings.Contains(fixed, cred) &&
						!strings.Contains("parse invalid URL escape port after host character in name connect dial refused", cred) {
						t.Fatalf("%s kept %q\nin:  %q\nout: %q", where, cred, s, out)
					}
				}
			}
			check("String", String(s))
			if _, err := url.Parse(s); err != nil {
				check("Error", Error(fmt.Errorf("connect: %w", err)).Error())
			}
			check("Error with Conns", Error(fmt.Errorf("dial %s: refused", s), Conns(s)).Error())
		}
	})
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestErrorAsInterfaceTargets(t *testing.T) {
	err := Error(&url.Error{Op: "Get", URL: "http://h/p?token=" + secret, Err: timeoutErr{}})

	var ne net.Error
	require.ErrorAs(t, err, &ne)
	assert.NotContains(t, ne.Error(), secret)
	assert.True(t, ne.Timeout())

	var uErr *url.Error
	require.ErrorAs(t, err, &uErr)
	assert.NotContains(t, uErr.Error(), secret)
	assert.True(t, uErr.Timeout())
}

// leakyTimeoutErr is a timeout error whose message includes a credential.
type leakyTimeoutErr struct{ timeoutErr }

func (leakyTimeoutErr) Error() string { return "dial postgres://u:" + secret + "@h/db: i/o timeout" }

func TestErrorKeepsTimeoutOfRedactedInnerErrors(t *testing.T) {
	err := Error(&url.Error{Op: "Get", URL: "http://h/p", Err: leakyTimeoutErr{}})

	var uErr *url.Error
	require.ErrorAs(t, err, &uErr)
	assert.NotContains(t, uErr.Error(), secret)
	assert.True(t, uErr.Timeout())
}

func TestErrorKeepsTheRestOfTheMessage(t *testing.T) {
	for _, msg := range []string{
		`Post "http://h:8081/subjects?x=1": x509: certificate for "*.corp" is not valid, contact ops@corp`,
		`Post "http://h:8081/subjects": oauth2: token for svc@corp.example rejected`,
	} {
		got := Error(errors.New(msg)).Error()
		_, tail, _ := strings.Cut(msg, `": `)
		assert.Contains(t, got, tail, msg)
	}
}

func TestErrorConnsValuesOnlyInKeyValueContext(t *testing.T) {
	err := Error(errors.New("dial tcp: lookup production-db: no such host"), Conns("host=production-db password=hunter22"))
	assert.Equal(t, "dial tcp: lookup production-db: no such host", err.Error())

	err = Error(errors.New("failed to connect localhost:9092"), Conns("http://h/p?bootstrap=localhost:9092"))
	assert.Equal(t, "failed to connect localhost:9092", err.Error())

	// A short credential echoed as a key=value pair is removed.
	err = Error(errors.New("invalid option token=abc1234 for request"), Conns("http://h/p?token=abc1234"))
	assert.Equal(t, "invalid option token=xxxxx for request", err.Error())

	err = Error(errors.New("rejected password='hunter22' (bad auth)"), Conns("host=h password=hunter22"))
	assert.Equal(t, "rejected password='xxxxx' (bad auth)", err.Error())
}

func TestErrorValues(t *testing.T) {
	const token = "a b/c" + secret
	for _, form := range []string{token, url.PathEscape(token), (&url.URL{Path: token}).EscapedPath(), url.QueryEscape(token)} {
		err := Error(fmt.Errorf("sync failed for %s", form), Values(token))
		assert.NotContains(t, err.Error(), secret, form)
	}

	uErr := &url.Error{Op: "Post", URL: (&url.URL{Scheme: "http", Host: "h", Path: "/sync/" + token}).String(), Err: errSentinel}
	err := Error(fmt.Errorf("sync: %w", uErr), Values(token))
	assert.NotContains(t, err.Error(), secret)
	var got *url.Error
	require.ErrorAs(t, err, &got)
	assert.NotContains(t, got.URL, secret)
}

// sliceErr is an error with an uncomparable dynamic type.
type sliceErr []string

func (e sliceErr) Error() string { return strings.Join(e, ",") }

func TestErrorUncomparableErrors(t *testing.T) {
	err := Error(&url.Error{Op: "Get", URL: "http://u:" + secret + "@h", Err: sliceErr{"a"}})
	assert.NotContains(t, err.Error(), secret)

	var ne net.Error
	assert.NotPanics(t, func() { errors.As(err, &ne) })
}

func TestErrorKeepsURLErrorType(t *testing.T) {
	err := Error(&url.Error{Op: "Get", URL: "http://h/p?token=" + secret, Err: timeoutErr{}})
	uErr, ok := err.(*url.Error)
	require.True(t, ok, "%T", err)
	assert.NotContains(t, uErr.Error(), secret)
	assert.True(t, os.IsTimeout(err))
}

func TestErrorConnsValuesInOtherContexts(t *testing.T) {
	conn := "http://h/api?api_key=SECRETKEY123"
	for _, msg := range []string{
		`server said {"api_key":"SECRETKEY123"}`,
		"invalid token: SECRETKEY123",
		"rejected 'SECRETKEY123'",
	} {
		assert.NotContains(t, Error(errors.New(msg), Conns(conn)).Error(), "SECRETKEY123", msg)
	}
	err := Error(errors.New("bad password 'S3cret Pass'"), Conns("host=h password='S3cret Pass'"))
	assert.NotContains(t, err.Error(), "S3cret Pass")
}

func TestErrorDecodedUserinfoPassword(t *testing.T) {
	err := Error(errors.New("auth failed for password a+b!cd"), Conns("postgres://u:a+b%21cd@h/db"))
	assert.NotContains(t, err.Error(), "a+b!cd")
}

func TestErrorShortUserinfoPasswordAsValue(t *testing.T) {
	err := Error(errors.New("bad password=abc"), Conns("postgres://u:abc@h/db"))
	assert.Equal(t, "bad password=xxxxx", err.Error())
}

func TestErrorIPv6URLInText(t *testing.T) {
	msg := "dial http://[::1] failed; mail admin@example.com"
	assert.Equal(t, msg, Error(errors.New(msg)).Error())
}

func TestStringKeepsOriginalText(t *testing.T) {
	assert.Equal(t, `http://h/${! meta("x") }?id=xxxxx`, String(`http://h/${! meta("x") }?id=${! this.id }`))
	assert.Equal(t, "http://u:xxxxx@h/a%2Fb?q=xxxxx#xxxxx", String("http://u:"+secret+"@h/a%2Fb?q=1#frag"))
	assert.Equal(t, "http://h/p?", String("http://h/p?"))
}
