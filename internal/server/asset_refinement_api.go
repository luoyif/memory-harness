package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/assettemplate"
	"github.com/luoyif/memory-harness/internal/dashboard"
	"github.com/luoyif/memory-harness/internal/growth"
)

func (s *Server) assetTemplates(w http.ResponseWriter, r *http.Request) {
	templates := assettemplate.Templates()
	writeJSON(w, http.StatusOK, map[string]any{
		"template_version": assettemplate.TemplateVersion,
		"schema_version":   assettemplate.SchemaVersion,
		"templates":        templates,
		"total":            len(templates),
	})
}

func (s *Server) refineProjectAssets(w http.ResponseWriter, r *http.Request) {
	var input growth.AssetRefinementInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_asset_refinement", err.Error())
		return
	}
	input.ProjectID = strings.TrimSpace(r.PathValue("id"))
	input.RequestedBy = ownerActor(r)
	result, err := s.app.Growth.RefineAssets(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "asset_refinement_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) projectActivityCalendar(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	calendar, err := dashboard.ReadProjectActivity(r.Context(), s.app, strings.TrimSpace(r.PathValue("id")), time.Now(), days)
	if err != nil {
		writeStoreError(w, "activity_calendar_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, calendar)
}
