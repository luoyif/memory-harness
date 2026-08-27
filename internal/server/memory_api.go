package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
)

func queryLimit(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value <= 0 {
		return 100
	}
	return value
}

func queryOffset(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if value < 0 {
		return 0
	}
	return value
}

func writeStoreError(w http.ResponseWriter, code string, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "record not found")
		return
	}
	writeErr(w, http.StatusInternalServerError, code, err.Error())
}

func (s *Server) projectAutoProcesses(ctx context.Context, projectID string) (bool, string, error) {
	setting, err := s.app.Portfolio.ProjectAutomation(ctx, projectID)
	if err != nil {
		return false, "", err
	}
	return setting.ImportMode == "auto_new", setting.ImportMode, nil
}

func (s *Server) layers(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	overview, err := s.app.Memory.Overview(r.Context())
	if projectID != "" {
		overview, err = s.app.Memory.OverviewForProject(r.Context(), projectID)
	}
	if err != nil {
		writeStoreError(w, "layers_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) episodes(w http.ResponseWriter, r *http.Request) {
	limit := queryLimit(r)
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID != "" {
		items, total, err := s.app.Memory.ListEpisodesForProject(r.Context(), projectID, limit, queryOffset(r))
		if err != nil {
			writeStoreError(w, "episodes_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"episodes": items, "total": total, "offset": queryOffset(r)})
		return
	}
	items, err := s.app.Memory.ListEpisodes(r.Context(), limit)
	if err != nil {
		writeStoreError(w, "episodes_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": items, "total": len(items)})
}

func (s *Server) episode(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := s.app.Memory.Episode(r.Context(), id)
	if err != nil {
		writeStoreError(w, "episode_failed", err)
		return
	}
	units, err := s.app.Memory.ListKnowledgeUnits(r.Context(), id, "", 500)
	if err != nil {
		writeStoreError(w, "episode_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episode": item, "knowledge_units": units})
}

func (s *Server) sessionParticipants(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Memory.SessionParticipants(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeStoreError(w, "session_participants_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"participants": items})
}

func (s *Server) replaceSessionParticipants(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Participants []memory.ParticipantIdentity `json:"participants"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_session_participants", err.Error())
		return
	}
	items, err := s.app.Memory.ReplaceSessionParticipants(r.Context(), strings.TrimSpace(r.PathValue("id")), request.Participants)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		writeErr(w, http.StatusBadRequest, "bad_session_participants", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"participants": items, "requires_reprocess": true})
}

func (s *Server) knowledgeUnits(w http.ResponseWriter, r *http.Request) {
	limit := queryLimit(r)
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID != "" {
		items, total, err := s.app.Memory.ListKnowledgeUnitsForProject(r.Context(), projectID, r.URL.Query().Get("type"), limit, queryOffset(r))
		if err != nil {
			writeStoreError(w, "knowledge_units_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"knowledge_units": items, "total": total, "offset": queryOffset(r)})
		return
	}
	items, err := s.app.Memory.ListKnowledgeUnits(r.Context(), r.URL.Query().Get("episode_id"), r.URL.Query().Get("type"), limit)
	if err != nil {
		writeStoreError(w, "knowledge_units_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"knowledge_units": items, "total": len(items)})
}

func (s *Server) knowledgeUnitProject(ctx context.Context, unitID, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if _, err := s.app.Memory.KnowledgeUnitForProject(ctx, explicit, unitID); err != nil {
			return "", err
		}
		return explicit, nil
	}
	return s.app.Portfolio.ProjectForRecord(ctx, "knowledge_unit", unitID)
}

func (s *Server) knowledgeUnit(w http.ResponseWriter, r *http.Request) {
	unitID := strings.TrimSpace(r.PathValue("id"))
	projectID, err := s.knowledgeUnitProject(r.Context(), unitID, r.URL.Query().Get("project_id"))
	if err != nil {
		writeStoreError(w, "knowledge_unit_failed", err)
		return
	}
	unit, err := s.app.Memory.KnowledgeUnitForProject(r.Context(), projectID, unitID)
	if err != nil {
		writeStoreError(w, "knowledge_unit_failed", err)
		return
	}
	objectID := memory.StructuredKnowledgeObjectID(projectID, unitID)
	var governance any
	object, err := s.app.Harness.Object(r.Context(), objectID)
	if err == nil {
		governance = object
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeStoreError(w, "knowledge_unit_governance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_unit": unit, "project_id": projectID, "object_id": objectID,
		"governance": governance, "legacy_projection_only": governance == nil,
	})
}

func (s *Server) proposeKnowledgeUnitRevision(w http.ResponseWriter, r *http.Request) {
	unitID := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ProjectID        string               `json:"project_id"`
		ExpectedRevision int                  `json:"expected_revision"`
		EditReason       string               `json:"edit_reason"`
		IdempotencyKey   string               `json:"idempotency_key"`
		KnowledgeUnit    memory.KnowledgeUnit `json:"knowledge_unit"`
	}
	if err := decodeStrictJSON(r, 2<<20, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_knowledge_revision", err.Error())
		return
	}
	if strings.TrimSpace(request.EditReason) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		writeErr(w, http.StatusBadRequest, "bad_knowledge_revision", "edit_reason and idempotency_key are required")
		return
	}
	projectID, err := s.knowledgeUnitProject(r.Context(), unitID, request.ProjectID)
	if err != nil {
		writeStoreError(w, "knowledge_revision_failed", err)
		return
	}
	base, err := s.app.Memory.KnowledgeUnitForProject(r.Context(), projectID, unitID)
	if err != nil {
		writeStoreError(w, "knowledge_revision_failed", err)
		return
	}
	objectID := memory.StructuredKnowledgeObjectID(projectID, unitID)
	object, objectErr := s.app.Harness.Object(r.Context(), objectID)
	if errors.Is(objectErr, sql.ErrNoRows) {
		if request.ExpectedRevision != 0 {
			writeErr(w, http.StatusConflict, "knowledge_revision_conflict", "legacy knowledge unit has no governed revision yet; expected_revision must be 0 for the first proposal")
			return
		}
		baseRaw, err := memory.StructuredKnowledgePayload(base)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "knowledge_revision_failed", err.Error())
			return
		}
		validFrom := base.Structure.Temporal.ValidFrom
		if validFrom == "" {
			validFrom = base.ObservedAt
		}
		object, err = s.app.Harness.Materialize(r.Context(), harness.MaterializeInput{
			ObjectID: objectID, TypeID: memory.StructuredKnowledgeUnitTypeV2, ProjectID: projectID, Status: "active", Payload: baseRaw,
			Confidence: base.Confidence, Importance: base.Structure.Epistemic.Importance, ValidFrom: validFrom, ValidUntil: base.Structure.Temporal.ValidUntil,
			SourceEvidenceIDs: []string{base.EvidenceID}, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0",
			IdempotencyKey: projectID + ":" + unitID + ":owner-bootstrap-v2",
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "knowledge_revision_failed", err.Error())
			return
		}
		request.ExpectedRevision = object.CurrentRevision
	} else if objectErr != nil {
		writeStoreError(w, "knowledge_revision_failed", objectErr)
		return
	} else if request.ExpectedRevision <= 0 {
		writeErr(w, http.StatusConflict, "knowledge_revision_conflict", "expected_revision is required once a governed knowledge object exists")
		return
	}

	candidate, err := memory.PrepareStructuredKnowledgeRevision(base, request.KnowledgeUnit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "knowledge_revision_failed", err.Error())
		return
	}
	payload, err := memory.StructuredKnowledgePayload(candidate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "knowledge_revision_failed", err.Error())
		return
	}
	validFrom := candidate.Structure.Temporal.ValidFrom
	if validFrom == "" {
		validFrom = candidate.ObservedAt
	}
	review, err := s.app.Harness.ProposeRevision(r.Context(), objectID, harness.ProposeRevisionInput{
		ExpectedRevision: request.ExpectedRevision, TargetStatus: "active", Payload: payload,
		Confidence: candidate.Confidence, Importance: candidate.Structure.Epistemic.Importance,
		ValidFrom: validFrom, ValidUntil: candidate.Structure.Temporal.ValidUntil,
		SourceEvidenceIDs: []string{candidate.EvidenceID}, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0",
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), RequestedBy: ownerActor(r), EditReason: strings.TrimSpace(request.EditReason),
		Validation: json.RawMessage(`{"status":"passed","kind":"owner_semantic_correction","evidence_immutable":true}`),
	})
	if err != nil {
		writeErr(w, http.StatusConflict, "knowledge_revision_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"review": review, "base_object": object, "project_id": projectID})
}

func (s *Server) correctionImpact(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	targetID := strings.TrimSpace(r.URL.Query().Get("id"))
	impact, err := s.app.Memory.PreviewCorrection(r.Context(), projectID, kind, targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "correction_target_not_found", "correction target not found")
			return
		}
		writeErr(w, http.StatusBadRequest, "correction_impact_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) memories(w http.ResponseWriter, r *http.Request) {
	limit := queryLimit(r)
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID != "" {
		items, total, err := s.app.Memory.ListMemoriesForProject(r.Context(), projectID, r.URL.Query().Get("tier"), r.URL.Query().Get("status"), limit, queryOffset(r))
		if err != nil {
			writeStoreError(w, "memories_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"memories": items, "total": total, "offset": queryOffset(r)})
		return
	}
	items, err := s.app.Memory.ListMemories(r.Context(), r.URL.Query().Get("tier"), r.URL.Query().Get("status"), limit)
	if err != nil {
		writeStoreError(w, "memories_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": items, "total": len(items)})
}

func (s *Server) memoryPins(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "bad_memory_pins", "project_id is required")
		return
	}
	items, err := s.app.Memory.ListPinnedMemories(r.Context(), projectID, queryLimit(r))
	if err != nil {
		writeStoreError(w, "memory_pins_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": items, "total": len(items)})
}

func (s *Server) setMemoryPin(w http.ResponseWriter, r *http.Request) {
	memoryID := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ProjectID string `json:"project_id"`
		Pinned    bool   `json:"pinned"`
	}
	if err := decodeStrictJSON(r, 64<<10, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_memory_pin", err.Error())
		return
	}
	projectID, err := s.memoryRecordProject(r.Context(), memoryID, request.ProjectID)
	if err != nil {
		writeStoreError(w, "memory_pin_failed", err)
		return
	}
	pinnedAt, err := s.app.Memory.SetMemoryPinned(r.Context(), projectID, memoryID, request.Pinned)
	if err != nil {
		writeStoreError(w, "memory_pin_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID, "memory_id": memoryID, "pinned": request.Pinned, "pinned_at": pinnedAt,
	})
}

func (s *Server) memoryRecordProject(ctx context.Context, memoryID, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if _, err := s.app.Memory.MemoryForProject(ctx, explicit, memoryID); err != nil {
			return "", err
		}
		return explicit, nil
	}
	return s.app.Portfolio.ProjectForRecord(ctx, "memory", memoryID)
}

func (s *Server) memoryRecord(w http.ResponseWriter, r *http.Request) {
	memoryID := strings.TrimSpace(r.PathValue("id"))
	projectID, err := s.memoryRecordProject(r.Context(), memoryID, r.URL.Query().Get("project_id"))
	if err != nil {
		writeStoreError(w, "memory_failed", err)
		return
	}
	item, err := s.app.Memory.MemoryForProject(r.Context(), projectID, memoryID)
	if err != nil {
		writeStoreError(w, "memory_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) memoryGovernance(w http.ResponseWriter, r *http.Request) {
	memoryID := strings.TrimSpace(r.PathValue("id"))
	projectID, err := s.memoryRecordProject(r.Context(), memoryID, r.URL.Query().Get("project_id"))
	if err != nil {
		writeStoreError(w, "memory_governance_failed", err)
		return
	}
	item, err := s.app.Memory.MemoryForProject(r.Context(), projectID, memoryID)
	if err != nil {
		writeStoreError(w, "memory_governance_failed", err)
		return
	}
	objectID := memory.StructuredMemoryObjectID(projectID, memoryID)
	var governance any
	object, err := s.app.Harness.Object(r.Context(), objectID)
	if err == nil {
		governance = object
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeStoreError(w, "memory_governance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory": item, "project_id": projectID, "object_id": objectID,
		"governance": governance, "legacy_projection_only": governance == nil,
	})
}

func (s *Server) proposeMemoryRevision(w http.ResponseWriter, r *http.Request) {
	memoryID := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ProjectID        string              `json:"project_id"`
		ExpectedRevision int                 `json:"expected_revision"`
		EditReason       string              `json:"edit_reason"`
		IdempotencyKey   string              `json:"idempotency_key"`
		Memory           memory.MemoryRecord `json:"memory"`
	}
	if err := decodeStrictJSON(r, 2<<20, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_memory_revision", err.Error())
		return
	}
	if strings.TrimSpace(request.EditReason) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		writeErr(w, http.StatusBadRequest, "bad_memory_revision", "edit_reason and idempotency_key are required")
		return
	}
	projectID, err := s.memoryRecordProject(r.Context(), memoryID, request.ProjectID)
	if err != nil {
		writeStoreError(w, "memory_revision_failed", err)
		return
	}
	base, err := s.app.Memory.MemoryForProject(r.Context(), projectID, memoryID)
	if err != nil {
		writeStoreError(w, "memory_revision_failed", err)
		return
	}
	objectID := memory.StructuredMemoryObjectID(projectID, memoryID)
	object, objectErr := s.app.Harness.Object(r.Context(), objectID)
	if errors.Is(objectErr, sql.ErrNoRows) {
		if request.ExpectedRevision != 0 {
			writeErr(w, http.StatusConflict, "memory_revision_conflict", "legacy memory has no governed revision yet; expected_revision must be 0 for the first proposal")
			return
		}
		baseRaw, payloadErr := memory.StructuredMemoryPayload(base)
		if payloadErr != nil {
			writeErr(w, http.StatusBadRequest, "memory_revision_failed", payloadErr.Error())
			return
		}
		object, err = s.app.Harness.Materialize(r.Context(), harness.MaterializeInput{
			ObjectID: objectID, TypeID: memory.StructuredMemoryRecordTypeV1, ProjectID: projectID, Status: "active", Payload: baseRaw,
			Confidence: base.Confidence, Importance: base.Importance, ValidFrom: base.ObservedAt,
			SourceEvidenceIDs: base.EvidenceIDs, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0",
			IdempotencyKey: projectID + ":" + memoryID + ":owner-bootstrap-memory-v1",
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "memory_revision_failed", err.Error())
			return
		}
		request.ExpectedRevision = object.CurrentRevision
	} else if objectErr != nil {
		writeStoreError(w, "memory_revision_failed", objectErr)
		return
	} else if request.ExpectedRevision <= 0 {
		writeErr(w, http.StatusConflict, "memory_revision_conflict", "expected_revision is required once a governed memory object exists")
		return
	}

	candidate, err := memory.PrepareStructuredMemoryRevision(base, request.Memory)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "memory_revision_failed", err.Error())
		return
	}
	payload, err := memory.StructuredMemoryPayload(candidate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "memory_revision_failed", err.Error())
		return
	}
	review, err := s.app.Harness.ProposeRevision(r.Context(), objectID, harness.ProposeRevisionInput{
		ExpectedRevision: request.ExpectedRevision, TargetStatus: "active", Payload: payload,
		Confidence: candidate.Confidence, Importance: candidate.Importance, ValidFrom: candidate.ObservedAt,
		SourceEvidenceIDs: candidate.EvidenceIDs, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0",
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), RequestedBy: ownerActor(r), EditReason: strings.TrimSpace(request.EditReason),
		Validation: json.RawMessage(`{"status":"passed","kind":"owner_memory_correction","evidence_immutable":true}`),
	})
	if err != nil {
		writeErr(w, http.StatusConflict, "memory_revision_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"review": review, "base_object": object, "project_id": projectID})
}

func (s *Server) memoryTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := s.app.Memory.Trace(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeStoreError(w, "trace_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) operations(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Memory.ListOperations(r.Context(), r.URL.Query().Get("status"), queryLimit(r))
	if err != nil {
		writeStoreError(w, "operations_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": items, "total": len(items)})
}

func (s *Server) operationDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.app.Memory.ReviewDetail(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeStoreError(w, "operation_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) reviewOperation(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Decision string `json:"decision"`
		Reviewer string `json:"reviewer"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_review", err.Error())
		return
	}
	reviewer := request.Reviewer
	if principal, ok := ownerFromContext(r.Context()); ok {
		reviewer = principal.SessionID
	}
	operation, err := s.app.Memory.ReviewOperation(r.Context(), strings.TrimSpace(r.PathValue("id")), request.Decision, reviewer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeStoreError(w, "review_failed", err)
		} else {
			writeErr(w, http.StatusConflict, "review_failed", err.Error())
		}
		return
	}
	projectID := ""
	if operation.TargetMemoryID != "" {
		projectID, _ = s.app.Portfolio.ProjectForRecord(r.Context(), "memory", operation.TargetMemoryID)
	} else if operation.UnitID != "" {
		projectID, _ = s.app.Portfolio.ProjectForRecord(r.Context(), "knowledge_unit", operation.UnitID)
	}
	if projectID != "" {
		if _, err := s.app.Memory.RebuildLivingViewsForProject(r.Context(), projectID); err != nil {
			writeStoreError(w, "review_living_failed", err)
			return
		}
		if _, err := s.app.Portfolio.RebuildProjectIndex(r.Context(), projectID); err != nil {
			writeStoreError(w, "review_index_failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, operation)
}

func (s *Server) livingViews(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Memory.ListLivingViews(r.Context())
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		items, err = s.app.Memory.ListLivingViewsForProject(r.Context(), projectID)
	}
	if err != nil {
		writeStoreError(w, "living_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": items, "total": len(items)})
}

func (s *Server) livingDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.app.Memory.LivingDetail(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeStoreError(w, "living_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	graph, err := s.app.Memory.Graph(r.Context(), queryLimit(r))
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		graph, err = s.app.Memory.GraphForProject(r.Context(), projectID, queryLimit(r))
	}
	if err != nil {
		writeStoreError(w, "graph_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (s *Server) semanticGraph(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "semantic_graph_failed", "project_id is required")
		return
	}
	graph, err := s.app.Memory.SemanticGraph(r.Context(), projectID, queryLimit(r))
	if err != nil {
		writeStoreError(w, "semantic_graph_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

type processSource struct {
	SessionID       string `json:"session_id"`
	ProjectID       string `json:"project_id"`
	Title           string `json:"title"`
	SourceSystem    string `json:"source_system"`
	EvidenceCount   int    `json:"evidence_count"`
	ImportedAt      string `json:"imported_at"`
	LastObservedAt  string `json:"last_observed_at"`
	LastProcessedAt string `json:"last_processed_at,omitempty"`
	Compiler        string `json:"compiler,omitempty"`
	QualityStatus   string `json:"quality_status,omitempty"`
	Status          string `json:"status"`
	StatusDetail    string `json:"status_detail"`
}

func sameEvidenceIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sourceTitle(value, sessionID string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return sessionID
	}
	runes := []rune(value)
	if len(runes) > 72 {
		value = string(runes[:71]) + "…"
	}
	return value
}

func (s *Server) processSourceForSession(ctx context.Context, projectID, sessionID string) (processSource, []string, error) {
	item := processSource{SessionID: sessionID, ProjectID: projectID, Status: "pending", StatusDetail: "尚未整理"}
	rows, err := s.app.SearchStore.DB.QueryContext(ctx, `SELECT evidence_id,source_system,captured_at,observed_at,body FROM turns WHERE session_id=? ORDER BY ordinal,id`, sessionID)
	if err != nil {
		return item, nil, err
	}
	evidenceIDs := []string{}
	for rows.Next() {
		var evidenceID, sourceSystem, capturedAt, observedAt, body string
		if err := rows.Scan(&evidenceID, &sourceSystem, &capturedAt, &observedAt, &body); err != nil {
			rows.Close()
			return item, nil, err
		}
		if len(evidenceIDs) == 0 {
			item.SourceSystem = sourceSystem
			item.ImportedAt = capturedAt
			item.Title = sourceTitle(body, sessionID)
		}
		item.LastObservedAt = observedAt
		evidenceIDs = append(evidenceIDs, evidenceID)
	}
	if err := rows.Close(); err != nil {
		return item, nil, err
	}
	if len(evidenceIDs) == 0 {
		return item, nil, sql.ErrNoRows
	}
	item.EvidenceCount = len(evidenceIDs)

	var episodeTitle, episodeEvidenceRaw, compiler, episodeUpdated string
	episodeErr := s.app.Control.DB.QueryRowContext(ctx, `SELECT title,evidence_ids_json,compiler,updated_at FROM episodes WHERE session_id=?`, sessionID).
		Scan(&episodeTitle, &episodeEvidenceRaw, &compiler, &episodeUpdated)
	var jobStatus, jobUpdated string
	jobErr := s.app.Control.DB.QueryRowContext(ctx, `SELECT status,updated_at FROM jobs WHERE json_extract(payload_json,'$.session_id')=? ORDER BY updated_at DESC LIMIT 1`, sessionID).Scan(&jobStatus, &jobUpdated)
	if jobErr != nil && jobErr != sql.ErrNoRows {
		return item, nil, jobErr
	}
	if jobStatus == "running" {
		item.Status = "processing"
		item.StatusDetail = "正在整理"
	}
	if episodeErr == sql.ErrNoRows {
		if jobStatus == "failed" {
			item.Status = "failed"
			item.StatusDetail = "上次整理失败，可以重试"
		}
		return item, evidenceIDs, nil
	}
	if episodeErr != nil {
		return item, nil, episodeErr
	}
	if strings.TrimSpace(episodeTitle) != "" {
		item.Title = episodeTitle
	}
	item.Compiler = compiler
	item.LastProcessedAt = episodeUpdated
	item.QualityStatus = "high_quality"
	if strings.Contains(compiler, "fallback") || strings.Contains(compiler, "partial-") {
		item.QualityStatus = "degraded"
	}
	var episodeEvidence []string
	_ = json.Unmarshal([]byte(episodeEvidenceRaw), &episodeEvidence)
	if !sameEvidenceIDs(episodeEvidence, evidenceIDs) {
		item.Status = "changed"
		item.StatusDetail = "原材料有新增内容，需要更新"
		return item, evidenceIDs, nil
	}
	if jobStatus == "failed" && jobUpdated > episodeUpdated {
		item.Status = "failed"
		item.StatusDetail = "上次更新失败，旧结果仍保留"
		return item, evidenceIDs, nil
	}
	if item.Status != "processing" {
		item.Status = "completed"
		item.StatusDetail = "已整理，内容没有变化"
	}
	return item, evidenceIDs, nil
}

func (s *Server) processSources(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "bad_process_sources", "project_id is required")
		return
	}
	rows, err := s.app.SearchStore.DB.QueryContext(r.Context(), `SELECT DISTINCT session_id FROM turns ORDER BY session_id`)
	if err != nil {
		writeStoreError(w, "process_sources_failed", err)
		return
	}
	sessions := []string{}
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			writeStoreError(w, "process_sources_failed", err)
			return
		}
		sessions = append(sessions, sessionID)
	}
	if err := rows.Close(); err != nil {
		writeStoreError(w, "process_sources_failed", err)
		return
	}
	items := []processSource{}
	for _, sessionID := range sessions {
		routedProject, _, err := s.sessionRoute(r.Context(), sessionID)
		if err != nil {
			writeStoreError(w, "process_sources_failed", err)
			return
		}
		if routedProject != projectID {
			continue
		}
		item, _, err := s.processSourceForSession(r.Context(), projectID, sessionID)
		if err != nil {
			writeStoreError(w, "process_sources_failed", err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": items, "total": len(items)})
}

func (s *Server) processMemory(w http.ResponseWriter, r *http.Request) {
	// A real memory-growth pass can legitimately outlive the server's short
	// default write deadline because model-backed compilation has its own
	// bounded pipeline timeout. Keep the authenticated request open so the UI
	// receives the final result instead of reporting a false network failure
	// while the pipeline continues in the background.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	var request struct {
		SessionID  string   `json:"session_id"`
		SessionIDs []string `json:"session_ids"`
		ProjectID  string   `json:"project_id"`
		Mode       string   `json:"mode"`
		Force      bool     `json:"force"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_process", err.Error())
		return
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_process", err.Error())
			return
		}
	}
	if strings.TrimSpace(request.SessionID) != "" {
		projectID, evidenceIDs, err := s.sessionRoute(r.Context(), request.SessionID)
		if err != nil {
			writeStoreError(w, "process_route_failed", err)
			return
		}
		result, err := s.growSession(r.Context(), projectID, request.SessionID, evidenceIDs, true, request.Force)
		if err != nil {
			writeStoreError(w, "process_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{result}, "total": 1})
		return
	}
	if len(request.SessionIDs) > 0 {
		request.ProjectID = strings.TrimSpace(request.ProjectID)
		if request.ProjectID == "" {
			writeErr(w, http.StatusBadRequest, "bad_process", "project_id is required with session_ids")
			return
		}
		mode := strings.TrimSpace(request.Mode)
		if mode == "" {
			mode = "incremental"
		}
		if mode != "incremental" && mode != "force" {
			writeErr(w, http.StatusBadRequest, "bad_process", "mode must be incremental or force")
			return
		}
		if len(request.SessionIDs) > 200 {
			writeErr(w, http.StatusBadRequest, "bad_process", "session_ids cannot exceed 200 items")
			return
		}
		type selectedSource struct {
			sessionID   string
			evidenceIDs []string
			status      string
		}
		selected := []selectedSource{}
		seen := map[string]bool{}
		for _, value := range request.SessionIDs {
			sessionID := strings.TrimSpace(value)
			if sessionID == "" || seen[sessionID] {
				continue
			}
			seen[sessionID] = true
			routedProject, evidenceIDs, err := s.sessionRoute(r.Context(), sessionID)
			if err != nil {
				writeStoreError(w, "process_route_failed", err)
				return
			}
			if routedProject != request.ProjectID {
				writeErr(w, http.StatusConflict, "process_scope_conflict", "every selected source must belong to the requested project")
				return
			}
			source, _, err := s.processSourceForSession(r.Context(), request.ProjectID, sessionID)
			if err != nil {
				writeStoreError(w, "process_sources_failed", err)
				return
			}
			selected = append(selected, selectedSource{sessionID: sessionID, evidenceIDs: evidenceIDs, status: source.Status})
		}
		if len(selected) == 0 {
			writeErr(w, http.StatusBadRequest, "bad_process", "session_ids must contain at least one source")
			return
		}
		results := []memory.RunResult{}
		items := []map[string]any{}
		succeeded, failed, skipped := 0, 0, 0
		for _, source := range selected {
			if mode == "incremental" && source.status == "completed" {
				skipped++
				items = append(items, map[string]any{"session_id": source.sessionID, "status": "skipped", "detail": "already completed and unchanged"})
				continue
			}
			result, err := s.growSession(r.Context(), request.ProjectID, source.sessionID, source.evidenceIDs, true, mode == "force")
			if err != nil {
				failed++
				items = append(items, map[string]any{"session_id": source.sessionID, "status": "failed", "error": err.Error()})
				continue
			}
			succeeded++
			results = append(results, result)
			items = append(items, map[string]any{"session_id": source.sessionID, "status": "completed", "result": result})
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results, "items": items, "total": len(selected), "succeeded": succeeded, "failed": failed, "skipped": skipped, "mode": mode})
		return
	}
	rows, err := s.app.SearchStore.DB.QueryContext(r.Context(), `SELECT DISTINCT session_id FROM turns ORDER BY session_id`)
	if err != nil {
		writeStoreError(w, "process_failed", err)
		return
	}
	sessions := []string{}
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			writeStoreError(w, "process_failed", err)
			return
		}
		sessions = append(sessions, sessionID)
	}
	if err := rows.Close(); err != nil {
		writeStoreError(w, "process_failed", err)
		return
	}
	results := make([]memory.RunResult, 0, len(sessions))
	for _, sessionID := range sessions {
		projectID, evidenceIDs, err := s.sessionRoute(r.Context(), sessionID)
		if err != nil {
			writeStoreError(w, "process_route_failed", err)
			return
		}
		if requested := strings.TrimSpace(request.ProjectID); requested != "" && requested != projectID {
			continue
		}
		result, err := s.growSession(r.Context(), projectID, sessionID, evidenceIDs, true, request.Force)
		if err != nil {
			writeStoreError(w, "process_failed", err)
			return
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(results)})
}

type batchEvidenceRequest struct {
	Items []json.RawMessage `json:"items"`
}

func (s *Server) captureBatch(w http.ResponseWriter, r *http.Request) {
	var request batchEvidenceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_batch", err.Error())
		return
	}
	if len(request.Items) == 0 || len(request.Items) > 500 {
		writeErr(w, http.StatusBadRequest, "bad_batch", "items must contain between 1 and 500 Evidence envelopes")
		return
	}
	// Validate the whole batch before the first durable append so a malformed
	// later item cannot leave an unexpected partial import.
	for i, raw := range request.Items {
		if _, _, err := contracts.ParseEvidence(raw); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_batch", "item "+strconv.Itoa(i)+": "+err.Error())
			return
		}
	}
	results := make([]any, 0, len(request.Items))
	sessions := []string{}
	sessionEvidence := map[string][]string{}
	sessionProjects := map[string][]string{}
	for i, raw := range request.Items {
		projectID, err := s.projectForEvidence(r.Context(), raw, "")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "batch_route_failed", "item "+strconv.Itoa(i)+": "+err.Error())
			return
		}
		result, err := s.app.Ledger.Append(r.Context(), raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "batch_capture_failed", "item "+strconv.Itoa(i)+": "+err.Error())
			return
		}
		results = append(results, result)
		sessions = append(sessions, result.SessionID)
		sessionEvidence[result.SessionID] = append(sessionEvidence[result.SessionID], result.EvidenceID)
		foundProject := false
		for _, existing := range sessionProjects[result.SessionID] {
			if existing == projectID {
				foundProject = true
			}
		}
		if !foundProject {
			sessionProjects[result.SessionID] = append(sessionProjects[result.SessionID], projectID)
		}
		if err := s.app.Portfolio.LinkRecord(r.Context(), "evidence", result.EvidenceID, projectID, len(sessionProjects[result.SessionID]) == 1); err != nil {
			writeErr(w, http.StatusInternalServerError, "batch_route_failed", err.Error())
			return
		}
	}
	processed := []any{}
	processingModes := map[string]string{}
	seen := map[string]bool{}
	for _, sessionID := range sessions {
		if seen[sessionID] {
			continue
		}
		seen[sessionID] = true
		for i, projectID := range sessionProjects[sessionID] {
			auto, mode, err := s.projectAutoProcesses(r.Context(), projectID)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "batch_route_failed", err.Error())
				return
			}
			processingModes[projectID] = mode
			if !auto {
				continue
			}
			result, err := s.growSession(r.Context(), projectID, sessionID, sessionEvidence[sessionID], i == 0, false)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "batch_route_failed", err.Error())
				return
			}
			if i == 0 {
				processed = append(processed, result)
			}
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"captured": results, "processed": processed, "total": len(results), "processing_modes": processingModes})
}

type textDocument struct {
	Title      string `json:"title"`
	Text       string `json:"text"`
	ObservedAt string `json:"observed_at,omitempty"`
}

func (s *Server) importText(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SourceSystem   string         `json:"source_system"`
		SessionID      string         `json:"session_id"`
		ProjectID      string         `json:"project_id"`
		ConnectorID    string         `json:"connector_id"`
		IdempotencyKey string         `json:"idempotency_key"`
		ScopeHints     []string       `json:"scope_hints"`
		Documents      []textDocument `json:"documents"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_import", err.Error())
		return
	}
	if len(request.Documents) == 0 || len(request.Documents) > 100 {
		writeErr(w, http.StatusBadRequest, "bad_import", "documents must contain between 1 and 100 text or Markdown documents")
		return
	}
	if strings.TrimSpace(request.SourceSystem) == "" {
		request.SourceSystem = "manual-import"
	}
	if strings.TrimSpace(request.SessionID) == "" {
		seed := request.SourceSystem
		for _, document := range request.Documents {
			seed += "\x00" + document.Title + "\x00" + document.Text
		}
		request.SessionID = "import-" + contracts.HashBytes([]byte(seed))[:20]
	}
	project, exact, err := s.app.Portfolio.Resolve(r.Context(), request.ProjectID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_import", err.Error())
		return
	}
	if !exact {
		for _, hint := range request.ScopeHints {
			value := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(hint), "project:"))
			candidate, matched, resolveErr := s.app.Portfolio.Resolve(r.Context(), value)
			if resolveErr != nil {
				writeErr(w, http.StatusBadRequest, "bad_import", resolveErr.Error())
				return
			}
			if matched {
				project, exact = candidate, true
				break
			}
		}
	}
	var batchID string
	if request.ConnectorID != "" && request.IdempotencyKey == "" {
		request.IdempotencyKey = "text:" + request.SessionID
	}
	if request.IdempotencyKey != "" {
		batch, duplicate, beginErr := s.app.Portfolio.BeginImportBatch(r.Context(), request.ConnectorID, project.ProjectID, request.IdempotencyKey)
		if beginErr != nil {
			writeErr(w, http.StatusBadRequest, "import_batch_failed", beginErr.Error())
			return
		}
		if duplicate && batch.Status == "completed" {
			writeJSON(w, http.StatusOK, map[string]any{"batch": batch, "duplicate": true})
			return
		}
		batchID = batch.BatchID
	}
	now := time.Now().UTC()
	envelopes := make([][]byte, 0, len(request.Documents))
	for i, document := range request.Documents {
		if strings.TrimSpace(document.Text) == "" {
			writeErr(w, http.StatusBadRequest, "bad_import", "document "+strconv.Itoa(i)+" text is empty")
			return
		}
		observed := now
		if document.ObservedAt != "" {
			parsed, err := time.Parse(time.RFC3339, document.ObservedAt)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_import", "invalid observed_at for document "+strconv.Itoa(i))
				return
			}
			observed = parsed.UTC()
		}
		text := strings.TrimSpace(document.Text)
		if strings.TrimSpace(document.Title) != "" {
			text = strings.TrimSpace(document.Title) + "\n\n" + text
		}
		evidenceID := "import_" + contracts.HashBytes([]byte(request.SourceSystem + "\x00" + request.SessionID + "\x00" + text))[:24]
		captured := now
		if prior, ok, receiptErr := s.app.Control.Receipt(r.Context(), evidenceID); receiptErr == nil && ok {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, prior.ObservedAt); parseErr == nil {
				observed = parsed
			}
			if parsed, parseErr := time.Parse(time.RFC3339Nano, prior.CapturedAt); parseErr == nil {
				captured = parsed
			}
		}
		role := "document"
		sessionID := request.SessionID
		parser := "plain-text-markdown"
		parserVersion := "1"
		envelope := contracts.EvidenceEnvelope{
			SchemaVersion:          "0.1",
			EvidenceID:             evidenceID,
			SourceSystem:           request.SourceSystem,
			ExternalConversationID: &sessionID,
			Role:                   &role,
			ObservedAt:             &observed,
			CapturedAt:             captured,
			Content:                []contracts.ContentBlock{{Type: "text", Text: text}},
			Provenance:             contracts.Provenance{CaptureMethod: "ui_text_import", Parser: &parser, ParserVersion: &parserVersion},
			ScopeHints:             request.ScopeHints,
			Visibility:             "private",
		}
		raw, _ := json.Marshal(envelope)
		envelopes = append(envelopes, raw)
	}
	results := make([]any, 0, len(envelopes))
	evidenceIDs := make([]string, 0, len(envelopes))
	for _, raw := range envelopes {
		result, err := s.app.Ledger.Append(r.Context(), raw)
		if err != nil {
			if batchID != "" {
				_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batchID, evidenceIDs, err.Error())
			}
			writeErr(w, http.StatusBadRequest, "import_failed", err.Error())
			return
		}
		if err := s.app.Portfolio.LinkRecord(r.Context(), "evidence", result.EvidenceID, project.ProjectID, true); err != nil {
			writeErr(w, http.StatusInternalServerError, "import_route_failed", err.Error())
			return
		}
		evidenceIDs = append(evidenceIDs, result.EvidenceID)
		results = append(results, result)
	}
	autoProcess, processingMode, err := s.projectAutoProcesses(r.Context(), project.ProjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "import_process_failed", err.Error())
		return
	}
	var pipeline any
	if autoProcess {
		pipeline, err = s.growSession(r.Context(), project.ProjectID, request.SessionID, evidenceIDs, true, false)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "import_process_failed", err.Error())
			return
		}
	}
	var batch any
	if batchID != "" {
		completed, completeErr := s.app.Portfolio.CompleteImportBatch(r.Context(), batchID, evidenceIDs, "")
		if completeErr != nil {
			writeErr(w, http.StatusInternalServerError, "import_batch_failed", completeErr.Error())
			return
		}
		batch = completed
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session_id": request.SessionID, "project_id": project.ProjectID, "captured": results, "pipeline": pipeline, "batch": batch, "processing_mode": processingMode})
}
