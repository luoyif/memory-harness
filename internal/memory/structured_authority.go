package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// StructuredKnowledgeObjectID is the stable project-scoped Object Store
// identity for one legacy Knowledge Unit. The legacy tables remain readable as
// a compatibility projection; once this object exists its current revision is
// the authoritative editable representation for that project.
func StructuredKnowledgeObjectID(projectID, unitID string) string {
	return stableID("obj-ku-v2-", strings.TrimSpace(projectID), strings.TrimSpace(unitID))
}

func ensureKnowledgeUnitV2(unit KnowledgeUnit) KnowledgeUnit {
	unit.UnitID = strings.TrimSpace(unit.UnitID)
	unit.EpisodeID = strings.TrimSpace(unit.EpisodeID)
	unit.EvidenceID = strings.TrimSpace(unit.EvidenceID)
	unit.Statement = strings.TrimSpace(unit.Statement)
	unit.NormalizedKey = normalizeStatement(unit.Statement)
	unit.UnitType = strings.ToLower(strings.TrimSpace(unit.UnitType))
	unit.TierHint = strings.ToLower(strings.TrimSpace(unit.TierHint))
	unit.RiskTier = strings.ToUpper(strings.TrimSpace(unit.RiskTier))
	unit.Status = strings.TrimSpace(unit.Status)
	unit.Scopes = unique(unit.Scopes)
	unit.SchemaVersion = KnowledgeUnitSchemaV2

	if unit.Structure.Attribution.Resolution == "" {
		unit.Structure.Attribution.Resolution = "unresolved"
	}
	if unit.Structure.Attribution.OwnerMapping == "" {
		unit.Structure.Attribution.OwnerMapping = "not_assumed"
	}
	if unit.Structure.Frame.Subject.Resolution == "" {
		unit.Structure.Frame.Subject.Resolution = unit.Structure.Attribution.Resolution
	}
	if unit.Structure.Temporal.ObservedAt == "" {
		unit.Structure.Temporal.ObservedAt = unit.ObservedAt
	}
	if unit.Structure.Temporal.AnchorEvidenceTime == "" {
		unit.Structure.Temporal.AnchorEvidenceTime = unit.ObservedAt
	}
	if unit.Structure.Temporal.Precision == "" {
		unit.Structure.Temporal.Precision = "unknown"
	}
	if unit.Structure.Temporal.Resolution == "" {
		unit.Structure.Temporal.Resolution = "not_applicable"
	}
	if unit.Structure.Epistemic.Polarity == "" {
		unit.Structure.Epistemic.Polarity = "positive"
	}
	if unit.Structure.Epistemic.Modality == "" {
		unit.Structure.Epistemic.Modality = defaultModality(unit.UnitType)
	}
	unit.Structure.Epistemic.Confidence = unit.Confidence
	if unit.Structure.Provenance.EvidenceID == "" {
		unit.Structure.Provenance.EvidenceID = unit.EvidenceID
	}
	if unit.Structure.Provenance.EpisodeID == "" {
		unit.Structure.Provenance.EpisodeID = unit.EpisodeID
	}
	if strings.TrimSpace(unit.Structure.Provenance.EvidenceSpan.Quote) == "" {
		unit.Structure.Provenance.EvidenceSpan = EvidenceSpan{Start: -1, End: -1, Quote: unit.Statement}
	}
	return unit
}

func StructuredKnowledgePayload(unit KnowledgeUnit) (json.RawMessage, error) {
	unit = ensureKnowledgeUnitV2(unit)
	if err := ValidateStructuredKnowledgeRevision(unit, unit); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(unit)
	return json.RawMessage(raw), err
}

func DecodeStructuredKnowledgePayload(raw json.RawMessage) (KnowledgeUnit, error) {
	var unit KnowledgeUnit
	if len(raw) == 0 || !json.Valid(raw) {
		return unit, errors.New("structured knowledge payload must be valid JSON")
	}
	if err := json.Unmarshal(raw, &unit); err != nil {
		return unit, err
	}
	unit = ensureKnowledgeUnitV2(unit)
	return unit, nil
}

