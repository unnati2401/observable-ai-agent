// Package config loads application configuration from environment variables.
package config

import "os"

// Config holds all runtime configuration for the HTTP application.
type Config struct {
	// Address is the host:port the HTTP server listens on.
	Address string
	// OTELExporterEndpoint is the address of the OpenTelemetry Collector
	// (grpc), e.g. "localhost:4317".
	OTELExporterEndpoint string
	// OpenAIAPIKey is the API key used to authenticate with the LLM provider.
	OpenAIAPIKey string
	// OpenAIModel is the model used for chat completions.
	OpenAIModel string
}

// FromEnv loads configuration from environment variables, applying defaults
// where sensible.
//
//	ADDRESS                    (default ":8080")
//	OTEL_EXPORTER_OTLP_ENDPOINT (default "localhost:4317")
//	OPENAI_API_KEY              (required at request time)
//	OPENAI_MODEL                (default "gpt-4o-mini")
func FromEnv() Config {
	return Config{
		Address:              getenv("ADDRESS", ":8080"),
		OTELExporterEndpoint: getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OpenAIAPIKey:         os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:          getenv("OPENAI_MODEL", "gpt-4o-mini"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
