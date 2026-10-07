// Copyright 2026 Redpanda Data, Inc.

// Package redact removes credentials from connection strings, such as URLs
// and DSNs, so that they can be included in logs and errors.
//
// Redaction fails closed where the structure is known: every query value is
// redacted, and a URL that cannot be parsed keeps only its scheme.
//
// Known limits: an unencoded password that url.Parse splits in the wrong place
// without failing, such as "postgres://u:1234/abc@host/db", is partly kept, as
// are credentials in a URL path. Such configurations cannot connect anyway.
// token as a path segment, are not detected.
package redact

import (
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Marker replaces redacted values. It matches the marker used by
// (*url.URL).Redacted.
const Marker = "xxxxx"

// String returns the connection string s with its credentials replaced by
// Marker. It accepts URLs, URL lists, scheme-less DSNs such as MySQL's, and
// keyword/value strings such as libpq's or ADO.NET's. The userinfo password
// and every query, fragment and keyword value are redacted, and a URL that
// cannot be parsed keeps only its scheme.
func String(s string) string {
	if t := trimLeftSpace(s); len(t) != len(s) {
		return s[:len(s)-len(t)] + String(t)
	}
	if parts, ok := splitURLList(s); ok {
		for i, p := range parts {
			parts[i] = String(p)
		}
		return strings.Join(parts, ",")
	}
	if !schemeRe.MatchString(s) {
		return dsn(s)
	}
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" {
		return schemeOnly(s)
	}
	return redactRawURL(s, u)
}

// redactRawURL redacts s, which url.Parse accepted as u, keeping the original
// text where url.Parse splits it rather than re-encoding it.
func redactRawURL(s string, u *url.URL) string {
	rest, fragment, hasFragment := strings.Cut(s, "#")
	rest, query, hasQuery := strings.Cut(rest, "?")
	if _, hasPwd := u.User.Password(); hasPwd {
		start := strings.Index(rest, "://") + 3
		end := len(rest)
		if i := strings.IndexByte(rest[start:], '/'); i >= 0 {
			end = start + i
		}
		at := start + strings.LastIndexByte(rest[start:end], '@')
		colon := start + strings.IndexByte(rest[start:at], ':')
		rest = rest[:colon+1] + Marker + rest[at:]
	}
	var b strings.Builder
	b.WriteString(rest)
	if hasQuery {
		b.WriteByte('?')
		b.WriteString(redactQuery(query))
	}
	if hasFragment {
		b.WriteByte('#')
		if fragment != "" {
			b.WriteString(Marker)
		}
	}
	return b.String()
}

func trimLeftSpace(s string) string {
	return strings.TrimLeft(s, " \t\r\n")
}

// splitURLList splits s into its parts when it is a comma-separated list of
// URLs, which is when it starts with a scheme and a later part does too.
func splitURLList(s string) ([]string, bool) {
	if !strings.Contains(s, ",") || !schemeRe.MatchString(s) {
		return nil, false
	}
	parts := strings.Split(s, ",")
	for _, p := range parts[1:] {
		if schemeRe.MatchString(trimLeftSpace(p)) {
			return parts, true
		}
	}
	return nil, false
}

// schemeOnly returns the scheme of the URL s followed by Marker, or Marker
// alone when s has no scheme.
func schemeOnly(s string) string {
	if m := schemeRe.FindString(s); m != "" {
		return m + Marker
	}
	return Marker
}

// URL returns the string form of u with the userinfo password, every query
// parameter value and the fragment replaced by Marker. u is not modified.
func URL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	if _, hasPwd := c.User.Password(); hasPwd {
		c.User = url.UserPassword(c.User.Username(), Marker)
	}
	if c.RawQuery != "" {
		c.RawQuery = redactQuery(c.RawQuery)
	}
	if c.Fragment != "" || c.RawFragment != "" {
		c.Fragment, c.RawFragment = Marker, ""
	}
	return c.String()
}

// redactQuery replaces the value of every parameter in rawQuery, and every
// parameter without a value, by Marker, keeping the keys and their order.
func redactQuery(rawQuery string) string {
	var b strings.Builder
	b.Grow(len(rawQuery) + len(Marker))
	for rest := rawQuery; ; {
		pair, next, more := strings.Cut(rest, "&")
		if pair != "" {
			if key, _, hasValue := strings.Cut(pair, "="); hasValue {
				b.WriteString(key)
				b.WriteByte('=')
			}
			b.WriteString(Marker)
		}
		if !more {
			break
		}
		b.WriteByte('&')
		rest = next
	}
	return b.String()
}

