package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/harness"
)

func (s *Server) agentProjectBlueprint(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionProjectRead)
	if !ok {
		return
	}
	projectID := r.PathValue("id")
	if !s.authorizeAgentProject(w, r, principal, projectID, "project.read") {
		return
	}
	item, err := s.app.Blueprints.Current(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_project_blueprint_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "project.read", "memory_blueprint", item.Blueprint.BlueprintID, projectID, "allowed", map[string]any{"version": item.Blueprint.Version, "hash": item.Blueprint.ContentHash})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) agentHarnessTypes(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	items, err := s.app.Harness.ListTypes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "agent_memory_types_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "memory_type_collection", "", "", "allowed", map[string]int{"count": len(items)})
	writeJSON(w, http.StatusOK, map[string]any{"types": items})
}

func agentGenericObjectAllowed(typeID string) bool {
	typeID = strings.TrimSpace(typeID)
	if typeID == harness.ProfileProjectionTypeV1 {
		return false
	}
	for _, prefix := range []string{"builtin.experience-bank.", "builtin.adaptation-lab.", "builtin.portable-bundle.", "builtin.team-memory."} {
		if strings.HasPrefix(typeID, prefix) {
			return false
		}
	}
	return true
}

func (s *Server) agentHarnessObjects(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	projectID := r.URL.Query().Get("project_id")
	if !s.authorizeAgentProject(w, r, principal, projectID, "memory.read") {
		return
	}
	typeID := strings.TrimSpace(r.URL.Query().Get("type_id"))
	if typeID != "" && !agentGenericObjectAllowed(typeID) {
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "harness_object_collection", "", projectID, "denied", map[string]string{"reason": "specialized_owner_or_scoped_api_required", "type_id": typeID})
		writeErr(w, http.StatusForbidden, "agent_object_type_forbidden", "This governed object type requires its specialized scoped API")
		return
	}
	items, page, err := s.app.Harness.ListObjectsPage(r.Context(), harness.ObjectListOptions{
		ProjectID: projectID, TypeID: typeID, Status: r.URL.Query().Get("status"), Limit: harnessQueryLimit(r), Offset: harnessQueryOffset(r),
		ExcludedTypeIDs:      []string{harness.ProfileProjectionTypeV1},
		ExcludedTypePrefixes: []string{"builtin.experience-bank.", "builtin.adaptation-lab.", "builtin.portable-bundle.", "builtin.team-memory."},
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_objects_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "harness_object_collection", "", projectID, "allowed", map[string]any{"count": len(items), "offset": page.Offset, "total_visible": page.Total})
	writeJSON(w, http.StatusOK, map[string]any{"objects": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "has_more": page.HasMore})
}

func (s *Server) agentHarnessObject(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryRead)
	if !ok {
		return
	}
	item, err := s.app.Harness.Object(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "agent_object_not_found", "memory object not found")
		return
	}
	if !agentGenericObjectAllowed(item.TypeID) {
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "harness_object", item.ObjectID, item.ProjectID, "denied", map[string]string{"reason": "specialized_owner_or_scoped_api_required", "type_id": item.TypeID})
		writeErr(w, http.StatusForbidden, "agent_object_type_forbidden", "This governed object type requires its specialized scoped API")
		return
	}
	if !s.authorizeAgentProject(w, r, principal, item.ProjectID, "memory.read") {
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.read", "harness_object", item.ObjectID, item.ProjectID, "allowed", map[string]any{"revision": item.CurrentRevision})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) agentProposeHarnessRevision(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionMemoryPropose)
	if !ok {
		return
	}
	object, err := s.app.Harness.Object(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "agent_object_not_found", "memory object not found")
		return
	}
	if !s.authorizeAgentProject(w, r, principal, object.ProjectID, "memory.propose") {
		return
	}
	if object.TypeID != harness.GovernedAgentAssetTypeV3 && object.TypeID != harness.GovernedAgentAssetTypeV4 {
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.propose", "harness_object_revision", object.ObjectID, object.ProjectID, "denied", map[string]any{"reason": "only governed Agent Asset v4 or v3 accepts Agent proposals"})
		writeErr(w, http.StatusBadRequest, "agent_revision_type_denied", "Agent revision proposals are limited to governed Agent Asset v3 or template-governed v4")
		return
	}
	var input struct {
		ExpectedRevision int             `json:"expected_revision"`
		EditReason       string          `json:"edit_reason"`
		Payload          json.RawMessage `json:"payload"`
		IdempotencyKey   string          `json:"idempotency_key"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_agent_revision_proposal", err.Error())
		return
	}
	review, err := s.app.Harness.ProposeRevision(r.Context(), object.ObjectID, harness.ProposeRevisionInput{
		Payload: input.Payload, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason,
		TargetStatus: "active", SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		PluginID: object.Revision.PluginID, PluginVersion: object.Revision.PluginVersion, IdempotencyKey: input.IdempotencyKey,
		RequestedBy: "agent:" + principal.AgentID, Validation: json.RawMessage(`{"status":"not_run"}`),
	})
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "memory.propose", "harness_object_revision", object.ObjectID, object.ProjectID, "denied", map[string]any{"error": err.Error()})
		writeErr(w, http.StatusConflict, "agent_revision_proposal_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "memory.propose", "harness_object_revision", object.ObjectID, object.ProjectID, "allowed", map[string]any{"review_id": review.ReviewID, "revision": review.Revision, "base_revision": review.BaseRevision})
	writeJSON(w, http.StatusCreated, review)
}

func (s *Server) agentHarnessRuns(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionTraceRead)
	if !ok {
		return
	}
	projectID := r.URL.Query().Get("project_id")
	if !s.authorizeAgentProject(w, r, principal, projectID, "trace.read") {
		return
	}
	items, page, err := s.app.Harness.ListRunsPage(r.Context(), projectID, r.URL.Query().Get("status"), harnessQueryLimit(r), harnessQueryOffset(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "agent_runs_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "trace.read", "harness_run_collection", "", projectID, "allowed", map[string]any{"count": len(items), "offset": page.Offset, "total": page.Total})
	writeJSON(w, http.StatusOK, map[string]any{"runs": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "has_more": page.HasMore})
}

func (s *Server) agentHarnessRun(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, agentauth.PermissionTraceRead)
	if !ok {
		return
	}
	detail, err := s.app.Harness.RunDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "agent_run_not_found", "run not found")
		return
	}
	if !s.authorizeAgentProject(w, r, principal, detail.Run.ProjectID, "trace.read") {
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "trace.read", "harness_run", detail.Run.RunID, detail.Run.ProjectID, "allowed", map[string]any{"events": len(detail.Events)})
	writeJSON(w, http.StatusOK, detail)
}
