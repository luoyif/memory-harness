package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/blueprint"
)

func (s *Server) blueprints(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Blueprints.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "blueprints_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blueprints": items})
}

func (s *Server) validateBlueprint(w http.ResponseWriter, r *http.Request) {
	var definition blueprint.Definition
	if err := decodeStrictJSON(r, 4<<20, &definition); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_blueprint", err.Error())
		return
	}
	result := blueprint.Validate(definition)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, result)
}

func (s *Server) publishBlueprint(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PluginID   string               `json:"plugin_id"`
		Definition blueprint.Definition `json:"definition"`
	}
	if err := decodeStrictJSON(r, 4<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_blueprint", err.Error())
		return
	}
	if strings.TrimSpace(input.PluginID) == "" {
		input.PluginID = "builtin.user-workflows"
	}
	item, err := s.app.Blueprints.Publish(r.Context(), input.PluginID, input.Definition)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "immutable") {
			status = http.StatusConflict
		}
		writeErr(w, status, "blueprint_publish_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) currentBlueprint(w http.ResponseWriter, r *http.Request) {
	item, err := s.app.Blueprints.Current(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "project_blueprint_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) activateBlueprint(w http.ResponseWriter, r *http.Request) {
	var input blueprint.ActivateInput
	if err := decodeStrictJSON(r, 1<<20, &input); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_blueprint_assignment", err.Error())
		return
	}
	principal, _ := ownerFromContext(r.Context())
	item, err := s.app.Blueprints.Activate(r.Context(), r.PathValue("id"), input.BlueprintID, input.Version, principal.SessionID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "blueprint_activation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}
