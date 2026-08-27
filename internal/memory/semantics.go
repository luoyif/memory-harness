package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const semanticExtractorVersion = "1.0.0"

type SemanticMaterialization struct {
	Units          int `json:"units"`
	Entities       int `json:"entities"`
	Assertions     int `json:"assertions"`
	TemporalFacts  int `json:"temporal_facts"`
	AmbiguousUnits int `json:"ambiguous_units"`
}

func normalizedToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	separator := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			separator = false
			continue
		}
		if out.Len() > 0 && !separator {
			out.WriteByte('_')
			separator = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func normalizeResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "resolved", "unresolved", "ambiguous", "not_applicable":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unresolved"
	}
}

func normalizeEntity(ref EntityRef) EntityRef {
	ref.EntityID = strings.TrimSpace(ref.EntityID)
	ref.EntityType = normalizedToken(ref.EntityType)
	ref.Surface = strings.TrimSpace(ref.Surface)
	ref.CanonicalName = strings.TrimSpace(ref.CanonicalName)
	ref.Aliases = unique(ref.Aliases)
	ref.Resolution = normalizeResolution(ref.Resolution)
	if ref.CanonicalName == "" {
		ref.CanonicalName = ref.Surface
	}
	if ref.Surface == "" {
		ref.Surface = ref.CanonicalName
	}
	return ref
}

func firstPersonOrGenericSubject(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "我", "我们", "本人", "用户", "该用户", "说话者", "讲者", "讲述者", "发言人", "受访者", "录音者", "作者", "speaker", "user", "owner", "the user", "the speaker", "the pair":
		return true
	}
	for _, prefix := range []string{"该用户的", "用户的", "说话者的", "讲者的", "讲述者的", "发言人的", "受访者的", "录音者的", "作者的", "speaker的", "speaker 的", "speaker's ", "the speaker's ", "the user's ", "user's "} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func explicitSemanticReference(ref EntityRef) string {
	name := entityName(ref)
	if name == "" || firstPersonOrGenericSubject(name) {
		return ""
	}
	typeID := normalizedToken(ref.EntityType)
	if typeID == "" {
		typeID = "concept"
	}
	return stableID("semantic-ref-", typeID, normalizeStatement(name))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func evidenceSpan(text, quote string) EvidenceSpan {
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return EvidenceSpan{Start: -1, End: -1}
	}
	byteStart := strings.Index(text, quote)
	start, end := -1, -1
	if byteStart >= 0 {
		start = utf8.RuneCountInString(text[:byteStart])
		end = start + utf8.RuneCountInString(quote)
	}
	sum := sha256.Sum256([]byte(quote))
	return EvidenceSpan{Start: start, End: end, Quote: quote, QuoteHash: "sha256:" + hex.EncodeToString(sum[:])}
}

func defaultPredicate(unitType string) string {
	switch unitType {
	case "decision":
		return "decided"
	case "goal":
		return "aims_for"
	case "risk":
		return "faces_risk"
	case "outcome":
		return "resulted_in"
	case "procedure":
		return "follows_procedure"
	case "identity":
		return "has_identity"
	case "correction":
		return "corrects"
	case "state":
		return "has_state"
	default:
		return "states"
	}
}

func defaultInverseLabel(predicate string) string {
	predicate = normalizedToken(predicate)
	if predicate == "" {
		return ""
	}
	switch predicate {
	case "characterized_by":
		return "characterizes"
	case "defined_as":
		return "defines"
	case "outputs":
		return "is_output_by"
	case "can_replace":
		return "can_be_replaced_by"
	case "requires":
		return "is_required_by"
	}
	return "is_" + predicate + "_of"
}

func defaultModality(unitType string) string {
	switch unitType {
	case "goal":
		return "desired"
	case "risk":
		return "uncertain"
	case "procedure":
		return "normative"
	default:
		return "asserted"
	}
}

func knownParticipant(ref string, participants map[string]bool) bool {
	ref = strings.TrimSpace(ref)
	return ref != "" && participants[ref]
}

