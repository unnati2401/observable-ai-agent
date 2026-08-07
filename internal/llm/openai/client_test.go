package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	if response.ToolCall.ID != "call_1" {
		t.Errorf("ToolCall.ID = %q, want call_1", response.ToolCall.ID)
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

func TestGenerateToolRoleRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		messages, ok := request["messages"].([]any)
		if !ok {
			t.Fatalf("messages = %T, want []any", request["messages"])
		}

		want := []any{
			map[string]any{"role": "system", "content": "You are a helpful assistant."},
			map[string]any{"role": "user", "content": "What is the weather?"},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"location":"New York"}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "content": "Sunny, 25C", "tool_call_id": "call_1"},
		}

		if !reflect.DeepEqual(messages, want) {
			t.Errorf("messages = %v\nwant      = %v", messages, want)
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"It is sunny in New York."},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	response, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.SystemRole, Content: "You are a helpful assistant."},
		{Role: agent.UserRole, Content: "What is the weather?"},
		{
			Role: agent.AssistantRole,
			ToolCall: &agent.ToolCall{
				ID:    "call_1",
				Name:  "get_weather",
				Input: `{"location":"New York"}`,
			},
		},
		{Role: agent.ToolRole, ToolCallID: "call_1", Content: "Sunny, 25C"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content != "It is sunny in New York." {
		t.Errorf("Content = %q, want %q", response.Content, "It is sunny in New York.")
	}
	if response.ToolCall != nil {
		t.Errorf("ToolCall = %+v, want nil", response.ToolCall)
	}
}

func TestGenerateSendsToolDefinitions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		tools, ok := request["tools"].([]any)
		if !ok {
			t.Fatalf("tools = %T, want []any", request["tools"])
		}
		if len(tools) != 1 {
			t.Fatalf("len(tools) = %d, want 1", len(tools))
		}

		want := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get the weather for a location.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type": "string",
						},
					},
					"required": []any{"location"},
				},
			},
		}
		if !reflect.DeepEqual(tools[0], want) {
			t.Errorf("tool = %v\nwant      = %v", tools[0], want)
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"I can help with that."},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	response, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.UserRole, Content: "What is the weather?"},
	}, []agent.Tool{testTool{name: "get_weather", description: "Get the weather for a location."}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content != "I can help with that." {
		t.Errorf("Content = %q, want response", response.Content)
	}
}

func TestGenerateSendsToolDefinitionsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if _, present := request["tools"]; present {
			t.Errorf("tools present in request, want absent when no tools registered")
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hi!"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("test-model"),
	)

	if _, err := client.Generate(context.Background(), []agent.Message{
		{Role: agent.UserRole, Content: "Hello!"},
	}, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

type testTool struct {
	name        string
	description string
}

func (t testTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        t.name,
		Description: t.description,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{"type": "string"},
			},
			"required": []any{"location"},
		},
	}
}

func (t testTool) Execute(_ context.Context, _ string) (string, error) {
	return "", nil
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
