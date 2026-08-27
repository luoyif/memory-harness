package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/experience"
)

func (s *Server) experienceCases(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Experience.ListCases(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_cases_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": items, "total": len(items)})
}

func (s *Server) rebuildExperienceCases(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	items, err := s.app.Experience.DiscoverCases(r.Context(), projectID, harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_rebuild_failed", err.Error())
		return
	}
	indexed, err := s.app.Experience.RebuildProjection(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "experience_projection_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "rebuilt", "cases": items, "total": len(items), "indexed": indexed})
}
func (s *Server) experienceCase(w http.ResponseWriter, r *http.Request) {
	object, value, err := s.app.Experience.Case(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "experience_case_failed", err.Error())
		return
	}
	evaluationObjects, evaluations, err := s.app.Experience.EvaluationsForTarget(r.Context(), object.ProjectID, object.ObjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "experience_case_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": object, "case": value, "evaluation_objects": evaluationObjects, "evaluations": evaluations})
}

func (s *Server) evaluateExperienceCase(w http.ResponseWriter, r *http.Request) {
	var input experience.EvaluateInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_experience_evaluation", err.Error())
		return
	}
	input.TargetKind, input.TargetID = "case", r.PathValue("id")
	object, err := s.app.Experience.Evaluate(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_evaluation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, object)
}
func (s *Server) proposeExperienceCaseActivation(w http.ResponseWriter, r *http.Request) {
	var input experience.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_experience_activation", err.Error())
		return
	}
	review, err := s.app.Experience.ProposeActivation(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_activation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) experienceEvaluations(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Experience.ListEvaluations(r.Context(), r.URL.Query().Get("project_id"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_evaluations_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": items, "total": len(items)})
}

func (s *Server) experiencePatterns(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Experience.ListPatterns(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_patterns_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patterns": items, "total": len(items)})
}
func (s *Server) createExperiencePattern(w http.ResponseWriter, r *http.Request) {
	var input experience.PatternInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_experience_pattern", err.Error())
		return
	}
	object, err := s.app.Experience.CreatePattern(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_pattern_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if object.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, object)
}

func (s *Server) experiencePattern(w http.ResponseWriter, r *http.Request) {
	object, value, err := s.app.Experience.Pattern(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "experience_pattern_failed", err.Error())
		return
	}
	evaluationObjects, evaluations, err := s.app.Experience.EvaluationsForTarget(r.Context(), object.ProjectID, object.ObjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "experience_pattern_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": object, "pattern": value, "evaluation_objects": evaluationObjects, "evaluations": evaluations})
}
func (s *Server) evaluateExperiencePattern(w http.ResponseWriter, r *http.Request) {
	var input experience.EvaluateInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_experience_evaluation", err.Error())
		return
	}
	input.TargetKind, input.TargetID = "pattern", r.PathValue("id")
	object, err := s.app.Experience.Evaluate(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_evaluation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, object)
}

func (s *Server) proposeExperiencePatternActivation(w http.ResponseWriter, r *http.Request) {
	var input experience.ActivationInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_experience_activation", err.Error())
		return
	}
	review, err := s.app.Experience.ProposeActivation(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "experience_activation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, review)
}
