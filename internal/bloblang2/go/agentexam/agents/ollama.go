package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// Ollama runs an Ollama model with a tool-calling loop. When dir is non-empty
// it provides the model with filesystem tools (read_file, write_file,
// list_files, grep) scoped to the working directory. When dir is empty,
// no tools are provided and the default system prompt omits file instructions.
type Ollama struct {
	// BaseURL is the Ollama API base URL. Default: "http://localhost:11434".
	BaseURL string

	// Model is the Ollama model to use (e.g., "llama3.1", "qwen2.5").
	// Required.
	Model string

	// MaxTurns limits the number of tool-calling round trips. Default: 200.
	MaxTurns int

	// SystemPrompt is prepended to the conversation. If empty, a default
	// system prompt is used that instructs the model to use tools.
	SystemPrompt string
}

// Run implements agentexam.Agent by calling the Ollama chat API in a loop.
// When dir is empty, file tools are not provided and the system prompt omits
// file-related instructions.
func (o *Ollama) Run(ctx context.Context, dir string, prompt string, output io.Writer) (*agentexam.RunResult, error) {
	baseURL := o.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
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

	messages := []ollamaMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	var tools []ollamaTool
	if useFiles {
		tools = ollamaToolDefs()
	}

	var response strings.Builder

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := ollamaChat(ctx, baseURL, o.Model, messages, tools)
		if err != nil {
			return nil, fmt.Errorf("ollama chat (turn %d): %w", turn, err)
		}

		messages = append(messages, resp.Message)

		// Write model's text output.
		if resp.Message.Content != "" {
			fmt.Fprintf(output, "[ollama turn %d] %s\n", turn, resp.Message.Content)
			if response.Len() > 0 {
				response.WriteByte('\n')
			}
			response.WriteString(resp.Message.Content)
		}

		if len(resp.Message.ToolCalls) == 0 {
			return &agentexam.RunResult{Response: response.String()}, nil
		}

		for _, tc := range resp.Message.ToolCalls {
			fmt.Fprintf(output, "[tool] %s(%v)\n", tc.Function.Name, tc.Function.Arguments)
			result := executeToolCall(dir, tc)
			fmt.Fprintf(output, "[result] %s\n", truncate(result, 500))
			messages = append(messages, ollamaMessage{
				Role:    "tool",
				Content: result,
			})
		}
	}

	return nil, fmt.Errorf("ollama agent exceeded %d turns", maxTurns)
}

// String implements fmt.Stringer.
func (o *Ollama) String() string {
	baseURL := o.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return fmt.Sprintf("Ollama(%s, model=%s)", baseURL, o.Model)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

// --- Ollama API types ---

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// --- Tool definitions ---

func ollamaToolDefs() []ollamaTool {
	return []ollamaTool{
		{
			Type: "function",
			Function: ollamaToolFunction{
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
			Function: ollamaToolFunction{
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
			Function: ollamaToolFunction{
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
			Function: ollamaToolFunction{
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

// --- Tool execution ---

func executeToolCall(dir string, tc ollamaToolCall) string {
	switch tc.Function.Name {
	case "read_file":
		return toolReadFile(dir, tc.Function.Arguments)
	case "write_file":
		return toolWriteFile(dir, tc.Function.Arguments)
	case "list_files":
		return toolListFiles(dir, tc.Function.Arguments)
	case "grep":
		return toolGrep(dir, tc.Function.Arguments)
	default:
		return fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
	}
}

func toolReadFile(dir string, args map[string]any) string {
	path, _ := args["path"].(string)
	if path == "" {
		return "error: path is required"
	}
	absPath := filepath.Join(dir, filepath.Clean(path))
	if !strings.HasPrefix(absPath, dir+string(filepath.Separator)) {
		return "error: path escapes working directory"
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(data)
}

func toolWriteFile(dir string, args map[string]any) string {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return "error: path is required"
	}
	absPath := filepath.Join(dir, filepath.Clean(path))
	if !strings.HasPrefix(absPath, dir+string(filepath.Separator)) {
		return "error: path escapes working directory"
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return "ok"
}

func toolListFiles(dir string, args map[string]any) string {
	pattern, _ := args["pattern"].(string)

	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		if pattern != "" {
			matched, matchErr := filepath.Match(pattern, rel)
			if matchErr != nil || !matched {
				// Also try matching just the base name for simple patterns.
				matched, _ = filepath.Match(pattern, filepath.Base(rel))
				if !matched {
					return nil
				}
			}
		}
		files = append(files, rel)
		return nil
	})

	sort.Strings(files)
	if len(files) == 0 {
		return "(no files found)"
	}
	return strings.Join(files, "\n")
}

func toolGrep(dir string, args map[string]any) string {
	query, _ := args["query"].(string)
	if query == "" {
		return "error: query is required"
	}

	var matches []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, query) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
				if len(matches) >= 200 {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return "(no matches)"
	}
	return strings.Join(matches, "\n")
}

// --- HTTP ---

var ollamaHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
}

func ollamaChat(ctx context.Context, baseURL, model string, messages []ollamaMessage, tools []ollamaTool) (*ollamaChatResponse, error) {
	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ollamaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return nil, fmt.Errorf("ollama returned %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(errBody))
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &chatResp, nil
}