// normalizeKnowledgeStructure converts provider output into an inspectable v2
// envelope. It deliberately abstains when a first-person or generic speaker
// label has not been bound to an explicit participant. A chat role is source
// metadata, never proof that the described subject is the local Owner.
func normalizeKnowledgeStructure(extracted KnowledgeStructure, item turn, statement, unitType, compiler string, confidence float64, participants map[string]bool) KnowledgeStructure {
	structure := extracted
	structure.Attribution.SourceSpeakerRef = strings.TrimSpace(structure.Attribution.SourceSpeakerRef)
	if structure.Attribution.SourceSpeakerRef == "" {
		role := normalizedToken(item.Role)
		if role == "" {
			role = "unknown"
		}
		structure.Attribution.SourceSpeakerRef = "source-role:" + role
	}
	structure.Attribution.AssertedByRef = strings.TrimSpace(structure.Attribution.AssertedByRef)
	if structure.Attribution.AssertedByRef == "" {
		structure.Attribution.AssertedByRef = structure.Attribution.SourceSpeakerRef
	}
	structure.Attribution.SubjectRef = strings.TrimSpace(structure.Attribution.SubjectRef)
	structure.Attribution.SubjectSurface = strings.TrimSpace(structure.Attribution.SubjectSurface)
	structure.Attribution.Resolution = normalizeResolution(structure.Attribution.Resolution)
	structure.Attribution.CandidateRefs = unique(structure.Attribution.CandidateRefs)
	structure.Attribution.ReasonCodes = unique(structure.Attribution.ReasonCodes)
	structure.Attribution.OwnerMapping = "not_assumed"

	structure.Frame.Subject = normalizeEntity(structure.Frame.Subject)
	structure.Frame.Object.Entity = normalizeEntity(structure.Frame.Object.Entity)
	structure.Frame.Predicate = normalizedToken(structure.Frame.Predicate)
	structure.Frame.InverseLabel = normalizedToken(structure.Frame.InverseLabel)
	structure.Frame.Action = strings.TrimSpace(structure.Frame.Action)
	structure.Frame.Context = strings.TrimSpace(structure.Frame.Context)
	if structure.Frame.Predicate == "" {
		structure.Frame.Predicate = defaultPredicate(unitType)
	}
	if structure.Frame.InverseLabel == "" {
		structure.Frame.InverseLabel = defaultInverseLabel(structure.Frame.Predicate)
	}
	if structure.Attribution.SubjectRef == "" {
		structure.Attribution.SubjectRef = structure.Frame.Subject.EntityID
	}
	if structure.Attribution.SubjectSurface == "" {
		structure.Attribution.SubjectSurface = structure.Frame.Subject.Surface
	}
	if structure.Frame.Subject.EntityID == "" {
		structure.Frame.Subject.EntityID = structure.Attribution.SubjectRef
	}
	if structure.Frame.Subject.Surface == "" {
		structure.Frame.Subject.Surface = structure.Attribution.SubjectSurface
	}

	genericSubject := firstPersonOrGenericSubject(structure.Attribution.SubjectSurface) || firstPersonOrGenericSubject(structure.Frame.Subject.Surface)
	forbiddenOwner := strings.EqualFold(structure.Attribution.SubjectRef, "owner") || strings.EqualFold(structure.Frame.Subject.EntityID, "owner")
	if (genericSubject || forbiddenOwner) && !knownParticipant(structure.Attribution.SubjectRef, participants) {
		structure.Attribution.Resolution = "unresolved"
		structure.Frame.Subject.Resolution = "unresolved"
		structure.Attribution.SubjectRef = ""
		structure.Frame.Subject.EntityID = ""
		if !containsString(structure.Attribution.ReasonCodes, "participant_identity_required") {
			structure.Attribution.ReasonCodes = append(structure.Attribution.ReasonCodes, "participant_identity_required")
		}
		if forbiddenOwner && !containsString(structure.Attribution.ReasonCodes, "owner_mapping_not_explicit") {
			structure.Attribution.ReasonCodes = append(structure.Attribution.ReasonCodes, "owner_mapping_not_explicit")
		}
	}
	// A project, organization, system, place, concept or explicitly named
	// person is already a valid project-local graph entity. It does not require
	// a speaker registry merely because the provider omitted an internal ID.
	// This never applies to generic first-person/speaker labels and never turns
	// an ambiguous model result into a resolved one.
	if structure.Attribution.Resolution == "unresolved" && structure.Attribution.SubjectRef == "" && !genericSubject {
		if ref := explicitSemanticReference(structure.Frame.Subject); ref != "" {
			structure.Attribution.SubjectRef = ref
			structure.Frame.Subject.EntityID = ref
			structure.Attribution.Resolution = "resolved"
		}
	}
	if structure.Attribution.SubjectSurface == "" && structure.Frame.Subject.Surface == "" {
		structure.Attribution.Resolution = "unresolved"
		if !containsString(structure.Attribution.ReasonCodes, "subject_missing") {
			structure.Attribution.ReasonCodes = append(structure.Attribution.ReasonCodes, "subject_missing")
		}
	}
	if structure.Attribution.Resolution == "resolved" && structure.Frame.Subject.Surface == "" {
		structure.Attribution.Resolution = "unresolved"
		structure.Attribution.ReasonCodes = append(structure.Attribution.ReasonCodes, "resolved_subject_has_no_surface")
	}
	structure.Frame.Subject.Resolution = structure.Attribution.Resolution
	if structure.Frame.Object.Entity.EntityID == "" && structure.Frame.Object.Entity.Resolution == "unresolved" {
		if ref := explicitSemanticReference(structure.Frame.Object.Entity); ref != "" {
			structure.Frame.Object.Entity.EntityID = ref
			structure.Frame.Object.Entity.Resolution = "resolved"
		}
	}
	for index := range structure.Frame.Participants {
		structure.Frame.Participants[index].Role = normalizedToken(structure.Frame.Participants[index].Role)
		structure.Frame.Participants[index].Entity = normalizeEntity(structure.Frame.Participants[index].Entity)
	}
	for index := range structure.Frame.Locations {
		structure.Frame.Locations[index].Role = normalizedToken(structure.Frame.Locations[index].Role)
		structure.Frame.Locations[index].Entity = normalizeEntity(structure.Frame.Locations[index].Entity)
	}

	structure.Temporal.ObservedAt = item.ObservedAt
	structure.Temporal.AnchorEvidenceTime = item.ObservedAt
	structure.Temporal.EventTimeText = strings.TrimSpace(structure.Temporal.EventTimeText)
	structure.Temporal.ValidFrom = strings.TrimSpace(structure.Temporal.ValidFrom)
	structure.Temporal.ValidUntil = strings.TrimSpace(structure.Temporal.ValidUntil)
	structure.Temporal.OccurredFrom = strings.TrimSpace(structure.Temporal.OccurredFrom)
	structure.Temporal.OccurredUntil = strings.TrimSpace(structure.Temporal.OccurredUntil)
	invalidTemporal := false
	for _, field := range []*string{
		&structure.Temporal.ValidFrom,
		&structure.Temporal.ValidUntil,
		&structure.Temporal.OccurredFrom,
		&structure.Temporal.OccurredUntil,
	} {
		if *field == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, *field); err != nil {
			*field = ""
			invalidTemporal = true
		}
	}
	if structure.Temporal.ValidFrom != "" && structure.Temporal.ValidUntil != "" {
		from, _ := time.Parse(time.RFC3339, structure.Temporal.ValidFrom)
		until, _ := time.Parse(time.RFC3339, structure.Temporal.ValidUntil)
		if until.Before(from) {
			structure.Temporal.ValidFrom = ""
			structure.Temporal.ValidUntil = ""
			invalidTemporal = true
		}
	}
	if structure.Temporal.OccurredFrom != "" && structure.Temporal.OccurredUntil != "" {
		from, _ := time.Parse(time.RFC3339, structure.Temporal.OccurredFrom)
		until, _ := time.Parse(time.RFC3339, structure.Temporal.OccurredUntil)
		if until.Before(from) {
			structure.Temporal.OccurredFrom = ""
			structure.Temporal.OccurredUntil = ""
			invalidTemporal = true
		}
	}
	if structure.Temporal.Precision == "" {
		structure.Temporal.Precision = "unknown"
	}
	if structure.Temporal.Resolution == "" {
		structure.Temporal.Resolution = "not_applicable"
	}
	if invalidTemporal {
		structure.Temporal.Resolution = "unresolved"
		structure.Epistemic.QualityFlags = appendUnique(structure.Epistemic.QualityFlags, "invalid_temporal_format")
		structure.Epistemic.ReviewReasons = appendUnique(structure.Epistemic.ReviewReasons, "temporal_normalization_required")
	}

	if structure.Epistemic.Polarity == "" {
		structure.Epistemic.Polarity = "positive"
	}
	if structure.Epistemic.Modality == "" {
		structure.Epistemic.Modality = defaultModality(unitType)
	}
	structure.Epistemic.Confidence = confidence
	structure.Epistemic.QualityFlags = unique(structure.Epistemic.QualityFlags)
	structure.Epistemic.ReviewReasons = unique(structure.Epistemic.ReviewReasons)
	if structure.Attribution.Resolution == "ambiguous" || structure.Attribution.Resolution == "unresolved" {
		structure.Epistemic.ReviewReasons = appendUnique(structure.Epistemic.ReviewReasons, "subject_"+structure.Attribution.Resolution)
	}

	quote := structure.Provenance.EvidenceSpan.Quote
	if strings.TrimSpace(quote) == "" {
		quote = statement
	}
	structure.Provenance.EvidenceID = item.EvidenceID
	structure.Provenance.ExtractorPlugin = "builtin.semantic-frame"
	structure.Provenance.ExtractorVersion = semanticExtractorVersion
	structure.Provenance.ModelProfile = strings.TrimSpace(compiler)
	structure.Provenance.EvidenceSpan = evidenceSpan(item.Body, quote)
	return structure
}

