package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeCodeArgs(t *testing.T) {
	tests := []struct {
		name   string
		cc     ClaudeCode
		dir    string
		prompt string
		want   []string
	}{
		{
			name:   "defaults with files",
			cc:     ClaudeCode{},
			dir:    "/tmp/work",
			prompt: "do stuff",
			want: []string{
				"-p", "do stuff",
				"--dangerously-skip-permissions",
				"--max-turns", "500",
				"--allowedTools", "Read,Write,Glob,Grep",
			},
		},
		{
			name:   "defaults no files",
			cc:     ClaudeCode{},
			dir:    "",
			prompt: "do stuff",
			want: []string{
				"-p", "do stuff",
				"--dangerously-skip-permissions",
				"--max-turns", "500",
			},
		},
		{
			name:   "with model",
			cc:     ClaudeCode{Model: "sonnet"},
			dir:    "/tmp/work",
			prompt: "test",
			want: []string{
				"--model", "sonnet",
				"-p", "test",
				"--dangerously-skip-permissions",
				"--max-turns", "500",
				"--allowedTools", "Read,Write,Glob,Grep",
			},
		},
		{
			name: "custom tools and turns",
			cc: ClaudeCode{
				MaxTurns:     100,
				AllowedTools: []string{"Read", "Grep"},
				ExtraArgs:    []string{"--verbose"},
			},
			dir:    "/tmp/work",
			prompt: "go",
			want: []string{
				"-p", "go",
				"--dangerously-skip-permissions",
				"--max-turns", "100",
				"--allowedTools", "Read,Grep",
				"--verbose",
			},
		},
		{
			name: "explicit tools override no-files default",
			cc: ClaudeCode{
				AllowedTools: []string{"WebSearch"},
			},
			dir:    "",
			prompt: "search",
			want: []string{
				"-p", "search",
				"--dangerously-skip-permissions",
				"--max-turns", "500",
				"--allowedTools", "WebSearch",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cc.Args(tc.dir, tc.prompt)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("args mismatch:\n  got:  %v\n  want: %v", got, tc.want)
			}
		})
	}
}

func TestToolReadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := toolReadFile(dir, map[string]any{"path": "hello.txt"})
	if got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}

	got = toolReadFile(dir, map[string]any{"path": "missing.txt"})
	if !strings.HasPrefix(got, "error:") {
		t.Errorf("expected error for missing file, got: %q", got)
	}
}

func TestToolReadFileEscapePrevention(t *testing.T) {
	dir := t.TempDir()
	got := toolReadFile(dir, map[string]any{"path": "../../etc/passwd"})
	if !strings.Contains(got, "error") {
		t.Errorf("expected error for path escape, got: %q", got)
	}
}

func TestToolWriteFile(t *testing.T) {
	dir := t.TempDir()

	got := toolWriteFile(dir, map[string]any{"path": "sub/out.txt", "content": "hello"})
	if got != "ok" {
		t.Fatalf("expected ok, got: %s", got)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q", data)
	}
}

func TestToolWriteFileEscapePrevention(t *testing.T) {
	dir := t.TempDir()
	got := toolWriteFile(dir, map[string]any{"path": "../../evil.txt", "content": "bad"})
	if !strings.Contains(got, "error") {
		t.Errorf("expected error for path escape, got: %q", got)
	}
}

func TestToolListFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	// All files.
	got := toolListFiles(dir, map[string]any{})
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 files, got: %q", got)
	}

	// With pattern.
	got = toolListFiles(dir, map[string]any{"pattern": "*.txt"})
	if !strings.Contains(got, "a.txt") {
		t.Errorf("expected a.txt in filtered results: %q", got)
	}
	if strings.Contains(got, "b.json") {
		t.Errorf("b.json should be filtered out: %q", got)
	}
}

func TestToolGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\ngoodbye world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("nothing here"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := toolGrep(dir, map[string]any{"query": "world"})
	if !strings.Contains(got, "a.txt:1:") {
		t.Errorf("expected a.txt:1 match: %q", got)
	}
	if !strings.Contains(got, "a.txt:2:") {
		t.Errorf("expected a.txt:2 match: %q", got)
	}
	if strings.Contains(got, "b.txt") {
		t.Errorf("b.txt should not match: %q", got)
	}

	got = toolGrep(dir, map[string]any{"query": "zzzzz"})
	if got != "(no matches)" {
		t.Errorf("expected no matches, got: %q", got)
	}
}

func TestToolListFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	got := toolListFiles(dir, map[string]any{})
	if got != "(no files found)" {
		t.Errorf("expected no files found, got: %q", got)
	}
}

