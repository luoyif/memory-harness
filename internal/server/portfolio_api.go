package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func decodeBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writePortfolioError(w http.ResponseWriter, code string, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "record not found")
		return
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unique") || strings.Contains(lower, "same project") || strings.Contains(lower, "superseded") {
		writeErr(w, http.StatusConflict, code, err.Error())
		return
	}
	writeErr(w, http.StatusBadRequest, code, err.Error())
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListProjects(r.Context(), r.URL.Query().Get("include_archived") == "1")
	if err != nil {
		writePortfolioError(w, "projects_failed", err)
		return
	}
	summaries := make([]portfolio.ProjectSummary, 0, len(items))
	for _, item := range items {
		summary, err := s.app.Portfolio.ProjectSummary(r.Context(), item.ProjectID)
		if err != nil {
			writePortfolioError(w, "projects_failed", err)
			return
		}
		summaries = append(summaries, summary)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": summaries, "total": len(summaries)})
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var input portfolio.ProjectInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_project", err.Error())
		return
	}
	project, err := s.app.Portfolio.CreateProject(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	summary, err := s.app.Portfolio.ProjectSummary(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	goals, err := s.app.Portfolio.ListGoals(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	milestones, err := s.app.Portfolio.ListMilestones(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	decisions, err := s.app.Portfolio.ListDecisions(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	risks, err := s.app.Portfolio.ListRisks(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	blocks, err := s.app.Portfolio.ListContextBlocks(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	accounts, err := s.app.Portfolio.ListFinanceAccounts(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	tasks, err := s.app.Portfolio.ListProjectTasks(r.Context(), projectID, "")
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	automation, err := s.app.Portfolio.ProjectAutomation(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "goals": goals, "milestones": milestones, "decisions": decisions, "risks": risks, "context_blocks": blocks, "finance_accounts": accounts, "tasks": tasks, "automation": automation})
}

func (s *Server) refreshProjectBrief(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	object, err := s.app.Growth.RefreshProjectBrief(r.Context(), projectID)
	if err != nil {
		writePortfolioError(w, "project_brief_refresh_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "refreshed", "object": object, "duplicate": object.Duplicate,
	})
}

func (s *Server) linkProjectRecord(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RecordType string `json:"record_type"`
		RecordID   string `json:"record_id"`
		ProjectID  string `json:"project_id"`
		Primary    bool   `json:"primary"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_link", err.Error())
		return
	}
	if err := s.app.Portfolio.LinkRecord(r.Context(), request.RecordType, request.RecordID, request.ProjectID, request.Primary); err != nil {
		writePortfolioError(w, "link_failed", err)
		return
	}
	if _, err := s.app.Portfolio.RebuildProjectIndex(r.Context(), request.ProjectID); err != nil {
		writePortfolioError(w, "index_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "linked"})
}

func (s *Server) facts(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListFacts(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("as_of"), r.URL.Query().Get("include_history") == "1", queryLimit(r))
	if err != nil {
		writePortfolioError(w, "facts_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"facts": items, "total": len(items), "as_of": r.URL.Query().Get("as_of")})
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	kinds := []string{}
	for _, value := range strings.Split(r.URL.Query().Get("kinds"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			kinds = append(kinds, value)
		}
	}
	result, err := s.app.Portfolio.Timeline(r.Context(), portfolio.TimelineQuery{
		ProjectID: r.URL.Query().Get("project_id"), AnchorAt: r.URL.Query().Get("anchor_at"),
		From: r.URL.Query().Get("from"), Until: r.URL.Query().Get("until"), Kinds: kinds, Limit: queryLimit(r),
	})
	if err != nil {
		writePortfolioError(w, "timeline_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createFact(w http.ResponseWriter, r *http.Request) {
	var input portfolio.FactInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_fact", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateFact(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "fact_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) contextBlocks(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListContextBlocks(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "context_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"context_blocks": items, "total": len(items)})
}

func (s *Server) upsertContextBlock(w http.ResponseWriter, r *http.Request) {
	var input portfolio.ContextBlockInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_context", err.Error())
		return
	}
	item, err := s.app.Portfolio.UpsertContextBlock(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "context_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) goals(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListGoals(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "goals_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"goals": items, "total": len(items)})
}

func (s *Server) createGoal(w http.ResponseWriter, r *http.Request) {
	var input portfolio.GoalInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_goal", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateGoal(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "goal_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func statusRequest(r *http.Request) (string, error) {
	var request struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &request); err != nil {
		return "", err
	}
	if strings.TrimSpace(request.Status) == "" {
		return "", errors.New("status is required")
	}
	return request.Status, nil
}

func (s *Server) updateGoal(w http.ResponseWriter, r *http.Request) {
	status, err := statusRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_goal", err.Error())
		return
	}
	item, err := s.app.Portfolio.UpdateGoalStatus(r.Context(), r.PathValue("id"), status)
	if err != nil {
		writePortfolioError(w, "goal_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) projectTasks(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListProjectTasks(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"))
	if err != nil {
		writePortfolioError(w, "project_tasks_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items, "total": len(items)})
}

func (s *Server) createProjectTask(w http.ResponseWriter, r *http.Request) {
	var input portfolio.ProjectTaskInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_project_task", err.Error())
		return
	}
	// The Owner task API creates authoritative manual todos. AI-derived
	// suggestions are materialized internally from provenance-bearing goals.
	input.SourceKind = "manual"
	input.SourceRecordID = ""
	item, err := s.app.Portfolio.CreateProjectTask(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "project_task_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateProjectTask(w http.ResponseWriter, r *http.Request) {
	status, err := statusRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_project_task", err.Error())
		return
	}
	item, err := s.app.Portfolio.UpdateProjectTaskStatus(r.Context(), r.PathValue("id"), status)
	if err != nil {
		writePortfolioError(w, "project_task_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) projectAutomation(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Portfolio.ProjectAutomation(r.Context(), r.PathValue("id"))
	if err != nil {
		writePortfolioError(w, "project_automation_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateProjectAutomation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ImportMode string `json:"import_mode"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_project_automation", err.Error())
		return
	}
	item, err := s.app.Portfolio.SetProjectAutomation(r.Context(), r.PathValue("id"), input.ImportMode)
	if err != nil {
		writePortfolioError(w, "project_automation_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) milestones(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListMilestones(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "milestones_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"milestones": items, "total": len(items)})
}

func (s *Server) createMilestone(w http.ResponseWriter, r *http.Request) {
	var input portfolio.MilestoneInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_milestone", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateMilestone(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "milestone_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateMilestone(w http.ResponseWriter, r *http.Request) {
	status, err := statusRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_milestone", err.Error())
		return
	}
	item, err := s.app.Portfolio.UpdateMilestoneStatus(r.Context(), r.PathValue("id"), status)
	if err != nil {
		writePortfolioError(w, "milestone_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) decisions(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListDecisions(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "decisions_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": items, "total": len(items)})
}

func (s *Server) createDecision(w http.ResponseWriter, r *http.Request) {
	var input portfolio.DecisionInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_decision", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateDecision(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "decision_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) risks(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListRisks(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "risks_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"risks": items, "total": len(items)})
}

func (s *Server) createRisk(w http.ResponseWriter, r *http.Request) {
	var input portfolio.RiskInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_risk", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateRisk(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "risk_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateRisk(w http.ResponseWriter, r *http.Request) {
	status, err := statusRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_risk", err.Error())
		return
	}
	item, err := s.app.Portfolio.UpdateRiskStatus(r.Context(), r.PathValue("id"), status)
	if err != nil {
		writePortfolioError(w, "risk_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) financeAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListFinanceAccounts(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "accounts_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": items, "total": len(items)})
}

func (s *Server) createFinanceAccount(w http.ResponseWriter, r *http.Request) {
	var input portfolio.FinanceAccount
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_account", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateFinanceAccount(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "account_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) financeEntries(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListFinanceEntries(r.Context(), r.URL.Query().Get("project_id"), queryLimit(r))
	if err != nil {
		writePortfolioError(w, "finance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": items, "total": len(items)})
}

func (s *Server) createFinanceEntry(w http.ResponseWriter, r *http.Request) {
	var input portfolio.FinanceEntryInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_finance", err.Error())
		return
	}
	item, duplicate, err := s.app.Portfolio.CreateFinanceEntry(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "finance_failed", err)
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"entry": item, "duplicate": duplicate})
}

func (s *Server) updateFinanceEntry(w http.ResponseWriter, r *http.Request) {
	status, err := statusRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_finance", err.Error())
		return
	}
	if status != "void" {
		writeErr(w, http.StatusBadRequest, "bad_finance", "finance entries can only transition to void")
		return
	}
	item, err := s.app.Portfolio.VoidFinanceEntry(r.Context(), r.PathValue("id"))
	if err != nil {
		writePortfolioError(w, "finance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) financeSummary(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Portfolio.FinanceSummary(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "finance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) connectors(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Portfolio.ListConnectors(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		writePortfolioError(w, "connectors_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": items, "total": len(items)})
}

func (s *Server) createConnector(w http.ResponseWriter, r *http.Request) {
	var input portfolio.ConnectorInput
	if err := decodeBody(r, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_connector", err.Error())
		return
	}
	item, err := s.app.Portfolio.CreateConnector(r.Context(), input)
	if err != nil {
		writePortfolioError(w, "connector_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) unifiedSearch(w http.ResponseWriter, r *http.Request) {
	var query unifiedsearch.Query
	if err := decodeBody(r, &query); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_search", err.Error())
		return
	}
	result, err := s.app.Unified.Search(r.Context(), query)
	if err != nil {
		writePortfolioError(w, "search_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) recallFeedback(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ProjectID string `json:"project_id"`
		ContextID string `json:"context_id"`
		ResultID  string `json:"result_id"`
		Rating    string `json:"rating"`
		Note      string `json:"note"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_feedback", err.Error())
		return
	}
	if err := s.app.Portfolio.RecordRecallFeedback(r.Context(), request.ProjectID, request.ContextID, request.ResultID, request.Rating, request.Note); err != nil {
		writePortfolioError(w, "feedback_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "recorded"})
}

func requestLimit(r *http.Request, fallback int) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value <= 0 {
		return fallback
	}
	return value
}
