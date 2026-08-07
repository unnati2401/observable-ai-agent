// Package agent provides a provider-independent AI agent that orchestrates a
// single LLM and a set of tools.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unnati2401/observable-ai-agent/internal/otelutil"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Agent runs conversations against an LLM, executing the tools the model
// requests. It depends only on the LLM and Tool interfaces, so it knows
// nothing about concrete LLM providers or tool implementations.
type Agent struct {
	llm   LLM
	tools map[string]Tool
}

// New returns an Agent that uses llm to generate responses and is able to
// execute the given tools.
func New(llm LLM, tools ...Tool) *Agent {
	toolMap := make(map[string]Tool)

	for _, t := range tools {
		toolMap[t.Definition().Name] = t
	}

	return &Agent{
		llm:   llm,
		tools: toolMap,
	}
}

// Run continues the conversation in messages and returns the final response.
// It executes at most one tool call: if the model requests a tool, Run appends
// the assistant message containing the tool call and the tool result to the
// conversation, then asks the LLM again for the final answer. If that response
// still requests a tool, it is returned to the caller unchanged.
//
// Run is the root of the agent's execution trace; the planning, tool lookup,
// tool execution, and each LLM invocation are recorded as child spans.
func (a *Agent) Run(ctx context.Context, messages []Message) (*LLMResponse, error) {
	ctx, span := tracer.Start(ctx, "Agent.Run", trace.WithAttributes(
		attribute.String("agent.name", AgentName),
		attribute.String("agent.version", AgentVersion),
		attribute.Int("conversation.message_count", len(messages)),
		attribute.Int("agent.tools.count", len(a.tools)),
		attribute.String("agent.tool_names", a.toolNames()),
	))
	defer span.End()

	response, err := a.plan(ctx, messages)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}

	if response.ToolCall == nil {
		return response, nil
	}

	messages = append(messages, assistantToolCallMessage(*response.ToolCall))

	result, err := a.executeTool(ctx, *response.ToolCall)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}

	messages = append(messages, Message{
		Role:       ToolRole,
		Content:    result,
		ToolCallID: response.ToolCall.ID,
	})

	final, err := a.generate(ctx, messages)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}
	return final, nil
}

// plan asks the model to decide the next step and records the planning phase
// as a child span, including the tool the model selected, if any.
func (a *Agent) plan(ctx context.Context, messages []Message) (*LLMResponse, error) {
	ctx, span := tracer.Start(ctx, "Planner")
	defer span.End()

	response, err := a.generate(ctx, messages)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}

	if response.ToolCall != nil {
		span.SetAttributes(attribute.String("planner.selected_tool", response.ToolCall.Name))
		if response.Content != "" {
			span.SetAttributes(attribute.String("planner.reason", response.Content))
		}
	}
	return response, nil
}

func (a *Agent) generate(ctx context.Context, messages []Message) (*LLMResponse, error) {
	response, err := a.llm.Generate(ctx, messages, a.toolList())
	if err != nil {
		return nil, fmt.Errorf("agent: generate: %w", err)
	}
	return response, nil
}

// executeTool looks up call.Name in the tool registry and runs it, wrapping
// both lookup and execution failures with context. Lookup and execution are
// recorded as separate child spans of the caller's span.
func (a *Agent) executeTool(ctx context.Context, call ToolCall) (string, error) {
	_, lookupSpan := tracer.Start(ctx, "Tool Lookup")
	tool, ok := a.tools[call.Name]
	if !ok {
		err := fmt.Errorf("agent: tool %q not found", call.Name)
		otelutil.RecordError(lookupSpan, err)
		lookupSpan.End()
		return "", err
	}
	lookupSpan.SetAttributes(attribute.String("tool.name", call.Name))
	lookupSpan.End()

	ctx, execSpan := tracer.Start(ctx, "Tool Execute")
	execSpan.SetAttributes(
		attribute.String("tool.name", call.Name),
		attribute.String("tool.input", call.Input),
	)

	argsValid := true
	if err := validateToolArguments(call.Input); err != nil {
		argsValid = false
		err = fmt.Errorf("agent: execute tool %q: %w", call.Name, err)
		execSpan.SetAttributes(
			attribute.Bool("tool.arguments_valid", argsValid),
			attribute.Bool("tool.success", false),
		)
		otelutil.RecordError(execSpan, err)
		execSpan.End()
		return "", err
	}

	start := time.Now()
	result, err := tool.Execute(ctx, call.Input)
	execSpan.SetAttributes(
		attribute.Bool("tool.arguments_valid", argsValid),
		attribute.Bool("tool.success", err == nil),
		attribute.Float64("tool.execution_time", time.Since(start).Seconds()),
	)
	if err != nil {
		err = fmt.Errorf("agent: execute tool %q: %w", call.Name, err)
		otelutil.RecordError(execSpan, err)
		execSpan.End()
		return "", err
	}
	execSpan.End()
	return result, nil
}

// toolList returns the registered tools in a deterministic order so the LLM
// always receives a stable set of tool definitions.
func (a *Agent) toolList() []Tool {
	tools := make([]Tool, 0, len(a.tools))
	for _, t := range a.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Definition().Name < tools[j].Definition().Name
	})
	return tools
}

// toolNames returns the registered tool names as a comma-separated list in
// deterministic order, for span attributes.
func (a *Agent) toolNames() string {
	tools := a.toolList()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Definition().Name)
	}
	return strings.Join(names, ",")
}

// validateToolArguments ensures the model-provided tool arguments are well
// formed before they are handed to a tool. An empty argument string is treated
// as "no arguments".
func validateToolArguments(input string) error {
	if input == "" {
		return nil
	}
	if !json.Valid([]byte(input)) {
		return errors.New("tool arguments are not valid JSON")
	}
	return nil
}

// assistantToolCallMessage records the model's tool request as an assistant
// message so it can be replayed to the LLM together with the tool result.
func assistantToolCallMessage(call ToolCall) Message {
	return Message{
		Role:     AssistantRole,
		ToolCall: &call,
	}
}
