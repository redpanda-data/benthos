// Copyright 2025 Redpanda Data, Inc.

package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	envRegex        = regexp.MustCompile(`\${[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?(?:\|[0-9A-Za-z]+)?}`)
	escapedEnvRegex = regexp.MustCompile(`\${({[0-9A-Za-z_.]+(:((\${[^}]+})|[^}])*)?})}`)
)

// ErrMissingEnvVars is returned when attempting environment variable
// interpolations where the referenced environment variables are missing.
type ErrMissingEnvVars struct {
	Variables []string

	// Our best attempt at parsing the config that's missing variables by simply
	// inserting an empty string. There's a good chance this is still a valid
	// config! :)
	BestAttempt []byte
}

// Error returns a rather sweet error message.
func (e *ErrMissingEnvVars) Error() string {
	// TODO: Deduplicate the variables as they might be repeated.
	return fmt.Sprintf("required environment variables were not set: %v", e.Variables)
}

// ReplaceEnvVariables will search a blob of data for the pattern
// `${FOO:bar|func}`, where `FOO` is an environment
// variable name, `bar` is an optional default value and `func` is an optional
// transform function. The `bar` section (including the colon) can be omitted if
// there is no appropriate default value for the field. The `func` section
// (including the pipe) can be omitted if no transformation is required.
//
// For each aforementioned pattern found in the blob, the contents of the
// respective environment variable will be read and, optionally, transformed,
// and will replace the pattern. If the environment variable is empty or does
// not exist then either the default value is used or the field will be left
// empty.
func (r *Reader) ReplaceEnvVariables(ctx context.Context, inBytes []byte) (replaced []byte, err error) {
	var missingVarsErr ErrMissingEnvVars

	var errs []error
	replaced = envRegex.ReplaceAllFunc(inBytes, func(content []byte) []byte {
		var value string
		var ok bool
		if len(content) > 3 {
			var funcName string
			if colonIndex := bytes.IndexByte(content, ':'); colonIndex == -1 {
				envVarSpecifier := content[2 : len(content)-1]

				varName := envVarSpecifier
				if pipeIndex := bytes.LastIndex(envVarSpecifier, []byte{'|'}); pipeIndex != -1 {
					varName = envVarSpecifier[:pipeIndex]
					funcName = string(envVarSpecifier[pipeIndex+1:])
				}

				if value, ok = r.envLookupFunc(ctx, string(varName)); !ok {
					missingVarsErr.Variables = append(missingVarsErr.Variables, string(varName))
				}
			} else {
				varName := content[2:colonIndex]
				remaining := content[colonIndex+1 : len(content)-1]
				defaultVal := remaining
				if pipeIndex := bytes.LastIndex(remaining, []byte{'|'}); pipeIndex != -1 {
					defaultVal = remaining[:pipeIndex]
					funcName = string(remaining[pipeIndex+1:])
				}

				value, _ = r.envLookupFunc(ctx, string(varName))
				if value == "" {
					value = string(defaultVal)
				}
			}

			var err error
			switch funcName {
			case "base64decode":
				var decoded []byte
				if decoded, err = base64.StdEncoding.DecodeString(value); err != nil {
					err = fmt.Errorf("failed to decode base64-encoded env var: %s", err)
				} else {
					value = string(decoded)
				}
			default:
				if funcName != "" {
					err = fmt.Errorf("unknown env var decode function: %s", funcName)
				}
			}
			if err != nil {
				errs = append(errs, err)
			}

			// Escape newlines, otherwise there's no way that they would work
			// within a config.
			value = strings.ReplaceAll(value, "\n", "\\n")
		}
		return []byte(value)
	})
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	replaced = escapedEnvRegex.ReplaceAll(replaced, []byte("$$$1"))

	if len(missingVarsErr.Variables) > 0 {
		missingVarsErr.BestAttempt = replaced
		err = &missingVarsErr
		replaced = nil
	}
	return
}
