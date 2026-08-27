package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/teammemory"
	"github.com/luoyif/memory-harness/internal/testutil"
)

type apiPage[T any] struct {
	Items   []T
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

func TestHarnessOwnerAndAgentPaginationKeepsVisibleTotalsScoped(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "api-paging", Name: "API Paging", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := a.Agents.Create(t.Context(), agentauth.CreateInput{
		Name: "Paging Agent", Kind: "paging", ProjectIDs: []string{project.ProjectID},
		Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionTraceRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
			ObjectID: fmt.Sprintf("api-visible-%02d", i), TypeID: "builtin.core-memory-growth.knowledge-point", ProjectID: project.ProjectID,
			Status: "candidate", Payload: json.RawMessage(fmt.Sprintf(`{"statement":"visible %02d","kind":"fact","scope":"project"}`, i)),
			PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: fmt.Sprintf("api-visible-%02d", i),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Harness.StartRun(t.Context(), harness.StartRunInput{
			ProjectID: project.ProjectID, CallerType: "owner", CallerID: "api-paging", Channel: "test",
			PipelineID: "api.paging", PipelineVersion: "1.0.0", PipelineHash: "sha256:api-paging",
			IdempotencyKey: fmt.Sprintf("api-run-%02d", i), Snapshot: json.RawMessage(`{"fixture":true}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	hidden, err := a.TeamMemory.CreateTask(t.Context(), teammemory.CreateTaskInput{
		ProjectID: project.ProjectID, Title: "Hidden specialized task", MemberAgentIDs: []string{credential.Agent.AgentID},
		TTLSeconds: 3600, IdempotencyKey: "api-hidden-team-task",
	})
	if err != nil || hidden.TypeID != teammemory.TaskTypeV1 {
		t.Fatalf("hidden=%#v err=%v", hidden, err)
	}

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var ownerObjects struct {
		Objects []harness.Object `json:"objects"`
		Total   int              `json:"total"`
		Limit   int              `json:"limit"`
		Offset  int              `json:"offset"`
		HasMore bool             `json:"has_more"`
	}
	getJSON(t, ts.URL+"/v1/harness/objects?project_id="+project.ProjectID+"&limit=5&offset=5", &ownerObjects)
	if ownerObjects.Total != 13 || ownerObjects.Limit != 5 || ownerObjects.Offset != 5 || len(ownerObjects.Objects) != 5 || !ownerObjects.HasMore {
		t.Fatalf("owner object page=%#v", ownerObjects)
	}
	var ownerRuns struct {
		Runs    []harness.Run `json:"runs"`
		Total   int           `json:"total"`
		Limit   int           `json:"limit"`
		Offset  int           `json:"offset"`
		HasMore bool          `json:"has_more"`
	}
	getJSON(t, ts.URL+"/v1/harness/runs?project_id="+project.ProjectID+"&limit=5&offset=10", &ownerRuns)
	if ownerRuns.Total != 12 || ownerRuns.Limit != 5 || ownerRuns.Offset != 10 || len(ownerRuns.Runs) != 2 || ownerRuns.HasMore {
		t.Fatalf("owner run page=%#v", ownerRuns)
	}

	resp, raw := agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects?project_id="+project.ProjectID+"&limit=20&offset=0", credential.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent objects status=%d body=%s", resp.StatusCode, raw)
	}
	var agentObjects struct {
		Objects []harness.Object `json:"objects"`
		Total   int              `json:"total"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(raw, &agentObjects); err != nil {
		t.Fatal(err)
	}
	if agentObjects.Total != 12 || len(agentObjects.Objects) != 12 || agentObjects.HasMore {
		t.Fatalf("specialized object leaked into Agent visible total/page: %#v", agentObjects)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/runs?project_id="+project.ProjectID+"&limit=5&offset=5", credential.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent runs status=%d body=%s", resp.StatusCode, raw)
	}
	var agentRuns struct {
		Runs    []harness.Run `json:"runs"`
		Total   int           `json:"total"`
		Limit   int           `json:"limit"`
		Offset  int           `json:"offset"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(raw, &agentRuns); err != nil {
		t.Fatal(err)
	}
	if agentRuns.Total != 12 || agentRuns.Limit != 5 || agentRuns.Offset != 5 || len(agentRuns.Runs) != 5 || !agentRuns.HasMore {
		t.Fatalf("agent run page=%#v", agentRuns)
	}
}
