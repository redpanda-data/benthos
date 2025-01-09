// Copyright 2025 Redpanda Data, Inc.

// Package servicetest provides functions and utilities that might be useful for
// testing custom Benthos builds.
package servicetest

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/redpanda-data/benthos/v4/internal/cli"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
)

// RunCLIWithArgs executes Benthos as a CLI with an explicit set of arguments.
// This is useful for testing commands without needing to modify os.Args.
//
// This call blocks until either:
//
// 1. The service shuts down gracefully due to the inputs closing
// 2. A termination signal is received
// 3. The provided context has a deadline that is reached, triggering graceful termination
// 4. The provided context is cancelled (WARNING, this prevents graceful termination)
//
// Deprecated: Use the service.CLIOptSetArgs opt func instead.
func RunCLIWithArgs(ctx context.Context, args ...string) {
	if err := cli.App(common.NewCLIOpts(cli.Version, cli.DateBuilt)).RunContext(ctx, args); err != nil {
		var cerr *common.ErrExitCode
		if errors.As(err, &cerr) {
			os.Exit(cerr.Code)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
