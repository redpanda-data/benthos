package llmtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxToolRounds = 20

// OllamaProvider implements Provider by calling an Ollama instance via its
// /api/chat endpoint.
type OllamaProvider struct {
	// URL is the base URL of the Ollama instance (e.g. "http://localhost:11434").
	URL string
	// Model is the Ollama model to use (e.g. "llama3").
	Model string
	// Client is an optional HTTP client. If nil, http.DefaultClient is used.
	Client *http.Client
}

// Judge implements Provider.
func (o *OllamaProvider) Judge(ctx context.Context, req JudgeRequest) (*JudgeResponse, error) {
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}

	messages := []ollamaMessage{
		{Role: "system", Content: req.SystemPrompt},
		{Role: "user", Content: req.UserPrompt},
	}

	var tools []ollamaTool
	toolIndex := map[string]Tool{}
	for _, t := range req.Tools {
		tools = append(tools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
		toolIndex[t.Name] = t
	}

	responseFormat := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score": map[string]any{
				"type":        "integer",
				"description": "Score from 0 to 100.",
			},
			"notes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Reasoning notes.",
			},
		},
		"required": []string{"score", "notes"},
	}

	for range maxToolRounds {
		chatReq := ollamaChatRequest{
			Model:    o.Model,
			Messages: messages,
			Format:   responseFormat,
			Stream:   boolPtr(false),
		}
		if len(tools) > 0 {
			chatReq.Tools = tools
		}

		resp, err := o.doChat(ctx, client, chatReq, req.DebugWriter)
		if err != nil {
			return nil, err
		}

		// If no tool calls, this is the final response.
		if len(resp.Message.ToolCalls) == 0 {
			judgeResp, parseErr := parseJudgeResponse(resp.Message.Content)
			if parseErr == nil {
				return judgeResp, nil
			}

			// Retry once: ask the model to fix its output.
			messages = append(messages, resp.Message)
			messages = append(messages, ollamaMessage{
				Role:    "user",
				Content: "Your previous response was not valid JSON matching the required schema. Please respond with ONLY a JSON object containing \"score\" (integer 0-100) and \"notes\" (array of strings).",
			})

			retryReq := ollamaChatRequest{
				Model:    o.Model,
				Messages: messages,
				Format:   responseFormat,
				Stream:   boolPtr(false),
			}
			retryResp, retryErr := o.doChat(ctx, client, retryReq, req.DebugWriter)
			if retryErr != nil {
				return nil, fmt.Errorf("ollama: retry after parse failure also failed: %w (original: %w)", retryErr, parseErr)
			}
			return parseJudgeResponse(retryResp.Message.Content)
		}

		// Append the assistant message with tool calls.
		messages = append(messages, resp.Message)

		// Execute each tool call and append results.
		for _, tc := range resp.Message.ToolCalls {
			result, execErr := executeToolCall(toolIndex, tc)
			if execErr != nil {
				result = "Error: " + execErr.Error()
			}
			messages = append(messages, ollamaMessage{
				Role:    "tool",
				Content: result,
			})
		}
	}

	return nil, fmt.Errorf("ollama: exceeded maximum tool call rounds (%d)", maxToolRounds)
}

func (o *OllamaProvider) doChat(ctx context.Context, client *http.Client, chatReq ollamaChatRequest, debugWriter io.Writer) (*ollamaChatResponse, error) {
	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to marshal request: %w", err)
	}

	if debugWriter != nil {
		fmt.Fprintf(debugWriter, "=== Ollama Request ===\n%s\n", body)
	}

	url := strings.TrimRight(o.URL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to read response: %w", err)
	}

	if debugWriter != nil {
		fmt.Fprintf(debugWriter, "=== Ollama Response (status %d) ===\n%s\n", httpResp.StatusCode, respBody)
	}

	if httpResp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		return nil, fmt.Errorf("ollama: %s returned status %d: %s", url, httpResp.StatusCode, snippet)
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("ollama: failed to decode response: %w", err)
	}

	return &chatResp, nil
}

func executeToolCall(tools map[string]Tool, tc ollamaToolCall) (string, error) {
	tool, ok := tools[tc.Function.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tc.Function.Name)
	}
	return tool.Execute(tc.Function.Arguments)
}

func parseJudgeResponse(content string) (*JudgeResponse, error) {
	var resp JudgeResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("ollama: failed to parse judge response: %w (raw: %s)", err, truncate(content, 200))
	}
	if resp.Score < 0 || resp.Score > 100 {
		return nil, fmt.Errorf("ollama: score %d is out of range [0, 100]", resp.Score)
	}
	return &resp, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func boolPtr(b bool) *bool { return &b }

// Ollama API types.

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Format   map[string]any  `json:"format,omitempty"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   *bool           `json:"stream,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
}
