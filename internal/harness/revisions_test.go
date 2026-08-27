package harness_test

import (
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func governedPayload(assetID, title, body string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"asset_id": assetID, "asset_type": "skill", "title": title, "body": body,
		"source_memory_ids": []string{}, "validation_status": "passed",
	})
	return raw
}

func TestRevisionProposalDoesNotMoveCurrentUntilApproval(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	initial, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-governed-test", TypeID: harness.GovernedAgentAssetTypeV2, ProjectID: portfolio.PersonalProjectID,
		Status: "candidate", Payload: governedPayload("asset-test", "Old title", "old body"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "initial-governed-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if initial.CurrentRevision != 1 {
		t.Fatalf("initial revision=%d", initial.CurrentRevision)
	}

	review, err := a.Harness.ProposeRevision(ctx, initial.ObjectID, harness.ProposeRevisionInput{
		ExpectedRevision: initial.CurrentRevision, EditReason: "owner updates governed asset",
		Payload: governedPayload("asset-test", "New title", "new body"), TargetStatus: "active",
		IdempotencyKey: "edit-governed-test", RequestedBy: "owner-test", Validation: json.RawMessage(`{"status":"passed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.Revision != 2 || review.BaseRevision != 1 || review.Status != "pending" || review.EditReason == "" {
		t.Fatalf("review=%#v", review)
	}
	current, err := a.Harness.Object(ctx, initial.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevision != 1 || current.Status != "candidate" {
		t.Fatalf("proposal moved current %#v", current)
	}
	var currentPayload map[string]any
	_ = json.Unmarshal(current.Revision.Payload, &currentPayload)
	if currentPayload["body"] != "old body" {
		t.Fatalf("current payload changed %#v", currentPayload)
	}

	approved, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "validated")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || approved.ActivatedAt == "" {
		t.Fatalf("approved=%#v", approved)
	}
	current, err = a.Harness.Object(ctx, initial.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevision != 2 || current.Status != "active" {
		t.Fatalf("not activated %#v", current)
	}
	_ = json.Unmarshal(current.Revision.Payload, &currentPayload)
	if currentPayload["body"] != "new body" {
		t.Fatalf("payload=%#v", currentPayload)
	}
}

func TestCompetingRevisionBecomesStaleAndRollbackAppendsRevision(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	initial, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-governed-rollback", TypeID: harness.GovernedAgentAssetTypeV2, ProjectID: portfolio.PersonalProjectID,
		Status: "candidate", Payload: governedPayload("asset-rollback", "R1", "body-r1"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "rollback-initial",
	})
	if err != nil {
		t.Fatal(err)
	}
	activate, err := a.Harness.ProposeRevision(ctx, initial.ObjectID, harness.ProposeRevisionInput{Payload: governedPayload("asset-rollback", "R2", "body-r2"), ExpectedRevision: 1, EditReason: "activate r2", TargetStatus: "active", IdempotencyKey: "rollback-r2", Validation: json.RawMessage(`{"status":"passed"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, activate.ReviewID, "approve", "owner-test", "activate r2"); err != nil {
		t.Fatal(err)
	}

	left, err := a.Harness.ProposeRevision(ctx, initial.ObjectID, harness.ProposeRevisionInput{Payload: governedPayload("asset-rollback", "R3-left", "left"), ExpectedRevision: 2, EditReason: "left edit", TargetStatus: "active", IdempotencyKey: "rollback-left", Validation: json.RawMessage(`{"status":"passed"}`)})
	if err != nil {
		t.Fatal(err)
	}
	right, err := a.Harness.ProposeRevision(ctx, initial.ObjectID, harness.ProposeRevisionInput{Payload: governedPayload("asset-rollback", "R3-right", "right"), ExpectedRevision: 2, EditReason: "right edit", TargetStatus: "active", IdempotencyKey: "rollback-right", Validation: json.RawMessage(`{"status":"passed"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if left.BaseRevision != 2 || right.BaseRevision != 2 {
		t.Fatalf("bases left=%d right=%d", left.BaseRevision, right.BaseRevision)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, left.ReviewID, "approve", "owner-test", "choose left"); err != nil {
		t.Fatal(err)
	}
	stale, err := a.Harness.RevisionReview(ctx, right.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "stale" {
		t.Fatalf("competing review=%#v", stale)
	}
	current, err := a.Harness.Object(ctx, initial.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevision != left.Revision {
		t.Fatalf("current=%d left=%d", current.CurrentRevision, left.Revision)
	}

	rollback, err := a.Harness.ProposeRollback(ctx, initial.ObjectID, 1, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Revision <= current.CurrentRevision || rollback.RollbackFromRevision != 1 {
		t.Fatalf("rollback=%#v current=%d", rollback, current.CurrentRevision)
	}
	beforeApproval, _ := a.Harness.Object(ctx, initial.ObjectID)
	if beforeApproval.CurrentRevision != current.CurrentRevision {
		t.Fatal("rollback proposal moved current pointer")
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, rollback.ReviewID, "approve", "owner-test", "restore r1"); err != nil {
		t.Fatal(err)
	}
	restored, err := a.Harness.Object(ctx, initial.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CurrentRevision != rollback.Revision {
		t.Fatalf("rollback did not append: %#v", restored)
	}
	var payload map[string]any
	_ = json.Unmarshal(restored.Revision.Payload, &payload)
	if payload["body"] != "body-r1" {
		t.Fatalf("rollback body=%#v", payload)
	}
}
