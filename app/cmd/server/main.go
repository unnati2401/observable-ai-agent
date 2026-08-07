package main

import (
	"context"
	"log"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/app/internal/api"
	"github.com/unnati2401/observable-ai-agent/app/internal/config"
	"github.com/unnati2401/observable-ai-agent/app/internal/tracing"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/llm/openai"
	"github.com/unnati2401/observable-ai-agent/internal/tool/currenttime"
)

func main() {
	ctx := context.Background()

	cfg := config.FromEnv()

	shutdown, err := tracing.InitTracing(ctx, cfg.OTELExporterEndpoint)
	if err != nil {
		log.Fatal(err)
	}
	defer shutdown(ctx)

	llmClient := openai.NewClient(
		openai.WithAPIKey(cfg.OpenAIAPIKey),
		openai.WithModel(cfg.OpenAIModel),
	)

	a := agent.New(llmClient, currenttime.Tool{})

	api.RegisterRoutes(a)

	log.Printf("Server listening on %s (traces -> %s)", cfg.Address, cfg.OTELExporterEndpoint)
	if err := http.ListenAndServe(cfg.Address, nil); err != nil {
		log.Fatal(err)
	}
}
