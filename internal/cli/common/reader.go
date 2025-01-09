// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"

	"github.com/urfave/cli/v2"
)

// ReadConfig attempts to read a general service wide config via a returned
// config.Reader based on input CLI flags. This includes applying any config
// overrides expressed by the --set flag.
func ReadConfig(c *cli.Context, cliOpts *CLIOpts, streamsMode bool) (mainPath string, inferred bool, conf *config.Reader) {
	path := cliOpts.RootFlags.GetConfig(c)
	if path == "" {
		// Iterate default config paths
		for _, dpath := range cliOpts.ConfigSearchPaths {
			if _, err := ifs.OS().Stat(dpath); err == nil {
				inferred = true
				path = dpath
				break
			}
		}
	}

	lintConf := docs.NewLintConfig(cliOpts.Environment)

	opts := []config.OptFunc{
		config.OptSetFullSpec(cliOpts.MainConfigSpecCtor),
		config.OptAddOverrides(cliOpts.RootFlags.GetSet(c)...),
		config.OptTestSuffix("_benthos_test"),
		config.OptSetLintConfig(lintConf),
	}
	if streamsMode {
		opts = append(opts, config.OptSetStreamPaths(c.Args().Slice()...))
	}
	if cliOpts.SecretAccessFn != nil {
		opts = append(opts, config.OptUseEnvLookupFunc(cliOpts.SecretAccessFn))
	}
	return path, inferred, config.NewReader(path, cliOpts.RootFlags.GetResources(c), opts...)
}