var (
	// schemeRe matches the scheme of a URL, which tells URLs apart from
	// scheme-less DSN formats.
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)

	// urlUserinfoRe and bareUserinfoRe capture the password of
	// "scheme://user:password@" and "user:password@", up to the last "@".
	urlUserinfoRe  = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.\-]*://[^:@/?#\s]*:)(.*)@`)
	bareUserinfoRe = regexp.MustCompile(`^([^:@/?#\s]*:)(.*)@`)

	// keywordDSNRe matches the start of a keyword/value string, such as
	// "host=h password=p" or "Server=h;Password=p".
	keywordDSNRe = regexp.MustCompile(`^\s*[A-Za-z][A-Za-z0-9_ .\-]*=`)

	// urlInTextRe matches URLs within free text, such as error messages,
	// stopping at whitespace, quotes and backticks.
	urlInTextRe = regexp.MustCompile("[A-Za-z][A-Za-z0-9+.\\-]*://[^\\s\"'`<>]+")

	// urlInTextEndRe matches an end to a URL found in free text: what follows
	// the last "@" on the line, up to whitespace, a quote or a backtick.
	urlInTextEndRe = regexp.MustCompile("^[^\\n]*@[^\\s\"'`<>]*")
)

// dsn redacts a connection string without a scheme, as described by String.
func dsn(s string) string {
	if isKeywordDSN(s) {
		return redactKeywordValues(s)
	}
	if m := bareUserinfoRe.FindStringSubmatchIndex(s); m != nil {
		prefix, password, rest := s[m[2]:m[3]], s[m[4]:m[5]], s[m[1]:]
		// A "?" in the password means the query might have started within
		// it, so what follows the "@" might be part of it.
		if strings.Contains(password, "?") {
			return prefix + Marker + "@" + Marker
		}
		s = prefix + Marker + "@" + rest
	}
	if head, query, ok := strings.Cut(s, "?"); ok {
		return head + "?" + redactQuery(query)
	}
	return s
}

// userinfoCredRe returns the regular expression that finds the userinfo
// password of s, which might be a URL or a scheme-less DSN.
func userinfoCredRe(s string) *regexp.Regexp {
	if schemeRe.MatchString(s) {
		return urlUserinfoRe
	}
	return bareUserinfoRe
}

func isKeywordDSN(s string) bool {
	return !schemeRe.MatchString(s) && keywordDSNRe.MatchString(s)
}

// keywordValues calls fn with the offsets of every value, and every token
// without a key, in the keyword/value string s, separated by ";" or spaces.
func keywordValues(s string, fn func(start, end int)) {
	isSep := func(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
	if strings.Contains(s, ";") {
		isSep = func(c byte) bool { return c == ';' }
	}
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

	for i := 0; i < len(s); {
		for i < len(s) && (isSep(s[i]) || isSpace(s[i])) {
			i++
		}
		start := i
		for i < len(s) && s[i] != '=' && !isSep(s[i]) {
			i++
		}
		// libpq allows whitespace around "=".
		j := i
		for j < len(s) && isSpace(s[j]) {
			j++
		}
		if j >= len(s) || s[j] != '=' {
			if i > start {
				fn(start, i)
			}
			continue
		}
		i = j + 1
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		vStart := i
		if i < len(s) {
			switch s[i] {
			case '\'', '"':
				q := s[i]
				for i++; i < len(s) && s[i] != q; i++ {
					if s[i] == '\\' {
						i++
					}
				}
				i++
			case '{':
				// ODBC escapes "}" within a value by doubling it.
				for i++; i < len(s); i++ {
					if s[i] == '}' {
						if i+1 < len(s) && s[i+1] == '}' {
							i++
							continue
						}
						break
					}
				}
				i++
			}
		}
		for i < len(s) && !isSep(s[i]) {
			i++
		}
		i = min(i, len(s))
		fn(vStart, i)
	}
}

func redactKeywordValues(s string) string {
	var b strings.Builder
	last := 0
	keywordValues(s, func(start, end int) {
		b.WriteString(s[last:start])
		b.WriteString(Marker)
		last = end
	})
	b.WriteString(s[last:])
	return b.String()
}

// ParseURL is url.Parse, except that an error has only the scheme of the
// input as its URL and a fixed description of the problem.
func ParseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	return u, parseError(err, s)
}

