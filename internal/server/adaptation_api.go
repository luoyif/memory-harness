package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/adaptation"
	"github.com/luoyif/memory-harness/internal/experience"
)

func (s *Server) adaptationProposals(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Adaptation.ListProposals(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_proposals_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": items, "total": len(items)})
}

func (s *Server) dryRunAdaptationProposal(w http.ResponseWriter, r *http.Request) {
	var input adaptation.ProposalInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_proposal", err.Error())
		return
	}
	result, err := s.app.Adaptation.DryRunProposal(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_dry_run_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) createAdaptationProposal(w http.ResponseWriter, r *http.Request) {
	var input adaptation.ProposalInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_proposal", err.Error())
		return
	}
	object, err := s.app.Adaptation.CreateProposal(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_proposal_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) adaptationProposal(w http.ResponseWriter, r *http.Request) {
	object, value, err := s.app.Adaptation.Proposal(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "adaptation_proposal_failed", err.Error())
		return
	}
	evaluationObjects, evaluations, err := s.app.Experience.EvaluationsForTarget(r.Context(), object.ProjectID, object.ObjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "adaptation_proposal_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": object, "proposal": value, "evaluation_objects": evaluationObjects, "evaluations": evaluations})
}
func (s *Server) evaluateAdaptationProposal(w http.ResponseWriter, r *http.Request) {
	var input experience.EvaluateInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_evaluation", err.Error())
		return
	}
	object, err := s.app.Adaptation.EvaluateProposal(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_evaluation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, object)
}

func (s *Server) proposeAdaptationApproval(w http.ResponseWriter, r *http.Request) {
	var input adaptation.ApprovalInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_approval", err.Error())
		return
	}
	review, err := s.app.Adaptation.ProposeApproval(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_approval_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) adaptationOverlays(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Adaptation.ListOverlays(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_overlays_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overlays": items, "total": len(items)})
}
func (s *Server) createAdaptationOverlay(w http.ResponseWriter, r *http.Request) {
	var input adaptation.OverlayInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_overlay", err.Error())
		return
	}
	object, err := s.app.Adaptation.CreateOverlay(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_overlay_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) adaptationOverlay(w http.ResponseWriter, r *http.Request) {
	object, value, err := s.app.Adaptation.Overlay(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "adaptation_overlay_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": object, "overlay": value})
}

func (s *Server) proposeAdaptationOverlayActivation(w http.ResponseWriter, r *http.Request) {
	var input experience.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_overlay_activation", err.Error())
		return
	}
	review, err := s.app.Adaptation.ProposeOverlayActivation(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_overlay_activation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}
func (s *Server) runAdaptationCanary(w http.ResponseWriter, r *http.Request) {
	var input adaptation.CanaryInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_canary", err.Error())
		return
	}
	input.OverlayID = r.PathValue("id")
	result, err := s.app.Adaptation.RunCanary(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_canary_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) rollbackAdaptationOverlay(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_rollback", err.Error())
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		writeErr(w, http.StatusBadRequest, "bad_adaptation_rollback", "idempotency_key is required")
		return
	}
	result, err := s.app.Adaptation.RollbackToBase(r.Context(), r.PathValue("id"), ownerActor(r), input.IdempotencyKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "adaptation_rollback_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
