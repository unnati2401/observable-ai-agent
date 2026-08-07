// Package currenttime provides a simple demonstration tool that returns the
// current time in a requested IANA timezone.
package currenttime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
)

const (
	ToolName        = "get_current_time"
	ToolDescription = "Get the current date and time in a given IANA timezone (e.g. America/New_York)."
)

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"timezone": map[string]any{
			"type":        "string",
			"description": "IANA timezone name, e.g. America/New_York.",
		},
	},
	"required": []any{"timezone"},
}

// Tool returns the current time for a requested timezone.
type Tool struct {
	// Now, when set, is used instead of time.Now so tests can run
	// deterministically.
	Now func() time.Time
}

// Definition describes the tool to the agent, which forwards it to the LLM.
func (t Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        ToolName,
		Description: ToolDescription,
		Schema:      schema,
	}
}

// Execute parses the {"timezone": "..."} arguments and returns the current
// time in that timezone as a JSON string.
func (t Tool) Execute(_ context.Context, input string) (string, error) {
	var args struct {
		Timezone string `json:"timezone"`
	}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			return "", fmt.Errorf("currenttime: parse arguments: %w", err)
		}
	}
	if args.Timezone == "" {
		return "", errors.New("currenttime: timezone is required")
	}

	loc, err := time.LoadLocation(args.Timezone)
	if err != nil {
		return "", fmt.Errorf("currenttime: unknown timezone %q: %w", args.Timezone, err)
	}

	now := time.Now()
	if t.Now != nil {
		now = t.Now()
	}

	result, err := json.Marshal(map[string]string{
		"timezone":   args.Timezone,
		"local_time": now.In(loc).Format(time.RFC3339),
	})
	if err != nil {
		return "", fmt.Errorf("currenttime: encode result: %w", err)
	}
	return string(result), nil
}
