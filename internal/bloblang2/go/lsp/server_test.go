package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// lspMessage builds a JSON-RPC message with Content-Length header.
func lspMessage(msg jsonrpcMessage) []byte {
	body, _ := json.Marshal(msg)
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}

func requestMsg(id int, method string, params any) jsonrpcMessage {
	raw, _ := json.Marshal(params)
	return jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", id)),
		Method:  method,
		Params:  raw,
	}
}

func notifyMsg(method string, params any) jsonrpcMessage {
	raw, _ := json.Marshal(params)
	return jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	}
}

// parseResponses reads all JSON-RPC messages from a buffer.
func parseResponses(data []byte) ([]jsonrpcMessage, error) {
	var msgs []jsonrpcMessage
	r := bytes.NewReader(data)
	for r.Len() > 0 {
		// Read Content-Length header.
		var header string
		for {
			b, err := r.ReadByte()
			if err != nil {
				if err == io.EOF && len(msgs) > 0 {
					return msgs, nil
				}
				return msgs, err
			}
			header += string(b)
			if strings.HasSuffix(header, "\r\n\r\n") {
				break
			}
		}

		var contentLength int
		for _, line := range strings.Split(header, "\r\n") {
			if strings.HasPrefix(line, "Content-Length:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				if _, err := fmt.Sscanf(val, "%d", &contentLength); err != nil {
					return msgs, fmt.Errorf("invalid Content-Length: %w", err)
				}
			}
		}
		if contentLength == 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(r, body); err != nil {
			return msgs, err
		}

		var msg jsonrpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return msgs, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func TestServerInitializeShutdown(t *testing.T) {
	var input bytes.Buffer
	var output bytes.Buffer

	// Send initialize, initialized, shutdown, exit.
	input.Write(lspMessage(requestMsg(1, "initialize", map[string]any{
		"rootUri": "file:///workspace",
	})))
	input.Write(lspMessage(notifyMsg("initialized", struct{}{})))
	input.Write(lspMessage(requestMsg(2, "shutdown", nil)))
	input.Write(lspMessage(notifyMsg("exit", nil)))

	s := NewServer(&input, &output)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	msgs, err := parseResponses(output.Bytes())
	if err != nil {
		t.Fatalf("parseResponses: %v", err)
	}

	// Expect initialize response and shutdown response.
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(msgs))
	}

	// Check initialize response.
	initResp := msgs[0]
	if string(initResp.ID) != "1" {
		t.Errorf("initialize response ID = %s, want 1", initResp.ID)
	}
	if initResp.Error != nil {
		t.Errorf("initialize returned error: %s", initResp.Error.Message)
	}

	// Verify capabilities are present.
	raw, _ := json.Marshal(initResp.Result)
	var result initializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}
	if result.ServerInfo.Name != "bloblang2-lsp" {
		t.Errorf("server name = %q, want %q", result.ServerInfo.Name, "bloblang2-lsp")
	}
	if !result.Capabilities.TextDocumentSync.OpenClose {
		t.Error("expected openClose = true")
	}
	if result.Capabilities.CompletionProvider == nil {
		t.Error("expected completionProvider to be set")
	}
}

func TestServerDidOpenPublishesDiagnostics(t *testing.T) {
	var input bytes.Buffer
	var output bytes.Buffer

	// Initialize, then open a file with an error.
	input.Write(lspMessage(requestMsg(1, "initialize", map[string]any{})))
	input.Write(lspMessage(notifyMsg("initialized", struct{}{})))
	input.Write(lspMessage(notifyMsg("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI:        "file:///test.blobl2",
			LanguageID: "blobl2",
			Version:    1,
			Text:       "output = $undeclared",
		},
	})))
	input.Write(lspMessage(requestMsg(2, "shutdown", nil)))
	input.Write(lspMessage(notifyMsg("exit", nil)))

	s := NewServer(&input, &output)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	msgs, err := parseResponses(output.Bytes())
	if err != nil {
		t.Fatalf("parseResponses: %v", err)
	}

	// Find the publishDiagnostics notification.
	var diagParams publishDiagnosticsParams
	found := false
	for _, msg := range msgs {
		if msg.Method == "textDocument/publishDiagnostics" {
			if err := json.Unmarshal(msg.Params, &diagParams); err != nil {
				t.Fatalf("unmarshal diagnostics: %v", err)
			}
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected publishDiagnostics notification")
	}
	if diagParams.URI != "file:///test.blobl2" {
		t.Errorf("diagnostics URI = %q, want %q", diagParams.URI, "file:///test.blobl2")
	}
	if len(diagParams.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic for undeclared variable")
	}
	if !strings.Contains(diagParams.Diagnostics[0].Message, "undeclared") {
		t.Errorf("diagnostic message = %q, expected it to contain 'undeclared'", diagParams.Diagnostics[0].Message)
	}
}

