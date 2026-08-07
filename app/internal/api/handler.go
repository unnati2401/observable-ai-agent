package api

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("observable-ai-agent")

type Response struct {
	Message string `json:"message"`
}

func HelloHandler(w http.ResponseWriter, r *http.Request) {

	_, span := tracer.Start(r.Context(), "HelloHandler")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.route", "/hello"),
	)

	w.Header().Set("Content-Type", "application/json")

	response := Response{
		Message: "Hello from Observable AI Agent",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		span.RecordError(err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	span.SetAttributes(
		attribute.String("response.status", "success"),
	)
}
