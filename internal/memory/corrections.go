package memory

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

type CorrectionImpact struct {
	UnitIDs              []string       `json:"unit_ids"`
	AssertionIDs         []string       `json:"assertion_ids"`
	TemporalFactIDs      []string       `json:"temporal_fact_ids"`
	EvidenceIDs          []string       `json:"evidence_ids"`
	AuthorityObjectIDs   []string       `json:"authority_object_ids"`
	CurrentRevisions     map[string]int `json:"current_revisions"`
	ProjectProjectionIDs []string       `json:"project_projection_ids"`
}

type CorrectionTarget struct {
	ProjectID string           `json:"project_id"`
	Kind      string           `json:"kind"`
	TargetID  string           `json:"target_id"`
	Label     string           `json:"label"`
	Impact    CorrectionImpact `json:"impact"`
}

func appendStringUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (e *Engine) PreviewCorrection(ctx context.Context, projectID, kind, targetID string) (CorrectionTarget, error) {
	projectID, kind, targetID = strings.TrimSpace(projectID), strings.TrimSpace(kind), strings.TrimSpace(targetID)
	if projectID == "" || targetID == "" {
		return CorrectionTarget{}, errors.New("project_id and target_id are required")
	}
	if kind != "knowledge_unit" && kind != "entity" && kind != "assertion" && kind != "temporal_fact" {
		return CorrectionTarget{}, errors.New("kind must be knowledge_unit, entity, assertion or temporal_fact")
	}
	out := CorrectionTarget{ProjectID: projectID, Kind: kind, TargetID: targetID, Impact: CorrectionImpact{CurrentRevisions: map[string]int{}}}
	unitIDs := []string{}
	switch kind {
	case "knowledge_unit":
		unitIDs = append(unitIDs, targetID)
		unit, err := e.KnowledgeUnitForProject(ctx, projectID, targetID)
		if err != nil {
			return CorrectionTarget{}, err
		}
		out.Label = unit.Statement
	case "assertion":
		var unitID, predicate string
		if err := e.control.DB.QueryRowContext(ctx, `SELECT unit_id,predicate FROM memory_assertions WHERE project_id=? AND assertion_id=? AND status='active'`, projectID, targetID).Scan(&unitID, &predicate); err != nil {
			return CorrectionTarget{}, err
		}
		unitIDs = append(unitIDs, unitID)
		out.Label = predicate
	case "entity":
		if err := e.control.DB.QueryRowContext(ctx, `SELECT canonical_name FROM memory_entities WHERE project_id=? AND entity_id=?`, projectID, targetID).Scan(&out.Label); err != nil {
			return CorrectionTarget{}, err
		}
		rows, err := e.control.DB.QueryContext(ctx, `SELECT DISTINCT unit_id FROM memory_assertions WHERE project_id=? AND status='active' AND (subject_entity_id=? OR object_entity_id=?) ORDER BY unit_id`, projectID, targetID, targetID)
		if err != nil {
			return CorrectionTarget{}, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return CorrectionTarget{}, err
			}
			unitIDs = appendStringUnique(unitIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CorrectionTarget{}, err
		}
		rows.Close()
	case "temporal_fact":
		var subject, predicate, object string
		if err := e.control.DB.QueryRowContext(ctx, `SELECT subject,predicate,object FROM temporal_facts WHERE project_id=? AND fact_id=? AND status='active'`, projectID, targetID).Scan(&subject, &predicate, &object); err != nil {
			return CorrectionTarget{}, err
		}
		out.Label = subject + " · " + predicate + " · " + object
		rows, err := e.control.DB.QueryContext(ctx, `SELECT assertion_id,unit_id FROM memory_assertions WHERE project_id=?`, projectID)
		if err != nil {
			return CorrectionTarget{}, err
		}
		for rows.Next() {
			var assertionID, unitID string
			if err := rows.Scan(&assertionID, &unitID); err != nil {
				rows.Close()
				return CorrectionTarget{}, err
			}
			if stableID("fact_", assertionID) == targetID {
				unitIDs = appendStringUnique(unitIDs, unitID)
				break
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CorrectionTarget{}, err
		}
		rows.Close()
		if len(unitIDs) == 0 {
			return CorrectionTarget{}, sql.ErrNoRows
		}
	}

	for _, unitID := range unitIDs {
		unit, err := e.KnowledgeUnitForProject(ctx, projectID, unitID)
		if err != nil {
			return CorrectionTarget{}, err
		}
		out.Impact.UnitIDs = appendStringUnique(out.Impact.UnitIDs, unitID)
		out.Impact.EvidenceIDs = appendStringUnique(out.Impact.EvidenceIDs, unit.EvidenceID)
		objectID := StructuredKnowledgeObjectID(projectID, unitID)
		var revision int
		err = e.control.DB.QueryRowContext(ctx, `SELECT current_revision FROM harness_objects WHERE object_id=? AND project_id=?`, objectID, projectID).Scan(&revision)
		if err == nil {
			out.Impact.AuthorityObjectIDs = appendStringUnique(out.Impact.AuthorityObjectIDs, objectID)
			out.Impact.CurrentRevisions[objectID] = revision
		} else if err != sql.ErrNoRows {
			return CorrectionTarget{}, err
		}
		assertionRows, err := e.control.DB.QueryContext(ctx, `SELECT assertion_id FROM memory_assertions WHERE project_id=? AND unit_id=? AND status='active' ORDER BY assertion_id`, projectID, unitID)
		if err != nil {
			return CorrectionTarget{}, err
		}
		unitAssertionIDs := []string{}
		for assertionRows.Next() {
			var assertionID string
			if err := assertionRows.Scan(&assertionID); err != nil {
				assertionRows.Close()
				return CorrectionTarget{}, err
			}
			unitAssertionIDs = append(unitAssertionIDs, assertionID)
		}
		if err := assertionRows.Err(); err != nil {
			assertionRows.Close()
			return CorrectionTarget{}, err
		}
		assertionRows.Close()
		for _, assertionID := range unitAssertionIDs {
			out.Impact.AssertionIDs = appendStringUnique(out.Impact.AssertionIDs, assertionID)
			factID := stableID("fact_", assertionID)
			var n int
			if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM temporal_facts WHERE project_id=? AND fact_id=?`, projectID, factID).Scan(&n); err != nil {
				return CorrectionTarget{}, err
			}
			if n > 0 {
				out.Impact.TemporalFactIDs = appendStringUnique(out.Impact.TemporalFactIDs, factID)
			}
		}

		for _, candidate := range []struct{ table, prefix string }{{"project_goals", "auto-goal-"}, {"project_risks", "auto-risk-"}, {"project_decisions", "auto-decision-"}} {
			id := stableID(candidate.prefix, projectID, unitID)
			var n int
			query := `SELECT count(*) FROM ` + candidate.table + ` WHERE project_id=? AND ` + map[string]string{"project_goals": "goal_id", "project_risks": "risk_id", "project_decisions": "decision_id"}[candidate.table] + `=?`
			if err := e.control.DB.QueryRowContext(ctx, query, projectID, id).Scan(&n); err != nil {
				return CorrectionTarget{}, err
			}
			if n > 0 {
				out.Impact.ProjectProjectionIDs = appendStringUnique(out.Impact.ProjectProjectionIDs, id)
			}
		}
	}
	sort.Strings(out.Impact.UnitIDs)
	sort.Strings(out.Impact.AssertionIDs)
	sort.Strings(out.Impact.TemporalFactIDs)
	sort.Strings(out.Impact.EvidenceIDs)
	sort.Strings(out.Impact.AuthorityObjectIDs)
	sort.Strings(out.Impact.ProjectProjectionIDs)
	return out, nil
}

type CorrectionEntity struct {
	EntityID      string   `json:"entity_id"`
	ProjectID     string   `json:"project_id"`
	EntityType    string   `json:"entity_type"`
	CanonicalName string   `json:"canonical_name"`
	Aliases       []string `json:"aliases"`
	Status        string   `json:"status"`
}

func (e *Engine) CorrectionEntity(ctx context.Context, projectID, entityID string) (CorrectionEntity, error) {
	var item CorrectionEntity
	var aliases string
	err := e.control.DB.QueryRowContext(ctx, `SELECT entity_id,project_id,entity_type,canonical_name,aliases_json,status FROM memory_entities WHERE project_id=? AND entity_id=?`, strings.TrimSpace(projectID), strings.TrimSpace(entityID)).Scan(&item.EntityID, &item.ProjectID, &item.EntityType, &item.CanonicalName, &aliases, &item.Status)
	if err != nil {
		return item, err
	}
	item.Aliases = decodeStrings(aliases)
	return item, nil
}

func (e *Engine) EntityRolesForUnit(ctx context.Context, projectID, entityID, unitID string) (subject, object bool, err error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT subject_entity_id,coalesce(object_entity_id,'') FROM memory_assertions WHERE project_id=? AND unit_id=? AND status='active'`, strings.TrimSpace(projectID), strings.TrimSpace(unitID))
	if err != nil {
		return false, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var subjectID, objectID string
		if err := rows.Scan(&subjectID, &objectID); err != nil {
			return false, false, err
		}
		subject = subject || subjectID == entityID
		object = object || objectID == entityID
	}
	return subject, object, rows.Err()
}
