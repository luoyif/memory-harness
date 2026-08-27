package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/mcpserver"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/search"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func decodeStrictJSON(r *http.Request, limit int64, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Agents.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "agents_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": items, "total": len(items), "allowed_permissions": agentauth.AllowedPermissions})
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var input agentauth.CreateInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent", err.Error())
		return
	}
	credential, err := s.app.Agents.Create(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"agent": credential.Agent, "token": credential.Token, "token_notice": "This token is shown once. Store it in the MCP client secret configuration."})
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	var input agentauth.UpdateInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent", err.Error())
		return
	}
	item, err := s.app.Agents.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "agent_update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) rotateAgentToken(w http.ResponseWriter, r *http.Request) {
	credential, err := s.app.Agents.RotateToken(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "agent_rotate_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": credential.Agent, "token": credential.Token, "token_notice": "The previous token is now invalid. This replacement is shown once."})
}

func (s *Server) agentAudit(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Agents.ListAudit(r.Context(), r.URL.Query().Get("agent_id"), 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "agent_audit_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items, "total": len(items)})
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 8 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func (s *Server) authorizeAgent(w http.ResponseWriter, r *http.Request, permission string) (agentauth.Principal, bool) {
	principal, err := s.app.Agents.Authenticate(r.Context(), bearerToken(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "agent_unauthorized", "a valid active Agent token is required")
		return agentauth.Principal{}, false
	}
	if permission != "" && !agentauth.HasPermission(principal, permission) {
		_ = s.app.Agents.Audit(r.Context(), principal, permission, "permission", "", "", "denied", map[string]string{"reason": "permission_missing"})
		writeErr(w, http.StatusForbidden, "agent_forbidden", "Agent does not have "+permission)
		return agentauth.Principal{}, false
	}
	return principal, true
}

func (s *Server) authorizeAgentProject(w http.ResponseWriter, r *http.Request, principal agentauth.Principal, projectID, action string) bool {
	if agentauth.CanAccessProject(principal, projectID) {
		return true
	}
	_ = s.app.Agents.Audit(r.Context(), principal, action, "project", projectID, projectID, "denied", map[string]string{"reason": "project_scope"})
	writeErr(w, http.StatusForbidden, "agent_project_forbidden", "Agent is not granted access to this project")
	return false
}

func (s *Server) agentMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, principal)
}

func (s *Server) agentProjects(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionProjectRead)
	if !ok {
		return
	}
	projects, err := s.app.Portfolio.ListProjects(r.Context(), false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "agent_projects_failed", err.Error())
		return
	}
	items := []portfolio.ProjectSummary{}
	includeFinance := agentauth.HasPermission(principal, agentauth.PermissionFinanceRead)
	for _, project := range projects {
		if !agentauth.CanAccessProject(principal, project.ProjectID) {
			continue
		}
		summary, err := s.app.Portfolio.ProjectSummary(r.Context(), project.ProjectID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "agent_projects_failed", err.Error())
			return
		}
		items = append(items, redactProjectFinance(summary, includeFinance))
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "project.read", "project_collection", "", "", "allowed", map[string]int{"count": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"projects": items, "total": len(items)})
}

func redactProjectFinance(summary portfolio.ProjectSummary, include bool) portfolio.ProjectSummary {
	if include {
		return summary
	}
	summary.Project.BudgetMinor = 0
	summary.Finance = portfolio.FinanceSummary{Currencies: []portfolio.CurrencySummary{}}
	return summary
}

