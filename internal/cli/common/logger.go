// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/log"
)

// CreateLogger from a CLI context and a stream config.
func CreateLogger(c *cli.Context, opts *CLIOpts, conf config.Type, streamsMode bool) (logger log.Modular, err error) {
	if overrideLogLevel := opts.RootFlags.GetLogLevel(c); overrideLogLevel != "" {
		conf.Logger.LogLevel = strings.ToUpper(overrideLogLevel)
	}

	defaultStream := opts.Stdout
	if !streamsMode && conf.Output.Type == "stdout" {
		defaultStream = opts.Stderr
	}
	if logger, err = log.New(defaultStream, ifs.OS(), conf.Logger); err != nil {
		return
	}
	if logger, err = opts.OnLoggerInit(logger); err != nil {
		return
	}
	return
}
