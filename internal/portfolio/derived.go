package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// DerivedProjection reports which ordinary project views were materialized
// from one compiled Episode. These projections are rebuildable; the source
// Evidence and memory records remain authoritative.
type DerivedProjection struct {
	ContextBlocks int `json:"context_blocks"`
	Goals         int `json:"goals"`
	Decisions     int `json:"decisions"`
	Risks         int `json:"risks"`
}

type derivedUnit struct {
	UnitID, EvidenceID, UnitType, Statement, ObservedAt, TargetAt string
	Confidence                                                    float64
}

func concise(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}

// SyncDerivedFromEpisode makes the default import useful without requiring an
// owner to open the advanced pipeline editor. Only explicit typed candidates
// become structured goals, decisions or risks. Everything else is summarized
// into one read-only, source-bearing context block.
func (s *Service) SyncDerivedFromEpisode(ctx context.Context, projectID, episodeID string) (DerivedProjection, error) {
	if err := s.projectExists(ctx, projectID); err != nil {
		return DerivedProjection{}, err
	}
	var evidenceRaw string
	if err := s.control.DB.QueryRowContext(ctx, `SELECT evidence_ids_json FROM episodes WHERE episode_id=?`, strings.TrimSpace(episodeID)).Scan(&evidenceRaw); err != nil {
		return DerivedProjection{}, err
	}
	// The legacy knowledge_units row is a compatibility projection. If a
	// project-scoped structured KU object exists, its current Revision is the
	// editable authority for type/statement/confidence while Evidence identity
	// and observed time remain anchored to the immutable source row.
	rows, err := s.control.DB.QueryContext(ctx, `SELECT u.unit_id,u.evidence_id,
		coalesce(json_extract(r.payload_json,'$.unit_type'),u.unit_type),
		coalesce(json_extract(r.payload_json,'$.statement'),u.statement),
		coalesce(json_extract(r.payload_json,'$.confidence'),u.confidence),u.observed_at,
		coalesce(json_extract(r.payload_json,'$.structure.temporal.occurred_from'),json_extract(r.payload_json,'$.structure.temporal.valid_from'),'')
		FROM knowledge_units u
		LEFT JOIN harness_objects o ON o.project_id=? AND o.type_id='builtin.core-memory-growth.knowledge-unit.v2' AND o.status='active'
			AND EXISTS (SELECT 1 FROM harness_object_revisions r0 WHERE r0.object_id=o.object_id AND r0.revision=o.current_revision AND json_extract(r0.payload_json,'$.unit_id')=u.unit_id)
		LEFT JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision
		WHERE u.episode_id=? AND u.status IN ('consolidated','attached','review_required')
		ORDER BY coalesce(json_extract(r.payload_json,'$.confidence'),u.confidence) DESC,u.unit_id`, projectID, episodeID)
	if err != nil {
		return DerivedProjection{}, err
	}
	units := []derivedUnit{}
	for rows.Next() {
		var item derivedUnit
		if err := rows.Scan(&item.UnitID, &item.EvidenceID, &item.UnitType, &item.Statement, &item.Confidence, &item.ObservedAt, &item.TargetAt); err != nil {
			rows.Close()
			return DerivedProjection{}, err
		}
		if item.Confidence >= 0.72 {
			units = append(units, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return DerivedProjection{}, err
	}
	rows.Close()

	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return DerivedProjection{}, err
	}
	defer tx.Rollback()
	// Reprocessing one source replaces only its previous automatic project
	// projection; manually authored project records have different IDs.
	for _, statement := range []string{
		`DELETE FROM project_goals WHERE project_id=? AND goal_id LIKE 'auto-goal-%' AND source_evidence_id IN (SELECT value FROM json_each(?))`,
		`DELETE FROM project_risks WHERE project_id=? AND risk_id LIKE 'auto-risk-%' AND source_evidence_id IN (SELECT value FROM json_each(?))`,
		`DELETE FROM project_decisions WHERE project_id=? AND decision_id LIKE 'auto-decision-%' AND EXISTS(SELECT 1 FROM json_each(project_decisions.source_evidence_ids_json) src WHERE src.value IN (SELECT value FROM json_each(?)))`,
	} {
		if _, err := tx.ExecContext(ctx, statement, projectID, evidenceRaw); err != nil {
			return DerivedProjection{}, err
		}
	}

	projection := DerivedProjection{}
	for _, unit := range units {
		switch unit.UnitType {
		case "goal":
			id := stableID("auto-goal-", projectID, unit.UnitID)
			_, err = tx.ExecContext(ctx, `INSERT INTO project_goals(goal_id,project_id,title,description,status,priority,target_at,source_evidence_id,created_at,updated_at) VALUES(?,?,?,?,'active',3,nullif(?,''),?,?,?) ON CONFLICT(goal_id) DO UPDATE SET title=excluded.title,description=excluded.description,target_at=excluded.target_at,source_evidence_id=excluded.source_evidence_id,updated_at=excluded.updated_at`, id, projectID, concise(unit.Statement, 120), "由记忆沉淀自动识别，可在项目工作台继续编辑和确认。", unit.TargetAt, unit.EvidenceID, now, now)
			if err == nil {
				taskID := stableID("suggested-task-", projectID, id)
				_, err = tx.ExecContext(ctx, `INSERT INTO project_tasks(task_id,project_id,title,description,status,priority,due_at,source_kind,source_record_id,source_evidence_ids_json,created_at,updated_at)
					VALUES(?,?,?,?,'suggested',3,nullif(?,''),'ai_suggestion',?,?,?,?)
					ON CONFLICT(project_id,source_kind,source_record_id) DO UPDATE SET title=excluded.title,description=excluded.description,due_at=excluded.due_at,source_evidence_ids_json=excluded.source_evidence_ids_json,updated_at=excluded.updated_at`,
					taskID, projectID, concise(unit.Statement, 120), "AI 从原材料中识别出的行动建议；确认后才会进入正式待办。", unit.TargetAt, id, encodeStrings([]string{unit.EvidenceID}), now, now)
			}
			projection.Goals++
		case "decision":
			id := stableID("auto-decision-", projectID, unit.UnitID)
			_, err = tx.ExecContext(ctx, `INSERT INTO project_decisions(decision_id,project_id,title,decision,rationale,status,decided_at,source_evidence_ids_json,created_at) VALUES(?,?,?,?,?,'active',?,?,?) ON CONFLICT(decision_id) DO UPDATE SET title=excluded.title,decision=excluded.decision,rationale=excluded.rationale,source_evidence_ids_json=excluded.source_evidence_ids_json`, id, projectID, concise(unit.Statement, 100), unit.Statement, "由有来源的记忆知识点自动沉淀。", func() string {
				if unit.TargetAt != "" {
					return unit.TargetAt
				}
				return unit.ObservedAt
			}(), encodeStrings([]string{unit.EvidenceID}), now)
			projection.Decisions++
		case "risk":
			id := stableID("auto-risk-", projectID, unit.UnitID)
			_, err = tx.ExecContext(ctx, `INSERT INTO project_risks(risk_id,project_id,title,description,probability,impact,status,mitigation,owner,source_evidence_id,created_at,updated_at) VALUES(?,?,?,?,3,3,'open','','',?,?,?) ON CONFLICT(risk_id) DO UPDATE SET title=excluded.title,description=excluded.description,source_evidence_id=excluded.source_evidence_id,updated_at=excluded.updated_at`, id, projectID, concise(unit.Statement, 120), unit.Statement, unit.EvidenceID, now, now)
			projection.Risks++
		}
		if err != nil {
			return DerivedProjection{}, err
		}
	}

	if len(units) > 0 {
		sort.SliceStable(units, func(i, j int) bool {
			priority := map[string]int{"decision": 0, "goal": 1, "risk": 2, "procedure": 3, "identity": 4, "state": 5, "outcome": 6, "fact": 7}
			return priority[units[i].UnitType] < priority[units[j].UnitType]
		})
		if len(units) > 16 {
			units = units[:16]
		}
		var content strings.Builder
		content.WriteString("这份上下文由当前项目的真实 Evidence 自动沉淀，按重要性保留：\n")
		sources := []string{}
		for _, unit := range units {
			fmt.Fprintf(&content, "\n- [%s] %s", unit.UnitType, unit.Statement)
			sources = append(sources, unit.EvidenceID)
		}
		blockID := stableID("block-", projectID, "自动沉淀摘要")
		_, err = tx.ExecContext(ctx, `INSERT INTO context_blocks(block_id,project_id,label,description,content,budget_chars,read_only,status,source_refs_json,created_at,updated_at) VALUES(?,?, '自动沉淀摘要','导入后自动生成；原始来源与结构化记录仍可独立检查。',?,8000,1,'active',?,?,?) ON CONFLICT(project_id,label) DO UPDATE SET description=excluded.description,content=excluded.content,budget_chars=excluded.budget_chars,read_only=excluded.read_only,source_refs_json=excluded.source_refs_json,updated_at=excluded.updated_at`, blockID, projectID, content.String(), encodeStrings(sources), now, now)
		if err != nil {
			return DerivedProjection{}, err
		}
		projection.ContextBlocks = 1
	}
	if err := tx.Commit(); err != nil {
		return DerivedProjection{}, err
	}
	return projection, nil
}
