package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/llm/openai"
	"github.com/unnati2401/observable-ai-agent/internal/tool/currenttime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testExporter records every span produced by the package tracer. The tracer
// binds to the first provider installed in the process, so a single provider
// is installed once in TestMain and the exporter is reset between tests.
var testExporter *tracetest.InMemoryExporter

func TestMain(m *testing.M) {
	testExporter = tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(testExporter)),
	)
	otel.SetTracerProvider(provider)
	os.Exit(m.Run())
}

// chatMock is a fake OpenAI-compatible server that returns one canned
// completion per request and records the request bodies it receives.
type chatMock struct {
	server  *httptest.Server
	bodies  [][]byte
	onError bool
}

func newChatMock(t *testing.T, responses ...string) *chatMock {
	t.Helper()
	m := &chatMock{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		m.bodies = append(m.bodies, body)

		if m.onError {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"error":{"message":"upstream down"}}`)
			return
		}
		if len(m.bodies) > len(responses) {
			t.Fatalf("unexpected extra LLM request %d", len(m.bodies))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, responses[len(m.bodies)-1])
	}))
	t.Cleanup(m.server.Close)
	return m
}

func (m *chatMock) messages(t *testing.T, call int) []any {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal(m.bodies[call-1], &request); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	messages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %T, want []any", request["messages"])
	}
	return messages
}

func newTestAgent(server *httptest.Server) *agent.Agent {
	client := openai.NewClient(
		openai.WithBaseURL(server.URL),
		openai.WithAPIKey("test-key"),
		openai.WithModel("test-model"),
	)
	return agent.New(client, currenttime.Tool{})
}

func postChat(t *testing.T, a *agent.Agent, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ChatHandler(a)(rec, req)
	return rec
}

func decodeChatResponse(t *testing.T, rec *httptest.ResponseRecorder) ChatResponse {
	t.Helper()
	var resp ChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestChatHandlerLLMResponse(t *testing.T) {
	testExporter.Reset()

	mock := newChatMock(t, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hello! How can I help?"},"finish_reason":"stop"}]}`)

	rec := postChat(t, newTestAgent(mock.server), `{"message":"Hello"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if got := decodeChatResponse(t, rec).Content; got != "Hello! How can I help?" {
		t.Errorf("Content = %q, want LLM response", got)
	}

	if len(mock.bodies) != 1 {
		t.Fatalf("LLM called %d times, want 1", len(mock.bodies))
	}
	if msgs := mock.messages(t, 1); len(msgs) != 1 {
		t.Errorf("request 1 has %d messages, want 1 (user only)", len(msgs))
	}

	spans := testExporter.GetSpans()
	chat, ok := findSpan(spans, "ChatHandler")
	if !ok {
		t.Fatalf("no ChatHandler span; got %v", spanNames(spans))
	}
	if chat.Parent.IsValid() {
		t.Error("ChatHandler span has a parent, want root")
	}
	run, _ := findSpan(spans, "Agent.Run")
	if !spanChildOf(run, chat) {
		t.Error("Agent.Run span is not a child of ChatHandler")
	}
	planner, _ := findSpan(spans, "Planner")
	if !spanChildOf(planner, run) {
		t.Error("Planner span is not a child of Agent.Run")
	}
	llms := spansByName(spans, "LLM.Generate")
	if len(llms) != 1 {
		t.Errorf("got %d LLM.Generate spans, want 1", len(llms))
	}
	if _, ok := findSpan(spans, "Tool Execute"); ok {
		t.Error("Tool Execute span present, want none (no tool call)")
	}
}

func TestChatHandlerToolCallFlow(t *testing.T) {
	testExporter.Reset()

	mock := newChatMock(t,
		`{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_current_time","arguments":"{\"timezone\":\"America/New_York\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"It is 12:30 PM in New York."},"finish_reason":"stop"}]}`,
	)

	rec := postChat(t, newTestAgent(mock.server), `{"message":"What time is it in New York?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if got := decodeChatResponse(t, rec).Content; got != "It is 12:30 PM in New York." {
		t.Errorf("Content = %q, want final answer", got)
	}

	if len(mock.bodies) != 2 {
		t.Fatalf("LLM called %d times, want 2", len(mock.bodies))
	}
	roundTrip := mock.messages(t, 2)
	if len(roundTrip) != 3 {
		t.Fatalf("request 2 has %d messages, want 3 (user, assistant tool call, tool result)", len(roundTrip))
	}
	assistant, _ := roundTrip[1].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Errorf("request 2 message 2 role = %v, want assistant", assistant["role"])
	}
	if _, ok := assistant["tool_calls"]; !ok {
		t.Error("request 2 message 2 has no tool_calls")
	}
	toolMsg, _ := roundTrip[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Errorf("request 2 message 3 role = %v, want tool", toolMsg["role"])
	}

	spans := testExporter.GetSpans()

	run, _ := findSpan(spans, "Agent.Run")
	if got, _ := attrInt(run, "agent.tools.count"); got != 1 {
		t.Errorf("agent.tools.count = %d, want 1", got)
	}
	if got, _ := attrString(run, "agent.tool_names"); got != currenttime.ToolName {
		t.Errorf("agent.tool_names = %q, want %q", got, currenttime.ToolName)
	}

	planner, _ := findSpan(spans, "Planner")
	if got, _ := attrString(planner, "planner.selected_tool"); got != currenttime.ToolName {
		t.Errorf("planner.selected_tool = %q, want %q", got, currenttime.ToolName)
	}

	llms := spansByName(spans, "LLM.Generate")
	if len(llms) != 2 {
		t.Fatalf("got %d LLM.Generate spans, want 2", len(llms))
	}
	if !spanChildOf(llms[0], planner) {
		t.Error("first LLM.Generate is not a child of Planner")
	}
	if !spanChildOf(llms[1], run) {
		t.Error("second LLM.Generate is not a child of Agent.Run")
	}
	if got, _ := attrString(llms[0], "llm.tool_calls"); got != currenttime.ToolName {
		t.Errorf("first llm.tool_calls = %q, want %q", got, currenttime.ToolName)
	}

	lookup, ok := findSpan(spans, "Tool Lookup")
	if !ok {
		t.Fatalf("no Tool Lookup span; got %v", spanNames(spans))
	}
	if !spanChildOf(lookup, run) {
		t.Error("Tool Lookup is not a child of Agent.Run")
	}
	exec, ok := findSpan(spans, "Tool Execute")
	if !ok {
		t.Fatalf("no Tool Execute span; got %v", spanNames(spans))
	}
	if !spanChildOf(exec, run) {
		t.Error("Tool Execute is not a child of Agent.Run")
	}
	if got, _ := attrBool(exec, "tool.success"); !got {
		t.Error("tool.success = false, want true")
	}
}

func TestChatHandlerToolError(t *testing.T) {
	testExporter.Reset()

	mock := newChatMock(t, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_current_time","arguments":"{\"timezone\":\"Not/AZone\"}"}}]},"finish_reason":"tool_calls"}]}`)

	rec := postChat(t, newTestAgent(mock.server), `{"message":"Do the impossible."}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	spans := testExporter.GetSpans()

	chat, _ := findSpan(spans, "ChatHandler")
	if chat.Status.Code != codes.Error {
		t.Errorf("ChatHandler status = %v, want Error", chat.Status.Code)
	}
	if !hasExceptionEvent(chat) {
		t.Error("ChatHandler span has no exception event")
	}
	run, _ := findSpan(spans, "Agent.Run")
	if run.Status.Code != codes.Error {
		t.Errorf("Agent.Run status = %v, want Error", run.Status.Code)
	}
	exec, ok := findSpan(spans, "Tool Execute")
	if !ok {
		t.Fatalf("no Tool Execute span; got %v", spanNames(spans))
	}
	if got, _ := attrBool(exec, "tool.success"); got {
		t.Error("tool.success = true, want false")
	}
}

func TestChatHandlerLLMError(t *testing.T) {
	testExporter.Reset()

	mock := newChatMock(t)
	mock.onError = true

	rec := postChat(t, newTestAgent(mock.server), `{"message":"Trigger a failure"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	spans := testExporter.GetSpans()

	chat, _ := findSpan(spans, "ChatHandler")
	if chat.Status.Code != codes.Error {
		t.Errorf("ChatHandler status = %v, want Error", chat.Status.Code)
	}
	if !hasExceptionEvent(chat) {
		t.Error("ChatHandler span has no exception event")
	}
	llmSpan, ok := findSpan(spans, "LLM.Generate")
	if !ok {
		t.Fatalf("no LLM.Generate span; got %v", spanNames(spans))
	}
	if llmSpan.Status.Code != codes.Error {
		t.Errorf("LLM.Generate status = %v, want Error", llmSpan.Status.Code)
	}
	if !hasExceptionEvent(llmSpan) {
		t.Error("LLM.Generate span has no exception event")
	}
}

func TestChatHandlerBadRequest(t *testing.T) {
	testExporter.Reset()

	mock := newChatMock(t)
	a := newTestAgent(mock.server)

	if rec := postChat(t, a, `{"message":`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON status = %d, want 400", rec.Code)
	}
	if rec := postChat(t, a, `{"message":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty message status = %d, want 400", rec.Code)
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

func spanChildOf(child, parent tracetest.SpanStub) bool {
	return child.Parent.IsValid() && child.Parent.SpanID() == parent.SpanContext.SpanID()
}

func hasExceptionEvent(span tracetest.SpanStub) bool {
	for _, event := range span.Events {
		if event.Name == "exception" {
			return true
		}
	}
	return false
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
