package api

import (
	"net/http"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
)

func RegisterRoutes(a *agent.Agent) {
	http.HandleFunc("/hello", HelloHandler)
	http.HandleFunc("/chat", ChatHandler(a))
}
