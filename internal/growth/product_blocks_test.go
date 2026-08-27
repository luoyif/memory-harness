package growth_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func findProductBlock(t *testing.T, blocks []growth.ProductBlock, label string) growth.ProductBlock {
	t.Helper()
	for _, block := range blocks {
		if block.Label == label {
			return block
		}
	}
	t.Fatalf("product block %q not found in %#v", label, blocks)
	return growth.ProductBlock{}
}
func TestProjectBriefBlockLockThreeWayMergeAndUnlock(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "block-merge", Name: "Block Merge", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "初始目标", Description: "baseline", Priority: 4}); err != nil {
		t.Fatal(err)
	}
	object, err := a.Growth.RefreshProjectBrief(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := a.Growth.ProductBlocks(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	locked := findProductBlock(t, blocks, "目标与里程碑")
	blocks, err = a.Growth.SetProductBlockLocks(ctx, object.ObjectID, object.CurrentRevision, []string{locked.BlockID})
	if err != nil {
		t.Fatal(err)
	}
	if !findProductBlock(t, blocks, "目标与里程碑").Locked {
		t.Fatal("target block was not locked")
	}
	current, err := a.Harness.Object(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(current.Revision.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	body, _ := payload["body"].(string)
	ownerBlock := "## 目标与里程碑\n- [active] Owner 人工维护：只保留提交前关键目标"
	body = strings.Replace(body, locked.Content, ownerBlock, 1)
	payload["body"] = body
	raw, _ := json.Marshal(payload)
	review, err := a.Harness.ProposeRevision(ctx, current.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: current.CurrentRevision, EditReason: "Owner edits one locked section",
		TargetStatus: "active", IdempotencyKey: "block-owner-r2", Validation: json.RawMessage(`{"status":"passed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "approve block edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "自动候选新增目标", Description: "candidate delta", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	preview, err := a.Growth.ProductMergePreview(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.HasConflicts {
		t.Fatalf("expected three-way conflict: %#v", preview)
	}
	var target growth.ProductMergeBlock
	for _, block := range preview.Blocks {
		if block.Label == "目标与里程碑" {
			target = block
			break
		}
	}
	if target.Status != "diverged_locked" || !target.RequiresOwner {
		t.Fatalf("target merge=%#v", target)
	}
	if !strings.Contains(target.Current, "Owner 人工维护") || !strings.Contains(target.Candidate, "自动候选新增目标") {
		t.Fatalf("three-way evidence missing: %#v", target)
	}
	if !strings.Contains(target.Merged, "Owner 人工维护") {
		t.Fatalf("locked current not preferred: %#v", target)
	}
	refreshed, err := a.Growth.RefreshProjectBrief(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	refreshedBlocks, err := a.Growth.ProductBlocks(ctx, refreshed.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	refreshedTarget := findProductBlock(t, refreshedBlocks, "目标与里程碑")
	if !strings.Contains(refreshedTarget.Content, "Owner 人工维护") || strings.Contains(refreshedTarget.Content, "自动候选新增目标") {
		t.Fatalf("safe refresh overwrote locked block:\n%s", refreshedTarget.Content)
	}
	blocks, err = a.Growth.SetProductBlockLocks(ctx, object.ObjectID, refreshed.CurrentRevision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if findProductBlock(t, blocks, "目标与里程碑").Locked {
		t.Fatal("block remained locked after explicit unlock")
	}
	unlocked, err := a.Growth.RefreshProjectBrief(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var unlockedPayload map[string]any
	if err := json.Unmarshal(unlocked.Revision.Payload, &unlockedPayload); err != nil {
		t.Fatal(err)
	}
	unlockedBody, _ := unlockedPayload["body"].(string)
	if !strings.Contains(unlockedBody, "自动候选新增目标") {
		t.Fatalf("unlocked refresh did not accept generated candidate:\n%s", unlockedBody)
	}
}
