package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestContextBridgePlanReceiptOutcomeIsScopedAndObservationOnly(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	allowed, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "context-allowed", Name: "Context Allowed", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "context-denied", Name: "Context Denied", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-context-1", "adapter-test", "context-session", "user", "2026-08-22T15:00:00Z", "Context Bridge 应证明受治理记忆是否真正进入外部 Harness。"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Portfolio.LinkRecord(ctx, "evidence", captured.EvidenceID, allowed.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"product_id":"context-report","product_type":"report","title":"Context Bridge","summary":"Context observability","body":"Plan 与 Receipt 必须可核验。","format":"markdown","source_refs":["ev-context-1"],"locked_fields":[],"generation_status":"auto"}`)
	object, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-context-report", TypeID: harness.KnowledgeProductTypeV1, ProjectID: allowed.ProjectID, Status: "active",
		Payload: payload, Confidence: 1, Importance: .8, SourceEvidenceIDs: []string{captured.EvidenceID},
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: "context-report-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.RebuildProjectIndex(ctx, allowed.ProjectID); err != nil {
		t.Fatal(err)
	}
	baseRevision, baseHash := object.CurrentRevision, object.Revision.ContentHash

	credential, err := a.Agents.Create(ctx, agentauth.CreateInput{
		Name: "Generic Rich Adapter", Kind: "adapter", ProjectIDs: []string{allowed.ProjectID},
		Permissions: []string{agentauth.PermissionContextPlan, agentauth.PermissionContextReceipt, agentauth.PermissionOutcomeReport},
	})
	if err != nil {
		t.Fatal(err)
	}
	limited, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "Legacy MCP", Kind: "mcp", ProjectIDs: []string{allowed.ProjectID}, Permissions: []string{agentauth.PermissionMemoryRead}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	capability := contextbridge.ContextCapabilitySet{
		SchemaVersion: contextbridge.CapabilitySchemaVersion, CapabilitySetID: "caps-generic-v1", AdapterID: "generic-http", Runtime: "test-harness", ProtocolVersion: "1", Transport: "http",
		Capabilities: []string{"recall", "pre_turn_injection", "context_receipt", "outcome_feedback"}, MaxContextItems: 16, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 2,
		SupportsIdempotency: true, Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"},
	}
	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/handshake", limited.Token, capability)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("legacy token gained context.plan: status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/handshake", credential.Token, capability)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("handshake status=%d body=%s", resp.StatusCode, raw)
	}
	var negotiation contextbridge.NegotiationResult
	if err := json.Unmarshal(raw, &negotiation); err != nil || negotiation.Level != "L5" || negotiation.CapabilitySetHash == "" {
		t.Fatalf("negotiation=%#v err=%v", negotiation, err)
	}

	planRequest := contextbridge.PlanRequest{ProjectID: allowed.ProjectID, Query: "Context Bridge Plan Receipt", CapabilitySet: capability, Budget: contextbridge.ContextBudget{MaxTokens: 2048}, IdempotencyKey: "context-plan-1"}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/plans", credential.Token, planRequest)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("plan status=%d body=%s", resp.StatusCode, raw)
	}
	var planned contextbridge.PlanResult
	if err := json.Unmarshal(raw, &planned); err != nil {
		t.Fatal(err)
	}
	if planned.RunID == "" || planned.Plan.PlanHash == "" || len(planned.Plan.Items) == 0 {
		t.Fatalf("plan=%#v", planned)
	}
	for _, status := range planned.DeliveryStatus {
		if status != "delivery_unverified" {
			t.Fatalf("plan claimed delivery before receipt: %#v", planned.DeliveryStatus)
		}
	}

	deniedRequest := planRequest
	deniedRequest.ProjectID, deniedRequest.IdempotencyKey = denied.ProjectID, "context-plan-denied"
	resp, _ = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/plans", credential.Token, deniedRequest)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-project plan status=%d", resp.StatusCode)
	}
	receiptItems := make([]contextbridge.ReceiptItem, 0, len(planned.Plan.Items))
	for _, item := range planned.Plan.Items {
		receiptItems = append(receiptItems, contextbridge.ReceiptItem{ItemID: item.ItemID, Status: "delivered", Revision: item.Revision, ContentHash: item.ContentHash, Presentation: item.Presentation})
	}
	badItems := append([]contextbridge.ReceiptItem(nil), receiptItems...)
	badItems[0].ContentHash = "tampered"
	badReceipt := contextbridge.ReceiptRequest{RunID: planned.RunID, Receipt: contextbridge.ContextReceipt{
		SchemaVersion: contextbridge.ReceiptSchemaVersion, ReceiptID: "receipt-bad", PlanID: planned.Plan.PlanID, EvidenceLevel: "harness_observed", Completeness: "complete", Items: badItems,
		Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"}, IdempotencyKey: "receipt-bad", ReceivedAt: "2026-08-22T15:00:03Z",
	}}
	resp, _ = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/receipts", credential.Token, badReceipt)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("tampered receipt status=%d", resp.StatusCode)
	}

	goodReceipt := contextbridge.ReceiptRequest{RunID: planned.RunID, Receipt: contextbridge.ContextReceipt{
		SchemaVersion: contextbridge.ReceiptSchemaVersion, ReceiptID: "receipt-good", PlanID: planned.Plan.PlanID, EvidenceLevel: "harness_observed", Completeness: "complete", Items: receiptItems,
		Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"}, IdempotencyKey: "receipt-good", ReceivedAt: "2026-08-22T15:00:04Z",
	}}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/context/receipts", credential.Token, goodReceipt)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("receipt status=%d body=%s", resp.StatusCode, raw)
	}
	var received contextbridge.ReceiptResult
	if err := json.Unmarshal(raw, &received); err != nil {
		t.Fatal(err)
	}
	for _, status := range received.DeliveryStatus {
		if status != "delivered" {
			t.Fatalf("receipt delivery=%#v", received.DeliveryStatus)
		}
	}
	outcome := contextbridge.OutcomeFeedback{
		SchemaVersion: contextbridge.OutcomeSchemaVersion, OutcomeID: "outcome-1", ProjectID: allowed.ProjectID, RunID: planned.RunID,
		PlanID: planned.Plan.PlanID, ReceiptID: received.Receipt.ReceiptID, Source: "user_feedback",
		Metrics: []contextbridge.OutcomeMetric{{Name: "task_success", Value: json.RawMessage(`true`), Confidence: .9}},
		Cost:    contextbridge.OutcomeCost{Tokens: 512, LatencyMS: 900}, IdempotencyKey: "outcome-1", ObservedAt: "2026-08-22T15:05:00Z",
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/outcomes", credential.Token, outcome)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("outcome status=%d body=%s", resp.StatusCode, raw)
	}
	var observed contextbridge.OutcomeResult
	if err := json.Unmarshal(raw, &observed); err != nil || observed.RunID == "" || observed.RunID == planned.RunID {
		t.Fatalf("outcome result=%#v err=%v", observed, err)
	}

	after, err := a.Harness.Object(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentRevision != baseRevision || after.Revision.ContentHash != baseHash {
		t.Fatalf("outcome mutated governed object: before=%d/%s after=%d/%s", baseRevision, baseHash, after.CurrentRevision, after.Revision.ContentHash)
	}
	exchange, err := a.Context.Exchange(ctx, planned.RunID)
	if err != nil || exchange.Receipt == nil || exchange.Plan == nil {
		t.Fatalf("exchange=%#v err=%v", exchange, err)
	}
	if exchange.Run.Status != "completed" {
		t.Fatalf("context run status=%s", exchange.Run.Status)
	}
}
