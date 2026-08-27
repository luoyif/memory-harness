package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func TestAgentGenericObjectAPIAndSearchCannotBypassSpecializedBoundaries(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "agent-specialized", Name: "Agent Specialized", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(t.Context(), portfolio.GoalInput{ProjectID: project.ProjectID, Title: "OWNER_PROFILE_SECRET_812", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	profiles, err := a.Profiles.ReconcileProject(t.Context(), project.ProjectID)
	if err != nil || len(profiles) == 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}

	candidate, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
		ObjectID: "portable-boundary", TypeID: portablebundle.ImportCandidateTypeV1, ProjectID: project.ProjectID, Status: "candidate",
		Payload:  json.RawMessage(`{"bundle_id":"bundle-boundary","original_project_id":"foreign","original_object":{"marker":"PORTABLE_SECRET_913"},"revision_payloads":{},"compatibility":{},"imported_at":"2026-08-24T00:00:00Z"}`),
		PluginID: portablebundle.PluginID, PluginVersion: portablebundle.PluginVersion, IdempotencyKey: "portable-boundary-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.RebuildProjectIndex(t.Context(), project.ProjectID); err != nil {
		t.Fatal(err)
	}

	credential, err := a.Agents.Create(t.Context(), agentauth.CreateInput{Name: "generic-reader", Kind: "test", Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionProjectRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a).Handler())
	defer ts.Close()

	resp, raw := agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects?project_id="+project.ProjectID+"&limit=100", credential.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, raw)
	}
	var listed struct {
		Objects []harness.Object `json:"objects"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Objects {
		if item.TypeID == harness.ProfileProjectionTypeV1 || item.TypeID == portablebundle.ImportCandidateTypeV1 {
			t.Fatalf("specialized object leaked via generic collection: %#v", item)
		}
	}
	resp, _ = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects/"+profiles[0].ObjectID, credential.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("profile direct generic read status=%d", resp.StatusCode)
	}
	resp, _ = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects/"+candidate.ObjectID, credential.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("portable candidate direct generic read status=%d", resp.StatusCode)
	}
	resp, _ = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects?project_id="+project.ProjectID+"&type_id="+portablebundle.ImportCandidateTypeV1, credential.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("portable type collection status=%d", resp.StatusCode)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/recall", credential.Token, map[string]any{"query": "PORTABLE_SECRET_913", "project_id": project.ProjectID, "kinds": []string{"object"}, "limit": 20})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recall status=%d body=%s", resp.StatusCode, raw)
	}
	var recall unifiedsearch.Result
	if err := json.Unmarshal(raw, &recall); err != nil {
		t.Fatal(err)
	}
	for _, hit := range recall.Hits {
		if hit.SourceID == candidate.ObjectID {
			t.Fatalf("portable candidate leaked through generic search: %#v", hit)
		}
	}
}
