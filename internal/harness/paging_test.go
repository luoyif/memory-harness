package harness_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestThousandRunAndObjectPaginationStaysBounded(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "paging-1000", Name: "Paging 1000", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if _, err := a.Harness.StartRun(t.Context(), harness.StartRunInput{
			ProjectID: project.ProjectID, CallerType: "owner", CallerID: "paging-test", Channel: "test",
			PipelineID: "paging.fixture", PipelineVersion: "1.0.0", PipelineHash: "sha256:paging-fixture",
			IdempotencyKey: fmt.Sprintf("run-%04d", i), Snapshot: json.RawMessage(`{"fixture":"1000-runs"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	runStart := time.Now()
	firstRuns, firstRunPage, err := a.Harness.ListRunsPage(t.Context(), project.ProjectID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondRuns, secondRunPage, err := a.Harness.ListRunsPage(t.Context(), project.ProjectID, "", 50, 50)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(runStart); elapsed > 5*time.Second {
		t.Fatalf("two 50-row Run pages took %s", elapsed)
	}
	if firstRunPage.Total != 1000 || secondRunPage.Total != 1000 || len(firstRuns) != 50 || len(secondRuns) != 50 || !firstRunPage.HasMore {
		t.Fatalf("run pages first=%#v/%d second=%#v/%d", firstRunPage, len(firstRuns), secondRunPage, len(secondRuns))
	}
	seenRuns := map[string]bool{}
	for _, item := range firstRuns {
		seenRuns[item.RunID] = true
	}
	for _, item := range secondRuns {
		if seenRuns[item.RunID] {
			t.Fatalf("Run repeated across pages: %s", item.RunID)
		}
	}
	if _, err := a.Harness.RunDetail(t.Context(), secondRuns[0].RunID); err != nil {
		t.Fatalf("Run detail failed after 1000-row history: %v", err)
	}

	for i := 0; i < 1000; i++ {
		_, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
			ObjectID: fmt.Sprintf("paging-object-%04d", i), TypeID: "builtin.core-memory-growth.knowledge-point",
			ProjectID: project.ProjectID, Status: "candidate",
			Payload:  json.RawMessage(fmt.Sprintf(`{"statement":"paging object %04d","kind":"fact","scope":"project"}`, i)),
			PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: fmt.Sprintf("object-%04d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	objectStart := time.Now()
	firstObjects, firstObjectPage, err := a.Harness.ListObjectsPage(t.Context(), harness.ObjectListOptions{ProjectID: project.ProjectID, Limit: 50, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	lastObjects, lastObjectPage, err := a.Harness.ListObjectsPage(t.Context(), harness.ObjectListOptions{ProjectID: project.ProjectID, Limit: 50, Offset: 950})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(objectStart); elapsed > 5*time.Second {
		t.Fatalf("two 50-row Object pages took %s", elapsed)
	}
	if firstObjectPage.Total != 1000 || lastObjectPage.Total != 1000 || len(firstObjects) != 50 || len(lastObjects) != 50 || lastObjectPage.HasMore {
		t.Fatalf("object pages first=%#v/%d last=%#v/%d", firstObjectPage, len(firstObjects), lastObjectPage, len(lastObjects))
	}
	seenObjects := map[string]bool{}
	for _, item := range firstObjects {
		seenObjects[item.ObjectID] = true
	}
	for _, item := range lastObjects {
		if seenObjects[item.ObjectID] {
			t.Fatalf("Object repeated across distant pages: %s", item.ObjectID)
		}
	}
}

func TestObjectPaginationExcludesSpecializedTypesBeforeCounting(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "paging-filter", Name: "Paging Filter", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.RegisterType(t.Context(), harness.RegisterTypeInput{
		TypeID: "builtin.team-memory.test-hidden", PluginID: "builtin.team-memory", DisplayName: "hidden test",
		SchemaVersion: "1.0.0", Schema: json.RawMessage(`{"type":"object","required":["value"],"properties":{"value":{"type":"string"}},"additionalProperties":false}`),
		Lifecycle: harness.Lifecycle{Initial: "active", States: []string{"active"}, Transitions: map[string][]string{}}, ProtectionClass: "standard", Renderer: json.RawMessage(`{"mode":"generic-card"}`),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
			ObjectID: fmt.Sprintf("visible-%d", i), TypeID: "builtin.core-memory-growth.knowledge-point", ProjectID: project.ProjectID,
			Status: "candidate", Payload: json.RawMessage(fmt.Sprintf(`{"statement":"visible %d","kind":"fact","scope":"project"}`, i)),
			PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: fmt.Sprintf("visible-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
		ObjectID: "hidden-team-object", TypeID: "builtin.team-memory.test-hidden", ProjectID: project.ProjectID, Status: "active",
		Payload: json.RawMessage(`{"value":"private specialized object"}`), PluginID: "builtin.team-memory", PluginVersion: "1.0.0", IdempotencyKey: "hidden-team-object",
	}); err != nil {
		t.Fatal(err)
	}
	items, page, err := a.Harness.ListObjectsPage(t.Context(), harness.ObjectListOptions{
		ProjectID: project.ProjectID, Limit: 10, ExcludedTypePrefixes: []string{"builtin.team-memory."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(items) != 3 {
		t.Fatalf("hidden specialized type leaked into filtered total/page: page=%#v items=%#v", page, items)
	}
}
