package contextbridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "context", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("fixture %s is not valid JSON", name)
	}
	return raw
}

func TestFrozenFixturesMatchRuntimeValidators(t *testing.T) {
	var capabilities ContextCapabilitySet
	if err := json.Unmarshal(fixtureBytes(t, "capability-set.json"), &capabilities); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCapabilitySet(capabilities); err != nil {
		t.Fatalf("capability fixture: %v", err)
	}

	var plan ContextPlan
	if err := json.Unmarshal(fixtureBytes(t, "context-plan.json"), &plan); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePlan(plan); err != nil {
		t.Fatalf("plan fixture: %v", err)
	}

	var receipt ContextReceipt
	if err := json.Unmarshal(fixtureBytes(t, "context-receipt.json"), &receipt); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceipt(plan, receipt); err != nil {
		t.Fatalf("receipt fixture: %v", err)
	}

	var outcome OutcomeFeedback
	if err := json.Unmarshal(fixtureBytes(t, "outcome-feedback.json"), &outcome); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOutcome(outcome); err != nil {
		t.Fatalf("outcome fixture: %v", err)
	}
}

func TestSchemaFilesAreValidJSONAndVersionPinned(t *testing.T) {
	files := map[string]string{
		"context-capability-set.v1alpha1.schema.json": CapabilitySchemaVersion,
		"context-plan.v1alpha1.schema.json":           PlanSchemaVersion,
		"context-receipt.v1alpha1.schema.json":        ReceiptSchemaVersion,
		"outcome-feedback.v1alpha1.schema.json":       OutcomeSchemaVersion,
	}
	for name, version := range files {
		raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "context", name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("schema %s: %v", name, err)
		}
		properties := schema["properties"].(map[string]any)
		schemaVersion := properties["schema_version"].(map[string]any)["const"]
		if schemaVersion != version {
			t.Fatalf("schema %s pins %v, want %s", name, schemaVersion, version)
		}
	}
}
