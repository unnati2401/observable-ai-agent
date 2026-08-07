package openai

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/unnati2401/observable-ai-agent/internal/agent"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func toChatParams(model string, messages []agent.Message, tools []agent.Tool) (openai.ChatCompletionNewParams, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)),
	}

	for _, msg := range messages {
		param, err := toOpenAIMessage(msg)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		params.Messages = append(params.Messages, param)
	}

	if len(tools) > 0 {
		params.Tools = toOpenAITools(tools)
	}

	return params, nil
}

// toOpenAITools converts the registered agent tools into OpenAI function tool
// definitions so the model knows which functions it can call.
func toOpenAITools(tools []agent.Tool) []openai.ChatCompletionToolUnionParam {
	converted := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		def := tool.Definition()
		function := shared.FunctionDefinitionParam{
			Name:       def.Name,
			Parameters: toFunctionParameters(def.Schema),
		}
		if def.Description != "" {
			function.Description = openai.String(def.Description)
		}
		converted = append(converted, openai.ChatCompletionFunctionTool(function))
	}
	return converted
}

// toFunctionParameters normalizes a ToolDefinition schema into the JSON schema
// format the OpenAI API expects.
func toFunctionParameters(schema any) shared.FunctionParameters {
	switch s := schema.(type) {
	case shared.FunctionParameters:
		return s
	case map[string]any:
		return shared.FunctionParameters(s)
	default:
		return nil
	}
}

func toOpenAIMessage(msg agent.Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case agent.SystemRole:
		return openai.SystemMessage(msg.Content), nil
	case agent.UserRole:
		return openai.UserMessage(msg.Content), nil
	case agent.AssistantRole:
		if msg.ToolCall != nil {
			return assistantToolCallMessage(msg)
		}
		return openai.AssistantMessage(msg.Content), nil
	case agent.ToolRole:
		return openai.ToolMessage(msg.Content, msg.ToolCallID), nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openai: unsupported message role %q", msg.Role)
	}
}

func assistantToolCallMessage(msg agent.Message) (openai.ChatCompletionMessageParamUnion, error) {
	content := openai.ChatCompletionAssistantMessageParamContentUnion{}
	if msg.Content != "" {
		content.OfString = openai.String(msg.Content)
	}

	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: content,
			ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
				{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: msg.ToolCall.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      msg.ToolCall.Name,
							Arguments: msg.ToolCall.Input,
						},
					},
				},
			},
		},
	}, nil
}

// requestSize returns the number of bytes the request payload would occupy.
func requestSize(params openai.ChatCompletionNewParams) int {
	data, err := json.Marshal(params)
	if err != nil {
		return 0
	}
	return len(data)
}

// responseSize returns the number of bytes the completion payload occupies.
func responseSize(completion *openai.ChatCompletion) int {
	data, err := json.Marshal(completion)
	if err != nil {
		return 0
	}
	return len(data)
}

func toLLMResponse(completion *openai.ChatCompletion) (*agent.LLMResponse, error) {
	if len(completion.Choices) == 0 {
		return nil, errors.New("openai: completion returned no choices")
	}

	msg := completion.Choices[0].Message
	if len(msg.ToolCalls) > 0 {
		toolCall := msg.ToolCalls[0]
		return &agent.LLMResponse{
			ToolCall: &agent.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: toolCall.Function.Arguments,
			},
		}, nil
	}

	return &agent.LLMResponse{Content: msg.Content}, nil
}
