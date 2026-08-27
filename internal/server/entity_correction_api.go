package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
)

type entityCorrectionPrepared struct {
	unit     memory.KnowledgeUnit
	payload  json.RawMessage
	object   harness.Object
	expected int
}

func (s *Server) ensureKnowledgeAuthority(ctx context.Context, projectID string, base memory.KnowledgeUnit) (harness.Object, error) {
	objectID := memory.StructuredKnowledgeObjectID(projectID, base.UnitID)
	object, err := s.app.Harness.Object(ctx, objectID)
	if err == nil {
		return object, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return harness.Object{}, err
	}
	baseRaw, err := memory.StructuredKnowledgePayload(base)
	if err != nil {
		return harness.Object{}, err
	}
	validFrom := base.Structure.Temporal.ValidFrom
	if validFrom == "" {
		validFrom = base.ObservedAt
	}
	return s.app.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: objectID, TypeID: memory.StructuredKnowledgeUnitTypeV2, ProjectID: projectID, Status: "active", Payload: baseRaw,
		Confidence: base.Confidence, Importance: base.Structure.Epistemic.Importance, ValidFrom: validFrom, ValidUntil: base.Structure.Temporal.ValidUntil,
		SourceEvidenceIDs: []string{base.EvidenceID}, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0",
		IdempotencyKey: projectID + ":" + base.UnitID + ":owner-bootstrap-v2",
	})
}

func applyEntityCanonical(unit memory.KnowledgeUnit, source memory.CorrectionEntity, target memory.CorrectionEntity, subjectRole, objectRole bool) memory.KnowledgeUnit {
	if subjectRole {
		unit.Structure.Frame.Subject.CanonicalName = target.CanonicalName
		unit.Structure.Frame.Subject.EntityType = target.EntityType
		unit.Structure.Frame.Subject.Resolution = "resolved"
		unit.Structure.Frame.Subject.Aliases = append(unit.Structure.Frame.Subject.Aliases, source.CanonicalName)
		unit.Structure.Attribution.Resolution = "resolved"
		unit.Structure.Attribution.OwnerMapping = "not_assumed"
	}
	if objectRole {
		unit.Structure.Frame.Object.Kind = "entity"
		unit.Structure.Frame.Object.Entity.CanonicalName = target.CanonicalName
		unit.Structure.Frame.Object.Entity.EntityType = target.EntityType
		unit.Structure.Frame.Object.Entity.Resolution = "resolved"
		unit.Structure.Frame.Object.Entity.Aliases = append(unit.Structure.Frame.Object.Entity.Aliases, source.CanonicalName)
	}
	return unit
}