// ParseRequestURI is url.ParseRequestURI, except that a returned error does
// not include the input, as with ParseURL.
func ParseRequestURI(s string) (*url.URL, error) {
	u, err := url.ParseRequestURI(s)
	return u, parseError(err, s)
}

func parseError(err error, input string) error {
	var uErr *url.Error
	if !errors.As(err, &uErr) {
		return err
	}
	return &url.Error{Op: uErr.Op, URL: schemeOnly(input), Err: parseProblem(uErr.Err)}
}

// staticParseProblems are the url.Parse error messages that never include any
// part of the input.
var staticParseProblems = map[string]struct{}{
	"missing protocol scheme":                        {},
	"first path segment in URL cannot contain colon": {},
	"net/url: invalid control character in URL":      {},
	"net/url: invalid userinfo":                      {},
	"invalid URI for request":                        {},
	"empty url":                                      {},
	"missing ']' in host":                            {},
}

// parseProblem describes a url.Parse error without quoting the input, which
// errors such as `invalid port ":<text>" after host` do.
func parseProblem(err error) error {
	var escErr url.EscapeError
	var hostErr url.InvalidHostError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &escErr):
		return errors.New("invalid URL escape")
	case errors.As(err, &hostErr):
		return errors.New("invalid character in host name")
	}
	msg := err.Error()
	if _, ok := staticParseProblems[msg]; ok {
		return err
	}
	if strings.HasPrefix(msg, "invalid port ") {
		return errors.New("invalid port after host")
	}
	return errors.New("invalid URL")
}

// safeURLError returns a copy of e without credentials. A parse error keeps
// only the scheme, as the input could not be split reliably.
func (o *options) safeURLError(e *url.Error) *url.Error {
	if e.Op == "parse" {
		return &url.Error{Op: e.Op, URL: schemeOnly(e.URL), Err: parseProblem(e.Err)}
	}
	inner := e.Err
	if r, changed := o.redactErr(e.Err); changed {
		// (*url.Error).Timeout and Temporary assert e.Err directly, so the
		// redacted copy must keep answering them.
		re, ok := r.(*redactedError)
		if !ok {
			return &url.Error{Op: e.Op, URL: o.replaceValues(String(e.URL)), Err: r}
		}
		t, isT := e.Err.(interface{ Timeout() bool })
		tmp, isTmp := e.Err.(interface{ Temporary() bool })
		if isT || isTmp {
			inner = &redactedNetError{
				redactedError: re,
				timeout:       isT && t.Timeout(),
				temporary:     isTmp && tmp.Temporary(),
			}
		} else {
			inner = re
		}
	}
	return &url.Error{Op: e.Op, URL: o.replaceValues(String(e.URL)), Err: inner}
}

// minSecretLen is the shortest password that Error removes wherever it
// appears, as shorter values would corrupt unrelated text.
const minSecretLen = 4

// Option configures Error.
type Option func(*options)

type options struct {
	conns  []string
	values []string
}

// Conns names connection strings that the error might include. They are
// redacted as by String, their password is removed wherever it appears, and
// their other values where they appear as a value, such as after "=" or ":"
// or in quotes, but not in prose, where they might be a host name.
func Conns(conns ...string) Option {
	return func(o *options) {
		o.conns = append(o.conns, conns...)
	}
}

// Values names credentials that the error might include in any form, such
// as a token in a URL path. They are removed raw, quoted or percent-encoded.
func Values(values ...string) Option {
	return func(o *options) {
		for _, v := range values {
			if v != "" {
				o.values = append(o.values, v)
			}
		}
	}
}

// replaceValues removes the values named with Values from s.
func (o *options) replaceValues(s string) string {
	for _, v := range o.values {
		for _, form := range valueForms(v) {
			s = strings.ReplaceAll(s, form, Marker)
		}
	}
	return s
}

// valueForms returns the forms in which v might appear in a message: raw,
// quoted, and percent-encoded as a path, a path segment and a query value.
func valueForms(v string) []string {
	forms := []string{v}
	add := func(f string) {
		if !slices.Contains(forms, f) {
			forms = append(forms, f)
		}
	}
	q := strconv.Quote(v)
	add(q[1 : len(q)-1])
	add((&url.URL{Path: v}).EscapedPath())
	add(url.PathEscape(v))
	add(url.QueryEscape(v))
	return forms
}

