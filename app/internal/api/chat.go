package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/otelutil"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Content string `json:"content"`
}

// ChatHandler runs a single user message through the agent and returns the
// final response. The whole agent execution - planning, LLM calls and tool
// executions - is recorded as child spans of this handler's span.
func ChatHandler(a *agent.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "ChatHandler", trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", "/chat"),
		))
		defer span.End()

		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			otelutil.RecordError(span, err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			err := errors.New("message is required")
			otelutil.RecordError(span, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response, err := a.Run(ctx, []agent.Message{{Role: agent.UserRole, Content: req.Message}})
		if err != nil {
			otelutil.RecordError(span, err)
			http.Error(w, "agent execution failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ChatResponse{Content: response.Content}); err != nil {
			otelutil.RecordError(span, err)
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}
