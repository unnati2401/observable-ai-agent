package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
)

func TestGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if request["model"] != "test-model" {
			t.Errorf("model = %v, want test-model", request["model"])
		}

		messages, ok := request["messages"].([]any)
		if !ok {
			t.Fatalf("messages = %T, want []any", request["messages"])
		}
		if len(messages) != 2 {
			t.Fatalf("len(messages) = %d, want 2", len(messages))
		}

		first, _ := messages[0].(map[string]any)
		second, _ := messages[1].(map[string]any)
		if first["role"] != "system" || first["content"] != "You are a helpful assistant." {
			t.Errorf("first message = %v", first)
		}
		if second["role"] != "user" || second["content"] != "Hello!" {
			t.Errorf("second message = %v", second)
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hi there!"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	response, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.SystemRole, Content: "You are a helpful assistant."},
		{Role: agent.UserRole, Content: "Hello!"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if response.ToolCall != nil {
		t.Errorf("ToolCall = %+v, want nil", response.ToolCall)
	}
	if response.Content != "Hi there!" {
		t.Errorf("Content = %q, want %q", response.Content, "Hi there!")
	}
}

func TestGenerateToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"New York\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	response, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.UserRole, Content: "What is the weather?"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if response.ToolCall == nil {
		t.Fatal("ToolCall = nil, want non-nil")
	}
	if response.ToolCall.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want get_weather", response.ToolCall.Name)
	}
	if response.ToolCall.Input != `{"location":"New York"}` {
		t.Errorf("ToolCall.Input = %q", response.ToolCall.Input)
	}
	if response.Content != "" {
		t.Errorf("Content = %q, want empty", response.Content)
	}
}

func TestGenerateNoMessages(t *testing.T) {
	client := NewClient(WithAPIKey("test-key"), WithModel("test-model"))

	response, err := client.Generate(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("Generate = nil error, want error")
	}
	if response != nil {
		t.Errorf("Generate = %+v, want nil", response)
	}
}

func TestGenerateToolRoleUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request to server")
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	_, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.ToolRole, Content: "result"},
	}, nil)
	if err == nil {
		t.Fatal("Generate = nil error, want error")
	}
	if !strings.Contains(err.Error(), "tool messages") {
		t.Errorf("error = %q, want mention of tool messages", err)
	}
}

func TestGenerateServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	_, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.UserRole, Content: "Hello!"},
	}, nil)
	if err == nil {
		t.Fatal("Generate = nil error, want error")
	}
	if !strings.Contains(err.Error(), "openai: chat completion") {
		t.Errorf("error = %q, want wrapped context", err)
	}
}

func TestNewClientDefaults(t *testing.T) {
	client := NewClient()

	if client.model != DefaultModel {
		t.Errorf("model = %q, want %q", client.model, DefaultModel)
	}
	if client == nil {
		t.Error("client is nil")
	}
}
