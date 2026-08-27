package server

import (
	"net/http"
	"time"

	"github.com/luoyif/memory-harness/internal/dashboard"
	"github.com/luoyif/memory-harness/internal/doctor"
	"github.com/luoyif/memory-harness/internal/harness"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	snapshot, err := dashboard.Build(r.Context(), s.app, time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "dashboard_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) today(w http.ResponseWriter, r *http.Request) {
	today, err := dashboard.ReadToday(r.Context(), s.app, time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "today_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, today)
}

func (s *Server) memory(w http.ResponseWriter, r *http.Request) {
	overview, err := s.app.Memory.Overview(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "memory_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	assetType := r.URL.Query().Get("type")
	assets, err := s.app.Memory.ListAssetsFiltered(r.Context(), r.URL.Query().Get("status"), assetType, 200)
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		assets, err = s.app.Memory.ListAssetsForProjectFiltered(r.Context(), projectID, r.URL.Query().Get("status"), assetType, 200)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "assets_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"assets":     assets,
		"total":      len(assets),
		"stage":      "protected_registry_active",
		"enabled":    true,
		"activation": "manual_review_only",
	})
}

func (s *Server) assetDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.app.Memory.AssetDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, "asset_failed", err)
		return
	}
	response := map[string]any{"asset": detail.Asset, "memories": detail.Memories}
	var objectID string
	err = s.app.Control.DB.QueryRowContext(r.Context(), `SELECT o.object_id FROM harness_objects o JOIN harness_object_revisions rev ON rev.object_id=o.object_id AND rev.revision=o.current_revision WHERE o.type_id IN (?,?,?) AND json_extract(rev.payload_json,'$.asset_id')=? ORDER BY CASE o.type_id WHEN ? THEN 0 WHEN ? THEN 1 ELSE 2 END,o.updated_at DESC LIMIT 1`, harness.GovernedAgentAssetTypeV4, harness.GovernedAgentAssetTypeV3, harness.GovernedAgentAssetTypeV2, detail.Asset.AssetID, harness.GovernedAgentAssetTypeV4, harness.GovernedAgentAssetTypeV3).Scan(&objectID)
	if err == nil {
		object, objectErr := s.app.Harness.Object(r.Context(), objectID)
		revisions, revisionErr := s.app.Harness.ListObjectRevisions(r.Context(), objectID, 100)
		reviews, reviewErr := s.app.Harness.ListRevisionReviews(r.Context(), objectID, "", 100)
		if objectErr == nil && revisionErr == nil && reviewErr == nil {
			response["governance"] = map[string]any{"object": object, "revisions": revisions, "reviews": reviews}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	sources, err := dashboard.ReadSources(r.Context(), s.app)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sources_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "total": len(sources)})
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := dashboard.ReadJobs(r.Context(), s.app)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "jobs_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) healthDetail(w http.ResponseWriter, r *http.Request) {
	report, err := doctor.Run(r.Context(), s.app)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "health_detail_failed", err.Error())
		return
	}
	totals, err := dashboard.ReadTotals(r.Context(), s.app)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "health_detail_failed", err.Error())
		return
	}
	search, err := dashboard.ReadSearch(r.Context(), s.app, totals.Evidence)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "health_detail_failed", err.Error())
		return
	}
	jobs, err := dashboard.ReadJobs(r.Context(), s.app)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "health_detail_failed", err.Error())
		return
	}
	status := "ok"
	if report.Status != "pass" || !search.Consistent {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         status,
		"version":        Version,
		"home":           s.app.Config.Home,
		"started_at":     s.startedAt.Format(time.RFC3339Nano),
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
		"doctor":         report,
		"search":         search,
		"jobs":           jobs,
	})
}

func (s *Server) resolveAssetClassification(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AssetType string `json:"asset_type"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_asset_classification", err.Error())
		return
	}
	asset, err := s.app.Memory.ResolveAssetClassification(r.Context(), r.PathValue("id"), input.AssetType, ownerActor(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "asset_classification_failed", err.Error())
		return
	}
	var projectID string
	if err := s.app.Control.DB.QueryRowContext(r.Context(), `SELECT project_id FROM record_projects WHERE record_type='asset' AND record_id=? ORDER BY is_primary DESC,created_at LIMIT 1`, asset.AssetID).Scan(&projectID); err != nil {
		writeStoreError(w, "asset_project_failed", err)
		return
	}
	object, err := s.app.Growth.EnsureGovernedAsset(r.Context(), projectID, asset)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "asset_governance_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"asset": asset, "governance_object": object})
}
