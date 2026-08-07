package main

import (
	"context"
	"log"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/app/internal/api"
	"github.com/unnati2401/observable-ai-agent/app/internal/tracing"
)

func main() {

	ctx := context.Background()

	shutdown, err := tracing.InitTracing(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer shutdown(ctx)

	api.RegisterRoutes()

	log.Println("Server listening on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
