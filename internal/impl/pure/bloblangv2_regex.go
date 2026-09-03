// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 regex methods that operate on a string receiver.

func init() {
	bloblangv2.MustRegisterMethod("re_replace",
		bloblangv2.NewPluginSpec().
			Category("Regex").
			Description("Alias for re_replace_all — replaces every regex match with the given value, supporting capture-group references like $1, $2.").
			Param(bloblangv2.NewStringParam("pattern").Description("Regular expression pattern.")).
			Param(bloblangv2.NewStringParam("value").Description("Replacement string. Supports $1, $2, ... capture-group references.")),
		reReplaceAllV2Ctor,
	)

	bloblangv2.MustRegisterMethod("re_find_object",
		bloblangv2.NewPluginSpec().
			Category("Regex").
			Description(`Finds the first regex match and returns it as an object whose keys are named capture groups (or numeric indices when the group has no name). Key "0" is the full match.`).
			Param(bloblangv2.NewStringParam("pattern").Description("Regular expression pattern.")),
		reFindObjectV2Ctor(false),
	)

	bloblangv2.MustRegisterMethod("re_find_all_object",
		bloblangv2.NewPluginSpec().
			Category("Regex").
			Description("Finds every regex match and returns an array of objects keyed by named capture groups (or numeric indices). Each object's `0` key is the full match.").
			Param(bloblangv2.NewStringParam("pattern").Description("Regular expression pattern.")),
		reFindObjectV2Ctor(true),
	)

	bloblangv2.MustRegisterMethod("re_find_all_submatch",
		bloblangv2.NewPluginSpec().
			Category("Regex").
			Description("Finds every regex match and returns an array of arrays — each inner array is the full match followed by its capture groups.").
			Param(bloblangv2.NewStringParam("pattern").Description("Regular expression pattern.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			re, err := compileRegexArg(args)
			if err != nil {
				return nil, err
			}
			return bloblangv2.StringMethod(func(s string) (any, error) {
				groups := re.FindAllStringSubmatch(s, -1)
				out := make([]any, 0, len(groups))
				for _, m := range groups {
					row := make([]any, len(m))
					for i, v := range m {
						row[i] = v
					}
					out = append(out, row)
				}
				return out, nil
			}), nil
		},
	)
}

func reReplaceAllV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	re, err := compileRegexArg(args)
	if err != nil {
		return nil, err
	}
	with, err := args.GetString("value")
	if err != nil {
		return nil, err
	}
	return bloblangv2.StringMethod(func(s string) (any, error) {
		return re.ReplaceAllString(s, with), nil
	}), nil
}

func reFindObjectV2Ctor(all bool) bloblangv2.MethodConstructor {
	return func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		re, err := compileRegexArg(args)
		if err != nil {
			return nil, err
		}
		groups := re.SubexpNames()
		for i, k := range groups {
			if k == "" {
				groups[i] = strconv.Itoa(i)
			}
		}
		if all {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				matches := re.FindAllStringSubmatch(s, -1)
				out := make([]any, 0, len(matches))
				for _, m := range matches {
					obj := make(map[string]any, len(groups))
					for i, v := range m {
						obj[groups[i]] = v
					}
					out = append(out, obj)
				}
				return out, nil
			}), nil
		}
		return bloblangv2.StringMethod(func(s string) (any, error) {
			match := re.FindStringSubmatch(s)
			obj := make(map[string]any, len(groups))
			for i, v := range match {
				obj[groups[i]] = v
			}
			return obj, nil
		}), nil
	}
}

func compileRegexArg(args *bloblangv2.ParsedParams) (*regexp.Regexp, error) {
	pat, err := args.GetString("pattern")
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return re, nil
}