func (s *Server) proposeEntityCorrection(w http.ResponseWriter, r *http.Request) {
	entityID := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ProjectID      string   `json:"project_id"`
		Action         string   `json:"action"`
		TargetEntityID string   `json:"target_entity_id"`
		CanonicalName  string   `json:"canonical_name"`
		EntityType     string   `json:"entity_type"`
		UnitIDs        []string `json:"unit_ids"`
		EditReason     string   `json:"edit_reason"`
		IdempotencyKey string   `json:"idempotency_key"`
	}
	if err := decodeStrictJSON(r, 1<<20, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_entity_correction", err.Error())
		return
	}
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.EditReason = strings.TrimSpace(request.EditReason)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ProjectID == "" || request.EditReason == "" || request.IdempotencyKey == "" {
		writeErr(w, http.StatusBadRequest, "bad_entity_correction", "project_id, edit_reason and idempotency_key are required")
		return
	}
	if request.Action != "rename" && request.Action != "merge" && request.Action != "split" {
		writeErr(w, http.StatusBadRequest, "bad_entity_correction", "action must be rename, merge or split")
		return
	}
	source, err := s.app.Memory.CorrectionEntity(r.Context(), request.ProjectID, entityID)
	if err != nil {
		writeStoreError(w, "entity_correction_failed", err)
		return
	}
	impact, err := s.app.Memory.PreviewCorrection(r.Context(), request.ProjectID, "entity", entityID)
	if err != nil {
		writeStoreError(w, "entity_correction_failed", err)
		return
	}
	target := memory.CorrectionEntity{ProjectID: request.ProjectID, EntityType: strings.TrimSpace(request.EntityType), CanonicalName: strings.TrimSpace(request.CanonicalName), Status: "candidate"}
	selected := impact.Impact.UnitIDs
	if request.Action == "merge" {
		target, err = s.app.Memory.CorrectionEntity(r.Context(), request.ProjectID, strings.TrimSpace(request.TargetEntityID))
		if err != nil {
			writeStoreError(w, "entity_correction_failed", err)
			return
		}
		if target.EntityID == source.EntityID {
			writeErr(w, http.StatusBadRequest, "bad_entity_correction", "source and target entity must differ")
			return
		}
	} else {
		if target.CanonicalName == "" {
			writeErr(w, http.StatusBadRequest, "bad_entity_correction", "canonical_name is required for rename/split")
			return
		}
		if target.EntityType == "" {
			target.EntityType = source.EntityType
		}
	}
	if request.Action == "split" {
		if len(request.UnitIDs) == 0 {
			writeErr(w, http.StatusBadRequest, "bad_entity_correction", "split requires explicit unit_ids")
			return
		}
		allowed := map[string]bool{}
		for _, id := range impact.Impact.UnitIDs {
			allowed[id] = true
		}
		selected = []string{}
		for _, id := range request.UnitIDs {
			id = strings.TrimSpace(id)
			if !allowed[id] {
				writeErr(w, http.StatusBadRequest, "bad_entity_correction", "split unit is outside source entity impact: "+id)
				return
			}
			if id != "" {
				selected = append(selected, id)
			}
		}
	}
	if len(selected) == 0 {
		writeErr(w, http.StatusConflict, "entity_correction_empty", "entity has no active knowledge units to revise")
		return
	}

	prepared := make([]entityCorrectionPrepared, 0, len(selected))
	for _, unitID := range selected {
		base, err := s.app.Memory.KnowledgeUnitForProject(r.Context(), request.ProjectID, unitID)
		if err != nil {
			writeStoreError(w, "entity_correction_failed", err)
			return
		}
		subjectRole, objectRole, err := s.app.Memory.EntityRolesForUnit(r.Context(), request.ProjectID, entityID, unitID)
		if err != nil {
			writeStoreError(w, "entity_correction_failed", err)
			return
		}
		if !subjectRole && !objectRole {
			writeErr(w, http.StatusConflict, "entity_correction_stale", "entity is no longer active in unit "+unitID)
			return
		}
		candidate := applyEntityCanonical(base, source, target, subjectRole, objectRole)
		candidate, err = memory.PrepareStructuredKnowledgeRevision(base, candidate)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "entity_correction_failed", err.Error())
			return
		}
		payload, err := memory.StructuredKnowledgePayload(candidate)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "entity_correction_failed", err.Error())
			return
		}
		object, err := s.ensureKnowledgeAuthority(r.Context(), request.ProjectID, base)
		if err != nil {
			writeErr(w, http.StatusConflict, "entity_correction_failed", err.Error())
			return
		}
		prepared = append(prepared, entityCorrectionPrepared{unit: candidate, payload: payload, object: object, expected: object.CurrentRevision})
	}
	batchID := "correction-" + contracts.HashBytes([]byte(request.IdempotencyKey))[:20]
	reviews := make([]harness.RevisionReview, 0, len(prepared))
	for _, item := range prepared {
		validFrom := item.unit.Structure.Temporal.ValidFrom
		if validFrom == "" {
			validFrom = item.unit.ObservedAt
		}
		review, err := s.app.Harness.ProposeRevision(r.Context(), item.object.ObjectID, harness.ProposeRevisionInput{Payload: item.payload, ExpectedRevision: item.expected, EditReason: request.EditReason, TargetStatus: "active", Confidence: item.unit.Confidence, Importance: item.unit.Structure.Epistemic.Importance, ValidFrom: validFrom, ValidUntil: item.unit.Structure.Temporal.ValidUntil, SourceEvidenceIDs: []string{item.unit.EvidenceID}, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: request.IdempotencyKey + ":" + item.unit.UnitID, RequestedBy: ownerActor(r), Validation: json.RawMessage(`{"status":"passed","kind":"owner_entity_correction","evidence_immutable":true}`)})
		if err != nil {
			writeErr(w, http.StatusConflict, "entity_correction_failed", fmt.Sprintf("batch %s proposal for %s failed: %v", batchID, item.unit.UnitID, err))
			return
		}
		reviews = append(reviews, review)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"batch_id": batchID, "action": request.Action, "source_entity": source, "target_entity": target, "impact": impact.Impact, "reviews": reviews})
}
