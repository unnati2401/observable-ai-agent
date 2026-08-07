package agent

import "context"

type ToolDefinition struct {
	Name        string
	Description string
	Schema      any
}

type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, input string) (string, error)
}
