// specagent is a utility for testing a coding agent's ability to understand
// the Bloblang V2 specification. It generates isolated "clean rooms" from
// the spec test suite and evaluates agent-produced outputs.
//
// Usage:
//
//	specagent prepare --output <dir>     Generate clean rooms from spec tests
//	specagent run     --dir <dir>        Run a coding agent in each clean room
//	specagent evaluate --dir <dir>       Score agent results and print a table
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "prepare":
		cmdPrepare(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "evaluate":
		cmdEvaluate(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `specagent — test a coding agent's Bloblang V2 comprehension

Usage:
  specagent prepare  --output <dir>  [--spec <dir>] [--tests <dir>]
  specagent run      --dir <dir>     [--mode predict-output|predict-mapping|both]
  specagent evaluate --dir <dir>     [--mode predict-output|predict-mapping|both] [--verbose]

Subcommands:
  prepare    Generate clean room directories from spec tests
  run        Invoke a coding agent in each clean room
  evaluate   Score agent outputs and print a results table

Run 'specagent <subcommand> --help' for subcommand-specific flags.
`)
}
