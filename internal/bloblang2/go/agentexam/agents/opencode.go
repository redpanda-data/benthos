package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// OpenCode invokes the OpenCode CLI as a subprocess.
type OpenCode struct {
	// Command is the CLI executable name. Default: "opencode".
	Command string

	// Model is passed via --model flag in provider/model format
	// (e.g., "anthropic/claude-sonnet-4-20250514"). Empty means use the CLI
	// default.
	Model string

	// ExtraArgs are appended to the command line after all other flags.
	ExtraArgs []string

	// WaitDelay is how long to wait after SIGINT before the process is
	// killed. Default: 10s.
	WaitDelay time.Duration
}

func (o *OpenCode) command() string {
	if o.Command != "" {
		return o.Command
	}
	return "opencode"
}

func (o *OpenCode) waitDelay() time.Duration {
	if o.WaitDelay != 0 {
		return o.WaitDelay
	}
	return 10 * time.Second
}

// Args returns the command-line arguments that would be passed to the CLI,
// without actually running it. Useful for testing.
func (o *OpenCode) Args(prompt string) []string {
	var args []string
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	args = append(args, "run")
	args = append(args, o.ExtraArgs...)
	args = append(args, "-p", prompt)
	return args
}

// Run implements agentexam.Agent by spawning the OpenCode CLI.
func (o *OpenCode) Run(ctx context.Context, dir string, prompt string, output io.Writer) (*agentexam.RunResult, error) {
	cmdArgs := o.Args(prompt)

	var responseBuf bytes.Buffer
	stdoutWriter := io.MultiWriter(output, &responseBuf)

	cmd := exec.CommandContext(ctx, o.command(), cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = output
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = o.waitDelay()

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, errors.New("agent timed out")
	}
	if err != nil {
		return nil, err
	}
	return &agentexam.RunResult{Response: responseBuf.String()}, nil
}

// String implements fmt.Stringer.
func (o *OpenCode) String() string {
	cmd := o.command()
	if o.Model != "" {
		return fmt.Sprintf("OpenCode(%s, model=%s)", cmd, o.Model)
	}
	return fmt.Sprintf("OpenCode(%s)", cmd)
}
