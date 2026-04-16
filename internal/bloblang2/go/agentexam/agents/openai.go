package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// OpenAI runs a model via the OpenAI-compatible /v1/chat/completions API.
// This works with llama-server, vLLM, and any other OpenAI-compatible endpoint.
// When dir is non-empty it provides the model with filesystem tools.
type OpenAI struct {
	// BaseURL is the API base URL (e.g., "http://localhost:8080").
	// Required — there is no default.
	BaseURL string

	// Model is the model identifier passed in the request.
	// May be empty if the server only serves one model.
	Model string

	// MaxTurns limits the number of tool-calling round trips. Default: 200.
	MaxTurns int

	// SystemPrompt is prepended to the conversation. If empty, a default
	// system prompt is used.
	SystemPrompt string
}

// Run implements agentexam.Agent.
func (o *OpenAI) Run(ctx context.Context, dir string, prompt string, output io.Writer) (*agentexam.RunResult, error) {
	if o.BaseURL == "" {
		return nil, errors.New("openai agent: base_url is required")
	}
	maxTurns := o.MaxTurns
	if maxTurns == 0 {
		maxTurns = 200
	}

	useFiles := dir != ""

	systemPrompt := o.SystemPrompt
	if systemPrompt == "" {
		if useFiles {
			systemPrompt = "You are a coding agent that completes tasks by calling tools. You MUST call tools to accomplish your task. Read files with read_file, write files with write_file, list files with list_files, and search with grep. Keep calling tools until the task is fully complete. Only stop calling tools when you have finished all work."
		} else {
			systemPrompt = "You are a helpful assistant. Answer the user's prompt directly."
		}
	}

	messages := []oaiMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	var tools []oaiTool
	if useFiles {
		tools = oaiToolDefs()
	}

	var response strings.Builder

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := oaiChat(ctx, o.BaseURL, o.Model, messages, tools)
		if err != nil {
			return nil, fmt.Errorf("openai chat (turn %d): %w", turn, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("openai chat (turn %d): no choices in response", turn)
		}
		msg := resp.Choices[0].Message

		// Append assistant message to history.
		messages = append(messages, msg)

		if msg.Content != "" {
			fmt.Fprintf(output, "[openai turn %d] %s\n", turn, msg.Content)
			if response.Len() > 0 {
				response.WriteByte('\n')
			}
			response.WriteString(msg.Content)
		}

		if len(msg.ToolCalls) == 0 {
			return &agentexam.RunResult{Response: response.String()}, nil
		}

		for _, tc := range msg.ToolCalls {
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

			fmt.Fprintf(output, "[tool] %s(%v)\n", tc.Function.Name, args)
			result := executeToolCall(dir, ollamaToolCall{
				Function: ollamaFunctionCall{Name: tc.Function.Name, Arguments: args},
			})
			fmt.Fprintf(output, "[result] %s\n", truncate(result, 500))

			messages = append(messages, oaiMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return nil, fmt.Errorf("openai agent exceeded %d turns", maxTurns)
}

// String implements fmt.Stringer.
func (o *OpenAI) String() string {
	model := o.Model
	if model == "" {
		model = "(default)"
	}
	return fmt.Sprintf("OpenAI(%s, model=%s)", o.BaseURL, model)
}

// --- OpenAI API types ---

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiFunctionCall `json:"function"`
}

type oaiFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type oaiChatRequest struct {
	Model    string       `json:"model,omitempty"`
	Messages []oaiMessage `json:"messages"`
	Tools    []oaiTool    `json:"tools,omitempty"`
}

type oaiChatResponse struct {
	Choices []oaiChoice `json:"choices"`
}

type oaiChoice struct {
	Message oaiMessage `json:"message"`
}

type oaiTool struct {
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// --- Tool definitions ---

func oaiToolDefs() []oaiTool {
	return []oaiTool{
		{
			Type: "function",
			Function: oaiToolFunction{
				Name:        "read_file",
				Description: "Read the contents of a file at the given relative path.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Relative file path to read",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: oaiToolFunction{
				Name:        "write_file",
				Description: "Write content to a file at the given relative path. Creates parent directories as needed.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Relative file path to write",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Content to write to the file",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: oaiToolFunction{
				Name:        "list_files",
				Description: "List all files in the working directory, optionally filtered by a glob pattern (e.g., \"*.json\", \"tests/**/*.txt\").",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{
							"type":        "string",
							"description": "Optional glob pattern to filter files. Empty means all files.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: oaiToolFunction{
				Name:        "grep",
				Description: "Search for a substring in all files under the working directory. Returns matching file paths and line contents.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "The substring to search for",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

// --- HTTP ---

var oaiHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
}

func oaiChat(ctx context.Context, baseURL, model string, messages []oaiMessage, tools []oaiTool) (*oaiChatResponse, error) {
	reqBody := oaiChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	}
	if len(tools) == 0 {
		reqBody.Tools = nil
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := oaiHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling openai-compatible API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("API returned %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(errBody))
	}

	var chatResp oaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &chatResp, nil
}