func (s *Server) projectContext(ctx context.Context, projectID, asOf string, includeFinance, includeMemory bool) (mcpserver.ProjectContext, error) {
	summary, err := s.app.Portfolio.ProjectSummary(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	blocks, err := s.app.Portfolio.ListContextBlocks(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	facts, err := s.app.Portfolio.ListFacts(ctx, projectID, asOf, false, 500)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	goals, err := s.app.Portfolio.ListGoals(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	milestones, err := s.app.Portfolio.ListMilestones(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	decisions, err := s.app.Portfolio.ListDecisions(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	risks, err := s.app.Portfolio.ListRisks(ctx, projectID)
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	finance := []portfolio.FinanceEntry{}
	if includeFinance {
		finance, err = s.app.Portfolio.ListFinanceEntries(ctx, projectID, 100)
		if err != nil {
			return mcpserver.ProjectContext{}, err
		}
	}
	timelineKinds := []string{"fact", "goal", "milestone", "decision", "run"}
	if includeMemory {
		timelineKinds = append(timelineKinds, "episode", "memory", "evidence")
	}
	if includeFinance {
		timelineKinds = append(timelineKinds, "finance")
	}
	timeline, err := s.app.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: projectID, AnchorAt: asOf, Kinds: timelineKinds, Limit: 100})
	if err != nil {
		return mcpserver.ProjectContext{}, err
	}
	return mcpserver.ProjectContext{Summary: summary, ContextBlocks: blocks, Facts: facts, Goals: goals, Milestones: milestones, Decisions: decisions, Risks: risks, Finance: finance, Timeline: timeline}, nil
}

func (s *Server) agentProjectContext(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionProjectRead)
	if !ok {
		return
	}
	projectID := r.PathValue("id")
	if !s.authorizeAgentProject(w, r, principal, projectID, "project.read") {
		return
	}
	includeFinance := agentauth.HasPermission(principal, agentauth.PermissionFinanceRead)
	includeMemory := agentauth.HasPermission(principal, agentauth.PermissionMemoryRead)
	contextView, err := s.projectContext(r.Context(), projectID, r.URL.Query().Get("as_of"), includeFinance, includeMemory)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_project_context_failed", err.Error())
		return
	}
	contextView.Summary = redactProjectFinance(contextView.Summary, includeFinance)
	_ = s.app.Agents.Audit(r.Context(), principal, "project.read", "project_context", projectID, projectID, "allowed", map[string]any{"finance_included": includeFinance, "memory_timeline_included": includeMemory})
	writeJSON(w, http.StatusOK, contextView)
}

type agentSearchRequest struct {
	ProjectID string `json:"project_id"`
	search.Query
}

func (s *Server) agentSearch(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	var input agentSearchRequest
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_search", err.Error())
		return
	}
	if !s.authorizeAgentProject(w, r, principal, input.ProjectID, "memory.read") {
		return
	}
	project, exact, err := s.app.Portfolio.Resolve(r.Context(), input.ProjectID)
	if err != nil || !exact {
		writeErr(w, http.StatusBadRequest, "bad_agent_search", "project_id must identify a registered project")
		return
	}
	input.Query.Scope = "project:" + project.Slug
	input.Query.SessionFusion = true
	result, err := s.app.Search.Search(r.Context(), input.Query)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_search_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "evidence_search", "", input.ProjectID, "allowed", map[string]any{"query": input.Text, "hits": len(result.Hits)})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) agentRecall(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	var input unifiedsearch.Query
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_recall", err.Error())
		return
	}
	if input.AllProjects {
		if !principal.AllProjects {
			_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "unified_recall", "", "", "denied", map[string]string{"reason": "all_projects_not_granted"})
			writeErr(w, http.StatusForbidden, "agent_project_forbidden", "Agent is not granted all-project recall")
			return
		}
	} else if !s.authorizeAgentProject(w, r, principal, input.ProjectID, "memory.read") {
		return
	}
	if !agentauth.HasPermission(principal, agentauth.PermissionFinanceRead) {
		if len(input.Kinds) == 0 {
			input.Kinds = []string{"evidence", "episode", "memory", "asset", "object", "project", "fact", "goal", "decision", "risk"}
		} else {
			for _, kind := range input.Kinds {
				kind = strings.TrimSpace(kind)
				if kind == "experience" {
					_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "unified_recall", "", input.ProjectID, "denied", map[string]string{"reason": "experience_owner_only"})
					writeErr(w, http.StatusForbidden, "agent_forbidden", "Experience Bank is Owner-only in FT3")
					return
				}
				if kind == "finance" {
					_ = s.app.Agents.Audit(r.Context(), principal, "finance.read", "unified_recall", "", input.ProjectID, "denied", map[string]string{"reason": "permission_missing"})
					writeErr(w, http.StatusForbidden, "agent_forbidden", "Agent does not have finance.read")
					return
				}
			}
		}
	}
	result, err := s.app.Unified.Search(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_recall_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "unified_recall", result.ContextID, input.ProjectID, "allowed", map[string]any{"query": input.Text, "hits": len(result.Hits), "all_projects": input.AllProjects})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) agentReadEvidence(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	allowed, err := s.app.Agents.CanAccessRecord(r.Context(), principal, "evidence", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "agent_evidence_failed", err.Error())
		return
	}
	if !allowed {
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "evidence", id, "", "denied", map[string]string{"reason": "project_scope"})
		writeErr(w, http.StatusForbidden, "agent_project_forbidden", "Agent is not granted this Evidence project")
		return
	}
	raw, err := s.app.Ledger.ReadEvidence(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "evidence not found")
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "evidence", id, "", "allowed", map[string]any{})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(raw, '\n'))
}

