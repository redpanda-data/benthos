package agentexam

import (
	"context"
	"fmt"
	"io"
)

// RunResult holds the structured output of an agent run.
type RunResult struct {
	// Response is the agent's actual response content, free of logging
	// prefixes, tool-call traces, and other framing. This is the text that
	// should be used for extraction and scoring.
	Response string
}

// Agent runs a prompt and writes its output. Implementations handle the
// details of invoking a particular AI system.
type Agent interface {
	fmt.Stringer

	// Run executes the agent with the given prompt. When dir is non-empty it
	// is a working directory containing exam files; the agent may read and
	// write files within it, and implementations should configure file-based
	// tools accordingly. When dir is empty, no working directory has been
	// prepared and implementations should omit file-related tools and
	// prompts. Verbose output (conversation text, tool calls, etc.) is
	// written to output. The returned RunResult contains the agent's clean
	// response text. Run blocks until the agent finishes or ctx is cancelled.
	Run(ctx context.Context, dir string, prompt string, output io.Writer) (*RunResult, error)
}

// AgentFunc adapts a plain function to the Agent interface.
type AgentFunc struct {
	// Fn is the function to call.
	Fn func(ctx context.Context, dir string, prompt string, output io.Writer) (*RunResult, error)

	// Label is returned by String(). If empty, defaults to "AgentFunc".
	Label string
}

// Run implements Agent.
func (a *AgentFunc) Run(ctx context.Context, dir string, prompt string, output io.Writer) (*RunResult, error) {
	return a.Fn(ctx, dir, prompt, output)
}

// String implements fmt.Stringer.
func (a *AgentFunc) String() string {
	if a.Label != "" {
		return a.Label
	}
	return "AgentFunc"
}
