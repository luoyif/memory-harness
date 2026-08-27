package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/mcpserver"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/teammemory"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type delayedAgentCaptureExtractor struct{}

func (delayedAgentCaptureExtractor) Compiler(context.Context) string { return "agent/delayed-test" }
func (delayedAgentCaptureExtractor) Extract(ctx context.Context, request memory.ExtractionRequest) (memory.ExtractionResult, error) {
	select {
	case <-ctx.Done():
		return memory.ExtractionResult{}, ctx.Err()
	case <-time.After(80 * time.Millisecond):
	}
	return memory.ExtractionResult{Compiler: "agent/delayed-test", Candidates: []memory.ExtractionCandidate{{
		EvidenceID: request.Turns[0].EvidenceID,
		Statement:  "Codex collection keeps the response open until durable memory growth completes.",
		UnitType:   "decision",
		TierHint:   "semantic",
		RiskTier:   "B",
		Confidence: .95,
	}}}, nil
}

func TestAgentCaptureOutlivesServerWriteDeadline(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(delayedAgentCaptureExtractor{})
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Name: "Slow Agent Capture"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := a.Agents.Create(t.Context(), agentauth.CreateInput{
		Name: "Slow Codex", Kind: "codex", ProjectIDs: []string{project.ProjectID},
		Permissions: []string{agentauth.PermissionMemoryCapture},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	ts.Config.WriteTimeout = 10 * time.Millisecond
	ts.Start()
	defer ts.Close()
	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/capture", credential.Token, map[string]any{
		"project_id": project.ProjectID, "idempotency_key": "slow-capture", "source_system": "codex", "session_id": "slow-capture", "role": "assistant", "text": "We decided that long collection calls must return their final durable receipt.",
	})
	if resp.StatusCode != http.StatusCreated || !strings.Contains(string(raw), `"evidence_id"`) || !strings.Contains(string(raw), `"status":"completed"`) {
		t.Fatalf("capture status=%d body=%s", resp.StatusCode, raw)
	}
}

