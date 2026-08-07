// Package integration verifies the complete agent execution cycle with a real
// OpenAI client and a real tool, backed by a mock OpenAI-compatible server.
package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/llm/openai"
	"github.com/unnati2401/observable-ai-agent/internal/tool/currenttime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testExporter records every span produced by the package tracers. The
// tracers bind to the first provider installed in the process, so a single
// provider is installed once in TestMain and the exporter is reset between
// tests.
var testExporter *tracetest.InMemoryExporter

func TestMain(m *testing.M) {
	testExporter = tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(testExporter)),
	)
	otel.SetTracerProvider(provider)
	os.Exit(m.Run())
}

func TestAgentOpenAICompleteToolCallCycle(t *testing.T) {
	testExporter.Reset()

	calls := 0
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

		calls++
		switch calls {
		case 1:
			assertToolDefinitions(t, request)
			assertMessageCount(t, request, 1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_current_time","arguments":"{\"timezone\":\"America/New_York\"}"}}]},"finish_reason":"tool_calls"}]}`)
		case 2:
			assertToolCallRoundTrip(t, request)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"It is 12:30 PM in New York."},"finish_reason":"stop"}]}`)
		default:
			t.Errorf("unexpected extra request %d", calls)
		}
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		openai.WithBaseURL(server.URL),
		openai.WithAPIKey("test-key"),
		openai.WithModel("test-model"),
	)

	a := agent.New(client, currenttime.Tool{})

	response, err := a.Run(context.Background(), []agent.Message{
		{Role: agent.UserRole, Content: "What time is it in New York?"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if response.Content != "It is 12:30 PM in New York." {
		t.Errorf("Content = %q, want the final answer", response.Content)
	}
	if response.ToolCall != nil {
		t.Errorf("ToolCall = %+v, want nil", response.ToolCall)
	}
	if calls != 2 {
		t.Errorf("llm called %d times, want 2", calls)
	}

	assertToolCallTrace(t)
}

// assertToolDefinitions verifies the first request advertises the registered
// tool to the model as an OpenAI function definition.
func assertToolDefinitions(t *testing.T, request map[string]any) {
	t.Helper()

	tools, ok := request["tools"].([]any)
	if !ok {
		t.Fatalf("tools = %T, want []any", request["tools"])
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}

	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool = %T, want map[string]any", tools[0])
	}
	if tool["type"] != "function" {
		t.Errorf("tool type = %v, want function", tool["type"])
	}

	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool function = %T, want map[string]any", tool["function"])
	}
	if fn["name"] != currenttime.ToolName {
		t.Errorf("tool name = %v, want %q", fn["name"], currenttime.ToolName)
	}
	if fn["description"] == "" {
		t.Error("tool description is empty")
	}

	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters = %T, want map[string]any", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters type = %v, want object", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters properties = %T, want map[string]any", params["properties"])
	}
	if _, ok := props["timezone"]; !ok {
		t.Error("parameters properties missing timezone")
	}
}

// assertToolCallRoundTrip verifies the second request replays the assistant's
// tool call together with the tool result, linked by tool_call_id.
func assertToolCallRoundTrip(t *testing.T, request map[string]any) {
	t.Helper()

	assertMessageCount(t, request, 3)
	messages, _ := request["messages"].([]any)

	assistant, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("assistant message = %T, want map[string]any", messages[1])
	}
	if assistant["role"] != "assistant" {
		t.Errorf("assistant role = %v, want assistant", assistant["role"])
	}
	toolCalls, ok := assistant["tool_calls"].([]any)
	if !ok {
		t.Fatalf("assistant tool_calls = %T, want []any", assistant["tool_calls"])
	}
	if len(toolCalls) != 1 {
		t.Fatalf("len(tool_calls) = %d, want 1", len(toolCalls))
	}
	call, ok := toolCalls[0].(map[string]any)
	if !ok {
		t.Fatalf("tool call = %T, want map[string]any", toolCalls[0])
	}
	if call["id"] != "call_1" {
		t.Errorf("tool call id = %v, want call_1", call["id"])
	}

	toolMsg, ok := messages[2].(map[string]any)
	if !ok {
		t.Fatalf("tool message = %T, want map[string]any", messages[2])
	}
	if toolMsg["role"] != "tool" {
		t.Errorf("tool message role = %v, want tool", toolMsg["role"])
	}
	if toolMsg["tool_call_id"] != "call_1" {
		t.Errorf("tool message tool_call_id = %v, want call_1", toolMsg["tool_call_id"])
	}
	content, ok := toolMsg["content"].(string)
	if !ok {
		t.Fatalf("tool message content = %T, want string", toolMsg["content"])
	}
	var result struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("tool message content is not JSON: %v", err)
	}
	if result.Timezone != "America/New_York" {
		t.Errorf("tool result timezone = %q, want America/New_York", result.Timezone)
	}
}

