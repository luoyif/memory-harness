package server

import (
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/agentauth"
)

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "profiles_failed", "project_id is required")
		return
	}
	items, err := s.app.Profiles.List(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "profiles_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": items, "total": len(items)})
}

func (s *Server) refreshProfiles(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	items, err := s.app.Profiles.RefreshProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "profile_refresh_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "refreshed", "profiles": items, "total": len(items)})
}

func (s *Server) setProfileLocks(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	viewKind := strings.TrimSpace(r.PathValue("view"))
	var request struct {
		BlockIDs []string `json:"block_ids"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_profile_locks", err.Error())
		return
	}
	object, err := s.app.Profiles.SetLockedBlocks(r.Context(), projectID, viewKind, request.BlockIDs)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "profile_lock_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "object": object})
}

func (s *Server) agentProfileView(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeAgent(w, r, "")
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	if !agentauth.HasPermission(principal, agentauth.PermissionProjectRead) && !agentauth.HasPermission(principal, agentauth.PermissionMemoryRead) {
		_ = s.app.Agents.Audit(r.Context(), principal, "profile.read", "profile", "", projectID, "denied", map[string]string{"reason": "permission_missing"})
		writeErr(w, http.StatusForbidden, "agent_forbidden", "Agent requires project.read or memory.read to read a compiled profile")
		return
	}
	if !s.authorizeAgentProject(w, r, principal, projectID, "profile.read") {
		return
	}
	view, err := s.app.Profiles.AgentView(r.Context(), principal, projectID)
	if err != nil {
		_ = s.app.Agents.Audit(r.Context(), principal, "profile.read", "profile", "", projectID, "denied", map[string]string{"reason": err.Error()})
		writeErr(w, http.StatusBadRequest, "agent_profile_failed", err.Error())
		return
	}
	_ = s.app.Agents.Audit(r.Context(), principal, "profile.read", "profile", "", projectID, "allowed", map[string]any{"blocks": len(view.Blocks), "source_profiles": len(view.SourceProjectionIDs)})
	writeJSON(w, http.StatusOK, view)
}
