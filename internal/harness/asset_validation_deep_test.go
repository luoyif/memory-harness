package harness_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func deepAssetPayload(kind, title, body, source string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"asset_id":          "asset-" + kind + "-" + title,
		"asset_type":        kind,
		"title":             title,
		"body":              body,
		"source_memory_ids": []string{source},
		"validation_status": "not_run",
	})
	return raw
}

func validationMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func findValidationCheck(t *testing.T, report map[string]any, name string) map[string]any {
	t.Helper()
	checks, _ := report["checks"].([]any)
	for _, raw := range checks {
		check, _ := raw.(map[string]any)
		if fmt.Sprint(check["name"]) == name {
			return check
		}
	}
	t.Fatalf("validation check %q not found in %#v", name, report)
	return nil
}

func TestDeepAssetValidationPromptFixtureAndKnownToolDryRun(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	prompt, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-deep-prompt", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate",
		Payload:  deepAssetPayload("prompt", "Prompt fixture", "请基于 {{project.name}} 输出不少于十二个字符的项目摘要。", "mem-prompt"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "deep-prompt-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := a.Harness.ProposeRevision(ctx, prompt.ObjectID, harness.ProposeRevisionInput{
		Payload:          deepAssetPayload("prompt", "Prompt fixture", "请基于 {{project.name}} 输出不少于十二个字符的项目摘要。", "mem-prompt"),
		ExpectedRevision: prompt.CurrentRevision, EditReason: "validate deterministic fixture",
		TargetStatus: "active", IdempotencyKey: "deep-prompt-r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := validationMap(t, review.Validation)
	if report["status"] != "passed" || report["validator"] != "governed-agent-asset-v3/deterministic-2" {
		t.Fatalf("report=%#v", report)
	}
	check := findValidationCheck(t, report, "prompt_fixture_render")
	if check["status"] != "passed" {
		t.Fatalf("prompt check=%#v", check)
	}
	data, _ := check["data"].(map[string]any)
	if data["mode"] != "deterministic_fixture" {
		t.Fatalf("prompt data=%#v", data)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "fixture verified"); err != nil {
		t.Fatal(err)
	}
}

func TestDeepAssetValidationUnknownToolFailsWithoutExecution(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	candidate, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-deep-tool", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate",
		Payload:  deepAssetPayload("tool_recipe", "Unsafe tool recipe", "使用 memory_delete_everything 工具执行输入，最后检查输出并重试。", "mem-tool"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "deep-tool-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := a.Harness.ProposeRevision(ctx, candidate.ObjectID, harness.ProposeRevisionInput{
		Payload:          deepAssetPayload("tool_recipe", "Unsafe tool recipe", "使用 memory_delete_everything 工具执行输入，最后检查输出并重试。", "mem-tool"),
		ExpectedRevision: candidate.CurrentRevision, EditReason: "probe unknown tool safely",
		TargetStatus: "active", IdempotencyKey: "deep-tool-r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := validationMap(t, review.Validation)
	if report["status"] != "failed" {
		t.Fatalf("report=%#v", report)
	}
	check := findValidationCheck(t, report, "tool_recipe_dry_run")
	if check["status"] != "failed" {
		t.Fatalf("tool check=%#v", check)
	}
	data, _ := check["data"].(map[string]any)
	if data["mode"] != "catalog_only" {
		t.Fatalf("tool data=%#v", data)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "must fail"); err == nil {
		t.Fatal("unknown tool revision unexpectedly activated")
	}
}

func TestDeepAssetValidationMCPProbeNeverClaimsConnectivity(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	candidate, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-deep-mcp", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate",
		Payload:  deepAssetPayload("mcp", "MCP static contract", "MCP 使用 stdio transport，tool memory_search，并声明 permission 边界。", "mem-mcp"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "deep-mcp-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := a.Harness.ProposeRevision(ctx, candidate.ObjectID, harness.ProposeRevisionInput{
		Payload:          deepAssetPayload("mcp", "MCP static contract", "MCP 使用 stdio transport，tool memory_search，并声明 permission 边界。", "mem-mcp"),
		ExpectedRevision: candidate.CurrentRevision, EditReason: "validate static MCP contract",
		TargetStatus: "active", IdempotencyKey: "deep-mcp-r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := validationMap(t, review.Validation)
	if report["status"] != "passed" {
		t.Fatalf("report=%#v", report)
	}
	probe := findValidationCheck(t, report, "mcp_tool_permission_probe")
	if probe["status"] != "passed" {
		t.Fatalf("permission probe=%#v", probe)
	}
	connectivity := findValidationCheck(t, report, "mcp_connectivity_probe")
	if connectivity["status"] != "not_executed" {
		t.Fatalf("connectivity=%#v", connectivity)
	}
	data, _ := connectivity["data"].(map[string]any)
	if data["mode"] != "no_side_effects" {
		t.Fatalf("connectivity data=%#v", data)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-test", "static contract only"); err != nil {
		t.Fatal(err)
	}
}

func TestDeepAssetValidationOppositeExactRuleIsWarningNotAutoVerdict(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	first, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-rule-allow", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate",
		Payload:  deepAssetPayload("rule", "Audit allow", "规则：必须记录审计日志，确保所有操作可以检查。", "mem-rule-a"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "rule-allow-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstReview, err := a.Harness.ProposeRevision(ctx, first.ObjectID, harness.ProposeRevisionInput{
		Payload:          deepAssetPayload("rule", "Audit allow", "规则：必须记录审计日志，确保所有操作可以检查。", "mem-rule-a"),
		ExpectedRevision: first.CurrentRevision, EditReason: "activate baseline rule",
		TargetStatus: "active", IdempotencyKey: "rule-allow-r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, firstReview.ReviewID, "approve", "owner-test", "baseline"); err != nil {
		t.Fatal(err)
	}

	second, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-rule-deny", TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: portfolio.PersonalProjectID, Status: "candidate",
		Payload:  deepAssetPayload("constraint", "Audit deny", "规则：不得记录审计日志，确保所有操作可以检查。", "mem-rule-b"),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "rule-deny-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondReview, err := a.Harness.ProposeRevision(ctx, second.ObjectID, harness.ProposeRevisionInput{
		Payload:          deepAssetPayload("constraint", "Audit deny", "规则：不得记录审计日志，确保所有操作可以检查。", "mem-rule-b"),
		ExpectedRevision: second.CurrentRevision, EditReason: "surface exact opposite rule",
		TargetStatus: "active", IdempotencyKey: "rule-deny-r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	report := validationMap(t, secondReview.Validation)
	if report["status"] != "passed" {
		t.Fatalf("warning should not become auto-verdict: %#v", report)
	}
	conflict := findValidationCheck(t, report, "normative_conflict_scan")
	if conflict["status"] != "warning" {
		t.Fatalf("conflict=%#v", conflict)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, secondReview.ReviewID, "approve", "owner-test", "Owner accepts explicit conflict after review"); err != nil {
		t.Fatal(err)
	}
}
