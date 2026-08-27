package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func ft3Capability() contextbridge.ContextCapabilitySet {
	return contextbridge.ContextCapabilitySet{
		SchemaVersion: contextbridge.CapabilitySchemaVersion, CapabilitySetID: "caps-ft3-api", AdapterID: "ft3-api-adapter", Runtime: "test-harness", ProtocolVersion: "1", Transport: "http",
		Capabilities:    []string{"recall", "pre_turn_injection", "context_receipt", "outcome_feedback"},
		MaxContextItems: 8, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 2, SupportsIdempotency: true,
		Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"},
	}
}

func ft3Receipt(plan contextbridge.ContextPlan) contextbridge.ContextReceipt {
	items := []contextbridge.ReceiptItem{}
	for _, item := range plan.Items {
		items = append(items, contextbridge.ReceiptItem{ItemID: item.ItemID, Status: "delivered", Revision: item.Revision, ContentHash: item.ContentHash, Presentation: item.Presentation})
	}
	return contextbridge.ContextReceipt{
		SchemaVersion: contextbridge.ReceiptSchemaVersion, ReceiptID: "receipt-ft3-api", PlanID: plan.PlanID,
		EvidenceLevel: "harness_observed", Completeness: "complete", Items: items,
		Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"}, IdempotencyKey: "receipt-ft3-api",
	}
}

func TestExperienceOwnerAPIRebuildEvaluateAndReviewGate(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "experience-api", Name: "Experience API", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "验证 FT3 API", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(ctx, project.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(ctx, project.ProjectID, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}
	planned, err := a.Context.CompilePlan(ctx, contextbridge.PlanRequest{
		ProjectID: project.ProjectID, AgentID: "agent-ft3-api", Query: "FT3 API negative case", CapabilitySet: ft3Capability(), IdempotencyKey: "plan-ft3-api",
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := a.Context.RecordReceipt(ctx, contextbridge.ReceiptRequest{RunID: planned.RunID, AgentID: "agent-ft3-api", Receipt: ft3Receipt(planned.Plan)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Context.RecordOutcome(ctx, "agent-ft3-api", contextbridge.OutcomeFeedback{
		SchemaVersion: contextbridge.OutcomeSchemaVersion, OutcomeID: "outcome-ft3-api", ProjectID: project.ProjectID, RunID: planned.RunID,
		PlanID: planned.Plan.PlanID, ReceiptID: receipt.Receipt.ReceiptID, Source: "ft3-api-adapter",
		Metrics: []contextbridge.OutcomeMetric{{Name: "turn_completed", Value: json.RawMessage(`1`), Confidence: 1}},
		Cost:    contextbridge.OutcomeCost{Tokens: 50, LatencyMS: 100}, IdempotencyKey: "outcome-ft3-api",
	}); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/projects/"+project.ProjectID+"/experience/rebuild", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rebuild status=%d body=%s", resp.StatusCode, raw)
	}
	var rebuilt struct {
		Cases []struct {
			ObjectID        string `json:"object_id"`
			CurrentRevision int    `json:"current_revision"`
			Status          string `json:"status"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &rebuilt); err != nil || len(rebuilt.Cases) != 1 {
		t.Fatalf("rebuild=%s err=%v", raw, err)
	}
	caseID := rebuilt.Cases[0].ObjectID

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/experience/cases/"+caseID, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("case status=%d body=%s", resp.StatusCode, raw)
	}
	var detail struct {
		Object struct {
			CurrentRevision int    `json:"current_revision"`
			Status          string `json:"status"`
		} `json:"object"`
		Case experience.Case `json:"case"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Case.Result != "unknown" || detail.Case.Delivery.Delivered != len(planned.Plan.Items) {
		t.Fatalf("raw Case claimed success: %#v", detail.Case)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/experience/cases/"+caseID+"/activation-proposal", "", map[string]any{
		"expected_revision": detail.Object.CurrentRevision, "edit_reason": "must fail before evaluation", "idempotency_key": "activate-before-eval",
	})
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "Evaluation") {
		t.Fatalf("unevaluated activation status=%d body=%s", resp.StatusCode, raw)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/experience/cases/"+caseID+"/evaluations", "", map[string]any{
		"protocol": "owner-fixture", "evaluator_id": "owner-review", "evaluator_version": "1.0.0", "verdict": "fail",
		"dimensions": []map[string]any{{"name": "task_correctness", "verdict": "fail", "confidence": 1.0}},
		"expected":   "answer from delivered context", "observed": "NO_CONTEXT", "confidence": 1.0, "sample_size": 1, "idempotency_key": "eval-ft3-api",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("evaluation status=%d body=%s", resp.StatusCode, raw)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/experience/cases/"+caseID, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("case after evaluation status=%d body=%s", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Case.Result != "fail" || detail.Case.PrimaryFailureDimension != "task_correctness" {
		t.Fatalf("negative evaluation missing: %#v", detail.Case)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/experience/cases/"+caseID+"/activation-proposal", "", map[string]any{
		"expected_revision": detail.Object.CurrentRevision, "edit_reason": "retain evaluated failure", "idempotency_key": "activate-ft3-api",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("activation proposal status=%d body=%s", resp.StatusCode, raw)
	}
	var review struct {
		ReviewID string `json:"review_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(raw, &review); err != nil || review.Status != "pending" {
		t.Fatalf("review=%s err=%v", raw, err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/experience/cases/"+caseID, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("candidate status=%d body=%s", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Object.Status != "candidate" {
		t.Fatalf("proposal bypassed review gate: %#v", detail.Object)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/revision-reviews/"+review.ReviewID+"/decision", "", map[string]any{"decision": "approve", "note": "Owner accepts governed negative Case"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("review decision status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/experience/cases/"+caseID, "", nil)
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Object.Status != "active" || detail.Case.Result != "fail" {
		t.Fatalf("approved negative Case=%#v %#v", detail.Object, detail.Case)
	}

	credential, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "experience-api-agent", Kind: "test", Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionProjectRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewServer(server.New(a).Handler())
	defer secure.Close()
	resp, raw = agentRequest(t, secure.Client(), http.MethodGet, secure.URL+"/v1/experience/cases?project_id="+project.ProjectID, credential.Token, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner Experience API status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, secure.Client(), http.MethodPost, secure.URL+"/v1/agent/recall", credential.Token, map[string]any{
		"project_id": project.ProjectID, "query": "task_correctness", "kinds": []string{"experience"}, "limit": 10,
	})
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(raw), "Owner-only") {
		t.Fatalf("Agent explicit Experience recall should fail closed status=%d body=%s", resp.StatusCode, raw)
	}
}
