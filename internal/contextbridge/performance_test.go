package contextbridge_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestContextPlanRemainsBoundedWithThousandIndexedObjects(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "context-scale-1000", Name: "Context Scale 1000", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		_, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
			ObjectID: fmt.Sprintf("context-scale-%04d", i), TypeID: "builtin.core-memory-growth.knowledge-point",
			ProjectID: project.ProjectID, Status: "candidate",
			Payload:  json.RawMessage(fmt.Sprintf(`{"statement":"context-scale shared marker item %04d","kind":"fact","scope":"project"}`, i)),
			PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: fmt.Sprintf("context-scale-%04d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Portfolio.RebuildProjectIndex(t.Context(), project.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(t.Context(), project.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(t.Context(), project.ProjectID, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion, "owner-scale-test"); err != nil {
		t.Fatal(err)
	}

	capability := richCapability()
	capability.CapabilitySetID = "caps-context-scale-1000"
	started := time.Now()
	planned, err := a.Context.CompilePlan(t.Context(), contextbridge.PlanRequest{
		ProjectID: project.ProjectID, AgentID: "agent-context-scale", Query: "context-scale shared marker",
		CapabilitySet: capability, Budget: contextbridge.ContextBudget{MaxTokens: 4096, MaxChars: 16000, MaxLatencyMS: 3000},
		IdempotencyKey: "context-scale-plan-1000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Context Plan over 1000 indexed objects took %s", elapsed)
	}
	if len(planned.Plan.Items) == 0 || len(planned.Plan.Items) > capability.MaxContextItems {
		t.Fatalf("plan item bound violated items=%d max=%d", len(planned.Plan.Items), capability.MaxContextItems)
	}
	if planned.Plan.BlueprintVersion != blueprint.ContextBlueprintVersion {
		t.Fatalf("unexpected blueprint version %s", planned.Plan.BlueprintVersion)
	}
	detailStart := time.Now()
	detail, err := a.Harness.RunDetail(t.Context(), planned.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(detailStart); elapsed > 2*time.Second {
		t.Fatalf("Run detail became slow after 1000-object Context Plan: %s", elapsed)
	}
	candidateCount := -1
	for _, event := range detail.Events {
		if event.EventType != "context.plan.created" {
			continue
		}
		var data struct {
			CandidateCount int `json:"candidate_count"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		candidateCount = data.CandidateCount
	}
	if candidateCount < 0 || candidateCount > 600 {
		t.Fatalf("Context search candidate work was not bounded: %d", candidateCount)
	}
}
