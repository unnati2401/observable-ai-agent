package agent

import (
	"go.opentelemetry.io/otel"
)

const (
	// AgentName identifies the agent on the Agent.Run span.
	AgentName = "observable-ai-agent"
	// AgentVersion identifies the agent release on the Agent.Run span.
	AgentVersion = "0.1.0"
)

var tracer = otel.Tracer("github.com/unnati2401/observable-ai-agent/internal/agent")
