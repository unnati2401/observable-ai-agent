package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("ADDRESS", "")

	cfg := FromEnv()

	if cfg.Address != ":8080" {
		t.Errorf("Address = %q, want :8080", cfg.Address)
	}
	if cfg.OTELExporterEndpoint != "localhost:4317" {
		t.Errorf("OTELExporterEndpoint = %q, want localhost:4317", cfg.OTELExporterEndpoint)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("OpenAIModel = %q, want gpt-4o-mini", cfg.OpenAIModel)
	}
	if cfg.OpenAIAPIKey != "" {
		t.Errorf("OpenAIAPIKey = %q, want empty", cfg.OpenAIAPIKey)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-123")
	t.Setenv("OPENAI_MODEL", "gpt-5")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.example:4317")
	t.Setenv("ADDRESS", ":9000")

	cfg := FromEnv()

	if cfg.Address != ":9000" {
		t.Errorf("Address = %q, want :9000", cfg.Address)
	}
	if cfg.OTELExporterEndpoint != "collector.example:4317" {
		t.Errorf("OTELExporterEndpoint = %q, want collector.example:4317", cfg.OTELExporterEndpoint)
	}
	if cfg.OpenAIModel != "gpt-5" {
		t.Errorf("OpenAIModel = %q, want gpt-5", cfg.OpenAIModel)
	}
	if cfg.OpenAIAPIKey != "sk-test-123" {
		t.Errorf("OpenAIAPIKey = %q, want sk-test-123", cfg.OpenAIAPIKey)
	}
}