func assertMessageCount(t *testing.T, request map[string]any, want int) {
	t.Helper()

	messages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %T, want []any", request["messages"])
	}
	if len(messages) != want {
		t.Errorf("len(messages) = %d, want %d", len(messages), want)
	}
}

// assertToolCallTrace verifies the spans record the tool-calling information
// end to end.
func assertToolCallTrace(t *testing.T) {
	t.Helper()

	spans := testExporter.GetSpans()

	run, ok := findSpan(spans, "Agent.Run")
	if !ok {
		t.Fatalf("no Agent.Run span; got %v", spanNames(spans))
	}
	if got, _ := attrInt(run, "agent.tools.count"); got != 1 {
		t.Errorf("agent.tools.count = %d, want 1", got)
	}
	if got, _ := attrString(run, "agent.tool_names"); got != currenttime.ToolName {
		t.Errorf("agent.tool_names = %q, want %q", got, currenttime.ToolName)
	}

	planner, ok := findSpan(spans, "Planner")
	if !ok {
		t.Fatalf("no Planner span; got %v", spanNames(spans))
	}
	if got, _ := attrString(planner, "planner.selected_tool"); got != currenttime.ToolName {
		t.Errorf("planner.selected_tool = %q, want %q", got, currenttime.ToolName)
	}

	llms := spansByName(spans, "LLM.Generate")
	if len(llms) != 2 {
		t.Fatalf("got %d LLM.Generate spans, want 2", len(llms))
	}
	if got, _ := attrInt(llms[0], "llm.tools.count"); got != 1 {
		t.Errorf("first llm.tools.count = %d, want 1", got)
	}
	if got, _ := attrString(llms[0], "llm.tool_calls"); got != currenttime.ToolName {
		t.Errorf("first llm.tool_calls = %q, want %q", got, currenttime.ToolName)
	}
	if _, ok := attrString(llms[1], "llm.tool_calls"); ok {
		t.Error("second llm.tool_calls is set, want it absent (no tool call)")
	}

	exec, ok := findSpan(spans, "Tool Execute")
	if !ok {
		t.Fatalf("no Tool Execute span; got %v", spanNames(spans))
	}
	if got, _ := attrString(exec, "tool.name"); got != currenttime.ToolName {
		t.Errorf("tool.name = %q, want %q", got, currenttime.ToolName)
	}
	if got, _ := attrBool(exec, "tool.arguments_valid"); !got {
		t.Error("tool.arguments_valid = false, want true")
	}
	if got, _ := attrBool(exec, "tool.success"); !got {
		t.Error("tool.success = false, want true")
	}
}

func findSpan(spans []tracetest.SpanStub, name string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}

func spansByName(spans []tracetest.SpanStub, name string) []tracetest.SpanStub {
	var found []tracetest.SpanStub
	for _, span := range spans {
		if span.Name == name {
			found = append(found, span)
		}
	}
	return found
}

func attrString(span tracetest.SpanStub, key string) (string, bool) {
	for _, kv := range span.Attributes {
		if kv.Key == attribute.Key(key) {
			return kv.Value.AsString(), true
		}
	}
	return "", false
}

func attrInt(span tracetest.SpanStub, key string) (int64, bool) {
	for _, kv := range span.Attributes {
		if kv.Key == attribute.Key(key) {
			return kv.Value.AsInt64(), true
		}
	}
	return 0, false
}

func attrBool(span tracetest.SpanStub, key string) (bool, bool) {
	for _, kv := range span.Attributes {
		if kv.Key == attribute.Key(key) {
			return kv.Value.AsBool(), true
		}
	}
	return false, false
}

func spanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}
