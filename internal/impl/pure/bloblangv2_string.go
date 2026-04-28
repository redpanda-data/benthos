// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

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

	bloblangv2.MustRegisterMethod("replace",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Replaces all occurrences of a substring with another string. Equivalent to replace_all, retained for V1 parity.`).
			Param(bloblangv2.NewStringParam("old").Description("A string to match against.")).
			Param(bloblangv2.NewStringParam("new").Description("A string to replace with.")),
		replaceAllV2Ctor,
	)

	bloblangv2.MustRegisterMethod("replace_many",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Performs multiple find-and-replace operations in sequence using an array of [old, new] pairs (alternating elements).").
			Param(bloblangv2.NewAnyParam("values").Description("An array of strings — each even-indexed entry is replaced with the following odd-indexed entry.")),
		replaceManyV2Ctor,
	)

	bloblangv2.MustRegisterMethod("replace_all_many",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Performs multiple find-and-replace operations in sequence. Equivalent to replace_many, retained for V1 parity.").
			Param(bloblangv2.NewAnyParam("values").Description("An array of strings — each even-indexed entry is replaced with the following odd-indexed entry.")),
		replaceManyV2Ctor,
	)

	bloblangv2.MustRegisterMethod("filepath_join",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Combines an array of path components into a single OS-specific file path."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.ArrayMethod(func(arr []any) (any, error) {
				parts := make([]string, 0, len(arr))
				for i, ele := range arr {
					s, ok := ele.(string)
					if !ok {
						return nil, fmt.Errorf("path element %d: expected string, got %T", i, ele)
					}
					parts = append(parts, s)
				}
				return filepath.Join(parts...), nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("filepath_split",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description("Separates a file path into a [directory, filename] pair."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				dir, file := filepath.Split(s)
				return []any{dir, file}, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("parse_url",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Parses a URL string into a structured result with fields scheme, host, path, raw_query, fragment, etc., and an optional user object."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				u, err := url.Parse(s)
				if err != nil {
					return nil, err
				}
				out := map[string]any{
					"scheme":       u.Scheme,
					"opaque":       u.Opaque,
					"host":         u.Host,
					"path":         u.Path,
					"raw_path":     u.RawPath,
					"raw_query":    u.RawQuery,
					"fragment":     u.Fragment,
					"raw_fragment": u.RawFragment,
				}
				if u.User != nil {
					user := map[string]any{"name": u.User.Username()}
					if pass, ok := u.User.Password(); ok {
						user["password"] = pass
					}
					out["user"] = user
				}
				return out, nil
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("format",
		bloblangv2.NewPluginSpec().
			Category("Strings").
			Description(`Formats the receiver string with Go's printf-style verbs (%s, %d, %v, ...) using the supplied argument array. V2 takes a single array argument because variadic parameters are not part of the V2 spec.`).
			Param(bloblangv2.NewAnyParam("args").Description("Array of arguments to substitute into the format verbs.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			raw, err := args.Get("args")
			if err != nil {
				return nil, err
			}
			vals, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("expected an array of format arguments, got %T", raw)
			}
			return bloblangv2.StringMethod(func(format string) (any, error) {
				return fmt.Sprintf(format, vals...), nil
			}), nil
		},
	)
}

func replaceAllV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	oldStr, err := args.GetString("old")
	if err != nil {
		return nil, err
	}
	newStr, err := args.GetString("new")
	if err != nil {
		return nil, err
	}
	return bloblangv2.StringMethod(func(s string) (any, error) {
		return strings.ReplaceAll(s, oldStr, newStr), nil
	}), nil
}

func replaceManyV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	raw, err := args.Get("values")
	if err != nil {
		return nil, err
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array argument, got %T", raw)
	}
	if len(items)%2 != 0 {
		return nil, fmt.Errorf("invalid arg, replacements should be in [old, new] pairs and must therefore be even: %v", items)
	}
	pairs := make([]string, 0, len(items))
	for i, ele := range items {
		s, ok := ele.(string)
		if !ok {
			return nil, fmt.Errorf("replacement value at index %d: expected string, got %T", i, ele)
		}
		pairs = append(pairs, s)
	}
	rep := strings.NewReplacer(pairs...)
	return bloblangv2.StringMethod(func(s string) (any, error) {
		return rep.Replace(s), nil
	}), nil
}