// PrepareStructuredKnowledgeRevision keeps identity and Evidence provenance
// immutable while allowing an Owner to correct the semantic interpretation.
func PrepareStructuredKnowledgeRevision(base, candidate KnowledgeUnit) (KnowledgeUnit, error) {
	base = ensureKnowledgeUnitV2(base)
	// Reject attempted provenance/identity rewrites explicitly. Silently
	// normalizing these fields would make a caller believe a source relink was
	// accepted even though Evidence is immutable.
	if strings.TrimSpace(candidate.UnitID) != "" && strings.TrimSpace(candidate.UnitID) != base.UnitID {
		return KnowledgeUnit{}, errors.New("unit_id is immutable")
	}
	if strings.TrimSpace(candidate.EpisodeID) != "" && strings.TrimSpace(candidate.EpisodeID) != base.EpisodeID {
		return KnowledgeUnit{}, errors.New("episode_id is immutable")
	}
	if strings.TrimSpace(candidate.EvidenceID) != "" && strings.TrimSpace(candidate.EvidenceID) != base.EvidenceID {
		return KnowledgeUnit{}, errors.New("evidence_id is immutable")
	}
	if strings.TrimSpace(candidate.ObservedAt) != "" && strings.TrimSpace(candidate.ObservedAt) != base.ObservedAt {
		return KnowledgeUnit{}, errors.New("observed_at is anchored to immutable Evidence")
	}
	if strings.TrimSpace(candidate.Structure.Provenance.EvidenceID) != "" && strings.TrimSpace(candidate.Structure.Provenance.EvidenceID) != base.EvidenceID {
		return KnowledgeUnit{}, errors.New("provenance.evidence_id is immutable")
	}
	if strings.TrimSpace(candidate.Structure.Provenance.EpisodeID) != "" && strings.TrimSpace(candidate.Structure.Provenance.EpisodeID) != base.EpisodeID {
		return KnowledgeUnit{}, errors.New("provenance.episode_id is immutable")
	}
	candidate = ensureKnowledgeUnitV2(candidate)
	candidate.UnitID = base.UnitID
	candidate.EpisodeID = base.EpisodeID
	candidate.EvidenceID = base.EvidenceID
	candidate.ObservedAt = base.ObservedAt
	candidate.CreatedAt = base.CreatedAt
	candidate.ProcessedAt = base.ProcessedAt
	candidate.Status = base.Status
	candidate.SchemaVersion = KnowledgeUnitSchemaV2
	candidate.NormalizedKey = normalizeStatement(candidate.Statement)

	// Evidence lineage is not editable through a semantic correction. The
	// Revision review itself records the Owner and edit_reason.
	candidate.Structure.Provenance = base.Structure.Provenance
	candidate.Structure.Provenance.EvidenceID = base.EvidenceID
	candidate.Structure.Provenance.EpisodeID = base.EpisodeID
	candidate.Structure.Temporal.ObservedAt = base.Structure.Temporal.ObservedAt
	if candidate.Structure.Temporal.ObservedAt == "" {
		candidate.Structure.Temporal.ObservedAt = base.ObservedAt
	}
	candidate.Structure.Temporal.AnchorEvidenceTime = base.Structure.Temporal.AnchorEvidenceTime
	if candidate.Structure.Temporal.AnchorEvidenceTime == "" {
		candidate.Structure.Temporal.AnchorEvidenceTime = base.ObservedAt
	}
	candidate.Structure.Temporal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	candidate.Structure.Attribution.OwnerMapping = "not_assumed"
	candidate.Structure.Epistemic.Confidence = candidate.Confidence
	if err := ValidateStructuredKnowledgeRevision(base, candidate); err != nil {
		return KnowledgeUnit{}, err
	}
	return candidate, nil
}

