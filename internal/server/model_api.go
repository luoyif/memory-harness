package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/modelusage"
)

func (s *Server) modelConfig(w http.ResponseWriter, r *http.Request) {
	secretStore, secretPersistent := s.app.Models.SecretStoreInfo()
	providers, err := s.app.Models.ListProviders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "model_config_failed", err.Error())
		return
	}
	runtime, err := s.app.Models.Runtime(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "model_config_failed", err.Error())
		return
	}
	usage, err := modelusage.DashboardForWindow(r.Context(), s.app.Control.DB, 24)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "model_usage_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runtime":                  runtime,
		"providers":                providers,
		"usage":                    usage,
		"presets":                  modelconfig.Presets(),
		"model_catalog":            modelconfig.ModelCatalog(),
		"model_catalog_updated_at": modelconfig.ModelCatalogUpdatedAt,
		"model_catalog_notice":     "内置目录只帮助选择正确调用方式；提供商可能随时调整模型，实际可用列表以测试连接返回的 /models 为准。",
		"secret_store":             secretStore,
		"secret_persistent":        secretPersistent,
		"privacy_notice":           "Agent mode sends the selected Evidence session to the configured provider. Rules mode stays entirely local.",
	})
}

func (s *Server) createModelProvider(w http.ResponseWriter, r *http.Request) {
	var input modelconfig.ProviderInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_model_provider", err.Error())
		return
	}
	provider, err := s.app.Models.SaveProvider(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "model_provider_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, provider)
}

func (s *Server) updateModelProvider(w http.ResponseWriter, r *http.Request) {
	var input modelconfig.ProviderInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_model_provider", err.Error())
		return
	}
	input.ProviderID = r.PathValue("id")
	provider, err := s.app.Models.SaveProvider(r.Context(), input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "model_provider_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, provider)
}

func (s *Server) testModelProvider(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.Models.Probe(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "model_provider_test_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) updateModelRuntime(w http.ResponseWriter, r *http.Request) {
	var input modelconfig.RuntimeInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_model_runtime", err.Error())
		return
	}
	runtime, err := s.app.Models.SetRuntime(r.Context(), input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "model_runtime_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runtime)
}
