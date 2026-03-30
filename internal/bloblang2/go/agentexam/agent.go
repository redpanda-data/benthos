package agentexam

import (
	"context"
	"fmt"
	"io"
)

// Agent runs a prompt against a working directory. Implementations handle
// the details of invoking a particular AI system.
type Agent interface {
	fmt.Stringer

	// Run executes the agent with the given prompt, using dir as the working
	// directory. The agent may read and write files within dir. Agent output
	// (conversation text, tool calls, etc.) is written to output. Run blocks
	// until the agent finishes or ctx is cancelled.
	Run(ctx context.Context, dir string, prompt string, output io.Writer) error
}

// AgentFunc adapts a plain function to the Agent interface.
type AgentFunc struct {
	// Fn is the function to call.
	Fn func(ctx context.Context, dir string, prompt string, output io.Writer) error

	// Label is returned by String(). If empty, defaults to "AgentFunc".
	Label string
}

// Run implements Agent.
func (a *AgentFunc) Run(ctx context.Context, dir string, prompt string, output io.Writer) error {
	return a.Fn(ctx, dir, prompt, output)
}

// String implements fmt.Stringer.
func (a *AgentFunc) String() string {
	if a.Label != "" {
		return a.Label
	}
	return "AgentFunc"
}
