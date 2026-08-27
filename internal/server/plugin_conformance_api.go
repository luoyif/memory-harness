package server

import (
	"net/http"
	"strings"
)

func (s *Server) pluginConformance(w http.ResponseWriter, r *http.Request) {
	report, err := s.app.Plugins.Conformance(
		r.Context(),
		strings.TrimSpace(r.PathValue("id")),
		strings.TrimSpace(r.PathValue("version")),
		strings.TrimSpace(r.URL.Query().Get("project_id")),
	)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_conformance_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
