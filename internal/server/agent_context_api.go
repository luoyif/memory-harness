package server

import (
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/contextbridge"
)

func (s *Server) agentContextHandshake(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionContextPlan)
	if !ok {
		return
	}
	var input contextbridge.ContextCapabilitySet
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_context_handshake", err.Error())
		return
	}
	result, err := s.app.Context.Negotiate(input)
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionContextPlan, "context_capability", input.CapabilitySetID, "", "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusBadRequest, "context_handshake_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "context.handshake", "context_capability", input.CapabilitySetID, "", "allowed", map[string]string{"level": result.Level, "hash": result.CapabilitySetHash})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) agentContextPlan(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionContextPlan)
	if !ok {
		return
	}
	var input contextbridge.PlanRequest
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_context_plan", err.Error())
		return
	}
	if !s.authorizeAgentProject(w, r, principal, strings.TrimSpace(input.ProjectID), agentauth.PermissionContextPlan) {
		return
	}
	input.AgentID = principal.AgentID
	result, err := s.app.Context.CompilePlan(r.Context(), input)
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionContextPlan, "context_plan", "", input.ProjectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusBadRequest, "context_plan_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionContextPlan, "context_plan", result.Plan.PlanID, input.ProjectID, "allowed", map[string]any{"run_id": result.RunID, "items": len(result.Plan.Items), "duplicate": result.Duplicate})
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) agentContextReceipt(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionContextReceipt)
	if !ok {
		return
	}
	var input contextbridge.ReceiptRequest
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_context_receipt", err.Error())
		return
	}
	run, err := s.app.Harness.Run(r.Context(), strings.TrimSpace(input.RunID))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "context_receipt_failed", err.Error())
		return
	}
	if !s.authorizeAgentProject(w, r, principal, run.ProjectID, agentauth.PermissionContextReceipt) {
		return
	}
	input.AgentID = principal.AgentID
	result, err := s.app.Context.RecordReceipt(r.Context(), input)
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionContextReceipt, "context_receipt", input.Receipt.ReceiptID, run.ProjectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusBadRequest, "context_receipt_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionContextReceipt, "context_receipt", result.Receipt.ReceiptID, run.ProjectID, "allowed", map[string]any{"run_id": result.RunID, "duplicate": result.Duplicate, "delivery_status": result.DeliveryStatus})
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) agentOutcomeFeedback(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionOutcomeReport)
	if !ok {
		return
	}
	var input contextbridge.OutcomeFeedback
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_outcome_feedback", err.Error())
		return
	}
	if !s.authorizeAgentProject(w, r, principal, strings.TrimSpace(input.ProjectID), agentauth.PermissionOutcomeReport) {
		return
	}
	result, err := s.app.Context.RecordOutcome(r.Context(), principal.AgentID, input)
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionOutcomeReport, "context_outcome", input.OutcomeID, input.ProjectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusBadRequest, "outcome_feedback_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, agentauth.PermissionOutcomeReport, "context_outcome", result.Outcome.OutcomeID, input.ProjectID, "allowed", map[string]any{"observation_run_id": result.RunID, "target_run_id": input.RunID, "duplicate": result.Duplicate})
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) contextExchange(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.Context.Exchange(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "context_exchange_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
