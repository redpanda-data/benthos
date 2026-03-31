package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// ClaudeCode invokes the Claude Code CLI as a subprocess.
type ClaudeCode struct {
	// Command is the CLI executable name. Default: "claude".
	Command string

	// Model is passed via --model flag. Empty means use the CLI default.
	Model string

	// MaxTurns limits agent iterations. Default: 500.
	MaxTurns int

	// AllowedTools restricts which tools the agent can use. When dir is
	// non-empty the default is []string{"Read", "Write", "Glob", "Grep"};
	// when dir is empty (no-file mode) no tools are included by default.
	// Setting this field explicitly always takes precedence.
	AllowedTools []string

	// ExtraArgs are appended to the command line after all other flags.
	ExtraArgs []string

	// WaitDelay is how long to wait after SIGINT before the process is
	// killed. Default: 10s.
	WaitDelay time.Duration
}

func (c *ClaudeCode) command() string {
	if c.Command != "" {
		return c.Command
	}
	return "claude"
}

func (c *ClaudeCode) maxTurns() int {
	if c.MaxTurns != 0 {
		return c.MaxTurns
	}
	return 500
}

func (c *ClaudeCode) allowedTools(useFiles bool) []string {
	if len(c.AllowedTools) != 0 {
		return c.AllowedTools
	}
	if !useFiles {
		return nil
	}
	return []string{"Read", "Write", "Glob", "Grep"}
}

func (c *ClaudeCode) waitDelay() time.Duration {
	if c.WaitDelay != 0 {
		return c.WaitDelay
	}
	return 10 * time.Second
}

// Args returns the command-line arguments that would be passed to the CLI,
// without actually running it. Useful for testing. When dir is non-empty,
// default tools include file-based tools (Read, Write, Glob, Grep); when
// dir is empty, no default tools are included.
func (c *ClaudeCode) Args(dir, prompt string) []string {
	args := []string{
		"-p", prompt,
		"--dangerously-skip-permissions",
		"--max-turns", strconv.Itoa(c.maxTurns()),
	}
	if tools := c.allowedTools(dir != ""); len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	}
	if c.Model != "" {
		args = append([]string{"--model", c.Model}, args...)
	}
	args = append(args, c.ExtraArgs...)
	return args
}

// Run implements agentexam.Agent by spawning the Claude Code CLI.
func (c *ClaudeCode) Run(ctx context.Context, dir string, prompt string, output io.Writer) (*agentexam.RunResult, error) {
	cmdArgs := c.Args(dir, prompt)

	// Capture the subprocess stdout separately for the clean response,
	// while still streaming everything to the verbose output writer.
	var responseBuf bytes.Buffer
	stdoutWriter := io.MultiWriter(output, &responseBuf)

	cmd := exec.CommandContext(ctx, c.command(), cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = output
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = c.waitDelay()

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
func (c *ClaudeCode) String() string {
	cmd := c.command()
	if c.Model != "" {
		return fmt.Sprintf("ClaudeCode(%s, model=%s)", cmd, c.Model)
	}
	return fmt.Sprintf("ClaudeCode(%s)", cmd)
}
