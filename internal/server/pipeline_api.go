package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/plugins"
)

func (s *Server) pipelines(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Pipelines.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pipelines_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipelines": items})
}

func (s *Server) pipelineStages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"stages": pipeline.StageCatalog()})
}

func (s *Server) validatePipeline(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PluginID   string              `json:"plugin_id"`
		Definition pipeline.Definition `json:"definition"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_pipeline", err.Error())
		return
	}
	result, err := s.app.Pipelines.ValidateStructured(input.PluginID, input.Definition)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) pipelineDrafts(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Pipelines.ListDrafts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "pipeline_drafts_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": items})
}

func (s *Server) savePipelineDraft(w http.ResponseWriter, r *http.Request) {
	var input pipeline.SaveDraftInput
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_pipeline_draft", err.Error())
		return
	}
	if strings.TrimSpace(r.PathValue("id")) != strings.TrimSpace(input.Definition.PipelineID) {
		writeErr(w, http.StatusBadRequest, "bad_pipeline_draft", "path pipeline id must match definition.pipeline_id")
		return
	}
	item, err := s.app.Pipelines.SaveDraft(r.Context(), input)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "revision conflict") {
			status = http.StatusConflict
		}
		writeErr(w, status, "pipeline_draft_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deletePipelineDraft(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Pipelines.DeleteDraft(r.Context(), r.PathValue("id")); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "pipeline_draft_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publishPipeline(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(r.URL.Query().Get("plugin_id"))
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		writeErr(w, http.StatusBadRequest, "bad_pipeline", "pipeline body exceeds 1 MiB")
		return
	}
	item, err := s.app.Pipelines.Publish(r.Context(), pluginID, raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_publish_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) executePipeline(w http.ResponseWriter, r *http.Request) {
	var input pipeline.ExecuteInput
	if err := decodeStrictJSON(r, 4<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_pipeline_execution", err.Error())
		return
	}
	principal, _ := ownerFromContext(r.Context())
	input.CallerType = "owner"
	input.CallerID = principal.SessionID
	input.Channel = "desktop"
	input.EffectiveCapabilities = pipeline.OwnerCapabilities()
	result, err := s.app.Pipelines.Execute(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_execution_failed", err.Error())
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) dryRunPipeline(w http.ResponseWriter, r *http.Request) {
	var input pipeline.ExecuteInput
	if err := decodeStrictJSON(r, 4<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_pipeline_dry_run", err.Error())
		return
	}
	principal, _ := ownerFromContext(r.Context())
	input.CallerType = "owner"
	input.CallerID = principal.SessionID
	input.Channel = "desktop-dry-run"
	input.EffectiveCapabilities = pipeline.OwnerCapabilities()
	result, err := s.app.Pipelines.DryRun(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_dry_run_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) pipelineReviews(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Pipelines.ListReviews(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("status"), harnessQueryLimit(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_reviews_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": items})
}

func (s *Server) decidePipelineReview(w http.ResponseWriter, r *http.Request) {
	var input pipeline.ReviewDecisionInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_review_decision", err.Error())
		return
	}
	principal, _ := ownerFromContext(r.Context())
	input.OwnerID = principal.SessionID
	result, err := s.app.Pipelines.DecideReview(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pipeline_review_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) cancelHarnessRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_run_cancel", err.Error())
		return
	}
	principal, _ := ownerFromContext(r.Context())
	item, err := s.app.Pipelines.Cancel(r.Context(), r.PathValue("id"), principal.SessionID, input.Reason)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "run_cancel_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) retryHarnessRun(w http.ResponseWriter, r *http.Request) {
	principal, _ := ownerFromContext(r.Context())
	result, err := s.app.Pipelines.Retry(r.Context(), r.PathValue("id"), principal.SessionID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "run_retry_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) forkHarnessRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NodeID string `json:"node_id"`
	}
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_run_fork", err.Error())
		return
	}
	if strings.TrimSpace(input.NodeID) == "" {
		writeErr(w, http.StatusBadRequest, "bad_run_fork", "node_id is required")
		return
	}
	principal, _ := ownerFromContext(r.Context())
	result, err := s.app.Pipelines.ForkFromNode(r.Context(), r.PathValue("id"), input.NodeID, principal.SessionID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "run_fork_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) plugins(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Plugins.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "plugins_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": items})
}

func (s *Server) installPlugin(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (32<<20)+1))
	if err != nil || len(raw) > 32<<20 {
		writeErr(w, http.StatusBadRequest, "bad_plugin", "plugin package exceeds 32 MiB")
		return
	}
	capabilities := []string{}
	for _, value := range strings.Split(r.URL.Query().Get("capabilities"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			capabilities = append(capabilities, value)
		}
	}
	item, err := s.app.Plugins.Install(r.Context(), raw, plugins.InstallOptions{
		DeveloperMode: r.URL.Query().Get("developer_mode") == "1",
		EnableProject: strings.TrimSpace(r.URL.Query().Get("project_id")),
		Capabilities:  capabilities,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_install_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) pluginImpact(w http.ResponseWriter, r *http.Request) {
	impact, err := s.app.Plugins.Impact(r.Context(), r.PathValue("id"), r.PathValue("version"), strings.TrimSpace(r.URL.Query().Get("project_id")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_impact_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) retirePlugin(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Plugins.Retire(r.Context(), r.PathValue("id"), r.PathValue("version"))
	if err != nil {
		writeErr(w, http.StatusConflict, "plugin_retire_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) setPluginProjectState(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status       string          `json:"status"`
		Capabilities []string        `json:"capabilities"`
		Config       json.RawMessage `json:"config"`
	}
	if err := decodeStrictJSON(r, 2<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_plugin_state", err.Error())
		return
	}
	item, err := s.app.Plugins.SetProjectState(r.Context(), r.PathValue("id"), r.PathValue("version"), r.PathValue("project"), input.Status, input.Capabilities, input.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_state_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) approvePluginSigner(w http.ResponseWriter, r *http.Request) {
	var input plugins.TrustSignerInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_plugin_signer", err.Error())
		return
	}
	item, err := s.app.Plugins.ApproveSigner(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_signer_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) revokePluginSigner(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Plugins.RevokeSigner(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, "plugin_signer_revoke_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
