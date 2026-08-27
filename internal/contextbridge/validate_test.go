package contextbridge

import (
	"encoding/json"
	"testing"
)

func fixturePlan() ContextPlan {
	return ContextPlan{SchemaVersion: PlanSchemaVersion, PlanID: "plan-1", ProjectID: "project-a", AgentID: "agent-a", RequestFingerprint: "sha256:req", IdempotencyKey: "idem-plan-1", CreatedAt: "2026-08-22T12:00:00Z", Budget: ContextBudget{MaxTokens: 4096}, Items: []ContextPlanItem{{ItemID: "item-1", SourceKind: "object", SourceID: "obj-1", Revision: 3, ContentHash: "sha256:abc", ProjectID: "project-a", ReasonCodes: []string{"project_scope", "semantic_match"}, Priority: 90, Presentation: "summary"}}}
}

func TestContractsFailClosedAndHashDeterministically(t *testing.T) {
	cap := ContextCapabilitySet{SchemaVersion: CapabilitySchemaVersion, CapabilitySetID: "caps-1", AdapterID: "adapter-1", Runtime: "generic", ProtocolVersion: "1", Transport: "mcp", Capabilities: []string{"recall", "context_receipt"}, MaxContextItems: 64, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 4, SupportsIdempotency: true, Retention: RetentionPolicy{Mode: "session", Redaction: "supported"}}
	if err := ValidateCapabilitySet(cap); err != nil {
		t.Fatal(err)
	}
	cap.Capabilities = append(cap.Capabilities, "magic_vendor_hook")
	if err := ValidateCapabilitySet(cap); err == nil {
		t.Fatal("unknown capability accepted")
	}

	plan := fixturePlan()
	if err := ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
	first, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPlan(plan)
	if err != nil || first != second {
		t.Fatalf("non-deterministic hash %q %q err=%v", first, second, err)
	}
	plan.Items[0].ProjectID = "project-b"
	if err := ValidatePlan(plan); err == nil {
		t.Fatal("cross-project plan item accepted")
	}
}

func TestReceiptCannotInventDeliveredContext(t *testing.T) {
	plan := fixturePlan()
	statuses := EffectiveDeliveryStatus(plan, nil)
	if statuses["item-1"] != "delivery_unverified" {
		t.Fatalf("missing receipt=%#v", statuses)
	}
	receipt := ContextReceipt{SchemaVersion: ReceiptSchemaVersion, ReceiptID: "receipt-1", PlanID: plan.PlanID, ProjectID: plan.ProjectID, AgentID: plan.AgentID, EvidenceLevel: "harness_observed", Completeness: "complete", Items: []ReceiptItem{{ItemID: "item-1", Status: "delivered", Revision: 3, ContentHash: "sha256:abc"}}, Retention: RetentionPolicy{Mode: "session", Redaction: "supported"}, IdempotencyKey: "idem-receipt-1", ReceivedAt: "2026-08-22T12:00:01Z"}
	if err := ValidateReceipt(plan, receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Items[0].ContentHash = "sha256:tampered"
	if err := ValidateReceipt(plan, receipt); err == nil {
		t.Fatal("receipt hash mismatch accepted")
	}
	receipt.Items = []ReceiptItem{{ItemID: "not-planned", Status: "delivered"}}
	if err := ValidateReceipt(plan, receipt); err == nil {
		t.Fatal("receipt invented an unplanned item")
	}
}

func TestOutcomeIsObservationNotTruthMutation(t *testing.T) {
	outcome := OutcomeFeedback{SchemaVersion: OutcomeSchemaVersion, OutcomeID: "outcome-1", ProjectID: "project-a", AgentID: "agent-a", RunID: "run-1", Source: "user_feedback", Metrics: []OutcomeMetric{{Name: "task_success", Value: json.RawMessage(`true`), Confidence: .8}}, Cost: OutcomeCost{Tokens: 400, LatencyMS: 1200}, IdempotencyKey: "idem-outcome-1", ObservedAt: "2026-08-22T12:05:00Z"}
	if err := ValidateOutcome(outcome); err != nil {
		t.Fatal(err)
	}
	outcome.Metrics[0].Confidence = 1.2
	if err := ValidateOutcome(outcome); err == nil {
		t.Fatal("invalid confidence accepted")
	}
}
