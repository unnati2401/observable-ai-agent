package main

import (
	"log"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/app/internal/api"
)

func main() {
	api.RegisterRoutes()

	log.Println("Server listening on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
