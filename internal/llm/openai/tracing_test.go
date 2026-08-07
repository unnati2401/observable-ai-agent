package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unnati2401/observable-ai-agent/internal/agent"

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

func newTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	testExporter.Reset()
	return testExporter
}

func TestGenerateTraceSpan(t *testing.T) {
	exporter := newTestTracer(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hi there!"},"finish_reason":"stop"}]}`)
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
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != "LLM.Generate" {
		t.Errorf("span name = %q, want %q", span.Name, "LLM.Generate")
	}

	if got, _ := attrString(span, "llm.provider"); got != "openai" {
		t.Errorf("llm.provider = %q, want openai", got)
	}
	if got, _ := attrString(span, "llm.model"); got != "test-model" {
		t.Errorf("llm.model = %q, want test-model", got)
	}
	if got, _ := attrInt(span, "llm.request_size"); got <= 0 {
		t.Errorf("llm.request_size = %d, want > 0", got)
	}
	if got, _ := attrInt(span, "llm.response_size"); got <= 0 {
		t.Errorf("llm.response_size = %d, want > 0", got)
	}
	if got, ok := attrInt(span, "llm.latency"); ok && got < 0 {
		t.Errorf("llm.latency = %d, want >= 0", got)
	}
}

func TestGenerateTraceErrorStatus(t *testing.T) {
	exporter := newTestTracer(t)

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

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Errorf("span status = %v, want Error", span.Status.Code)
	}
	if span.Status.Description == "" {
		t.Error("span status description is empty")
	}
	found := false
	for _, event := range span.Events {
		if event.Name == "exception" {
			found = true
		}
	}
	if !found {
		t.Error("span has no exception event")
	}
}

func TestGenerateTraceToolCall(t *testing.T) {
	exporter := newTestTracer(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"NYC\"}"}}]},"finish_reason":"tool_calls"}]}`)
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
	if response.ToolCall == nil || response.ToolCall.Name != "get_weather" {
		t.Fatalf("ToolCall = %+v, want get_weather", response.ToolCall)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	span := spans[0]
	if got, _ := attrInt(span, "llm.tools.count"); got != 1 {
		t.Errorf("llm.tools.count = %d, want 1", got)
	}
	if got, _ := attrString(span, "llm.tool_calls"); got != "get_weather" {
		t.Errorf("llm.tool_calls = %q, want get_weather", got)
	}
	if span.Status.Code != codes.Unset && span.Status.Code != codes.Ok {
		t.Errorf("span status = %v, want unset/ok", span.Status.Code)
	}
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
