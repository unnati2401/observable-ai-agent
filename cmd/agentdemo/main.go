// Command agentdemo runs one conversation against the real OpenAI API using
// the Current Time tool, demonstrating a complete agent execution cycle with
// actual LLM tool calling.
//
// It requires an OPENAI_API_KEY environment variable. An optional first
// argument overrides the default prompt.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/llm/openai"
	"github.com/unnati2401/observable-ai-agent/internal/tool/currenttime"
)

func main() {
	ctx := context.Background()

	prompt := "What is the current time in New York?"
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	client := openai.NewClient(openai.WithModel(openai.DefaultModel))
	a := agent.New(client, currenttime.Tool{})

	response, err := a.Run(ctx, []agent.Message{{Role: agent.UserRole, Content: prompt}})
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	fmt.Println(response.Content)
}