type agentCaptureInput struct {
	ProjectID      string `json:"project_id"`
	IdempotencyKey string `json:"idempotency_key"`
	SourceSystem   string `json:"source_system"`
	SessionID      string `json:"session_id"`
	Role           string `json:"role"`
	Text           string `json:"text"`
	ObservedAt     string `json:"observed_at"`
}

func (s *Server) agentCapture(w http.ResponseWriter, r *http.Request) {
	// Model-backed collection can take longer than the server-wide write
	// deadline. Keep the connection open until the governed pipeline returns so
	// Codex receives the canonical receipt instead of a false empty response
	// after the Evidence and derived objects have already been committed.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryCapture)
	if !ok {
		return
	}
	var input agentCaptureInput
	if err := decodeStrictJSON(r, 4<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_capture", err.Error())
		return
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SourceSystem = strings.TrimSpace(input.SourceSystem)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Text = strings.TrimSpace(input.Text)
	if input.IdempotencyKey == "" || input.SourceSystem == "" || input.SessionID == "" || input.Text == "" {
		writeErr(w, http.StatusBadRequest, "bad_agent_capture", "project_id, idempotency_key, source_system, session_id and text are required")
		return
	}
	if input.Role == "" {
		input.Role = "assistant"
	}
	if input.Role != "user" && input.Role != "assistant" && input.Role != "system" && input.Role != "tool" {
		writeErr(w, http.StatusBadRequest, "bad_agent_capture", "role must be user, assistant, system or tool")
		return
	}
	if !s.authorizeAgentProject(w, r, principal, input.ProjectID, "memory.capture") {
		return
	}
	project, exact, err := s.app.Portfolio.Resolve(r.Context(), input.ProjectID)
	if err != nil || !exact {
		writeErr(w, http.StatusBadRequest, "bad_agent_capture", "project_id must identify a registered project")
		return
	}
	observed := time.Now().UTC()
	if input.ObservedAt != "" {
		observed, err = time.Parse(time.RFC3339, input.ObservedAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_agent_capture", "observed_at must be RFC3339")
			return
		}
	}
	captured := time.Now().UTC()
	evidenceID := "agent_" + contracts.HashBytes([]byte(principal.AgentID + "\x00" + input.IdempotencyKey))[:24]
	if priorRaw, readErr := s.app.Ledger.ReadEvidence(r.Context(), evidenceID); readErr == nil {
		prior, _, parseErr := contracts.ParseEvidence(priorRaw)
		roleMatches := prior.Role != nil && *prior.Role == input.Role
		contentMatches := len(prior.Content) == 1 && prior.Content[0].Type == "text" && prior.Content[0].Text == input.Text
		observedMatches := input.ObservedAt == "" || prior.EffectiveObservedAt().Equal(observed.UTC())
		scopeMatches := slices.Contains(prior.ScopeHints, "project:"+project.Slug)
		if parseErr != nil || prior.SourceSystem != input.SourceSystem || prior.LogicalSessionID() != input.SessionID || !roleMatches || !contentMatches || !observedMatches || prior.Provenance.CaptureMethod != "agent_mcp" || !scopeMatches {
			err := fmt.Errorf("%w: %s", ledger.ErrEvidenceConflict, evidenceID)
			_ = s.app.Agents.Audit(r.Context(), principal, "memory.capture", "evidence", evidenceID, project.ProjectID, "failed", map[string]string{"error": err.Error()})
			writeErr(w, http.StatusConflict, "agent_capture_failed", err.Error())
			return
		}
		receipt, ok, receiptErr := s.app.Control.Receipt(r.Context(), evidenceID)
		if receiptErr != nil || !ok {
			writeErr(w, http.StatusInternalServerError, "agent_capture_receipt_failed", "existing Evidence receipt is missing")
			return
		}
		if err := s.app.Portfolio.LinkRecord(r.Context(), "evidence", evidenceID, project.ProjectID, true); err != nil {
			writeErr(w, http.StatusInternalServerError, "agent_capture_route_failed", err.Error())
			return
		}
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.capture", "evidence", evidenceID, project.ProjectID, "allowed", map[string]any{"duplicate": true, "session_id": receipt.SessionID, "pipeline_rerun": false})
		writeJSON(w, http.StatusOK, map[string]any{
			"evidence_id": evidenceID, "session_id": receipt.SessionID, "project_id": project.ProjectID,
			"duplicate": true, "ledger_path": receipt.LedgerRelPath, "pipeline_status": "not_rerun",
		})
		return
	} else if !errors.Is(readErr, os.ErrNotExist) {
		writeErr(w, http.StatusInternalServerError, "agent_capture_read_failed", readErr.Error())
		return
	}
	envelope := contracts.EvidenceEnvelope{
		SchemaVersion:          "0.1",
		EvidenceID:             evidenceID,
		SourceSystem:           input.SourceSystem,
		ExternalConversationID: &input.SessionID,
		Role:                   &input.Role,
		ObservedAt:             &observed,
		CapturedAt:             captured,
		Content:                []contracts.ContentBlock{{Type: "text", Text: input.Text}},
		Provenance:             contracts.Provenance{CaptureMethod: "agent_mcp"},
		ScopeHints:             []string{"project:" + project.Slug},
		Visibility:             "private",
	}
	raw, _ := json.Marshal(envelope)
	result, err := s.app.Ledger.Append(r.Context(), raw)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ledger.ErrEvidenceConflict) {
			status = http.StatusConflict
		}
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.capture", "evidence", evidenceID, project.ProjectID, "failed", map[string]string{"error": err.Error()})
		writeErr(w, status, "agent_capture_failed", err.Error())
		return
	}
	if err := s.app.Portfolio.LinkRecord(r.Context(), "evidence", result.EvidenceID, project.ProjectID, true); err != nil {
		writeErr(w, http.StatusInternalServerError, "agent_capture_route_failed", err.Error())
		return
	}
	pipeline, err := s.growSession(r.Context(), project.ProjectID, result.SessionID, []string{result.EvidenceID}, true, false)
	status := "allowed"
	if err != nil {
		status = "derived_failed"
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.capture", "evidence", result.EvidenceID, project.ProjectID, status, map[string]any{"duplicate": result.Duplicate, "session_id": result.SessionID})
	response := map[string]any{"evidence_id": result.EvidenceID, "session_id": result.SessionID, "project_id": project.ProjectID, "duplicate": result.Duplicate, "ledger_path": result.LedgerPath}
	if err == nil {
		response["pipeline"] = pipeline
	} else {
		response["pipeline_status"] = "failed"
		response["pipeline_error"] = err.Error()
	}
	httpStatus := http.StatusCreated
	if result.Duplicate {
		httpStatus = http.StatusOK
	}
	writeJSON(w, httpStatus, response)
}

