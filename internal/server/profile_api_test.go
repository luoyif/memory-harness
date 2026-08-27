package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/profile"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestProfileOwnerAndAgentAPIsRespectProjectionBoundaries(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-api", Name: "Profile API", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "验证 Profile API", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(ctx, project.ProjectID); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	resp, raw := agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/profiles?project_id="+project.ProjectID, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner list status=%d body=%s", resp.StatusCode, raw)
	}
	var listed struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil || listed.Total == 0 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}
	projectOnly, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "profile-project-reader", Kind: "test", Permissions: []string{agentauth.PermissionProjectRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/projects/"+project.ProjectID+"/profile", projectOnly.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent profile status=%d body=%s", resp.StatusCode, raw)
	}
	var projectView profile.AgentView
	if err := json.Unmarshal(raw, &projectView); err != nil {
		t.Fatal(err)
	}
	if projectView.DeliveryStatus != "not_delivered" || len(projectView.SourceProjectionIDs) != 1 {
		t.Fatalf("project view=%#v", projectView)
	}

	memoryReader, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "profile-memory-reader", Kind: "test", Permissions: []string{agentauth.PermissionProjectRead, agentauth.PermissionMemoryRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/projects/"+project.ProjectID+"/profile", memoryReader.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("memory profile status=%d body=%s", resp.StatusCode, raw)
	}
	var memoryView profile.AgentView
	if err := json.Unmarshal(raw, &memoryView); err != nil {
		t.Fatal(err)
	}
	if len(memoryView.SourceProjectionIDs) < len(projectView.SourceProjectionIDs) {
		t.Fatalf("memory view lost authorized projections: %#v", memoryView)
	}

	denied, _ := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-api-denied", Name: "Denied", DefaultCurrency: "CNY"})
	resp, _ = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/projects/"+denied.ProjectID+"/profile", memoryReader.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-project profile status=%d", resp.StatusCode)
	}
}