func TestClaudeCodeString(t *testing.T) {
	tests := []struct {
		name string
		cc   ClaudeCode
		want string
	}{
		{name: "defaults", cc: ClaudeCode{}, want: "ClaudeCode(claude)"},
		{name: "custom command", cc: ClaudeCode{Command: "my-claude"}, want: "ClaudeCode(my-claude)"},
		{name: "with model", cc: ClaudeCode{Model: "opus"}, want: "ClaudeCode(claude, model=opus)"},
		{name: "custom both", cc: ClaudeCode{Command: "cc", Model: "haiku"}, want: "ClaudeCode(cc, model=haiku)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cc.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenCodeArgs(t *testing.T) {
	tests := []struct {
		name   string
		oc     OpenCode
		prompt string
		want   []string
	}{
		{
			name:   "defaults",
			oc:     OpenCode{},
			prompt: "do stuff",
			want: []string{
				"run",
				"-p", "do stuff",
			},
		},
		{
			name:   "with model",
			oc:     OpenCode{Model: "anthropic/claude-sonnet-4-20250514"},
			prompt: "test",
			want: []string{
				"--model", "anthropic/claude-sonnet-4-20250514",
				"run",
				"-p", "test",
			},
		},
		{
			name:   "extra args",
			oc:     OpenCode{ExtraArgs: []string{"--print-logs"}},
			prompt: "go",
			want: []string{
				"run",
				"--print-logs",
				"-p", "go",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.oc.Args(tc.prompt)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("args mismatch:\n  got:  %v\n  want: %v", got, tc.want)
			}
		})
	}
}

func TestOpenCodeString(t *testing.T) {
	tests := []struct {
		name string
		oc   OpenCode
		want string
	}{
		{name: "defaults", oc: OpenCode{}, want: "OpenCode(opencode)"},
		{name: "custom command", oc: OpenCode{Command: "my-oc"}, want: "OpenCode(my-oc)"},
		{name: "with model", oc: OpenCode{Model: "anthropic/opus"}, want: "OpenCode(opencode, model=anthropic/opus)"},
		{name: "custom both", oc: OpenCode{Command: "oc", Model: "openai/gpt-4"}, want: "OpenCode(oc, model=openai/gpt-4)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.oc.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOllamaString(t *testing.T) {
	o := Ollama{Model: "llama3.1"}
	want := "Ollama(http://localhost:11434, model=llama3.1)"
	if got := o.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	o2 := Ollama{BaseURL: "http://example.com:1234", Model: "qwen"}
	want2 := "Ollama(http://example.com:1234, model=qwen)"
	if got := o2.String(); got != want2 {
		t.Errorf("got %q, want %q", got, want2)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("got %q", got)
	}
	if got := truncate("exact", 5); got != "exact" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteToolCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		contains string
	}{
		{name: "read", toolName: "read_file", args: map[string]any{"path": "test.txt"}, contains: "data"},
		{name: "write", toolName: "write_file", args: map[string]any{"path": "out.txt", "content": "hi"}, contains: "ok"},
		{name: "list", toolName: "list_files", args: map[string]any{}, contains: "test.txt"},
		{name: "grep", toolName: "grep", args: map[string]any{"query": "data"}, contains: "test.txt"},
		{name: "unknown", toolName: "nope", args: map[string]any{}, contains: "unknown tool"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := executeToolCall(dir, ollamaToolCall{
				Function: ollamaFunctionCall{Name: tc.toolName, Arguments: tc.args},
			})
			if !strings.Contains(got, tc.contains) {
				t.Errorf("got %q, want substring %q", got, tc.contains)
			}
		})
	}
}

func TestToolReadFileEmptyPath(t *testing.T) {
	got := toolReadFile(t.TempDir(), map[string]any{})
	if !strings.Contains(got, "error") {
		t.Errorf("expected error for empty path, got: %q", got)
	}
}

func TestToolWriteFileEmptyPath(t *testing.T) {
	got := toolWriteFile(t.TempDir(), map[string]any{"content": "x"})
	if !strings.Contains(got, "error") {
		t.Errorf("expected error for empty path, got: %q", got)
	}
}

func TestToolGrepEmptyQuery(t *testing.T) {
	got := toolGrep(t.TempDir(), map[string]any{})
	if !strings.Contains(got, "error") {
		t.Errorf("expected error for empty query, got: %q", got)
	}
}

func TestOllamaChatSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		resp := ollamaChatResponse{
			Message: ollamaMessage{Role: "assistant", Content: "hello"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	got, err := ollamaChat(context.Background(), srv.URL, "test-model", []ollamaMessage{
		{Role: "user", Content: "hi"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hello" {
		t.Errorf("got %q", got.Message.Content)
	}
}

func TestOllamaChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("something broke"))
	}))
	defer srv.Close()

	_, err := ollamaChat(context.Background(), srv.URL, "test-model", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "something broke") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOllamaRunNoToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := ollamaChatResponse{
			Message: ollamaMessage{Role: "assistant", Content: "done"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	o := &Ollama{BaseURL: srv.URL, Model: "test"}
	var buf strings.Builder
	result, err := o.Run(context.Background(), t.TempDir(), "do nothing", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "done") {
		t.Errorf("output missing model response: %q", buf.String())
	}
	if result.Response != "done" {
		t.Errorf("response: got %q, want %q", result.Response, "done")
	}
}

func TestOllamaRunWithToolCalls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var resp ollamaChatResponse
		if call == 0 {
			resp.Message = ollamaMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{{
					Function: ollamaFunctionCall{
						Name:      "read_file",
						Arguments: map[string]any{"path": "input.txt"},
					},
				}},
			}
		} else {
			resp.Message = ollamaMessage{Role: "assistant", Content: "all done"}
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	o := &Ollama{BaseURL: srv.URL, Model: "test"}
	var buf strings.Builder
	result, err := o.Run(context.Background(), dir, "read the file", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "read_file") {
		t.Errorf("output missing tool call: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output missing tool result: %q", buf.String())
	}
	if result.Response != "all done" {
		t.Errorf("response: got %q, want %q", result.Response, "all done")
	}
}

func TestOllamaRunMaxTurns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{{
					Function: ollamaFunctionCall{
						Name:      "list_files",
						Arguments: map[string]any{},
					},
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	o := &Ollama{BaseURL: srv.URL, Model: "test", MaxTurns: 3}
	var buf strings.Builder
	_, err := o.Run(context.Background(), t.TempDir(), "loop forever", &buf)
	if err == nil || !strings.Contains(err.Error(), "exceeded 3 turns") {
		t.Errorf("expected max turns error, got: %v", err)
	}
}
