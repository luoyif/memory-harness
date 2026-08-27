package harness_test

import (
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func v3AssetPayload(body string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"asset_id": "asset-v3-test", "asset_type": "mcp", "title": "MCP contract", "body": body,
		"source_memory_ids": []string{"mem-source-test"}, "validation_status": "not_run",
	})
	return raw
}

func TestGovernedAssetV3ValidationIsServerOwnedAndBlocksUnsafeActivation(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	created, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-governed-v3-validation", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate", Payload: v3AssetPayload("MCP candidate only; external interface remains unspecified"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "v3-validation-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := a.Harness.ProposeRevision(ctx, created.ObjectID, harness.ProposeRevisionInput{
		Payload: v3AssetPayload("MCP candidate only; external interface remains unspecified"), ExpectedRevision: 1,
		EditReason: "attempt activation", TargetStatus: "active", IdempotencyKey: "v3-validation-r2",
		Validation: json.RawMessage(`{"status":"passed","source":"client-claim"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var badValidation map[string]any
	if err := json.Unmarshal(bad.Validation, &badValidation); err != nil {
		t.Fatal(err)
	}
	if badValidation["status"] != "failed" || badValidation["source"] != nil {
		t.Fatalf("client validation was trusted: %#v", badValidation)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, bad.ReviewID, "approve", "owner-test", "should fail"); err == nil {
		t.Fatal("unsafe V3 asset activation was approved")
	}

	good, err := a.Harness.ProposeRevision(ctx, created.ObjectID, harness.ProposeRevisionInput{
		Payload:          v3AssetPayload("MCP server uses stdio transport, exposes a tool manifest, and declares permission boundaries."),
		ExpectedRevision: 1, EditReason: "add transport, manifest and permissions", TargetStatus: "active",
		IdempotencyKey: "v3-validation-r3", Validation: json.RawMessage(`{"status":"failed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var goodValidation map[string]any
	_ = json.Unmarshal(good.Validation, &goodValidation)
	if goodValidation["status"] != "passed" {
		t.Fatalf("server validation=%#v", goodValidation)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, good.ReviewID, "approve", "owner-test", "validated"); err != nil {
		t.Fatal(err)
	}
	current, err := a.Harness.Object(ctx, created.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "active" || current.CurrentRevision != good.Revision {
		t.Fatalf("current=%#v", current)
	}
	if _, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: created.ObjectID, TypeID: harness.GovernedAgentAssetTypeV3, ProjectID: portfolio.PersonalProjectID,
		Status: "active", Payload: v3AssetPayload("MCP server uses HTTP transport and tool manifest."),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "v3-bypass-attempt",
	}); err == nil {
		t.Fatal("protected active object accepted direct Materialize bypass")
	}
}
