package contextbridge_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func richCapability() contextbridge.ContextCapabilitySet {
	return contextbridge.ContextCapabilitySet{
		SchemaVersion:   contextbridge.CapabilitySchemaVersion,
		CapabilitySetID: "caps-context-policy", AdapterID: "test-adapter", Runtime: "test-harness",
		ProtocolVersion: "1", Transport: "http", Capabilities: []string{"recall", "pre_turn_injection", "context_receipt"},
		MaxContextItems: 8, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 2,
		SupportsIdempotency: true, Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"},
	}
}

func hasReason(item contextbridge.ContextPlanItem, reason string) bool {
	for _, value := range item.ReasonCodes {
		if value == reason {
			return true
		}
	}
	return false
}
func TestContextTrackIsOptionalAndInjectsOnlyAgentSafeProfiles(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "context-policy", Name: "Context Policy", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "验证 Context Profile", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(ctx, project.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(ctx, project.ProjectID, blueprint.DefaultBlueprintID, blueprint.DefaultBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}

	legacy, err := a.Context.CompilePlan(ctx, contextbridge.PlanRequest{
		ProjectID: project.ProjectID, AgentID: "agent-context", Query: "完全不存在的检索词-context-policy-legacy",
		CapabilitySet: richCapability(), IdempotencyKey: "legacy-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range legacy.Plan.Items {
		if hasReason(item, "context_profile") {
			t.Fatalf("legacy blueprint injected profile: %#v", item)
		}
	}
	if legacy.Plan.BlueprintVersion != blueprint.DefaultBlueprintVersion {
		t.Fatalf("legacy version=%s", legacy.Plan.BlueprintVersion)
	}

	if _, err := a.Blueprints.Activate(ctx, project.ProjectID, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}
	planned, err := a.Context.CompilePlan(ctx, contextbridge.PlanRequest{
		ProjectID: project.ProjectID, AgentID: "agent-context", Query: "完全不存在的检索词-context-policy-v11",
		CapabilitySet: richCapability(), IdempotencyKey: "context-plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if planned.Plan.BlueprintVersion != blueprint.ContextBlueprintVersion {
		t.Fatalf("context version=%s", planned.Plan.BlueprintVersion)
	}
	if planned.Plan.Budget.MaxChars != 12000 {
		t.Fatalf("context default budget=%d", planned.Plan.Budget.MaxChars)
	}
	foundDynamic := false
	for _, item := range planned.Plan.Items {
		if hasReason(item, "profile:owner_identity") {
			t.Fatalf("Owner identity leaked into Agent plan: %#v", item)
		}
		if hasReason(item, "profile:dynamic_project") {
			foundDynamic = true
			if item.Presentation != "profile" || item.SourceKind != "object" || item.Revision <= 0 || item.ContentHash == "" {
				t.Fatalf("invalid profile item: %#v", item)
			}
		}
	}
	if !foundDynamic {
		t.Fatalf("context blueprint did not inject dynamic project profile: %#v", planned.Plan.Items)
	}

	detail, err := a.Harness.RunDetail(ctx, planned.RunID)
	if err != nil {
		t.Fatal(err)
	}
	foundPolicyEvent := false
	for _, event := range detail.Events {
		if event.EventType == "context.policy.compiled" {
			foundPolicyEvent = true
		}
	}
	if !foundPolicyEvent {
		t.Fatal("context policy compilation was not recorded in Run trace")
	}
}
