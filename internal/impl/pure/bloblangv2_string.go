// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"html"
	"net/url"
	"strconv"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 string methods that don't depend on outside state. The
// public/bloblangv2 typed wrappers are strict about receiver types — callers
// whose upstream value isn't already a string should chain .string() first.
// See PARITY.md for the broader plan.

func init() {
	bloblangv2.MustRegisterMethod("capitalize",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Converts the first letter of each word in a string to uppercase (title case)."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			titler := cases.Title(language.English)
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return titler.String(s), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("escape_html",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Escapes special HTML characters ("<", ">", "&", "'", "\"") to make a string safe for HTML output.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return html.EscapeString(s), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("unescape_html",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Converts HTML entities back to their original characters. Handles named, decimal, and hexadecimal entities."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return html.UnescapeString(s), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("escape_url_query",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Encodes a string for safe use in URL query parameters. Converts spaces to "+" and special characters to percent-encoded values.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return url.QueryEscape(s), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("unescape_url_query",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Decodes URL query parameter encoding, converting "+" to spaces and percent-encoded characters to their original values.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return url.QueryUnescape(s)
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("quote",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Wraps a string in double quotes and escapes special characters (newlines, tabs, etc.) using Go escape sequences."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return strconv.Quote(s), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("unquote",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Removes surrounding quotes and interprets escape sequences (\n, \t, etc.) to their literal characters.`),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				return strconv.Unquote(s)
			}), nil
		},
	)
}
