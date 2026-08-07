package agent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

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

func findSpan(spans []tracetest.SpanStub, name string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
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

func spanChildOf(child, parent tracetest.SpanStub) bool {
	return child.Parent.IsValid() && child.Parent.SpanID() == parent.SpanContext.SpanID()
}

func TestRunTraceStructure(t *testing.T) {
	exporter := newTestTracer(t)

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

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Weather?"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	spans := exporter.GetSpans()
	run, ok := findSpan(spans, "Agent.Run")
	if !ok {
		t.Fatalf("no Agent.Run span; got %v", spanNames(spans))
	}
	if run.Parent.IsValid() {
		t.Error("Agent.Run span has a parent, want root")
	}

	planner, ok := findSpan(spans, "Planner")
	if !ok {
		t.Fatalf("no Planner span; got %v", spanNames(spans))
	}
	if !spanChildOf(planner, run) {
		t.Error("Planner span is not a child of Agent.Run")
	}

	lookup, ok := findSpan(spans, "Tool Lookup")
	if !ok {
		t.Fatalf("no Tool Lookup span; got %v", spanNames(spans))
	}
	if !spanChildOf(lookup, run) {
		t.Error("Tool Lookup span is not a child of Agent.Run")
	}

	exec, ok := findSpan(spans, "Tool Execute")
	if !ok {
		t.Fatalf("no Tool Execute span; got %v", spanNames(spans))
	}
	if !spanChildOf(exec, run) {
		t.Error("Tool Execute span is not a child of Agent.Run")
	}
	if !spanChildOf(lookup, run) {
		t.Error("Tool Lookup span is not a child of Agent.Run")
	}
}

func TestRunTraceAttributes(t *testing.T) {
	exporter := newTestTracer(t)

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

	_, err := a.Run(context.Background(), []Message{
		{Role: SystemRole, Content: "You are helpful."},
		{Role: UserRole, Content: "Weather?"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	spans := exporter.GetSpans()

	run, _ := findSpan(spans, "Agent.Run")
	if got, _ := attrString(run, "agent.name"); got != AgentName {
		t.Errorf("agent.name = %q, want %q", got, AgentName)
	}
	if got, _ := attrString(run, "agent.version"); got != AgentVersion {
		t.Errorf("agent.version = %q, want %q", got, AgentVersion)
	}
	if got, _ := attrInt(run, "conversation.message_count"); got != 2 {
		t.Errorf("conversation.message_count = %d, want 2", got)
	}
	if got, _ := attrInt(run, "agent.tools.count"); got != 1 {
		t.Errorf("agent.tools.count = %d, want 1", got)
	}
	if got, _ := attrString(run, "agent.tool_names"); got != "weather" {
		t.Errorf("agent.tool_names = %q, want weather", got)
	}

	planner, _ := findSpan(spans, "Planner")
	if got, _ := attrString(planner, "planner.selected_tool"); got != "weather" {
		t.Errorf("planner.selected_tool = %q, want weather", got)
	}

	exec, _ := findSpan(spans, "Tool Execute")
	if got, _ := attrString(exec, "tool.name"); got != "weather" {
		t.Errorf("tool.name = %q, want weather", got)
	}
	if got, _ := attrString(exec, "tool.input"); got != `{"city":"NYC"}` {
		t.Errorf("tool.input = %q, want %q", got, `{"city":"NYC"}`)
	}
	if got, _ := attrBool(exec, "tool.arguments_valid"); !got {
		t.Error("tool.arguments_valid = false, want true")
	}
	if got, _ := attrBool(exec, "tool.success"); !got {
		t.Error("tool.success = false, want true")
	}
	if got, ok := attrInt(exec, "tool.execution_time"); ok && got < 0 {
		t.Errorf("tool.execution_time = %d, want >= 0", got)
	}
}

func TestRunTraceErrorStatus(t *testing.T) {
	exporter := newTestTracer(t)

	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
			return nil, errors.New("api down")
		},
	}
	a := New(llm)

	_, err := a.Run(context.Background(), []Message{{Role: UserRole, Content: "Go"}})
	if err == nil {
		t.Fatal("Run = nil error, want error")
	}

	spans := exporter.GetSpans()

	run, ok := findSpan(spans, "Agent.Run")
	if !ok {
		t.Fatalf("no Agent.Run span; got %v", spanNames(spans))
	}
	if run.Status.Code != codes.Error {
		t.Errorf("Agent.Run status = %v, want Error", run.Status.Code)
	}
	if got := strings.Count(string(run.Status.Description), ""); got == 0 {
		t.Errorf("Agent.Run status description = %q, want error message", run.Status.Description)
	}
	if !hasExceptionEvent(run) {
		t.Error("Agent.Run span has no exception event")
	}

	planner, ok := findSpan(spans, "Planner")
	if !ok {
		t.Fatalf("no Planner span; got %v", spanNames(spans))
	}
	if planner.Status.Code != codes.Error {
		t.Errorf("Planner status = %v, want Error", planner.Status.Code)
	}
	if !hasExceptionEvent(planner) {
		t.Error("Planner span has no exception event")
	}
}

func TestRunTraceToolErrorStatus(t *testing.T) {
	exporter := newTestTracer(t)

	llm := &fakeLLM{
		generate: func(messages []Message, tools []Tool) (*LLMResponse, error) {
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

	spans := exporter.GetSpans()

	exec, ok := findSpan(spans, "Tool Execute")
	if !ok {
		t.Fatalf("no Tool Execute span; got %v", spanNames(spans))
	}
	if exec.Status.Code != codes.Error {
		t.Errorf("Tool Execute status = %v, want Error", exec.Status.Code)
	}
	if !hasExceptionEvent(exec) {
		t.Error("Tool Execute span has no exception event")
	}
	if got, _ := attrBool(exec, "tool.success"); got {
		t.Error("tool.success = true, want false")
	}

	run, _ := findSpan(spans, "Agent.Run")
	if run.Status.Code != codes.Error {
		t.Errorf("Agent.Run status = %v, want Error", run.Status.Code)
	}
}

func hasExceptionEvent(span tracetest.SpanStub) bool {
	for _, event := range span.Events {
		if event.Name == "exception" {
			return true
		}
	}
	return false
}

func spanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}