func semanticQuality(structure KnowledgeStructure) string {
	switch structure.Attribution.Resolution {
	case "ambiguous":
		return "ambiguous"
	case "unresolved":
		return "review_required"
	}
	if strings.TrimSpace(structure.Frame.Predicate) == "" || strings.TrimSpace(structure.Frame.Subject.Surface) == "" {
		return "partial"
	}
	return "structured"
}

func (e *Engine) persistKnowledgeStructure(ctx context.Context, unitID string, structure KnowledgeStructure) error {
	raw, err := json.Marshal(structure)
	if err != nil {
		return err
	}
	now := nowString()
	_, err = e.control.DB.ExecContext(ctx, `INSERT INTO knowledge_unit_semantics(unit_id,schema_version,structure_json,quality_status,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(unit_id) DO UPDATE SET schema_version=excluded.schema_version,structure_json=excluded.structure_json,quality_status=excluded.quality_status,updated_at=excluded.updated_at`, unitID, KnowledgeUnitSchemaV2, string(raw), semanticQuality(structure), now, now)
	return err
}

func (e *Engine) hydrateKnowledgeUnits(ctx context.Context, units []KnowledgeUnit) error {
	if len(units) == 0 {
		return nil
	}
	ids := make([]string, 0, len(units))
	index := make(map[string]int, len(units))
	for i := range units {
		units[i].SchemaVersion = "memoryos.knowledge-unit/v1"
		ids = append(ids, units[i].UnitID)
		index[units[i].UnitID] = i
	}
	query := fmt.Sprintf(`SELECT unit_id,schema_version,structure_json FROM knowledge_unit_semantics WHERE unit_id IN (%s)`, placeholders(len(ids)))
	rows, err := e.control.DB.QueryContext(ctx, query, anyStrings(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, version, raw string
		if err := rows.Scan(&id, &version, &raw); err != nil {
			return err
		}
		position, ok := index[id]
		if !ok {
			continue
		}
		units[position].SchemaVersion = version
		if err := json.Unmarshal([]byte(raw), &units[position].Structure); err != nil {
			return fmt.Errorf("decode semantics for %s: %w", id, err)
		}
	}
	return rows.Err()
}

func (e *Engine) hydrateKnowledgeUnit(ctx context.Context, unit *KnowledgeUnit) error {
	items := []KnowledgeUnit{*unit}
	if err := e.hydrateKnowledgeUnits(ctx, items); err != nil {
		return err
	}
	*unit = items[0]
	return nil
}

func entityName(ref EntityRef) string {
	if strings.TrimSpace(ref.CanonicalName) != "" {
		return strings.TrimSpace(ref.CanonicalName)
	}
	return strings.TrimSpace(ref.Surface)
}

func (e *Engine) upsertSemanticEntity(ctx context.Context, projectID string, ref EntityRef, confidence float64) (string, bool, error) {
	name := entityName(ref)
	if name == "" {
		return "", false, nil
	}
	typeID := normalizedToken(ref.EntityType)
	if typeID == "" {
		typeID = "concept"
	}
	normalized := normalizeStatement(name)
	if normalized == "" {
		return "", false, nil
	}
	entityID := stableID("entity_", projectID, typeID, normalized)
	now := nowString()
	aliases := append([]string{name}, ref.Aliases...)
	result, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_entities(entity_id,project_id,entity_type,canonical_name,normalized_name,aliases_json,properties_json,status,confidence,created_at,updated_at) VALUES(?,?,?,?,?,?,'{}','active',?,?,?) ON CONFLICT(entity_id) DO UPDATE SET aliases_json=excluded.aliases_json,confidence=max(memory_entities.confidence,excluded.confidence),updated_at=excluded.updated_at`, entityID, projectID, typeID, name, normalized, jsonStrings(aliases), confidence, now, now)
	if err != nil {
		return "", false, err
	}
	created := false
	if changed, _ := result.RowsAffected(); changed > 0 {
		var createdAt string
		if scanErr := e.control.DB.QueryRowContext(ctx, `SELECT created_at FROM memory_entities WHERE entity_id=?`, entityID).Scan(&createdAt); scanErr == nil {
			created = createdAt == now
		}
	}
	return entityID, created, nil
}

// materializeSemanticUnit projects one authoritative Knowledge Unit without
// touching unrelated units. It returns the current assertion/fact IDs so an
// Owner correction can supersede stale projections after the replacement has
// been materialized successfully.
func (e *Engine) materializeSemanticUnit(ctx context.Context, projectID string, unit KnowledgeUnit, runID, stageID string) (SemanticMaterialization, string, string, error) {
	result := SemanticMaterialization{Units: 1}
	structure := unit.Structure
	if unit.SchemaVersion != KnowledgeUnitSchemaV2 || structure.Attribution.Resolution != "resolved" {
		result.AmbiguousUnits = 1
		return result, "", "", nil
	}
	subjectRef := structure.Frame.Subject
	if entityName(subjectRef) == "" {
		result.AmbiguousUnits = 1
		return result, "", "", nil
	}
	subjectID, created, err := e.upsertSemanticEntity(ctx, projectID, subjectRef, unit.Confidence)
	if err != nil {
		return result, "", "", err
	}
	if subjectID == "" {
		result.AmbiguousUnits = 1
		return result, "", "", nil
	}
	if created {
		result.Entities++
	}
	objectID := ""
	objectLiteral := strings.TrimSpace(structure.Frame.Object.Value)
	if entityName(structure.Frame.Object.Entity) != "" {
		objectID, created, err = e.upsertSemanticEntity(ctx, projectID, structure.Frame.Object.Entity, unit.Confidence)
		if err != nil {
			return result, "", "", err
		}
		if created {
			result.Entities++
		}
	}
	predicate := normalizedToken(structure.Frame.Predicate)
	if predicate == "" || (objectID == "" && objectLiteral == "") {
		return result, "", "", nil
	}
	assertionID := stableID("assertion_", projectID, unit.UnitID, subjectID, predicate, objectID, objectLiteral)
	now := nowString()
	insert, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_assertions(assertion_id,project_id,unit_id,subject_entity_id,predicate,inverse_label,object_entity_id,object_literal,status,valid_from,valid_until,recorded_at,confidence,source_evidence_ids_json,run_id,stage_id,created_at,updated_at) VALUES(?,?,?,?,?,?,nullif(?,''),?,'active',nullif(?,''),nullif(?,''),?,?,?,nullif(?,''),nullif(?,''),?,?) ON CONFLICT(assertion_id) DO UPDATE SET inverse_label=excluded.inverse_label,status='active',valid_from=excluded.valid_from,valid_until=excluded.valid_until,recorded_at=excluded.recorded_at,confidence=excluded.confidence,source_evidence_ids_json=excluded.source_evidence_ids_json,run_id=excluded.run_id,stage_id=excluded.stage_id,updated_at=excluded.updated_at`, assertionID, projectID, unit.UnitID, subjectID, predicate, normalizedToken(structure.Frame.InverseLabel), objectID, objectLiteral, structure.Temporal.ValidFrom, structure.Temporal.ValidUntil, now, unit.Confidence, jsonStrings([]string{unit.EvidenceID}), runID, stageID, now, now)
	if err != nil {
		return result, "", "", err
	}
	if changed, _ := insert.RowsAffected(); changed > 0 {
		result.Assertions++
	}
	factID := ""
	if structure.Temporal.Resolution == "resolved" && strings.TrimSpace(structure.Temporal.ValidFrom) != "" {
		factID = stableID("fact_", assertionID)
		objectText := objectLiteral
		if objectID != "" {
			if err := e.control.DB.QueryRowContext(ctx, `SELECT canonical_name FROM memory_entities WHERE entity_id=?`, objectID).Scan(&objectText); err != nil {
				return result, "", "", err
			}
		}
		_, err = e.control.DB.ExecContext(ctx, `INSERT INTO temporal_facts(fact_id,project_id,subject,predicate,object,status,observed_at,recorded_at,valid_from,valid_until,supersedes_fact_id,source_memory_id,source_evidence_ids_json,confidence) VALUES(?,?,?,?,?,'active',?,?,?,nullif(?,''),NULL,NULL,?,?) ON CONFLICT(fact_id) DO UPDATE SET object=excluded.object,status='active',observed_at=excluded.observed_at,recorded_at=excluded.recorded_at,valid_from=excluded.valid_from,valid_until=excluded.valid_until,source_evidence_ids_json=excluded.source_evidence_ids_json,confidence=excluded.confidence`, factID, projectID, entityName(subjectRef), predicate, objectText, unit.ObservedAt, now, structure.Temporal.ValidFrom, structure.Temporal.ValidUntil, jsonStrings([]string{unit.EvidenceID}), unit.Confidence)
		if err != nil {
			return result, "", "", err
		}
		result.TemporalFacts++
	}
	return result, assertionID, factID, nil
}

