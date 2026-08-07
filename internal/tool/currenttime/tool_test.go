package currenttime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefinition(t *testing.T) {
	def := Tool{}.Definition()

	if def.Name != ToolName {
		t.Errorf("Name = %q, want %q", def.Name, ToolName)
	}
	if def.Description == "" {
		t.Error("Description is empty")
	}

	schema, ok := def.Schema.(map[string]any)
	if !ok {
		t.Fatalf("Schema = %T, want map[string]any", def.Schema)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	if _, ok := properties["timezone"]; !ok {
		t.Error("schema properties missing timezone")
	}
}

func TestExecute(t *testing.T) {
	fixed := time.Date(2026, 8, 7, 12, 30, 0, 0, time.FixedZone("UTC", 0))
	tool := Tool{Now: func() time.Time { return fixed }}

	result, err := tool.Execute(context.Background(), `{"timezone":"America/New_York"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var out struct {
		Timezone  string `json:"timezone"`
		LocalTime string `json:"local_time"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if out.Timezone != "America/New_York" {
		t.Errorf("timezone = %q, want America/New_York", out.Timezone)
	}
	if out.LocalTime != "2026-08-07T08:30:00-04:00" {
		t.Errorf("local_time = %q, want 2026-08-07T08:30:00-04:00 (EDT)", out.LocalTime)
	}
}

func TestExecuteEmptyArguments(t *testing.T) {
	tool := Tool{Now: func() time.Time { return time.Unix(0, 0) }}

	_, err := tool.Execute(context.Background(), "")
	if err == nil {
		t.Fatal("Execute = nil error, want error (timezone required)")
	}
	if !strings.Contains(err.Error(), "timezone is required") {
		t.Errorf("error = %q, want mention of required timezone", err)
	}
}

func TestExecuteInvalidJSON(t *testing.T) {
	tool := Tool{}

	_, err := tool.Execute(context.Background(), "{not json")
	if err == nil {
		t.Fatal("Execute = nil error, want error")
	}
	if !strings.Contains(err.Error(), "parse arguments") {
		t.Errorf("error = %q, want mention of parsing", err)
	}
}

func TestExecuteUnknownTimezone(t *testing.T) {
	tool := Tool{}

	_, err := tool.Execute(context.Background(), `{"timezone":"Not/AZone"}`)
	if err == nil {
		t.Fatal("Execute = nil error, want error")
	}
	if !strings.Contains(err.Error(), "unknown timezone") {
		t.Errorf("error = %q, want mention of unknown timezone", err)
	}
}
