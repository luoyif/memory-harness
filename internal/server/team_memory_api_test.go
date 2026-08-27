package server_test

import (
	"encoding/json"
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

func serverTeamAgent(t *testing.T, a *agentauth.Service, name, projectID string) agentauth.Credential {
	t.Helper()
	credential, err := a.Create(t.Context(), agentauth.CreateInput{
		Name: name, Kind: "team-api", ProjectIDs: []string{projectID},
		Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionTeamPrivate, agentauth.PermissionTeamBlackboardRead, agentauth.PermissionTeamBlackboardWrite, agentauth.PermissionTeamBlackboardShare},
	})
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func TestTeamMemoryAgentAPIsEnforcePrivateDirectShareAndOwnerBoundaries(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "team-api", Name: "Team API", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	aAgent := serverTeamAgent(t, a.Agents, "Agent A", project.ProjectID)
	bAgent := serverTeamAgent(t, a.Agents, "Agent B", project.ProjectID)
	cAgent := serverTeamAgent(t, a.Agents, "Agent C", project.ProjectID)
	task, err := a.TeamMemory.CreateTask(t.Context(), teammemory.CreateTaskInput{
		ProjectID: project.ProjectID, Title: "API task", TTLSeconds: 3600, IdempotencyKey: "api-task",
		MemberAgentIDs: []string{aAgent.Agent.AgentID, bAgent.Agent.AgentID, cAgent.Agent.AgentID},
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.New(a).Handler())
	defer ts.Close()
	resp, raw := agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/team/tasks?project_id="+project.ProjectID, aAgent.Token, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner team API status=%d body=%s", resp.StatusCode, raw)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks", aAgent.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent tasks status=%d body=%s", resp.StatusCode, raw)
	}
	var tasks struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &tasks); err != nil || len(tasks.Tasks) != 1 {
		t.Fatalf("tasks=%s err=%v", raw, err)
	}

	privateBody := map[string]any{"content": "A-PRIVATE-API", "confidence": 0.9, "epistemic_status": "hypothesis", "ttl_seconds": 600, "idempotency_key": "private-api-a"}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/private", aAgent.Token, privateBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("private write status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/private", bAgent.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("B private read status=%d body=%s", resp.StatusCode, raw)
	}
	var privateResult struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(raw, &privateResult); err != nil || privateResult.Total != 0 {
		t.Fatalf("B saw A private body=%s err=%v", raw, err)
	}

	boardBody := map[string]any{
		"content": "A shares B only", "confidence": 0.95, "epistemic_status": "observed", "ttl_seconds": 600, "idempotency_key": "board-a-b",
		"topic": "status", "claim_key": "release.status", "claim_value": "ready", "direct_share_agent_ids": []string{bAgent.Agent.AgentID},
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/blackboard", aAgent.Token, boardBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("board write status=%d body=%s", resp.StatusCode, raw)
	}
	var boardResult teammemory.BlackboardResult
	if err := json.Unmarshal(raw, &boardResult); err != nil || boardResult.Entry.EntryID == "" {
		t.Fatalf("board result=%s err=%v", raw, err)
	}

	for _, check := range []struct {
		token string
		want  int
	}{{bAgent.Token, 1}, {cAgent.Token, 0}} {
		resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/blackboard", check.token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("blackboard read status=%d body=%s", resp.StatusCode, raw)
		}
		var visible struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal(raw, &visible); err != nil || visible.Total != check.want {
			t.Fatalf("visible=%s want=%d err=%v", raw, check.want, err)
		}
	}
	shareBody := map[string]any{"direct_share_agent_ids": []string{cAgent.Agent.AgentID}, "idempotency_key": "b-forward-c"}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/team/blackboard/"+boardResult.Entry.EntryID+"/share", bAgent.Token, shareBody)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("B forwarded share status=%d body=%s", resp.StatusCode, raw)
	}

	leaveBody := map[string]any{"expected_revision": task.CurrentRevision, "edit_reason": "Agent C leaves voluntarily", "idempotency_key": "api-leave-c"}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/leave-proposal", cAgent.Token, leaveBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("C leave proposal status=%d body=%s", resp.StatusCode, raw)
	}
	var leaveReview harness.RevisionReview
	if err := json.Unmarshal(raw, &leaveReview); err != nil || leaveReview.Status != "pending" {
		t.Fatalf("leave review=%s err=%v", raw, err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks", cAgent.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("C pending leave tasks status=%d body=%s", resp.StatusCode, raw)
	}
	var beforeLeave struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(raw, &beforeLeave); err != nil || beforeLeave.Total != 1 {
		t.Fatalf("pending leave changed membership=%s err=%v", raw, err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), leaveReview.ReviewID, "approve", "owner-api", "approve C leave"); err != nil {
		t.Fatal(err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks", cAgent.Token, nil)
	var afterLeave struct {
		Total int `json:"total"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &afterLeave) != nil || afterLeave.Total != 0 {
		t.Fatalf("approved leave kept C tasks=%s", raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/team/tasks/"+task.ObjectID+"/blackboard", cAgent.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("departed C retained Blackboard status=%d body=%s", resp.StatusCode, raw)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/objects?project_id="+project.ProjectID+"&type_id="+teammemory.PrivateScratchTypeV1, aAgent.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("generic Object API exposed team private status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/team/durables", aAgent.Token, map[string]any{})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner durable API status=%d body=%s", resp.StatusCode, raw)
	}
}