// Error returns err with credentials removed from its message: those within
// any *url.Error in its chain, those named with Conns or Values, and every
// URL in the message. An err without credentials is returned unchanged.
//
// A *url.Error that needs only its own redaction is returned as a redacted
// copy, so that os.IsTimeout and type assertions keep working. Otherwise the
// result does not unwrap to err, but supports errors.Is and errors.As against
// its chain, with *url.Error values redacted. Other error types are returned
// as they are and might hold credentials in their fields.
func Error(err error, opts ...Option) error {
	if err == nil {
		return nil
	}
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o.redact(err)
}

func (o *options) redact(err error) error {
	r, _ := o.redactErr(err)
	return r
}

// redactErr also reports whether anything was removed, as errors cannot be
// compared with == when their dynamic types are not comparable.
func (o *options) redactErr(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	if _, ok := err.(*redactedError); ok && len(o.conns) == 0 && len(o.values) == 0 {
		return err, false
	}

	msg := err.Error()
	redacted := msg
	conns := slices.Clone(o.conns)
	unsafeURLErr := false
	walkChain(err, func(e error) {
		uErr, ok := e.(*url.Error)
		if !ok {
			return
		}
		safe := o.safeURLError(uErr)
		if raw, clean := uErr.Error(), safe.Error(); raw != clean {
			unsafeURLErr = true
			redacted = strings.ReplaceAll(redacted, raw, clean)
			conns = append(conns, uErr.URL)
		}
	})

	// Several components accept comma-separated lists of URLs, and errors
	// usually include only the URL that failed.
	for _, c := range conns {
		if strings.Contains(c, ",") {
			conns = append(conns, strings.Split(c, ",")...)
		}
	}
	var standalone, keyed []string
	for _, c := range conns {
		if c == "" {
			continue
		}
		rc := String(c)
		redacted = strings.ReplaceAll(redacted, c, rc)
		q, rq := strconv.Quote(c), strconv.Quote(rc)
		if qc := q[1 : len(q)-1]; qc != c {
			redacted = strings.ReplaceAll(redacted, qc, rq[1:len(rq)-1])
		}
		for _, cred := range credentials(c) {
			if cred.standalone && len(cred.value) >= minSecretLen {
				standalone = append(standalone, cred.value)
			} else if cred.value != "" {
				keyed = append(keyed, cred.value)
			}
		}
	}
	for _, v := range standalone {
		redacted = strings.ReplaceAll(redacted, v, Marker)
	}
	redacted = removeKeyedValues(redacted, keyed)
	redacted = o.replaceValues(redacted)

	// Libraries might include their own rewrite of a connection string, such
	// as a request URL.
	redacted = redactURLsInText(redacted)

	if redacted == msg && !unsafeURLErr {
		return err, false
	}
	// A *url.Error keeps its type when it needed only its own redaction.
	if uErr, ok := err.(*url.Error); ok {
		if safe := o.safeURLError(uErr); safe.Error() == redacted {
			return safe, true
		}
	}
	return &redactedError{msg: redacted, err: err, opts: o}, true
}

// removeKeyedValues removes values from msg where they appear as a value,
// after "=" or ":" or in quotes, matching bytes as they need not be UTF-8.
func removeKeyedValues(msg string, values []string) string {
	if len(values) == 0 {
		return msg
	}
	values = slices.Clone(values)
	// Longer values first, so that a value is not cut short by one of its
	// prefixes.
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })

	var b strings.Builder
	last := 0
	for i := 0; i < len(msg); i++ {
		start := valueStart(msg, i)
		if start < 0 {
			continue
		}
		for _, v := range values {
			end := start + len(v)
			if strings.HasPrefix(msg[start:], v) && (end == len(msg) || !isValueByte(msg[end])) {
				b.WriteString(msg[last:start])
				b.WriteString(Marker)
				last = end
				i = end - 1
				break
			}
		}
	}
	if last == 0 {
		return msg
	}
	b.WriteString(msg[last:])
	return b.String()
}

// valueStart returns the offset of the value that starts after the delimiter
// at msg[i], or -1 when msg[i] does not start a value.
func valueStart(msg string, i int) int {
	switch msg[i] {
	case '\'', '"':
		return i + 1
	case '=', ':':
		j := i + 1
		for j < len(msg) && (msg[j] == ' ' || msg[j] == '\t') {
			j++
		}
		if j < len(msg) && (msg[j] == '\'' || msg[j] == '"' || msg[j] == '{') {
			j++
		}
		return j
	}
	return -1
}

func isValueByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_.~%-", c) >= 0
}

