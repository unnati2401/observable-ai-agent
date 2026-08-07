package api

import (
	"encoding/json"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/internal/otelutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/unnati2401/observable-ai-agent/app/internal/api")

type Response struct {
	Message string `json:"message"`
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	_, span := tracer.Start(r.Context(), "HelloHandler", trace.WithAttributes(attribute.String("http.method", r.Method)))
	defer span.End()

	w.Header().Set("Content-Type", "application/json")

	response := Response{
		Message: "Hello from Observable AI Agent",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		otelutil.RecordError(span, err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
