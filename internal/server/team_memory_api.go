package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/teammemory"
)

func teamQueryLimit(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value <= 0 || value > 500 {
		return 100
	}
	return value
}

func (s *Server) teamTasks(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.TeamMemory.ListTasks(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), teamQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_tasks_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items, "total": len(items)})
}

func (s *Server) createTeamTask(w http.ResponseWriter, r *http.Request) {
	var input teammemory.CreateTaskInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_task", err.Error())
		return
	}
	object, err := s.app.TeamMemory.CreateTask(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_task_create_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) teamTask(w http.ResponseWriter, r *http.Request) {
	object, value, err := s.app.TeamMemory.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "team_task_failed", err.Error())
		return
	}
	entries, err := s.app.TeamMemory.BlackboardForOwner(r.Context(), object.ObjectID, r.URL.Query().Get("include_expired") == "true")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "team_task_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": object, "task": value, "blackboard": entries})
}

func (s *Server) proposeTeamTaskClosure(w http.ResponseWriter, r *http.Request) {
	var input teammemory.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_task_close", err.Error())
		return
	}
	review, err := s.app.TeamMemory.ProposeTaskClosure(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_task_close_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) teamConflicts(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.TeamMemory.ListConflicts(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), teamQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_conflicts_failed", err.Error())
		return
	}
	reviewStatus := map[string]string{}
	for _, object := range items {
		reviews, reviewErr := s.app.Harness.ListRevisionReviews(r.Context(), object.ObjectID, "", 1)
		if reviewErr != nil {
			writeErr(w, http.StatusInternalServerError, "team_conflicts_failed", reviewErr.Error())
			return
		}
		if len(reviews) == 0 {
			reviewStatus[object.ObjectID] = "missing"
		} else {
			reviewStatus[object.ObjectID] = reviews[0].Status
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": items, "total": len(items), "review_status": reviewStatus})
}

func (s *Server) teamDurables(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.TeamMemory.ListDurables(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), teamQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_durables_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"durables": items, "total": len(items)})
}

func (s *Server) createTeamDurable(w http.ResponseWriter, r *http.Request) {
	var input teammemory.DurableInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_durable", err.Error())
		return
	}
	object, err := s.app.TeamMemory.CreateDurable(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_durable_create_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) proposeTeamDurableActivation(w http.ResponseWriter, r *http.Request) {
	var input teammemory.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_durable_activation", err.Error())
		return
	}
	review, err := s.app.TeamMemory.ProposeDurableActivation(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "team_durable_activation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

func auditTeam(s *Server, r *http.Request, principal agentauth.Principal, action, resource, id, projectID, status string, detail any) {
	_ = s.app.Agents.Audit(r.Context(), principal, action, resource, id, projectID, status, detail)
}

func (s *Server) agentTeamTasks(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	items, err := s.app.TeamMemory.TasksForAgent(r.Context(), principal)
	if err != nil {
		auditTeam(s, r, principal, "team.tasks.read", "team_task", "", "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_tasks_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.tasks.read", "team_task", "", "", "allowed", map[string]int{"total": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items, "total": len(items)})
}

func (s *Server) agentTeamLeaveProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	var input teammemory.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_leave", err.Error())
		return
	}
	review, err := s.app.TeamMemory.ProposeSelfLeave(r.Context(), principal, r.PathValue("id"), input)
	if err != nil {
		auditTeam(s, r, principal, "team.task.leave", "team_task", r.PathValue("id"), "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_leave_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.task.leave", "team_task", r.PathValue("id"), "", "allowed", map[string]string{"review_id": review.ReviewID})
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) agentTeamPrivate(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	items, err := s.app.TeamMemory.PrivateForAgent(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		auditTeam(s, r, principal, "team.private.read", "team_task", r.PathValue("id"), "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_private_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.private.read", "team_task", r.PathValue("id"), "", "allowed", map[string]int{"total": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"private": items, "total": len(items)})
}

func (s *Server) agentWriteTeamPrivate(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	var input teammemory.ContributionInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_private", err.Error())
		return
	}
	object, err := s.app.TeamMemory.WritePrivate(r.Context(), principal, r.PathValue("id"), input)
	if err != nil {
		auditTeam(s, r, principal, "team.private.write", "team_task", r.PathValue("id"), "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_private_write_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.private.write", "team_private", object.ObjectID, object.ProjectID, "allowed", nil)
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) agentTeamBlackboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	items, err := s.app.TeamMemory.BlackboardForAgent(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		auditTeam(s, r, principal, "team.blackboard.read", "team_task", r.PathValue("id"), "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_blackboard_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.blackboard.read", "team_task", r.PathValue("id"), "", "allowed", map[string]int{"total": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"blackboard": items, "total": len(items)})
}

func (s *Server) agentWriteTeamBlackboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	var input teammemory.BlackboardInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_blackboard", err.Error())
		return
	}
	result, err := s.app.TeamMemory.WriteBlackboard(r.Context(), principal, r.PathValue("id"), input)
	if err != nil {
		auditTeam(s, r, principal, "team.blackboard.write", "team_task", r.PathValue("id"), "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusForbidden, "team_blackboard_write_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.blackboard.write", "team_blackboard", result.Entry.EntryID, result.Entry.ProjectID, "allowed", map[string]int{"conflicts": len(result.Conflicts)})
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) agentShareTeamBlackboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	var input teammemory.ShareInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_team_share", err.Error())
		return
	}
	object, err := s.app.TeamMemory.SetShare(r.Context(), principal, r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusForbidden, "team_share_forbidden", err.Error())
		return
	}
	auditTeam(s, r, principal, "team.blackboard.share", "team_blackboard", object.ObjectID, object.ProjectID, "allowed", nil)
	writeJSON(w, http.StatusOK, object)
}