// walkChain calls fn with err and every error in its chain.
func walkChain(err error, fn func(error)) {
	if err == nil {
		return
	}
	fn(err)
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		for _, ee := range e.Unwrap() {
			walkChain(ee, fn)
		}
	case interface{ Unwrap() error }:
		walkChain(e.Unwrap(), fn)
	}
}

// redactURLsInText redacts every URL in s. A URL ends at whitespace or a
// quote, unless it seems cut short within a password.
func redactURLsInText(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range urlInTextRe.FindAllStringIndex(s, -1) {
		start, end := loc[0], loc[1]
		if start < last {
			continue
		}
		if looksCutInUserinfo(s[start:end]) {
			if m := urlInTextEndRe.FindString(s[end:]); m != "" {
				end += len(m)
			}
		}
		trimmed := trimTrailingPunctuation(s[start:end])
		b.WriteString(s[last:start])
		b.WriteString(redactURLInText(trimmed))
		last = start + len(trimmed)
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// looksCutInUserinfo reports whether m ends in a ":" followed by something
// other than a port, with no "@", such as "postgres://user:pass".
func looksCutInUserinfo(m string) bool {
	_, rest, _ := strings.Cut(m, "://")
	authority := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority = rest[:i]
	}
	if strings.Contains(authority, "@") || authority != rest {
		return false
	}
	i := strings.LastIndexByte(authority, ':')
	if i < 0 || i < strings.LastIndexByte(authority, ']') {
		return false
	}
	port := authority[i+1:]
	return strings.Trim(port, "0123456789") != ""
}

// trimTrailingPunctuation removes sentence punctuation from the end of m,
// keeping a closing bracket that has an opening one, as in "[::1]".
func trimTrailingPunctuation(m string) string {
	for m != "" {
		switch c := m[len(m)-1]; c {
		case '.', ',', ':', ';':
		case ')', ']', '}':
			open := map[byte]byte{')': '(', ']': '[', '}': '{'}[c]
			if strings.Count(m, string(open)) >= strings.Count(m, string(c)) {
				return m
			}
		default:
			return m
		}
		m = m[:len(m)-1]
	}
	return m
}

func redactURLInText(m string) string {
	u, err := url.Parse(m)
	if err != nil || u.Opaque != "" {
		return schemeOnly(m)
	}
	if !strings.ContainsAny(m, "@?#") {
		return m
	}
	return URL(u)
}

type credential struct {
	value string
	// standalone credentials are removed wherever they appear, others only
	// where they appear as a value.
	standalone bool
}

// credentials returns the values in s that might be credentials: the
// userinfo password and every query or keyword value.
func credentials(s string) []credential {
	var out []credential
	add := func(v string, standalone bool, unescape func(string) (string, error)) {
		out = append(out, credential{v, standalone})
		if uv, err := unescape(v); err == nil && uv != v {
			out = append(out, credential{uv, standalone})
		}
	}
	if isKeywordDSN(s) {
		keywordValues(s, func(start, end int) {
			add(strings.Trim(s[start:end], `'"{}`), false, url.QueryUnescape)
		})
		return out
	}
	if m := userinfoCredRe(s).FindStringSubmatch(s); m != nil {
		add(m[2], true, url.PathUnescape)
	}
	if _, query, ok := strings.Cut(s, "?"); ok {
		for pair := range strings.SplitSeq(query, "&") {
			if _, v, hasValue := strings.Cut(pair, "="); hasValue {
				add(v, false, url.QueryUnescape)
			}
		}
	}
	return out
}

// redactedError does not unwrap to the original error, so that its message
// cannot be recovered, and answers errors.Is and errors.As against it.
type redactedError struct {
	msg  string
	err  error
	opts *options
}

func (e *redactedError) Error() string { return e.msg }

func (e *redactedError) Is(target error) bool { return errors.Is(e.err, target) }

// As matches target against the original chain, replacing a *url.Error with
// a redacted copy whatever the type of target.
func (e *redactedError) As(target any) bool {
	if !errors.As(e.err, target) {
		return false
	}
	v := reflect.ValueOf(target).Elem()
	if uErr, ok := v.Interface().(*url.Error); ok {
		v.Set(reflect.ValueOf(e.opts.safeURLError(uErr)))
	}
	return true
}

// redactedNetError is a redactedError of an error that reports whether it is
// a timeout or temporary, as net.Error does.
type redactedNetError struct {
	*redactedError
	timeout, temporary bool
}

func (e *redactedNetError) Timeout() bool   { return e.timeout }
func (e *redactedNetError) Temporary() bool { return e.temporary }
