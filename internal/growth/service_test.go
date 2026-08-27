package growth_test

import (
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestDefaultPipelineExecutesBusinessStagesAndMaterializesInspectableObjects(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "growth-test", Name: "Growth Test", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-growth", "recording", "session-growth", "user", "2026-08-21T13:00:00Z", "我们决定采用项目隔离的长期记忆，并且每次发布前必须先完成回滚演练。"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Growth.Process(t.Context(), growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Compilation.EpisodeID == "" || result.Execution.Status != "completed" || result.Execution.RunID == "" {
		t.Fatalf("result=%#v", result)
	}
	run, err := a.Harness.RunDetail(t.Context(), result.Execution.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Spans) != 6 {
		t.Fatalf("expected six real growth stages, got %#v", run.Spans)
	}
	want := []string{"evidence", "compile", "project", "semantic", "materialize", "index"}
	for index, nodeID := range want {
		if run.Spans[index].NodeID != nodeID || run.Spans[index].Status != "completed" {
			t.Fatalf("span[%d]=%#v", index, run.Spans[index])
		}
	}
	objects, err := a.Harness.ListObjects(t.Context(), project.ProjectID, "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) < 3 {
		t.Fatalf("six-layer materialization did not create inspectable objects: %#v", objects)
	}
	foundEvidence := false
	for _, object := range objects {
		for _, evidenceID := range object.Revision.SourceEvidenceIDs {
			if evidenceID == captured.EvidenceID {
				foundEvidence = true
			}
		}
		if object.Revision.RunID == "" || object.Revision.StageID == "" {
			t.Fatalf("object lost run provenance: %#v", object)
		}
	}
	if !foundEvidence {
		t.Fatalf("no materialized object points back to %s", captured.EvidenceID)
	}
	duplicate, err := a.Growth.Process(t.Context(), growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil || !duplicate.Execution.Duplicate || duplicate.Compilation.EpisodeID != result.Compilation.EpisodeID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}
}

