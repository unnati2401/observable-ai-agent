package openai

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("github.com/unnati2401/observable-ai-agent/internal/llm/openai")