func TestServerDidOpenCleanFile(t *testing.T) {
	var input bytes.Buffer
	var output bytes.Buffer

	input.Write(lspMessage(requestMsg(1, "initialize", map[string]any{})))
	input.Write(lspMessage(notifyMsg("initialized", struct{}{})))
	input.Write(lspMessage(notifyMsg("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI:        "file:///clean.blobl2",
			LanguageID: "blobl2",
			Version:    1,
			Text:       "output = input.name.uppercase()",
		},
	})))
	input.Write(lspMessage(requestMsg(2, "shutdown", nil)))
	input.Write(lspMessage(notifyMsg("exit", nil)))

	s := NewServer(&input, &output)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	msgs, err := parseResponses(output.Bytes())
	if err != nil {
		t.Fatalf("parseResponses: %v", err)
	}

	for _, msg := range msgs {
		if msg.Method == "textDocument/publishDiagnostics" {
			var diagParams publishDiagnosticsParams
			if err := json.Unmarshal(msg.Params, &diagParams); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(diagParams.Diagnostics) != 0 {
				t.Errorf("expected 0 diagnostics for clean file, got %d", len(diagParams.Diagnostics))
				for _, d := range diagParams.Diagnostics {
					t.Logf("  diagnostic: %s", d.Message)
				}
			}
			return
		}
	}
	t.Fatal("expected publishDiagnostics notification for clean file")
}

func TestServerCompletion(t *testing.T) {
	var input bytes.Buffer
	var output bytes.Buffer

	input.Write(lspMessage(requestMsg(1, "initialize", map[string]any{})))
	input.Write(lspMessage(notifyMsg("initialized", struct{}{})))
	// Open a file so the document store has content.
	input.Write(lspMessage(notifyMsg("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI:        "file:///comp.blobl2",
			LanguageID: "blobl2",
			Version:    1,
			Text:       "$foo = 1\noutput = input.",
		},
	})))
	// Request completion after the dot.
	input.Write(lspMessage(requestMsg(2, "textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///comp.blobl2"},
		Position:     position{Line: 1, Character: 15},
		Context:      &completionContext{TriggerKind: 2, TriggerCharacter: "."},
	})))
	input.Write(lspMessage(requestMsg(3, "shutdown", nil)))
	input.Write(lspMessage(notifyMsg("exit", nil)))

	s := NewServer(&input, &output)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	msgs, err := parseResponses(output.Bytes())
	if err != nil {
		t.Fatalf("parseResponses: %v", err)
	}

	// Find the completion response (id=2).
	var completionResp jsonrpcMessage
	found := false
	for _, msg := range msgs {
		if string(msg.ID) == "2" {
			completionResp = msg
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected completion response with id=2")
	}

	raw, _ := json.Marshal(completionResp.Result)
	var items []completionItem
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal completion items: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected completion items after dot")
	}

	// All items should be methods.
	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
		if item.Kind != completionKindMethod {
			t.Errorf("completion item %q has kind %d, want %d (method)", item.Label, item.Kind, completionKindMethod)
		}
	}
	if !labels["uppercase"] {
		t.Error("expected 'uppercase' in method completions")
	}
	if !labels["filter"] {
		t.Error("expected 'filter' in method completions")
	}
}

func TestServerUnknownMethodReturnsError(t *testing.T) {
	var input bytes.Buffer
	var output bytes.Buffer

	input.Write(lspMessage(requestMsg(1, "initialize", map[string]any{})))
	input.Write(lspMessage(notifyMsg("initialized", struct{}{})))
	input.Write(lspMessage(requestMsg(2, "textDocument/bogus", nil)))
	input.Write(lspMessage(requestMsg(3, "shutdown", nil)))
	input.Write(lspMessage(notifyMsg("exit", nil)))

	s := NewServer(&input, &output)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	msgs, err := parseResponses(output.Bytes())
	if err != nil {
		t.Fatalf("parseResponses: %v", err)
	}

	for _, msg := range msgs {
		if string(msg.ID) == "2" {
			if msg.Error == nil {
				t.Error("expected error response for unknown method")
			} else if msg.Error.Code != -32601 {
				t.Errorf("error code = %d, want -32601", msg.Error.Code)
			}
			return
		}
	}
	t.Fatal("expected response for unknown method request")
}