func TestFullPermissionCodexBulkCaptureProducesReadableMemoryAndDashboardActivity(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Name: "Codex Bulk Acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", "", map[string]any{
		"name": "Codex Full Permission Stress", "kind": "codex", "permissions": agentauth.AllowedPermissions,
		"project_ids": []string{project.ProjectID}, "all_projects": false,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create full-permission Codex status=%d body=%s", resp.StatusCode, raw)
	}
	var credential struct {
		Token string              `json:"token"`
		Agent agentauth.Principal `json:"agent"`
	}
	if err := json.Unmarshal(raw, &credential); err != nil || credential.Token == "" || len(credential.Agent.Permissions) != len(agentauth.AllowedPermissions) {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	backend, err := mcpserver.NewAgentHTTPBackend(ts.URL, credential.Token)
	if err != nil || backend.ValidateAgent(t.Context()) != nil {
		t.Fatalf("backend err=%v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpserver.New(backend).Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "codex-full-permission-stress", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	const samples = 24
	var last mcpserver.CaptureOutput
	for index := 1; index <= samples; index++ {
		marker := fmt.Sprintf("STRESS-MEMORY-%02d", index)
		text := fmt.Sprintf("压力样本 %02d：我们决定采用批次方案 %02d，并计划本周完成验证。发布前先运行完整测试，再检查回滚条件。验收标记 %s。", index, index, marker)
		result, callErr := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_capture", Arguments: map[string]any{
			"project_id": project.ProjectID, "idempotency_key": fmt.Sprintf("bulk-capture-%02d", index),
			"source_system": "codex-stress", "session_id": fmt.Sprintf("codex-stress-session-%02d", index),
			"role": "assistant", "observed_at": fmt.Sprintf("2026-08-%02dT09:00:00Z", index), "text": text,
		}})
		if callErr != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("capture %d result=%#v err=%v", index, result, callErr)
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &last); err != nil {
			t.Fatal(err)
		}
		if last.EvidenceID == "" || last.Duplicate || last.Pipeline.EpisodeID == "" || last.Pipeline.KnowledgeUnits == 0 {
			t.Fatalf("capture %d did not durably distill: %#v", index, last)
		}
	}

	search, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_search", Arguments: map[string]any{"project_id": project.ProjectID, "query": "STRESS-MEMORY-24", "limit": 10}})
	if err != nil || search.IsError || !strings.Contains(search.Content[0].(*mcp.TextContent).Text, last.EvidenceID) {
		t.Fatalf("bulk marker was not searchable: result=%#v err=%v", search, err)
	}
	read, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_read_evidence", Arguments: map[string]any{"evidence_id": last.EvidenceID}})
	if err != nil || read.IsError || !strings.Contains(read.Content[0].(*mcp.TextContent).Text, "STRESS-MEMORY-24") {
		t.Fatalf("last Evidence was not exactly readable: result=%#v err=%v", read, err)
	}
	write, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_write", Arguments: map[string]any{
		"kind": "goal", "project_id": project.ProjectID, "title": "复核批量沉淀结果", "priority": 8,
		"source_evidence_id": last.EvidenceID, "target_at": "2026-08-24T18:00:00Z",
	}})
	if err != nil || write.IsError || !strings.Contains(write.Content[0].(*mcp.TextContent).Text, "复核批量沉淀结果") {
		t.Fatalf("full-permission project write failed: result=%#v err=%v", write, err)
	}
	runs, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_runs", Arguments: map[string]any{"project_id": project.ProjectID, "limit": 100}})
	if err != nil || runs.IsError || !strings.Contains(runs.Content[0].(*mcp.TextContent).Text, "completed") {
		t.Fatalf("bulk runs were not visible: result=%#v err=%v", runs, err)
	}
	if _, err := a.Portfolio.CreateProjectTask(t.Context(), portfolio.ProjectTaskInput{
		ProjectID: project.ProjectID, Title: "Owner 复核压力测试结果", Priority: 2, SourceKind: "manual",
		SourceEvidenceIDs: []string{last.EvidenceID}, DueAt: time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/projects/"+project.ProjectID+"/activity-calendar?days=28", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activity calendar status=%d body=%s", resp.StatusCode, raw)
	}
	var activity struct {
		Days []struct {
			Evidence int `json:"evidence"`
			Output   int `json:"output"`
			TasksDue int `json:"tasks_due"`
		} `json:"days"`
	}
	if err := json.Unmarshal(raw, &activity); err != nil {
		t.Fatal(err)
	}
	totalEvidence, totalOutput, totalTasks := 0, 0, 0
	for _, day := range activity.Days {
		totalEvidence += day.Evidence
		totalOutput += day.Output
		totalTasks += day.TasksDue
	}
	if totalEvidence != samples || totalOutput < samples || totalTasks != 1 {
		t.Fatalf("dashboard activity did not reflect bulk run: evidence=%d output=%d tasks=%d", totalEvidence, totalOutput, totalTasks)
	}
	var auditEvents int
	if err := a.Control.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM agent_audit_log WHERE agent_id=? AND status='allowed'`, credential.Agent.AgentID).Scan(&auditEvents); err != nil {
		t.Fatal(err)
	}
	if auditEvents < samples+4 {
		t.Fatalf("bulk operations were not auditable: %d events", auditEvents)
	}
}

func TestCodexTeamMCPKeepsPrivateDraftsPrivateAndDirectSharesNonTransitive(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Name: "Codex Team MCP"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	createAgent := func(name string) (string, agentauth.Principal) {
		resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", "", map[string]any{
			"name": name, "kind": "codex", "project_ids": []string{project.ProjectID},
			"permissions": []string{agentauth.PermissionProjectRead, agentauth.PermissionTeamPrivate, agentauth.PermissionTeamBlackboardRead, agentauth.PermissionTeamBlackboardWrite, agentauth.PermissionTeamBlackboardShare},
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s status=%d body=%s", name, resp.StatusCode, raw)
		}
		var credential struct {
			Token string              `json:"token"`
			Agent agentauth.Principal `json:"agent"`
		}
		if err := json.Unmarshal(raw, &credential); err != nil {
			t.Fatal(err)
		}
		return credential.Token, credential.Agent
	}
	tokenA, agentA := createAgent("Codex A")
	tokenB, agentB := createAgent("Codex B")
	task, err := a.TeamMemory.CreateTask(t.Context(), teammemory.CreateTaskInput{ProjectID: project.ProjectID, Title: "MCP privacy acceptance", MemberAgentIDs: []string{agentA.AgentID, agentB.AgentID}, TTLSeconds: 3600, IdempotencyKey: "mcp-team-task"})
	if err != nil {
		t.Fatal(err)
	}
	connectAgent := func(token, name string) *mcp.ClientSession {
		backend, err := mcpserver.NewAgentHTTPBackend(ts.URL, token)
		if err != nil {
			t.Fatal(err)
		}
		st, ct := mcp.NewInMemoryTransports()
		ss, err := mcpserver.New(backend).Connect(t.Context(), st, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ss.Close() })
		client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "1"}, nil)
		cs, err := client.Connect(t.Context(), ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cs.Close() })
		return cs
	}
	aSession := connectAgent(tokenA, "codex-a-test")
	bSession := connectAgent(tokenB, "codex-b-test")
	privateMarker := "PRIVATE-A-ONLY-731"
	privateWrite, err := aSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_write_private", Arguments: map[string]any{"task_id": task.ObjectID, "content": privateMarker, "confidence": .8, "epistemic_status": "hypothesis", "ttl_seconds": 1800, "idempotency_key": "private-a-1"}})
	if err != nil || privateWrite.IsError {
		t.Fatalf("private write=%#v err=%v", privateWrite, err)
	}
	privateB, err := bSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_read_private", Arguments: map[string]any{"task_id": task.ObjectID}})
	if err != nil || privateB.IsError || strings.Contains(privateB.Content[0].(*mcp.TextContent).Text, privateMarker) {
		t.Fatalf("B saw A private draft=%#v err=%v", privateB, err)
	}
	sharedMarker := "DIRECT-SHARE-A-TO-B-842"
	shared, err := aSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_write_shared", Arguments: map[string]any{"task_id": task.ObjectID, "content": sharedMarker, "topic": "architecture", "claim_key": "storage", "claim_value": "sqlite", "direct_share_agent_ids": []string{agentB.AgentID}, "confidence": .9, "epistemic_status": "observed", "ttl_seconds": 1800, "idempotency_key": "shared-a-1"}})
	if err != nil || shared.IsError {
		t.Fatalf("shared write=%#v err=%v", shared, err)
	}
	sharedText := shared.Content[0].(*mcp.TextContent).Text
	var sharedOut struct {
		Entry struct {
			EntryID string `json:"entry_id"`
		} `json:"entry"`
	}
	if err := json.Unmarshal([]byte(sharedText), &sharedOut); err != nil || sharedOut.Entry.EntryID == "" {
		t.Fatalf("shared output=%s err=%v", sharedText, err)
	}
	sharedB, err := bSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_read_shared", Arguments: map[string]any{"task_id": task.ObjectID}})
	if err != nil || sharedB.IsError || !strings.Contains(sharedB.Content[0].(*mcp.TextContent).Text, sharedMarker) {
		t.Fatalf("B missing direct share=%#v err=%v", sharedB, err)
	}
	forward, err := bSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_update_share", Arguments: map[string]any{"entry_id": sharedOut.Entry.EntryID, "direct_share_agent_ids": []string{agentA.AgentID}, "idempotency_key": "forbidden-forward"}})
	if err != nil || !forward.IsError {
		t.Fatalf("recipient forwarded author content=%#v err=%v", forward, err)
	}
	tasks, err := bSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_team_list_tasks", Arguments: map[string]any{}})
	if err != nil || tasks.IsError || !strings.Contains(tasks.Content[0].(*mcp.TextContent).Text, "MCP privacy acceptance") {
		t.Fatalf("team task list=%#v err=%v", tasks, err)
	}
}

func agentRequest(t *testing.T, client *http.Client, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

func TestAgentHTTPProjectPermissionCaptureRecallAndAudit(t *testing.T) {
	a, _ := testutil.Open(t)
	allowed, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "allowed-agent", Name: "Allowed Agent", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "denied-agent", Name: "Denied Agent", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Portfolio.CreateFinanceEntry(t.Context(), portfolio.FinanceEntryInput{ProjectID: allowed.ProjectID, EntryType: "expense", Category: "private", Description: "finance-secret-884", AmountMinor: -1200, Currency: "CNY", OccurredAt: "2026-08-21T10:00:00Z", IdempotencyKey: "private-finance-1"}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", "", map[string]any{
		"name": "Codex Scoped", "kind": "codex", "permissions": []string{agentauth.PermissionMemoryRead, agentauth.PermissionMemoryCapture, agentauth.PermissionProjectRead}, "project_ids": []string{allowed.ProjectID},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, raw)
	}
	var credential struct {
		Token string              `json:"token"`
		Agent agentauth.Principal `json:"agent"`
	}
	if err := json.Unmarshal(raw, &credential); err != nil || credential.Token == "" {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/projects", credential.Token, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), allowed.Name) || strings.Contains(string(raw), denied.Name) {
		t.Fatalf("projects status=%d body=%s", resp.StatusCode, raw)
	}

	backend, err := mcpserver.NewAgentHTTPBackend(ts.URL, credential.Token)
	if err != nil || backend.ValidateAgent(t.Context()) != nil {
		t.Fatalf("backend err=%v", err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(backend).Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "scoped-agent-test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	capture, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_capture", Arguments: map[string]any{"project_id": allowed.ProjectID, "idempotency_key": "agent-capture-1", "source_system": "codex", "session_id": "agent-session", "role": "assistant", "text": "Agent permission acceptance marker alpha-517."}})
	if err != nil || capture.IsError || len(capture.Content) != 1 || !strings.Contains(capture.Content[0].(*mcp.TextContent).Text, "agent_") {
		t.Fatalf("capture=%#v err=%v", capture, err)
	}
	var captured mcpserver.CaptureOutput
	if err := json.Unmarshal([]byte(capture.Content[0].(*mcp.TextContent).Text), &captured); err != nil || captured.EvidenceID == "" {
		t.Fatalf("capture output=%#v err=%v", captured, err)
	}
	repeatedCapture, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_capture", Arguments: map[string]any{"project_id": allowed.ProjectID, "idempotency_key": "agent-capture-1", "source_system": "codex", "session_id": "agent-session", "role": "assistant", "text": "Agent permission acceptance marker alpha-517."}})
	if err != nil || repeatedCapture.IsError {
		t.Fatalf("idempotent capture retry=%#v err=%v", repeatedCapture, err)
	}
	var repeated mcpserver.CaptureOutput
	if err := json.Unmarshal([]byte(repeatedCapture.Content[0].(*mcp.TextContent).Text), &repeated); err != nil || !repeated.Duplicate || repeated.EvidenceID != captured.EvidenceID {
		t.Fatalf("idempotent capture result=%#v err=%v", repeated, err)
	}
	changedCapture, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_capture", Arguments: map[string]any{"project_id": allowed.ProjectID, "idempotency_key": "agent-capture-1", "source_system": "codex", "session_id": "agent-session", "role": "assistant", "text": "Changed content must conflict."}})
	if err == nil && !changedCapture.IsError {
		t.Fatalf("changed content reused an idempotency key without conflict: %#v", changedCapture)
	}
	recall, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_recall", Arguments: map[string]any{"project_id": allowed.ProjectID, "query": "alpha-517"}})
	if err != nil || recall.IsError || !strings.Contains(recall.Content[0].(*mcp.TextContent).Text, "alpha-517") {
		t.Fatalf("recall=%#v err=%v", recall, err)
	}
	projectContext, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_context", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || projectContext.IsError || strings.Contains(projectContext.Content[0].(*mcp.TextContent).Text, "finance-secret-884") {
		t.Fatalf("project context leaked finance: %#v err=%v", projectContext, err)
	}
	timeline, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_timeline", Arguments: map[string]any{"project_id": allowed.ProjectID, "anchor_at": "2026-08-22T00:00:00Z"}})
	if err != nil || timeline.IsError {
		t.Fatalf("timeline=%#v err=%v", timeline, err)
	}
	timelineText := timeline.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(timelineText, captured.EvidenceID) || strings.Contains(timelineText, "finance-secret-884") {
		t.Fatalf("timeline permission/content mismatch: %s", timelineText)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", "", map[string]any{
		"name": "Project Read Only", "kind": "readonly", "permissions": []string{agentauth.PermissionProjectRead}, "project_ids": []string{allowed.ProjectID},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project-only agent status=%d body=%s", resp.StatusCode, raw)
	}
	var projectOnly struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &projectOnly); err != nil || projectOnly.Token == "" {
		t.Fatalf("project-only credential=%#v err=%v", projectOnly, err)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/projects/"+allowed.ProjectID+"/context", projectOnly.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project-only context status=%d body=%s", resp.StatusCode, raw)
	}
	projectOnlyText := string(raw)
	if strings.Contains(projectOnlyText, captured.EvidenceID) || strings.Contains(projectOnlyText, "alpha-517") || strings.Contains(projectOnlyText, "finance-secret-884") {
		t.Fatalf("project.read context leaked memory/evidence/finance: %s", projectOnlyText)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent/timeline?project_id="+allowed.ProjectID, projectOnly.Token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("project-only dedicated timeline must require memory.read: status=%d body=%s", resp.StatusCode, raw)
	}
	projectBlueprint, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_blueprint", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || projectBlueprint.IsError || !strings.Contains(projectBlueprint.Content[0].(*mcp.TextContent).Text, "builtin.memory-harness-core.default") {
		t.Fatalf("project blueprint=%#v err=%v", projectBlueprint, err)
	}
	deniedBlueprint, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_blueprint", Arguments: map[string]any{"project_id": denied.ProjectID}})
	if err != nil || !deniedBlueprint.IsError {
		t.Fatalf("denied project blueprint=%#v err=%v", deniedBlueprint, err)
	}
	object, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{TypeID: "builtin.core-memory-growth.semantic", ProjectID: allowed.ProjectID, Payload: json.RawMessage(`{"statement":"Generic object through MCP","domain":"acceptance"}`), PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: "agent-object-1"})
	if err != nil {
		t.Fatal(err)
	}
	memoryTypes, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_types", Arguments: map[string]any{}})
	if err != nil || memoryTypes.IsError || !strings.Contains(memoryTypes.Content[0].(*mcp.TextContent).Text, "builtin.core-memory-growth.semantic") {
		t.Fatalf("types=%#v err=%v", memoryTypes, err)
	}
	objectList, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_objects", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || objectList.IsError || !strings.Contains(objectList.Content[0].(*mcp.TextContent).Text, "Generic object through MCP") {
		t.Fatalf("objects=%#v err=%v", objectList, err)
	}
	readObject, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_read_object", Arguments: map[string]any{"object_id": object.ObjectID}})
	if err != nil || readObject.IsError || !strings.Contains(readObject.Content[0].(*mcp.TextContent).Text, object.ObjectID) {
		t.Fatalf("object=%#v err=%v", readObject, err)
	}
	activeAsset, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{ObjectID: "obj-agent-active-asset", TypeID: harness.GovernedAgentAssetTypeV3, ProjectID: allowed.ProjectID, Status: "candidate", Payload: json.RawMessage(`{"asset_id":"asset-agent-active","asset_type":"skill","title":"Active governed skill","body":"使用测试工具完成 ACTIVE-ASSET-MARKER-921，并检查输出结果。","source_memory_ids":["mem-test-source"],"validation_status":"not_run"}`), PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "agent-active-asset-r1"})
	if err != nil {
		t.Fatal(err)
	}
	activeReview, err := a.Harness.ProposeRevision(t.Context(), activeAsset.ObjectID, harness.ProposeRevisionInput{Payload: json.RawMessage(`{"asset_id":"asset-agent-active","asset_type":"skill","title":"Active governed skill","body":"使用测试工具完成 ACTIVE-ASSET-MARKER-921，并检查输出结果。","source_memory_ids":["mem-test-source"],"validation_status":"not_run"}`), ExpectedRevision: activeAsset.CurrentRevision, EditReason: "activate tested skill", TargetStatus: "active", IdempotencyKey: "agent-active-asset-r2", Validation: json.RawMessage(`{"status":"passed"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), activeReview.ReviewID, "approve", "owner-test", "safe for Agent consumption"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{ObjectID: "obj-agent-candidate-asset", TypeID: harness.GovernedAgentAssetTypeV3, ProjectID: allowed.ProjectID, Status: "candidate", Payload: json.RawMessage(`{"asset_id":"asset-agent-candidate","asset_type":"constraint","title":"Candidate constraint","body":"不得自动暴露 CANDIDATE-ASSET-MARKER-922。","source_memory_ids":["mem-test-source"],"validation_status":"not_run"}`), PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "agent-candidate-asset-r1"}); err != nil {
		t.Fatal(err)
	}
	activeAssets, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_active_assets", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || activeAssets.IsError {
		t.Fatalf("active assets=%#v err=%v", activeAssets, err)
	}
	activeAssetsText := activeAssets.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(activeAssetsText, "ACTIVE-ASSET-MARKER-921") || strings.Contains(activeAssetsText, "CANDIDATE-ASSET-MARKER-922") {
		t.Fatalf("active asset surface leaked non-active object: %s", activeAssetsText)
	}
	currentAsset, err := a.Harness.Object(t.Context(), activeAsset.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	proposalArgs := map[string]any{
		"object_id": currentAsset.ObjectID, "expected_revision": currentAsset.CurrentRevision,
		"edit_reason": "Agent observed that output verification should be explicit", "idempotency_key": "agent-asset-proposal-1",
		"payload": map[string]any{"asset_id": "asset-agent-active", "asset_type": "skill", "title": "Active governed skill", "body": "使用测试工具完成 ACTIVE-ASSET-MARKER-921，然后检查输出结果与失败条件。", "source_memory_ids": []string{"mem-test-source"}, "validation_status": "not_run"},
	}
	deniedProposal, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_propose_asset_revision", Arguments: proposalArgs})
	if err != nil || !deniedProposal.IsError {
		t.Fatalf("memory.propose should be denied before grant: %#v err=%v", deniedProposal, err)
	}
	runList, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_runs", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || !runList.IsError {
		t.Fatalf("trace.read should be denied: %#v err=%v", runList, err)
	}
	projectWrite, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_write", Arguments: map[string]any{"kind": "goal", "project_id": allowed.ProjectID, "title": "must be denied"}})
	if err != nil || !projectWrite.IsError {
		t.Fatalf("expected project.write denial: %#v err=%v", projectWrite, err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPatch, ts.URL+"/v1/agents/"+credential.Agent.AgentID, "", map[string]any{
		"status": "active", "permissions": []string{agentauth.PermissionMemoryRead, agentauth.PermissionMemoryCapture, agentauth.PermissionMemoryPropose, agentauth.PermissionProjectRead, agentauth.PermissionProjectWrite, agentauth.PermissionFinanceWrite, agentauth.PermissionTraceRead}, "project_ids": []string{allowed.ProjectID}, "all_projects": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("grant write status=%d body=%s", resp.StatusCode, raw)
	}
	proposal, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_propose_asset_revision", Arguments: proposalArgs})
	if err != nil || proposal.IsError || len(proposal.Content) != 1 {
		t.Fatalf("asset revision proposal=%#v err=%v", proposal, err)
	}
	var proposedReview harness.RevisionReview
	if err := json.Unmarshal([]byte(proposal.Content[0].(*mcp.TextContent).Text), &proposedReview); err != nil {
		t.Fatal(err)
	}
	if proposedReview.Status != "pending" || proposedReview.BaseRevision != currentAsset.CurrentRevision || proposedReview.RequestedBy != "agent:"+credential.Agent.AgentID || proposedReview.EditReason == "" {
		t.Fatalf("proposal review=%#v", proposedReview)
	}
	unchangedAsset, err := a.Harness.Object(t.Context(), currentAsset.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if unchangedAsset.CurrentRevision != currentAsset.CurrentRevision || unchangedAsset.Revision.ContentHash != currentAsset.Revision.ContentHash {
		t.Fatalf("Agent proposal moved current pointer: before=%#v after=%#v", currentAsset, unchangedAsset)
	}
	semanticProposal := map[string]any{"expected_revision": object.CurrentRevision, "edit_reason": "attempt ordinary memory edit", "idempotency_key": "agent-semantic-proposal", "payload": map[string]any{"statement": "changed", "domain": "acceptance"}}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/objects/"+object.ObjectID+"/revision-proposals", credential.Token, semanticProposal)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "governed Agent Asset v3") {
		t.Fatalf("ordinary memory proposal must fail closed: status=%d body=%s", resp.StatusCode, raw)
	}
	projectWrite, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_project_write", Arguments: map[string]any{"kind": "goal", "project_id": allowed.ProjectID, "title": "Agent-created goal", "priority": 7, "source_evidence_id": captured.EvidenceID}})
	if err != nil || projectWrite.IsError || !strings.Contains(projectWrite.Content[0].(*mcp.TextContent).Text, "Agent-created goal") {
		t.Fatalf("project write=%#v err=%v", projectWrite, err)
	}
	financeWrite, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_finance_write", Arguments: map[string]any{"project_id": allowed.ProjectID, "entry_type": "expense", "category": "agent", "description": "Agent finance acceptance", "amount_minor": -345, "currency": "CNY", "occurred_at": "2026-08-21T11:00:00Z", "idempotency_key": "agent-finance-1"}})
	if err != nil || financeWrite.IsError || !strings.Contains(financeWrite.Content[0].(*mcp.TextContent).Text, "Agent finance acceptance") {
		t.Fatalf("finance write=%#v err=%v", financeWrite, err)
	}
	run, err := a.Harness.StartRun(t.Context(), harness.StartRunInput{ProjectID: allowed.ProjectID, CallerType: "agent", CallerID: credential.Agent.AgentID, Channel: "mcp", PipelineID: "builtin.test.trace", PipelineVersion: "1.0.0", PipelineHash: "sha256:test", IdempotencyKey: "agent-trace-1", Snapshot: json.RawMessage(`{"redacted":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	runList, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_list_runs", Arguments: map[string]any{"project_id": allowed.ProjectID}})
	if err != nil || runList.IsError || !strings.Contains(runList.Content[0].(*mcp.TextContent).Text, run.RunID) {
		t.Fatalf("runs=%#v err=%v", runList, err)
	}
	readRun, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "memory_read_run", Arguments: map[string]any{"run_id": run.RunID}})
	if err != nil || readRun.IsError || !strings.Contains(readRun.Content[0].(*mcp.TextContent).Text, "builtin.test.trace") {
		t.Fatalf("run=%#v err=%v", readRun, err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/capture", credential.Token, map[string]any{"project_id": denied.ProjectID, "idempotency_key": "denied-1", "source_system": "codex", "session_id": "denied", "text": "must not be stored"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agent-audit?agent_id="+credential.Agent.AgentID, "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "denied") || !strings.Contains(string(raw), "memory.capture") {
		t.Fatalf("audit status=%d body=%s", resp.StatusCode, raw)
	}
}

func TestAgentRevisionProposalCannotCrossOwnerApprovalBoundary(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "proposal-boundary", Name: "Proposal Boundary", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	object, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "obj-agent-owner-boundary", TypeID: harness.GovernedAgentAssetTypeV3, ProjectID: project.ProjectID, Status: "candidate",
		Payload:  json.RawMessage(`{"asset_id":"asset-boundary","asset_type":"skill","title":"Boundary skill","body":"使用测试工具执行任务，然后检查输出结果。","source_memory_ids":["mem-boundary"],"validation_status":"not_run"}`),
		PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: "boundary-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "Proposal Agent", Kind: "mcp", Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionMemoryPropose}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a).Handler())
	defer ts.Close()
	body := map[string]any{
		"expected_revision": object.CurrentRevision, "edit_reason": "Agent adds an explicit failure check", "idempotency_key": "agent-owner-boundary-r2",
		"payload": map[string]any{"asset_id": "asset-boundary", "asset_type": "skill", "title": "Boundary skill", "body": "使用测试工具执行任务，然后检查输出结果和失败条件。", "source_memory_ids": []string{"mem-boundary"}, "validation_status": "not_run"},
	}
	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agent/objects/"+object.ObjectID+"/revision-proposals", credential.Token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("proposal status=%d body=%s", resp.StatusCode, raw)
	}
	var review harness.RevisionReview
	if err := json.Unmarshal(raw, &review); err != nil {
		t.Fatal(err)
	}
	if review.Status != "pending" || review.RequestedBy != "agent:"+credential.Agent.AgentID {
		t.Fatalf("review=%#v", review)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/revision-reviews/"+review.ReviewID+"/decision", credential.Token, map[string]any{"decision": "approve", "note": "self approve"})
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(raw), "owner_unauthorized") {
		t.Fatalf("Agent token crossed Owner boundary: status=%d body=%s", resp.StatusCode, raw)
	}
	stillPending, err := a.Harness.RevisionReview(ctx, review.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if stillPending.Status != "pending" {
		t.Fatalf("owner boundary changed review=%#v", stillPending)
	}
	current, err := a.Harness.Object(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevision != object.CurrentRevision || current.Status != "candidate" {
		t.Fatalf("Agent proposal or failed approval moved current=%#v", current)
	}
}
