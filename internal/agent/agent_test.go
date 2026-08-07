package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeLLM struct {
	generate func(messages []Message, tools []Tool) (*LLMResponse, error)
	calls    [][]Message
}

func (f *fakeLLM) Generate(_ context.Context, messages []Message, _ []Tool) (*LLMResponse, error) {
	f.calls = append(f.calls, append([]Message(nil), messages...))
	return f.generate(messages, nil)
}

type fakeTool struct {
	name      string
	result    string
	err       error
	executed  bool
	lastInput string
}

func (t *fakeTool) Definition() ToolDefinition {
	return ToolDefinition{Name: t.name}
}

func (t *fakeTool) Execute(_ context.Context, input string) (string, error) {
	t.executed = true
	t.lastInput = input
	if t.err != nil {
		return "", t.err
	}
	return t.result, nil
}

func TestRunReturnsContentWithoutToolCall(t *testing.T) {
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			return &LLMResponse{Content: "Hello!"}, nil
		},
	}
	tool := &fakeTool{name: "greet", result: "hi"}
	a := New(llm, tool)

	response, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Hi"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if response.Content != "Hello!" {
		t.Errorf("Content = %q, want %q", response.Content, "Hello!")
	}
	if response.ToolCall != nil {
		t.Errorf("ToolCall = %+v, want nil", response.ToolCall)
	}
	if len(llm.calls) != 1 {
		t.Errorf("llm called %d times, want 1", len(llm.calls))
	}
	if tool.executed {
		t.Error("tool was executed, want no execution")
	}
}

func TestRunExecutesToolAndMaintainsConversation(t *testing.T) {
	turns := 0
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			turns++
			if turns == 1 {
				return &LLMResponse{
					ToolCall: &ToolCall{ID: "call_1", Name: "weather", Input: `{"city":"NYC"}`},
				}, nil
			}
			return &LLMResponse{Content: "It is sunny."}, nil
		},
	}
	tool := &fakeTool{name: "weather", result: "Sunny, 25C"}
	a := New(llm, tool)

	initial := []Message{{Role: UserRole, Content: "Weather in NYC?"}}
	response, err := a.Run(context.Background(), initial)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if response.Content != "It is sunny." {
		t.Errorf("Content = %q, want %q", response.Content, "It is sunny.")
	}
	if !tool.executed {
		t.Fatal("tool was not executed")
	}
	if tool.lastInput != `{"city":"NYC"}` {
		t.Errorf("tool input = %q, want %q", tool.lastInput, `{"city":"NYC"}`)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("llm called %d times, want 2", len(llm.calls))
	}

	first := llm.calls[0]
	if !reflect.DeepEqual(first, initial) {
		t.Errorf("first call messages = %v, want %v", first, initial)
	}
	if len(initial) != 1 {
		t.Errorf("caller's messages mutated: len = %d, want 1", len(initial))
	}

	second := llm.calls[1]
	if len(second) != len(initial)+2 {
		t.Fatalf("second call has %d messages, want %d", len(second), len(initial)+2)
	}

	assistantMsg := second[len(initial)]
	if assistantMsg.Role != AssistantRole {
		t.Errorf("assistant message role = %q, want %q", assistantMsg.Role, AssistantRole)
	}
	if assistantMsg.ToolCall == nil {
		t.Fatal("assistant message has no ToolCall")
	}
	if assistantMsg.ToolCall.ID != "call_1" || assistantMsg.ToolCall.Name != "weather" {
		t.Errorf("assistant message ToolCall = %+v", assistantMsg.ToolCall)
	}

	toolMsg := second[len(initial)+1]
	if toolMsg.Role != ToolRole {
		t.Errorf("tool message role = %q, want %q", toolMsg.Role, ToolRole)
	}
	if toolMsg.Content != "Sunny, 25C" {
		t.Errorf("tool message content = %q, want %q", toolMsg.Content, "Sunny, 25C")
	}
	if toolMsg.ToolCallID != "call_1" {
		t.Errorf("tool message ToolCallID = %q, want call_1", toolMsg.ToolCallID)
	}
}

func TestRunToolNotFound(t *testing.T) {
	turns := 0
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			turns++
			return &LLMResponse{
				ToolCall: &ToolCall{ID: "call_1", Name: "missing"},
			}, nil
		},
	}
	a := New(llm)

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}
	if !strings.Contains(err.Error(), `"missing"`) {
		t.Errorf("error = %q, want mention of missing tool", err)
	}
	if turns != 1 {
		t.Errorf("llm called %d times, want 1 (no second call)", turns)
	}
}

func TestRunToolExecutionError(t *testing.T) {
	turns := 0
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			turns++
			return &LLMResponse{
				ToolCall: &ToolCall{ID: "call_1", Name: "boom"},
			}, nil
		},
	}
	tool := &fakeTool{name: "boom", err: errors.New("exploded")}
	a := New(llm, tool)

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "exploded") {
		t.Errorf("error = %q, want tool name and cause", err)
	}
	if !tool.executed {
		t.Error("tool was not executed")
	}
	if turns != 1 {
		t.Errorf("llm called %d times, want 1 (no second call)", turns)
	}
}

func TestRunInvalidToolArguments(t *testing.T) {
	turns := 0
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			turns++
			return &LLMResponse{
				ToolCall: &ToolCall{ID: "call_1", Name: "weather", Input: "{not json"},
			}, nil
		},
	}
	tool := &fakeTool{name: "weather", result: "Sunny"}
	a := New(llm, tool)

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}
	if !strings.Contains(err.Error(), "weather") || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error = %q, want tool name and JSON cause", err)
	}
	if tool.executed {
		t.Error("tool was executed despite invalid arguments")
	}
	if turns != 1 {
		t.Errorf("llm called %d times, want 1 (no second call)", turns)
	}
}

func TestRunFirstLLMError(t *testing.T) {
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			return nil, errors.New("api down")
		},
	}
	a := New(llm)

	response, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}
	if response != nil {
		t.Errorf("response = %+v, want nil", response)
	}
	if !strings.Contains(err.Error(), "api down") || !strings.Contains(err.Error(), "generate") {
		t.Errorf("error = %q, want cause and context", err)
	}
}

func TestRunFinalLLMError(t *testing.T) {
	turns := 0
	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			turns++
			if turns == 1 {
				return &LLMResponse{
					ToolCall: &ToolCall{ID: "call_1", Name: "weather"},
				}, nil
			}
			return nil, errors.New("api down")
		},
	}
	tool := &fakeTool{name: "weather", result: "Sunny"}
	a := New(llm, tool)

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}
	if !strings.Contains(err.Error(), "api down") || !strings.Contains(err.Error(), "generate") {
		t.Errorf("error = %q, want cause and context", err)
	}
	if !tool.executed {
		t.Error("tool was not executed")
	}
}
