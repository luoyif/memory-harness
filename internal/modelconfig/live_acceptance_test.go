package modelconfig_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/store"
)

// TestLiveStructuredOutputAcceptance is opt-in because it uses the configured
// provider and consumes real model tokens. The database path must point to an
// isolated SQLite backup; the test never needs or accepts an API key in an
// environment variable.
func TestLiveStructuredOutputAcceptance(t *testing.T) {
	databasePath := os.Getenv("MEMORYOS_REAL_MODEL_ACCEPTANCE_DB")
	secretHome := os.Getenv("MEMORYOS_REAL_MODEL_SECRET_HOME")
	if databasePath == "" || secretHome == "" {
		t.Skip("set isolated acceptance DB and secret-store home to run the real provider check")
	}
	control, err := store.OpenControl(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	service := modelconfig.New(control, modelconfig.NewDefaultSecretStore(secretHome), nil)
	result, err := service.GenerateJSON(t.Context(), modelconfig.JSONGenerationRequest{
		SystemPrompt: "Copy the supplied acceptance marker into the result. Do not add facts.",
		Input:        json.RawMessage(`{"acceptance_marker":"memoryos-structured-output-ok"}`),
		OutputSchema: json.RawMessage(`{"type":"object","required":["acceptance_marker"],"properties":{"acceptance_marker":{"type":"string","enum":["memoryos-structured-output-ok"]}},"additionalProperties":false}`),
		MaxTokens:    300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Output) != `{"acceptance_marker":"memoryos-structured-output-ok"}` {
		t.Fatalf("unexpected structured output: %s", result.Output)
	}
}