func ValidateStructuredKnowledgeRevision(base, candidate KnowledgeUnit) error {
	if candidate.UnitID == "" || candidate.EpisodeID == "" || candidate.EvidenceID == "" {
		return errors.New("unit_id, episode_id and evidence_id are required")
	}
	if base.UnitID != "" && (candidate.UnitID != base.UnitID || candidate.EpisodeID != base.EpisodeID || candidate.EvidenceID != base.EvidenceID) {
		return errors.New("knowledge unit identity and Evidence provenance are immutable")
	}
	if candidate.SchemaVersion != KnowledgeUnitSchemaV2 {
		return fmt.Errorf("schema_version must be %s", KnowledgeUnitSchemaV2)
	}
	if candidate.Statement == "" || len([]rune(candidate.Statement)) > 4000 {
		return errors.New("statement must contain 1-4000 characters")
	}
	allowedTypes := map[string]bool{"fact": true, "state": true, "event": true, "decision": true, "goal": true, "risk": true, "outcome": true, "procedure": true, "identity": true, "correction": true}
	allowedTiers := map[string]bool{"episodic": true, "semantic": true, "procedural": true, "identity_core": true}
	allowedRisks := map[string]bool{"A": true, "B": true, "C": true, "D": true}
	if !allowedTypes[candidate.UnitType] {
		return fmt.Errorf("unsupported unit_type %q", candidate.UnitType)
	}
	if !allowedTiers[candidate.TierHint] {
		return fmt.Errorf("unsupported tier_hint %q", candidate.TierHint)
	}
	if !allowedRisks[candidate.RiskTier] {
		return fmt.Errorf("unsupported risk_tier %q", candidate.RiskTier)
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if candidate.Structure.Provenance.EvidenceID != candidate.EvidenceID || candidate.Structure.Provenance.EpisodeID != candidate.EpisodeID {
		return errors.New("structured provenance must point to the immutable unit Evidence and Episode")
	}
	for _, value := range []struct {
		name, raw string
	}{
		{"observed_at", candidate.ObservedAt},
		{"temporal.observed_at", candidate.Structure.Temporal.ObservedAt},
		{"temporal.valid_from", candidate.Structure.Temporal.ValidFrom},
		{"temporal.valid_until", candidate.Structure.Temporal.ValidUntil},
		{"temporal.occurred_from", candidate.Structure.Temporal.OccurredFrom},
		{"temporal.occurred_until", candidate.Structure.Temporal.OccurredUntil},
	} {
		if strings.TrimSpace(value.raw) == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value.raw); err != nil {
			return fmt.Errorf("%s must be RFC3339", value.name)
		}
	}
	if candidate.Structure.Temporal.ValidFrom != "" && candidate.Structure.Temporal.ValidUntil != "" {
		from, _ := time.Parse(time.RFC3339, candidate.Structure.Temporal.ValidFrom)
		until, _ := time.Parse(time.RFC3339, candidate.Structure.Temporal.ValidUntil)
		if until.Before(from) {
			return errors.New("temporal.valid_until cannot be before valid_from")
		}
	}
	return nil
}

func (e *Engine) overlayStructuredKnowledgeObjects(ctx context.Context, projectID string, units []KnowledgeUnit) error {
	if len(units) == 0 || strings.TrimSpace(projectID) == "" {
		return nil
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT r.payload_json FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE o.project_id=? AND o.type_id=? AND o.status='active'`, strings.TrimSpace(projectID), StructuredKnowledgeUnitTypeV2)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[string]KnowledgeUnit{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		unit, err := DecodeStructuredKnowledgePayload(json.RawMessage(raw))
		if err != nil {
			return err
		}
		byID[unit.UnitID] = unit
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range units {
		if current, ok := byID[units[i].UnitID]; ok {
			units[i] = current
		}
	}
	return nil
}

// KnowledgeUnitForProject reads the current project-scoped authority. If a
// rich Object Store revision does not exist yet it returns the hydrated legacy
// compatibility projection without writing anything.
func (e *Engine) KnowledgeUnitForProject(ctx context.Context, projectID, unitID string) (KnowledgeUnit, error) {
	projectID, unitID = strings.TrimSpace(projectID), strings.TrimSpace(unitID)
	if projectID == "" || unitID == "" {
		return KnowledgeUnit{}, errors.New("project_id and unit_id are required")
	}
	var linked int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE record_type='knowledge_unit' AND record_id=? AND project_id=?`, unitID, projectID).Scan(&linked); err != nil {
		return KnowledgeUnit{}, err
	}
	if linked == 0 {
		return KnowledgeUnit{}, sql.ErrNoRows
	}
	unit, err := e.knowledgeUnit(ctx, unitID)
	if err != nil {
		return KnowledgeUnit{}, err
	}
	unit = ensureKnowledgeUnitV2(unit)
	items := []KnowledgeUnit{unit}
	if err := e.overlayStructuredKnowledgeObjects(ctx, projectID, items); err != nil {
		return KnowledgeUnit{}, err
	}
	return items[0], nil
}
