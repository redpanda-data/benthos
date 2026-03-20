package llmtest

import (
	"context"
	"io"
)

// Provider is the interface that LLM backends must implement. This is the
// extension point for adding new backends. Providers own the full tool-call
// loop — they know their wire format best. The core package provides portable
// tool specs (Tool) that providers adapt to their native mechanism.
type Provider interface {
	// Judge sends the assembled prompt (system + user) and optional tools to
	// the LLM and returns the structured response. The provider is responsible
	// for registering any tools with the model's native tool-calling mechanism
	// and handling tool call rounds until the model produces a final response.
	Judge(ctx context.Context, req JudgeRequest) (*JudgeResponse, error)
}

// JudgeRequest is the input to a single judge invocation.
type JudgeRequest struct {
	SystemPrompt string
	UserPrompt   string
	// Tools to register with the model, if any.
	Tools []Tool
	// DebugWriter, if set, receives the full LLM exchange (prompts, tool
	// calls, responses) for debugging. Nil means no debug output.
	DebugWriter io.Writer
}

// JudgeResponse is the structured output from a single judge invocation.
type JudgeResponse struct {
	Score int      `json:"score"`
	Notes []string `json:"notes"`
}
