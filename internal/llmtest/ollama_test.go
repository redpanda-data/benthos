package llmtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaProvider_Judge_Simple(t *testing.T) {
	respPayload := JudgeResponse{Score: 72, Notes: []string{"mostly correct"}}
	respJSON, err := json.Marshal(respPayload)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req ollamaChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-model", req.Model)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "user", req.Messages[1].Role)

		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role:    "assistant",
				Content: string(respJSON),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := &OllamaProvider{URL: srv.URL, Model: "test-model"}
	result, err := p.Judge(context.Background(), JudgeRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	require.NoError(t, err)
	assert.Equal(t, 72, result.Score)
	assert.Equal(t, []string{"mostly correct"}, result.Notes)
}

func TestOllamaProvider_Judge_WithToolCalls(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req ollamaChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		var resp ollamaChatResponse
		if callCount == 1 {
			// First call: model requests a tool call.
			assert.Len(t, req.Tools, 1)
			resp.Message = ollamaMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{
					{Function: ollamaToolCallFunction{
						Name:      "greet",
						Arguments: map[string]any{"name": "world"},
					}},
				},
			}
		} else {
			// Second call: model produces final response after tool result.
			assert.GreaterOrEqual(t, len(req.Messages), 4) // system + user + assistant + tool
			respPayload := JudgeResponse{Score: 95, Notes: []string{"tool worked"}}
			respJSON, _ := json.Marshal(respPayload)
			resp.Message = ollamaMessage{
				Role:    "assistant",
				Content: string(respJSON),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	tools := []Tool{
		{
			Name:        "greet",
			Description: "Say hello",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
			Execute: func(args map[string]any) (string, error) {
				return "Hello, " + args["name"].(string) + "!", nil
			},
		},
	}

	p := &OllamaProvider{URL: srv.URL, Model: "test-model"}
	result, err := p.Judge(context.Background(), JudgeRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Tools:        tools,
	})
	require.NoError(t, err)
	assert.Equal(t, 95, result.Score)
	assert.Equal(t, 2, callCount)
}

func TestOllamaProvider_Judge_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("something broke"))
	}))
	defer srv.Close()

	p := &OllamaProvider{URL: srv.URL, Model: "test-model"}
	_, err := p.Judge(context.Background(), JudgeRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
	assert.Contains(t, err.Error(), "something broke")
}

func TestOllamaProvider_Judge_InvalidJSON_Retry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		var resp ollamaChatResponse
		if callCount == 1 {
			// First call: return invalid JSON.
			resp.Message = ollamaMessage{Role: "assistant", Content: "not json"}
		} else {
			// Retry: return valid response.
			respPayload := JudgeResponse{Score: 60, Notes: []string{"fixed"}}
			respJSON, _ := json.Marshal(respPayload)
			resp.Message = ollamaMessage{Role: "assistant", Content: string(respJSON)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &OllamaProvider{URL: srv.URL, Model: "test-model"}
	result, err := p.Judge(context.Background(), JudgeRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	require.NoError(t, err)
	assert.Equal(t, 60, result.Score)
	assert.Equal(t, 2, callCount)
}

func TestOllamaProvider_Judge_ScoreOutOfRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respPayload := map[string]any{"score": 150, "notes": []string{}}
		resp := ollamaChatResponse{Message: ollamaMessage{Role: "assistant"}}
		respJSON, _ := json.Marshal(respPayload)
		resp.Message.Content = string(respJSON)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &OllamaProvider{URL: srv.URL, Model: "test-model"}
	_, err := p.Judge(context.Background(), JudgeRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	// First parse fails (score out of range), retry also fails (same response).
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}
