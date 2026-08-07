package agent

import "context"

type LLM interface {
	Generate(
		ctx context.Context,
		messages []Message,
		tools []Tool,
	) (*LLMResponse, error)
}