func (s *Server) validateAgentEvidenceProject(ctx context.Context, principal agentauth.Principal, projectID string, evidenceIDs []string) error {
	for _, evidenceID := range evidenceIDs {
		evidenceID = strings.TrimSpace(evidenceID)
		if evidenceID == "" {
			continue
		}
		var linked int
		if err := s.app.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE project_id=? AND record_type='evidence' AND record_id=?`, projectID, evidenceID).Scan(&linked); err != nil {
			return err
		}
		if linked != 1 {
			return errors.New("source Evidence must belong to the target project")
		}
		allowed, err := s.app.Agents.CanAccessRecord(ctx, principal, "evidence", evidenceID)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.New("Agent is not granted the source Evidence")
		}
	}
	return nil
}

func (s *Server) agentProjectRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionProjectWrite)
	if !ok {
		return
	}
	var input mcpserver.ProjectRecordInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_project_record", err.Error())
		return
	}
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	if !s.authorizeAgentProject(w, r, principal, input.ProjectID, "project.write") {
		return
	}
	evidenceIDs := append([]string{}, input.SourceEvidenceIDs...)
	evidenceIDs = append(evidenceIDs, input.SourceEvidenceID)
	if err := s.validateAgentEvidenceProject(r.Context(), principal, input.ProjectID, evidenceIDs); err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "project.write", input.Kind, "", input.ProjectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "agent_source_forbidden", err.Error())
		return
	}
	var (
		output mcpserver.ProjectRecordOutput
		err    error
	)
	switch input.Kind {
	case "goal":
		var item portfolio.Goal
		item, err = s.app.Portfolio.CreateGoal(r.Context(), portfolio.GoalInput{ProjectID: input.ProjectID, Title: input.Title, Description: input.Description, Priority: input.Priority, TargetAt: input.TargetAt, SourceEvidenceID: input.SourceEvidenceID})
		output = mcpserver.ProjectRecordOutput{Kind: input.Kind, Record: item}
	case "decision":
		var item portfolio.Decision
		item, err = s.app.Portfolio.CreateDecision(r.Context(), portfolio.DecisionInput{ProjectID: input.ProjectID, Title: input.Title, Decision: input.Decision, Rationale: input.Rationale, DecidedAt: input.DecidedAt, SourceEvidenceIDs: input.SourceEvidenceIDs})
		output = mcpserver.ProjectRecordOutput{Kind: input.Kind, Record: item}
	case "risk":
		var item portfolio.Risk
		item, err = s.app.Portfolio.CreateRisk(r.Context(), portfolio.RiskInput{ProjectID: input.ProjectID, Title: input.Title, Description: input.Description, Probability: input.Probability, Impact: input.Impact, Mitigation: input.Mitigation, Owner: input.Owner, SourceEvidenceID: input.SourceEvidenceID})
		output = mcpserver.ProjectRecordOutput{Kind: input.Kind, Record: item}
	default:
		writeErr(w, http.StatusBadRequest, "bad_agent_project_record", "kind must be goal, decision or risk")
		return
	}
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "project.write", input.Kind, "", input.ProjectID, "failed", map[string]string{"error": err.Error()})
		writeErr(w, http.StatusBadRequest, "agent_project_write_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "project.write", input.Kind, "", input.ProjectID, "allowed", map[string]any{})
	writeJSON(w, http.StatusCreated, output)
}

func (s *Server) agentFinanceEntry(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionFinanceWrite)
	if !ok {
		return
	}
	var input mcpserver.FinanceWriteInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_finance", err.Error())
		return
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	if !s.authorizeAgentProject(w, r, principal, input.ProjectID, "finance.write") {
		return
	}
	if err := s.validateAgentEvidenceProject(r.Context(), principal, input.ProjectID, []string{input.SourceEvidenceID}); err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "finance.write", "finance", "", input.ProjectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "agent_source_forbidden", err.Error())
		return
	}
	item, duplicate, err := s.app.Portfolio.CreateFinanceEntry(r.Context(), portfolio.FinanceEntryInput{ProjectID: input.ProjectID, AccountID: input.AccountID, EntryType: input.EntryType, Category: input.Category, Description: input.Description, AmountMinor: input.AmountMinor, Currency: input.Currency, OccurredAt: input.OccurredAt, SourceEvidenceID: input.SourceEvidenceID, IdempotencyKey: input.IdempotencyKey})
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "finance.write", "finance", "", input.ProjectID, "failed", map[string]string{"error": err.Error()})
		writeErr(w, http.StatusBadRequest, "agent_finance_write_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "finance.write", "finance", item.EntryID, input.ProjectID, "allowed", map[string]any{"duplicate": duplicate})
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, mcpserver.FinanceWriteOutput{Entry: item, Duplicate: duplicate})
}

func (s *Server) agentTimeline(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if !s.authorizeAgentProject(w, r, principal, projectID, "memory.read") {
		return
	}
	kinds := []string{}
	for _, raw := range strings.Split(r.URL.Query().Get("kinds"), ",") {
		kind := strings.TrimSpace(raw)
		if kind != "" {
			kinds = append(kinds, kind)
		}
	}
	includeFinance := agentauth.HasPermission(principal, agentauth.PermissionFinanceRead)
	if !includeFinance {
		if len(kinds) == 0 {
			kinds = []string{"fact", "goal", "milestone", "decision", "episode", "memory", "run", "evidence"}
		} else {
			filtered := kinds[:0]
			for _, kind := range kinds {
				if kind != "finance" {
					filtered = append(filtered, kind)
				}
			}
			kinds = filtered
		}
	}
	result, err := s.app.Portfolio.Timeline(r.Context(), portfolio.TimelineQuery{
		ProjectID: projectID,
		AnchorAt:  r.URL.Query().Get("anchor_at"),
		From:      r.URL.Query().Get("from"),
		Until:     r.URL.Query().Get("until"),
		Kinds:     kinds,
		Limit:     queryLimit(r),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_timeline_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "project_timeline", projectID, projectID, "allowed", map[string]any{"events": len(result.Events), "finance_included": includeFinance, "anchor_at": result.AnchorAt})
	writeJSON(w, http.StatusOK, result)
}
