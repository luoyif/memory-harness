package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/luoyif/memory-harness/internal/harness"
)

func harnessQueryLimit(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return value
}

func harnessQueryOffset(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if value < 0 {
		return 0
	}
	return value
}

func (s *Server) harnessTypes(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Harness.ListTypes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "harness_types_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": items})
}

func (s *Server) registerHarnessType(w http.ResponseWriter, r *http.Request) {
	var input harness.RegisterTypeInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_type", err.Error())
		return
	}
	item, err := s.app.Harness.RegisterType(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_type_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) harnessObjects(w http.ResponseWriter, r *http.Request) {
	items, page, err := s.app.Harness.ListObjectsPage(r.Context(), harness.ObjectListOptions{
		ProjectID: r.URL.Query().Get("project_id"), TypeID: r.URL.Query().Get("type_id"), Status: r.URL.Query().Get("status"),
		Limit: harnessQueryLimit(r), Offset: harnessQueryOffset(r),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_objects_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "has_more": page.HasMore})
}

func (s *Server) materializeHarnessObject(w http.ResponseWriter, r *http.Request) {
	var input harness.MaterializeInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_object", err.Error())
		return
	}
	item, err := s.app.Harness.Materialize(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_object_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if item.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, item)
}

func (s *Server) harnessObject(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Harness.Object(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "harness_object_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) harnessRuns(w http.ResponseWriter, r *http.Request) {
	items, page, err := s.app.Harness.ListRunsPage(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r), harnessQueryOffset(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_runs_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "has_more": page.HasMore})
}

func (s *Server) startHarnessRun(w http.ResponseWriter, r *http.Request) {
	var input harness.StartRunInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_run", err.Error())
		return
	}
	item, err := s.app.Harness.StartRun(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_run_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if item.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, item)
}

func (s *Server) harnessRun(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Harness.RunDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "harness_run_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) appendHarnessEvent(w http.ResponseWriter, r *http.Request) {
	var input struct {
		EventType string `json:"event_type"`
		Producer  string `json:"producer"`
		Data      any    `json:"data"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_event", err.Error())
		return
	}
	item, err := s.app.Harness.AppendEvent(r.Context(), r.PathValue("id"), input.EventType, input.Producer, input.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_event_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) startHarnessSpan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ParentSpanID string `json:"parent_span_id"`
		NodeID       string `json:"node_id"`
		StageType    string `json:"stage_type"`
		StageVersion string `json:"stage_version"`
		PluginID     string `json:"plugin_id"`
		InputHash    string `json:"input_hash"`
		Detail       any    `json:"detail"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_span", err.Error())
		return
	}
	item, err := s.app.Harness.StartSpan(r.Context(), r.PathValue("id"), input.ParentSpanID, input.NodeID, input.StageType, input.StageVersion, input.PluginID, input.InputHash, input.Detail)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_span_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) finishHarnessSpan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status     string `json:"status"`
		OutputHash string `json:"output_hash"`
		Detail     any    `json:"detail"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_span", err.Error())
		return
	}
	item, err := s.app.Harness.FinishSpan(r.Context(), r.PathValue("id"), input.Status, input.OutputHash, input.Detail)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_span_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) harnessEffect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action      string `json:"action"`
		NodeID      string `json:"node_id"`
		EffectKey   string `json:"effect_key"`
		RequestHash string `json:"request_hash"`
		ProviderKey string `json:"provider_idempotency_key"`
		Outcome     string `json:"outcome"`
		ResultHash  string `json:"result_hash"`
		Receipt     any    `json:"receipt"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_harness_effect", err.Error())
		return
	}
	var (
		item harness.Effect
		err  error
	)
	switch input.Action {
	case "intent":
		item, err = s.app.Harness.RecordEffectIntent(r.Context(), r.PathValue("id"), input.NodeID, input.EffectKey, input.RequestHash)
	case "dispatch":
		item, err = s.app.Harness.MarkEffectDispatched(r.Context(), r.PathValue("id"), input.NodeID, input.EffectKey, input.ProviderKey)
	case "receipt":
		item, err = s.app.Harness.RecordEffectReceipt(r.Context(), r.PathValue("id"), input.NodeID, input.EffectKey, input.Outcome, input.ResultHash, input.Receipt)
	case "materialize":
		item, err = s.app.Harness.MarkEffectMaterialized(r.Context(), r.PathValue("id"), input.NodeID, input.EffectKey)
	default:
		err = errors.New("action must be intent, dispatch, receipt or materialize")
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_effect_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func ownerActor(r *http.Request) string {
	if principal, ok := ownerFromContext(r.Context()); ok && principal.SessionID != "" {
		return principal.SessionID
	}
	return "local-owner"
}

func (s *Server) harnessObjectRevisions(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Harness.ListObjectRevisions(r.Context(), r.PathValue("id"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "harness_revisions_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": items, "total": len(items)})
}

func (s *Server) proposeHarnessObjectRevision(w http.ResponseWriter, r *http.Request) {
	var input harness.ProposeRevisionInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_revision_proposal", err.Error())
		return
	}
	input.RequestedBy = ownerActor(r)
	item, err := s.app.Harness.ProposeRevision(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "revision_proposal_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) rollbackHarnessObject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int `json:"revision"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_rollback", err.Error())
		return
	}
	if input.Revision < 1 {
		writeErr(w, http.StatusBadRequest, "bad_rollback", "revision must be >= 1")
		return
	}
	item, err := s.app.Harness.ProposeRollback(r.Context(), r.PathValue("id"), input.Revision, ownerActor(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "rollback_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) harnessRevisionReviews(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Harness.ListRevisionReviews(r.Context(), r.URL.Query().Get("object_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "revision_reviews_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": items, "total": len(items)})
}

func (s *Server) harnessRevisionReview(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Harness.RevisionReview(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "revision_review_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) decideHarnessRevisionReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_revision_decision", err.Error())
		return
	}
	item, err := s.app.Harness.DecideRevisionReview(r.Context(), r.PathValue("id"), input.Decision, ownerActor(r), input.Note)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "revision_decision_failed", err.Error())
		return
	}
	if item.Status == "approved" {
		object, objectErr := s.app.Harness.Object(r.Context(), item.ObjectID)
		if objectErr != nil {
			writeErr(w, http.StatusInternalServerError, "revision_approved_projection_failed", "revision was approved but its object could not be read: "+objectErr.Error())
			return
		}
		if _, projectionErr := s.app.Reprojection.RefreshApprovedObject(r.Context(), object); projectionErr != nil {
			writeErr(w, http.StatusInternalServerError, "revision_approved_projection_failed", "revision was approved but downstream projection refresh failed: "+projectionErr.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, item)
}
