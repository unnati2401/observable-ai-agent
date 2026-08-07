package openai

import (
	"errors"
	"fmt"

	"github.com/unnati2401/observable-ai-agent/internal/agent"

	"github.com/openai/openai-go/v3"
)

func toChatParams(model string, messages []agent.Message) (openai.ChatCompletionNewParams, error) {
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

	return params, nil
}

func toOpenAIMessage(msg agent.Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch msg.Role {
	case agent.SystemRole:
		return openai.SystemMessage(msg.Content), nil
	case agent.UserRole:
		return openai.UserMessage(msg.Content), nil
	case agent.AssistantRole:
		return openai.AssistantMessage(msg.Content), nil
	case agent.ToolRole:
		return openai.ChatCompletionMessageParamUnion{}, errors.New("openai: tool messages require tool call metadata and are not yet supported")
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openai: unsupported message role %q", msg.Role)
	}
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
				Name:  toolCall.Function.Name,
				Input: toolCall.Function.Arguments,
			},
		}, nil
	}

	return &agent.LLMResponse{Content: msg.Content}, nil
}
