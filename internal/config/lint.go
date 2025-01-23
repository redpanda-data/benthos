// Copyright 2025 Redpanda Data, Inc.

package config

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
	"unicode/utf8"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// ReadYAMLFileLinted will attempt to read a configuration file path into a
// structure. Returns an array of lint messages or an error.
func (r *Reader) ReadYAMLFileLinted(ctx context.Context, spec docs.FieldSpecs, path string, skipEnvVarCheck bool, lConf docs.LintConfig) (Type, []docs.Lint, error) {
	configBytes, lints, _, err := r.ReadFileEnvSwap(ctx, path)
	if err != nil {
		return Type{}, nil, err
	}

	if skipEnvVarCheck {
		var newLints []docs.Lint
		for _, l := range lints {
			if l.Type != docs.LintMissingEnvVar {
				newLints = append(newLints, l)
			}
		}
		lints = newLints
	}

	cNode, err := docs.UnmarshalYAML(configBytes)
	if err != nil {
		return Type{}, nil, err
	}

	var rawSource any
	_ = cNode.Decode(&rawSource)

	var pConf *docs.ParsedConfig
	if pConf, err = spec.ParsedConfigFromAny(cNode); err != nil {
		return Type{}, nil, err
	}

	conf, err := FromParsed(lConf.DocsProvider, pConf, rawSource)
	if err != nil {
		return Type{}, nil, err
	}

	if !bytes.HasPrefix(configBytes, []byte("# BENTHOS LINT DISABLE")) {
		lints = append(lints, spec.LintYAML(docs.NewLintContext(lConf), cNode)...)
	}
	return conf, lints, nil
}

// LintYAMLBytes attempts to report errors within a user config. Returns a slice of
// lint results.
func LintYAMLBytes(lintConf docs.LintConfig, rawBytes []byte) ([]docs.Lint, error) {
	if bytes.HasPrefix(rawBytes, []byte("# BENTHOS LINT DISABLE")) {
		return nil, nil
	}

	rawNode, err := docs.UnmarshalYAML(rawBytes)
	if err != nil {
		return nil, err
	}

	return Spec().LintYAML(docs.NewLintContext(lintConf), rawNode), nil
}

// ReadFileEnvSwap reads a file and replaces any environment variable
// interpolations before returning the contents. Linting errors are returned if
// the file has an unexpected higher level format, such as invalid utf-8
// encoding.
//
// An modTime timestamp is returned if the modtime of the file is available.
func (r *Reader) ReadFileEnvSwap(ctx context.Context, path string) (configBytes []byte, lints []docs.Lint, modTime time.Time, err error) {
	var configFile fs.File
	if configFile, err = r.fs.Open(path); err != nil {
		return
	}

	if info, ierr := configFile.Stat(); ierr == nil {
		modTime = info.ModTime()
	}

	if configBytes, err = io.ReadAll(configFile); err != nil {
		return
	}

	if !utf8.Valid(configBytes) {
		lints = append(lints, docs.NewLintError(
			1, docs.LintFailedRead,
			errors.New("detected invalid utf-8 encoding in config, this may result in interpolation functions not working as expected"),
		))
	}

	if configBytes, err = r.ReplaceEnvVariables(ctx, configBytes); err != nil {
		var errEnvMissing *ErrMissingEnvVars
		if errors.As(err, &errEnvMissing) {
			configBytes = errEnvMissing.BestAttempt
			lints = append(lints, docs.NewLintError(1, docs.LintMissingEnvVar, err))
			err = nil
		} else {
			return
		}
	}
	return
}