// MaterializeSemantics projects resolved Knowledge Unit v2 frames into the
// project-scoped semantic and temporal stores. Project-scoped structured KU
// objects override the legacy compatibility rows, so an approved Owner
// correction survives later automatic pipeline runs.
func (e *Engine) MaterializeSemantics(ctx context.Context, projectID, episodeID, runID, stageID string) (SemanticMaterialization, error) {
	units, err := e.ListKnowledgeUnits(ctx, episodeID, "", 500)
	if err != nil {
		return SemanticMaterialization{}, err
	}
	if err := e.overlayStructuredKnowledgeObjects(ctx, projectID, units); err != nil {
		return SemanticMaterialization{}, err
	}
	result := SemanticMaterialization{Units: len(units)}
	for _, unit := range units {
		partial, _, _, err := e.materializeSemanticUnit(ctx, projectID, unit, runID, stageID)
		if err != nil {
			return result, err
		}
		result.Entities += partial.Entities
		result.Assertions += partial.Assertions
		result.TemporalFacts += partial.TemporalFacts
		result.AmbiguousUnits += partial.AmbiguousUnits
	}
	return result, nil
}

// ReplaceKnowledgeUnitProjection incrementally refreshes only one corrected
// unit. New projection rows are materialized first; only after that succeeds
// are obsolete assertion/fact rows superseded, preserving readable history.
func (e *Engine) ReplaceKnowledgeUnitProjection(ctx context.Context, projectID string, unit KnowledgeUnit) (SemanticMaterialization, error) {
	projectID = strings.TrimSpace(projectID)
	unit = ensureKnowledgeUnitV2(unit)
	if projectID == "" {
		return SemanticMaterialization{}, errors.New("project_id is required")
	}
	if err := ValidateStructuredKnowledgeRevision(unit, unit); err != nil {
		return SemanticMaterialization{}, err
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT assertion_id,subject_entity_id,coalesce(object_entity_id,'') FROM memory_assertions WHERE project_id=? AND unit_id=? AND status='active'`, projectID, unit.UnitID)
	if err != nil {
		return SemanticMaterialization{}, err
	}
	type priorAssertion struct{ ID, SubjectID, ObjectID string }
	oldAssertions := []priorAssertion{}
	for rows.Next() {
		var item priorAssertion
		if err := rows.Scan(&item.ID, &item.SubjectID, &item.ObjectID); err != nil {
			rows.Close()
			return SemanticMaterialization{}, err
		}
		oldAssertions = append(oldAssertions, item)
	}
	if err := rows.Close(); err != nil {
		return SemanticMaterialization{}, err
	}

	result, currentAssertion, currentFact, err := e.materializeSemanticUnit(ctx, projectID, unit, "", "")
	if err != nil {
		return result, err
	}
	now := nowString()
	for _, prior := range oldAssertions {
		assertionID := prior.ID
		if assertionID != currentAssertion {
			if _, err := e.control.DB.ExecContext(ctx, `UPDATE memory_assertions SET status='superseded',valid_until=coalesce(valid_until,?),updated_at=? WHERE assertion_id=? AND status='active'`, now, now, assertionID); err != nil {
				return result, err
			}
		}
		factID := stableID("fact_", assertionID)
		if factID != currentFact {
			if _, err := e.control.DB.ExecContext(ctx, `UPDATE temporal_facts SET status='superseded',valid_until=coalesce(valid_until,?) WHERE fact_id=? AND status='active'`, now, factID); err != nil {
				return result, err
			}
		}
		for _, entityID := range []string{prior.SubjectID, prior.ObjectID} {
			if entityID == "" {
				continue
			}
			if _, err := e.control.DB.ExecContext(ctx, `UPDATE memory_entities SET status='superseded',updated_at=? WHERE project_id=? AND entity_id=? AND status='active' AND NOT EXISTS (SELECT 1 FROM memory_assertions a WHERE a.project_id=? AND a.status='active' AND (a.subject_entity_id=? OR a.object_entity_id=?))`, now, projectID, entityID, projectID, entityID, entityID); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func (e *Engine) SemanticGraph(ctx context.Context, projectID string, limit int) (Graph, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Graph{}, errors.New("project_id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 80
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT a.assertion_id,a.subject_entity_id,s.canonical_name,s.entity_type,s.status,s.confidence,a.predicate,a.inverse_label,coalesce(a.object_entity_id,''),coalesce(o.canonical_name,''),coalesce(o.entity_type,''),coalesce(o.status,''),coalesce(o.confidence,0),a.object_literal,a.confidence,coalesce(json_extract(a.source_evidence_ids_json,'$[0]'),'') FROM memory_assertions a JOIN memory_entities s ON s.entity_id=a.subject_entity_id LEFT JOIN memory_entities o ON o.entity_id=a.object_entity_id WHERE a.project_id=? AND a.status='active' ORDER BY a.updated_at DESC,a.assertion_id LIMIT ?`, projectID, limit)
	if err != nil {
		return Graph{}, err
	}
	defer rows.Close()
	nodes := map[string]GraphNode{}
	edges := []GraphEdge{}
	for rows.Next() {
		var assertionID, subjectID, subjectName, subjectType, subjectStatus, predicate, inverse, objectID, objectName, objectType, objectStatus, objectLiteral, evidenceID string
		var subjectConfidence, objectConfidence, assertionConfidence float64
		if err := rows.Scan(&assertionID, &subjectID, &subjectName, &subjectType, &subjectStatus, &subjectConfidence, &predicate, &inverse, &objectID, &objectName, &objectType, &objectStatus, &objectConfidence, &objectLiteral, &assertionConfidence, &evidenceID); err != nil {
			return Graph{}, err
		}
		nodes[subjectID] = GraphNode{ID: subjectID, Layer: "entity", Label: subjectName, Status: subjectStatus, EntityType: subjectType, Confidence: subjectConfidence}
		targetID := objectID
		if targetID != "" {
			nodes[targetID] = GraphNode{ID: targetID, Layer: "entity", Label: objectName, Status: objectStatus, EntityType: objectType, Confidence: objectConfidence}
		} else {
			targetID = "literal:" + assertionID
			nodes[targetID] = GraphNode{ID: targetID, Layer: "literal", Label: objectLiteral, Status: "active", EntityType: "literal", Confidence: assertionConfidence}
		}
		edges = append(edges, GraphEdge{ID: assertionID, From: subjectID, To: targetID, Kind: "semantic", Label: predicate, InverseLabel: inverse, Confidence: assertionConfidence, EvidenceID: evidenceID})
	}
	if err := rows.Err(); err != nil {
		return Graph{}, err
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	graph := Graph{Nodes: make([]GraphNode, 0, len(ids)), Edges: edges}
	for _, id := range ids {
		graph.Nodes = append(graph.Nodes, nodes[id])
	}
	return graph, nil
}

func (e *Engine) SemanticCounts(ctx context.Context, projectID string) (entities, assertions int, err error) {
	if err = e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_entities WHERE project_id=?`, projectID).Scan(&entities); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	err = e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_assertions WHERE project_id=?`, projectID).Scan(&assertions)
	return entities, assertions, err
}