func TestProjectBriefHumanLockedBodySurvivesAutomaticRegeneration(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "brief-lock", Name: "Brief Lock", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-brief-1", "recording", "session-brief-1", "user", "2026-08-21T10:00:00Z", "我们决定使用项目简报跟踪当前开发状态。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: first.SessionID, EvidenceIDs: []string{first.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	products, err := a.Harness.ListObjects(ctx, project.ProjectID, harness.KnowledgeProductTypeV1, "", 10)
	if err != nil || len(products) != 1 {
		t.Fatalf("products=%#v err=%v", products, err)
	}
	product := products[0]
	var payload map[string]any
	if err := json.Unmarshal(product.Revision.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload["body"] = "# Owner locked brief\n\n这段正文由 Owner 人工维护，自动重建不得覆盖。"
	payload["locked_fields"] = []string{"body"}
	payload["generation_status"] = "human_mixed"
	raw, _ := json.Marshal(payload)
	review, err := a.Harness.ProposeRevision(ctx, product.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: product.CurrentRevision, EditReason: "Owner locks the narrative body",
		TargetStatus: "active", IdempotencyKey: "brief-owner-edit", Validation: json.RawMessage(`{"status":"not_run"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "preserve owner narrative"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "新增验收目标", Description: "触发项目简报自动更新", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	second, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-brief-2", "recording", "session-brief-2", "user", "2026-08-22T10:00:00Z", "当前项目正在推进第二阶段开发。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: second.SessionID, EvidenceIDs: []string{second.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	current, err := a.Harness.Object(ctx, product.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	var updated map[string]any
	if err := json.Unmarshal(current.Revision.Payload, &updated); err != nil {
		t.Fatal(err)
	}
	if updated["body"] != payload["body"] {
		t.Fatalf("automatic regeneration overwrote locked body: %#v", updated)
	}
	if updated["generation_status"] != "human_mixed" {
		t.Fatalf("generation status=%#v", updated["generation_status"])
	}
	locked, _ := updated["locked_fields"].([]any)
	if len(locked) != 1 || locked[0] != "body" {
		t.Fatalf("locked_fields=%#v", updated["locked_fields"])
	}
	if current.CurrentRevision <= review.Revision {
		t.Fatalf("automatic rebuild did not append a revision: current=%d review=%d", current.CurrentRevision, review.Revision)
	}
}

func TestActiveBlueprintChangesDefaultGrowthMaterialization(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "blueprint-growth", Name: "Blueprint Growth", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	current, err := a.Blueprints.Current(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(current.Blueprint.Definition)
	var custom blueprint.Definition
	if err := json.Unmarshal(raw, &custom); err != nil {
		t.Fatal(err)
	}
	custom.BlueprintID = "builtin.user-workflows.no-agent-assets"
	custom.Version = "1.0.0"
	custom.Name = "不物化 Agent 资产"
	found := false
	for trackIndex := range custom.Tracks {
		for nodeIndex := range custom.Tracks[trackIndex].Nodes {
			node := &custom.Tracks[trackIndex].Nodes[nodeIndex]
			if node.Role == "growth.agent-asset" {
				node.Enabled = false
				found = true
			}
		}
	}
	if !found {
		t.Fatal("default blueprint lost growth.agent-asset role")
	}
	published, err := a.Blueprints.Publish(ctx, "builtin.user-workflows", custom)
	if err != nil {
		t.Fatal(err)
	}
	activated, err := a.Blueprints.Activate(ctx, project.ProjectID, published.BlueprintID, published.Version, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	if activated.Inherited || activated.Blueprint.BlueprintID != custom.BlueprintID {
		t.Fatalf("activation=%#v", activated)
	}

	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-blueprint-growth", "recording", "session-blueprint-growth", "user", "2026-08-22T02:00:00Z", "每次发布前必须先运行完整测试，然后检查回滚条件，最后再部署。"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Execution.Status != "completed" {
		t.Fatalf("execution=%#v", result.Execution)
	}
	assets, err := a.Memory.ListAssetsForProject(ctx, project.ProjectID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) == 0 {
		t.Fatal("fixture did not produce a derived Agent asset candidate")
	}
	governed, err := a.Harness.ListObjects(ctx, project.ProjectID, harness.GovernedAgentAssetTypeV3, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(governed) != 0 {
		t.Fatalf("disabled blueprint role still materialized governed assets: %#v", governed)
	}
	knowledge, err := a.Harness.ListObjects(ctx, project.ProjectID, memory.StructuredKnowledgeUnitTypeV2, "", 20)
	if err != nil || len(knowledge) == 0 {
		t.Fatalf("enabled growth role stopped working: objects=%#v err=%v", knowledge, err)
	}
	var structured memory.KnowledgeUnit
	if err := json.Unmarshal(knowledge[0].Revision.Payload, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.SchemaVersion != memory.KnowledgeUnitSchemaV2 || structured.Structure.Provenance.EvidenceID != captured.EvidenceID {
		t.Fatalf("growth materialized a lossy or source-free KU: %#v", structured)
	}
	detail, err := a.Harness.RunDetail(ctx, result.Execution.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var sawDisabled bool
	for _, span := range detail.Spans {
		if span.NodeID != "materialize" {
			continue
		}
		var stageDetail struct {
			Result struct {
				BlueprintID   string   `json:"blueprint_id"`
				DisabledRoles []string `json:"disabled_roles"`
			} `json:"result"`
		}
		if err := json.Unmarshal(span.Detail, &stageDetail); err != nil {
			t.Fatal(err)
		}
		if stageDetail.Result.BlueprintID != published.BlueprintID {
			t.Fatalf("materialize trace blueprint=%q want=%q", stageDetail.Result.BlueprintID, published.BlueprintID)
		}
		for _, role := range stageDetail.Result.DisabledRoles {
			sawDisabled = sawDisabled || role == "growth.agent-asset"
		}
	}
	if !sawDisabled {
		t.Fatal("Run trace did not explain disabled growth.agent-asset role")
	}
}
