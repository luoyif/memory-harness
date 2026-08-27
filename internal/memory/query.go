package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func boundedLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func boundedOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func (e *Engine) Overview(ctx context.Context) (Overview, error) {
	type countSpec struct {
		query string
		args  []any
	}
	counts := make([]int, 6)
	specs := []countSpec{
		{`SELECT count(*) FROM evidence_receipts`, nil},
		{`SELECT count(*) FROM knowledge_units`, nil},
		{`SELECT count(*) FROM episodes`, nil},
		{`SELECT count(*) FROM memory_records`, nil},
		{`SELECT count(*) FROM living_views`, nil},
		{`SELECT count(*) FROM agent_assets`, nil},
	}
	for i, spec := range specs {
		if err := e.control.DB.QueryRowContext(ctx, spec.query, spec.args...).Scan(&counts[i]); err != nil {
			return Overview{}, err
		}
	}
	layers := []LayerSummary{
		{0, "evidence", "Evidence", "原始证据", "不可变的对话、文件、操作与产物。", counts[0], "canonical"},
		{1, "knowledge", "Knowledge Units", "知识点", "带精确来源的事实、偏好、决定、目标、状态与方法候选。", counts[1], "derived"},
		{2, "episodes", "Episodes", "会话复盘", "一次会话的目标、行动、结果与遗留问题。", counts[2], "compiled"},
		{3, "memory", "Memory Records", "长期记忆", "跨会话沉淀的情景、语义、程序与身份记忆。", counts[3], "governed"},
		{4, "living", "Living Knowledge", "活知识", "Memory Index、Hot Index、Active Context 与可重建视图。", counts[4], "projected"},
		{5, "assets", "Agent Assets", "能力资产", "由受保护记忆提出的 Procedure、Skill、Rule 等候选。", counts[5], "protected"},
	}
	var review, jobs int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_operations WHERE status='review_required'`).Scan(&review); err != nil {
		return Overview{}, err
	}
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE status='completed'`).Scan(&jobs); err != nil {
		return Overview{}, err
	}
	compiler := CompilerVersion
	if e.extractor != nil {
		if configured := strings.TrimSpace(e.extractor.Compiler(ctx)); configured != "" {
			compiler = configured
		}
	}
	return Overview{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Compiler: compiler, Policy: PolicyVersion, Layers: layers, NeedsReview: review, CompletedJobs: jobs}, nil
}

// OverviewForProject returns layer counts for one project without loading the
// layer contents. record_projects is the authoritative project boundary.
func (e *Engine) OverviewForProject(ctx context.Context, projectID string) (Overview, error) {
	counts := make([]int, 6)
	recordTypes := []string{"evidence", "knowledge_unit", "episode", "memory", "living", "asset"}
	for i, recordType := range recordTypes {
		if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE project_id=? AND record_type=?`, projectID, recordType).Scan(&counts[i]); err != nil {
			return Overview{}, err
		}
	}
	layers := []LayerSummary{
		{0, "evidence", "Evidence", "原始证据", "不可变的对话、文件、操作与产物。", counts[0], "canonical"},
		{1, "knowledge", "Knowledge Units", "知识点", "带精确来源的事实、偏好、决定、目标、状态与方法候选。", counts[1], "derived"},
		{2, "episodes", "Episodes", "会话复盘", "一次会话的目标、行动、结果与遗留问题。", counts[2], "compiled"},
		{3, "memory", "Memory Records", "长期记忆", "跨会话沉淀的情景、语义、程序与身份记忆。", counts[3], "governed"},
		{4, "living", "Living Knowledge", "活知识", "Memory Index、Hot Index、Active Context 与可重建视图。", counts[4], "projected"},
		{5, "assets", "Agent Assets", "能力资产", "由受保护记忆提出的 Procedure、Skill、Rule 等候选。", counts[5], "protected"},
	}
	var review int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_operations o JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=o.target_memory_id WHERE rp.project_id=? AND o.status='review_required'`, projectID).Scan(&review); err != nil {
		return Overview{}, err
	}
	compiler := CompilerVersion
	if e.extractor != nil {
		if configured := strings.TrimSpace(e.extractor.Compiler(ctx)); configured != "" {
			compiler = configured
		}
	}
	return Overview{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Compiler: compiler, Policy: PolicyVersion, Layers: layers, NeedsReview: review}, nil
}

func (e *Engine) ListEpisodes(ctx context.Context, limit int) ([]Episode, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT episode_id,session_id,source_system,title,summary,status,evidence_ids_json,started_at,ended_at,compiler,revision,created_at,updated_at,(SELECT count(*) FROM knowledge_units u WHERE u.episode_id=episodes.episode_id) FROM episodes ORDER BY ended_at DESC LIMIT ?`, boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		var item Episode
		var evidence string
		if err := rows.Scan(&item.EpisodeID, &item.SessionID, &item.SourceSystem, &item.Title, &item.Summary, &item.Status, &evidence, &item.StartedAt, &item.EndedAt, &item.Compiler, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &item.Units); err != nil {
			return nil, err
		}
		item.EvidenceIDs = decodeStrings(evidence)
		out = append(out, item)
	}
	if out == nil {
		out = []Episode{}
	}
	return out, rows.Err()
}

func (e *Engine) ListEpisodesForProject(ctx context.Context, projectID string, limit, offset int) ([]Episode, int, error) {
	var total int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM episodes e JOIN record_projects rp ON rp.record_type='episode' AND rp.record_id=e.episode_id WHERE rp.project_id=?`, projectID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT e.episode_id,e.session_id,e.source_system,e.title,e.summary,e.status,e.evidence_ids_json,e.started_at,e.ended_at,e.compiler,e.revision,e.created_at,e.updated_at,(SELECT count(*) FROM knowledge_units u WHERE u.episode_id=e.episode_id) FROM episodes e JOIN record_projects rp ON rp.record_type='episode' AND rp.record_id=e.episode_id WHERE rp.project_id=? ORDER BY e.ended_at DESC LIMIT ? OFFSET ?`, projectID, boundedLimit(limit), boundedOffset(offset))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Episode{}
	for rows.Next() {
		var item Episode
		var evidence string
		if err := rows.Scan(&item.EpisodeID, &item.SessionID, &item.SourceSystem, &item.Title, &item.Summary, &item.Status, &evidence, &item.StartedAt, &item.EndedAt, &item.Compiler, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &item.Units); err != nil {
			return nil, 0, err
		}
		item.EvidenceIDs = decodeStrings(evidence)
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (e *Engine) episode(ctx context.Context, episodeID string) (Episode, error) {
	var item Episode
	var evidence string
	err := e.control.DB.QueryRowContext(ctx, `SELECT episode_id,session_id,source_system,title,summary,status,evidence_ids_json,started_at,ended_at,compiler,revision,created_at,updated_at,(SELECT count(*) FROM knowledge_units u WHERE u.episode_id=episodes.episode_id) FROM episodes WHERE episode_id=?`, episodeID).
		Scan(&item.EpisodeID, &item.SessionID, &item.SourceSystem, &item.Title, &item.Summary, &item.Status, &evidence, &item.StartedAt, &item.EndedAt, &item.Compiler, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &item.Units)
	item.EvidenceIDs = decodeStrings(evidence)
	return item, err
}

func (e *Engine) Episode(ctx context.Context, episodeID string) (Episode, error) {
	return e.episode(ctx, episodeID)
}

func (e *Engine) ListKnowledgeUnits(ctx context.Context, episodeID, unitType string, limit int) ([]KnowledgeUnit, error) {
	query := `SELECT unit_id,episode_id,evidence_id,unit_type,tier_hint,statement,normalized_key,confidence,risk_tier,status,scope_json,observed_at,created_at,coalesce(processed_at,'') FROM knowledge_units WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(episodeID) != "" {
		query += ` AND episode_id=?`
		args = append(args, episodeID)
	}
	if strings.TrimSpace(unitType) != "" {
		query += ` AND unit_type=?`
		args = append(args, unitType)
	}
	query += ` ORDER BY observed_at DESC,unit_id LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeUnit
	for rows.Next() {
		var item KnowledgeUnit
		var scopes string
		if err := rows.Scan(&item.UnitID, &item.EpisodeID, &item.EvidenceID, &item.UnitType, &item.TierHint, &item.Statement, &item.NormalizedKey, &item.Confidence, &item.RiskTier, &item.Status, &scopes, &item.ObservedAt, &item.CreatedAt, &item.ProcessedAt); err != nil {
			return nil, err
		}
		item.Scopes = decodeStrings(scopes)
		out = append(out, item)
	}
	if out == nil {
		out = []KnowledgeUnit{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := e.hydrateKnowledgeUnits(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) ListKnowledgeUnitsForProject(ctx context.Context, projectID, unitType string, limit, offset int) ([]KnowledgeUnit, int, error) {
	where := ` FROM knowledge_units u JOIN record_projects rp ON rp.record_type='knowledge_unit' AND rp.record_id=u.unit_id WHERE rp.project_id=?`
	args := []any{projectID}
	if strings.TrimSpace(unitType) != "" {
		where += ` AND u.unit_type=?`
		args = append(args, unitType)
	}
	var total int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT u.unit_id,u.episode_id,u.evidence_id,u.unit_type,u.tier_hint,u.statement,u.normalized_key,u.confidence,u.risk_tier,u.status,u.scope_json,u.observed_at,u.created_at,coalesce(u.processed_at,'')` + where + ` ORDER BY u.observed_at DESC,u.unit_id LIMIT ? OFFSET ?`
	args = append(args, boundedLimit(limit), boundedOffset(offset))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []KnowledgeUnit{}
	for rows.Next() {
		var item KnowledgeUnit
		var scopes string
		if err := rows.Scan(&item.UnitID, &item.EpisodeID, &item.EvidenceID, &item.UnitType, &item.TierHint, &item.Statement, &item.NormalizedKey, &item.Confidence, &item.RiskTier, &item.Status, &scopes, &item.ObservedAt, &item.CreatedAt, &item.ProcessedAt); err != nil {
			return nil, 0, err
		}
		item.Scopes = decodeStrings(scopes)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := e.hydrateKnowledgeUnits(ctx, out); err != nil {
		return nil, 0, err
	}
	if err := e.overlayStructuredKnowledgeObjects(ctx, projectID, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (e *Engine) ListMemories(ctx context.Context, tier, status string, limit int) ([]MemoryRecord, error) {
	query := `SELECT memory_id,tier,asset_form,domain,status,summary,body,canonical_key,confidence,importance,strength,evidence_ids_json,episode_ids_json,scopes_json,visibility,observed_at,created_at,updated_at,coalesce(last_reinforced_at,'') FROM memory_records WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(tier) != "" {
		query += ` AND tier=?`
		args = append(args, tier)
	}
	if strings.TrimSpace(status) != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC,memory_id LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryRecord
	for rows.Next() {
		item, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []MemoryRecord{}
	}
	return out, rows.Err()
}

func (e *Engine) ListMemoriesForProject(ctx context.Context, projectID, tier, status string, limit, offset int) ([]MemoryRecord, int, error) {
	// Project-scoped structured memory objects are the editable authority. The
	// legacy memory_records table remains a compatibility projection, so every
	// filter and page must use the current Revision when it exists rather than
	// filtering the stale legacy tier/status first.
	where := ` FROM memory_records m
		JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=m.memory_id
		LEFT JOIN harness_objects o ON o.project_id=? AND o.type_id=? AND o.status='active'
			AND EXISTS (SELECT 1 FROM harness_object_revisions r0 WHERE r0.object_id=o.object_id AND r0.revision=o.current_revision AND json_extract(r0.payload_json,'$.memory_id')=m.memory_id)
		LEFT JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision
		WHERE rp.project_id=?`
	args := []any{projectID, StructuredMemoryRecordTypeV1, projectID}
	if strings.TrimSpace(tier) != "" {
		where += ` AND coalesce(json_extract(r.payload_json,'$.tier'),m.tier)=?`
		args = append(args, tier)
	}
	if strings.TrimSpace(status) != "" {
		where += ` AND coalesce(json_extract(r.payload_json,'$.status'),m.status)=?`
		args = append(args, status)
	}
	var total int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT m.memory_id,
		coalesce(json_extract(r.payload_json,'$.tier'),m.tier),
		coalesce(json_extract(r.payload_json,'$.asset_form'),m.asset_form),
		coalesce(json_extract(r.payload_json,'$.domain'),m.domain),
		coalesce(json_extract(r.payload_json,'$.status'),m.status),
		coalesce(json_extract(r.payload_json,'$.summary'),m.summary),
		coalesce(json_extract(r.payload_json,'$.body'),m.body),
		coalesce(json_extract(r.payload_json,'$.canonical_key'),m.canonical_key),
		coalesce(json_extract(r.payload_json,'$.confidence'),m.confidence),
		coalesce(json_extract(r.payload_json,'$.importance'),m.importance),
		coalesce(json_extract(r.payload_json,'$.strength'),m.strength),
		coalesce(json_extract(r.payload_json,'$.source_evidence_ids'),m.evidence_ids_json),
		coalesce(json_extract(r.payload_json,'$.source_episode_ids'),m.episode_ids_json),
		coalesce(json_extract(r.payload_json,'$.scopes'),m.scopes_json),
		coalesce(json_extract(r.payload_json,'$.visibility'),m.visibility),
		coalesce(json_extract(r.payload_json,'$.observed_at'),m.observed_at),
		coalesce(json_extract(r.payload_json,'$.created_at'),m.created_at),
		coalesce(json_extract(r.payload_json,'$.updated_at'),m.updated_at),
		coalesce(json_extract(r.payload_json,'$.last_reinforced_at'),m.last_reinforced_at,'')` + where + ` ORDER BY coalesce(json_extract(r.payload_json,'$.updated_at'),m.updated_at) DESC,m.memory_id LIMIT ? OFFSET ?`
	args = append(args, boundedLimit(limit), boundedOffset(offset))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []MemoryRecord{}
	for rows.Next() {
		item, err := scanMemory(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanMemory(row scanner) (MemoryRecord, error) {
	var item MemoryRecord
	var evidence, episodes, scopes string
	err := row.Scan(&item.MemoryID, &item.Tier, &item.AssetForm, &item.Domain, &item.Status, &item.Summary, &item.Body, &item.CanonicalKey, &item.Confidence, &item.Importance, &item.Strength, &evidence, &episodes, &scopes, &item.Visibility, &item.ObservedAt, &item.CreatedAt, &item.UpdatedAt, &item.LastReinforcedAt)
	item.EvidenceIDs = decodeStrings(evidence)
	item.EpisodeIDs = decodeStrings(episodes)
	item.Scopes = decodeStrings(scopes)
	return item, err
}

func (e *Engine) Memory(ctx context.Context, memoryID string) (MemoryRecord, error) {
	row := e.control.DB.QueryRowContext(ctx, `SELECT memory_id,tier,asset_form,domain,status,summary,body,canonical_key,confidence,importance,strength,evidence_ids_json,episode_ids_json,scopes_json,visibility,observed_at,created_at,updated_at,coalesce(last_reinforced_at,'') FROM memory_records WHERE memory_id=?`, memoryID)
	return scanMemory(row)
}

func (e *Engine) ListOperations(ctx context.Context, status string, limit int) ([]MemoryOperation, error) {
	query := `SELECT o.operation_id,o.operation_type,o.status,coalesce(o.target_memory_id,''),coalesce(o.unit_id,''),coalesce(o.episode_id,''),o.evidence_ids_json,o.reason_codes_json,o.risk_tier,o.confidence,o.patch_json,o.created_at,coalesce(o.decided_at,''),coalesce(o.applied_at,''),coalesce(o.reviewed_by,''),coalesce(nullif(ku.statement,''),nullif(mr.body,''),mr.summary,'')
		FROM memory_operations o
		LEFT JOIN knowledge_units ku ON ku.unit_id=o.unit_id
		LEFT JOIN memory_records mr ON mr.memory_id=o.target_memory_id`
	args := []any{}
	if strings.TrimSpace(status) != "" {
		query += ` WHERE o.status=?`
		args = append(args, status)
	}
	query += ` ORDER BY o.created_at DESC,o.operation_id LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryOperation
	for rows.Next() {
		var item MemoryOperation
		var evidence, reasons string
		err := rows.Scan(&item.OperationID, &item.Type, &item.Status, &item.TargetMemoryID, &item.UnitID, &item.EpisodeID, &evidence, &reasons, &item.RiskTier, &item.Confidence, &item.PatchJSON, &item.CreatedAt, &item.DecidedAt, &item.AppliedAt, &item.ReviewedBy, &item.Summary)
		if err != nil {
			return nil, err
		}
		item.EvidenceIDs = decodeStrings(evidence)
		item.ReasonCodes = decodeStrings(reasons)
		out = append(out, item)
	}
	if out == nil {
		out = []MemoryOperation{}
	}
	return out, rows.Err()
}

func scanOperation(row scanner) (MemoryOperation, error) {
	var item MemoryOperation
	var evidence, reasons string
	err := row.Scan(&item.OperationID, &item.Type, &item.Status, &item.TargetMemoryID, &item.UnitID, &item.EpisodeID, &evidence, &reasons, &item.RiskTier, &item.Confidence, &item.PatchJSON, &item.CreatedAt, &item.DecidedAt, &item.AppliedAt, &item.ReviewedBy)
	item.EvidenceIDs = decodeStrings(evidence)
	item.ReasonCodes = decodeStrings(reasons)
	return item, err
}

func (e *Engine) operation(ctx context.Context, operationID string) (MemoryOperation, error) {
	row := e.control.DB.QueryRowContext(ctx, `SELECT operation_id,operation_type,status,coalesce(target_memory_id,''),coalesce(unit_id,''),coalesce(episode_id,''),evidence_ids_json,reason_codes_json,risk_tier,confidence,patch_json,created_at,coalesce(decided_at,''),coalesce(applied_at,''),coalesce(reviewed_by,'') FROM memory_operations WHERE operation_id=?`, operationID)
	return scanOperation(row)
}

func (e *Engine) knowledgeUnit(ctx context.Context, unitID string) (KnowledgeUnit, error) {
	var item KnowledgeUnit
	var scopes string
	err := e.control.DB.QueryRowContext(ctx, `SELECT unit_id,episode_id,evidence_id,unit_type,tier_hint,statement,normalized_key,confidence,risk_tier,status,scope_json,observed_at,created_at,coalesce(processed_at,'') FROM knowledge_units WHERE unit_id=?`, unitID).
		Scan(&item.UnitID, &item.EpisodeID, &item.EvidenceID, &item.UnitType, &item.TierHint, &item.Statement, &item.NormalizedKey, &item.Confidence, &item.RiskTier, &item.Status, &scopes, &item.ObservedAt, &item.CreatedAt, &item.ProcessedAt)
	if err != nil {
		return item, err
	}
	item.Scopes = decodeStrings(scopes)
	if err := e.hydrateKnowledgeUnit(ctx, &item); err != nil {
		return item, err
	}
	return item, nil
}

func (e *Engine) ReviewDetail(ctx context.Context, operationID string) (ReviewDetail, error) {
	operation, err := e.operation(ctx, operationID)
	if err != nil {
		return ReviewDetail{}, err
	}
	detail := ReviewDetail{Operation: operation, Evidence: []EvidenceRef{}}
	evidenceIDs := append([]string{}, operation.EvidenceIDs...)
	if operation.TargetMemoryID != "" {
		memory, err := e.Memory(ctx, operation.TargetMemoryID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ReviewDetail{}, err
		}
		if err == nil {
			detail.ProposedMemory = &memory
			evidenceIDs = append(evidenceIDs, memory.EvidenceIDs...)
		}
	}
	episodeID := operation.EpisodeID
	if operation.UnitID != "" {
		unit, err := e.knowledgeUnit(ctx, operation.UnitID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ReviewDetail{}, err
		}
		if err == nil {
			detail.KnowledgeUnit = &unit
			evidenceIDs = append(evidenceIDs, unit.EvidenceID)
			if episodeID == "" {
				episodeID = unit.EpisodeID
			}
		}
	}
	if episodeID != "" {
		episode, err := e.episode(ctx, episodeID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ReviewDetail{}, err
		}
		if err == nil {
			detail.Episode = &episode
			evidenceIDs = append(evidenceIDs, episode.EvidenceIDs...)
		}
	}
	detail.Evidence, err = e.evidenceRefs(ctx, unique(evidenceIDs))
	return detail, err
}

func (e *Engine) ListLivingViews(ctx context.Context) ([]LivingView, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT view_id,project_id,view_type,title,summary,status,source_memory_ids_json,canonical_path,updated_at FROM living_views WHERE status='active' AND project_id<>'' ORDER BY project_id,view_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LivingView
	for rows.Next() {
		var item LivingView
		var ids string
		if err := rows.Scan(&item.ViewID, &item.ProjectID, &item.ViewType, &item.Title, &item.Summary, &item.Status, &ids, &item.CanonicalPath, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.SourceMemoryIDs = decodeStrings(ids)
		out = append(out, item)
	}
	if out == nil {
		out = []LivingView{}
	}
	return out, rows.Err()
}

func (e *Engine) livingView(ctx context.Context, viewID string) (LivingView, error) {
	var item LivingView
	var ids string
	err := e.control.DB.QueryRowContext(ctx, `SELECT view_id,project_id,view_type,title,summary,status,source_memory_ids_json,canonical_path,updated_at FROM living_views WHERE view_id=?`, strings.TrimSpace(viewID)).Scan(&item.ViewID, &item.ProjectID, &item.ViewType, &item.Title, &item.Summary, &item.Status, &ids, &item.CanonicalPath, &item.UpdatedAt)
	item.SourceMemoryIDs = decodeStrings(ids)
	return item, err
}

func (e *Engine) ListLivingViewsForProject(ctx context.Context, projectID string) ([]LivingView, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT view_id,project_id,view_type,title,summary,status,source_memory_ids_json,canonical_path,updated_at FROM living_views WHERE project_id=? AND status='active' ORDER BY updated_at DESC,view_id`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LivingView{}
	for rows.Next() {
		var item LivingView
		var ids string
		if err := rows.Scan(&item.ViewID, &item.ProjectID, &item.ViewType, &item.Title, &item.Summary, &item.Status, &ids, &item.CanonicalPath, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.SourceMemoryIDs = decodeStrings(ids)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (e *Engine) LivingDetail(ctx context.Context, viewID string) (LivingDetail, error) {
	view, err := e.livingView(ctx, viewID)
	if err != nil {
		return LivingDetail{}, err
	}
	content, err := os.ReadFile(filepath.Join(e.memoryDir, filepath.Base(view.CanonicalPath)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return LivingDetail{}, err
	}
	memories := make([]MemoryRecord, 0, len(view.SourceMemoryIDs))
	for _, id := range view.SourceMemoryIDs {
		var item MemoryRecord
		var err error
		if view.ProjectID != "" {
			item, err = e.MemoryForProject(ctx, view.ProjectID, id)
		} else {
			item, err = e.Memory(ctx, id)
		}
		if err == nil {
			memories = append(memories, item)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return LivingDetail{}, err
		}
	}
	return LivingDetail{View: view, Content: string(content), Memories: memories}, nil
}

func hydrateAgentAsset(item *AgentAsset, idsRaw, scoresRaw, reasonsRaw string) {
	item.SourceMemoryIDs = decodeStrings(idsRaw)
	item.ClassificationScores = map[string]int{}
	_ = json.Unmarshal([]byte(scoresRaw), &item.ClassificationScores)
	item.ClassificationReasons = decodeStrings(reasonsRaw)
}

func (e *Engine) ListAssets(ctx context.Context, status string, limit int) ([]AgentAsset, error) {
	return e.ListAssetsFiltered(ctx, status, "", limit)
}

func (e *Engine) ListAssetsFiltered(ctx context.Context, status, assetType string, limit int) ([]AgentAsset, error) {
	query := `SELECT a.asset_id,a.asset_type,a.title,a.summary,a.status,a.version,a.risk_level,a.source_memory_ids_json,a.validation_status,a.created_at,a.updated_at,coalesce(c.classification_status,'legacy'),coalesce(c.scores_json,'{}'),coalesce(c.reasons_json,'[]') FROM agent_assets a LEFT JOIN agent_asset_classifications c ON c.asset_id=a.asset_id`
	where := []string{}
	args := []any{}
	if strings.TrimSpace(status) != "" {
		where = append(where, "a.status=?")
		args = append(args, strings.TrimSpace(status))
	}
	if strings.TrimSpace(assetType) != "" {
		where = append(where, "a.asset_type=?")
		args = append(args, strings.ToLower(strings.TrimSpace(assetType)))
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY a.updated_at DESC,a.asset_id LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentAsset{}
	for rows.Next() {
		var item AgentAsset
		var ids, scores, reasons string
		if err := rows.Scan(&item.AssetID, &item.AssetType, &item.Title, &item.Summary, &item.Status, &item.Version, &item.RiskLevel, &ids, &item.ValidationStatus, &item.CreatedAt, &item.UpdatedAt, &item.ClassificationStatus, &scores, &reasons); err != nil {
			return nil, err
		}
		hydrateAgentAsset(&item, ids, scores, reasons)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (e *Engine) asset(ctx context.Context, assetID string) (AgentAsset, error) {
	var item AgentAsset
	var ids string
	var scores, reasons string
	err := e.control.DB.QueryRowContext(ctx, `SELECT a.asset_id,a.asset_type,a.title,a.summary,a.status,a.version,a.risk_level,a.source_memory_ids_json,a.validation_status,a.created_at,a.updated_at,coalesce(c.classification_status,'legacy'),coalesce(c.scores_json,'{}'),coalesce(c.reasons_json,'[]') FROM agent_assets a LEFT JOIN agent_asset_classifications c ON c.asset_id=a.asset_id WHERE a.asset_id=?`, strings.TrimSpace(assetID)).Scan(&item.AssetID, &item.AssetType, &item.Title, &item.Summary, &item.Status, &item.Version, &item.RiskLevel, &ids, &item.ValidationStatus, &item.CreatedAt, &item.UpdatedAt, &item.ClassificationStatus, &scores, &reasons)
	hydrateAgentAsset(&item, ids, scores, reasons)
	return item, err
}

func (e *Engine) ListAssetsForProject(ctx context.Context, projectID, status string, limit int) ([]AgentAsset, error) {
	return e.ListAssetsForProjectFiltered(ctx, projectID, status, "", limit)
}

func (e *Engine) ListAssetsForProjectFiltered(ctx context.Context, projectID, status, assetType string, limit int) ([]AgentAsset, error) {
	query := `SELECT DISTINCT a.asset_id,a.asset_type,a.title,a.summary,a.status,a.version,a.risk_level,a.source_memory_ids_json,a.validation_status,a.created_at,a.updated_at,coalesce(c.classification_status,'legacy'),coalesce(c.scores_json,'{}'),coalesce(c.reasons_json,'[]') FROM agent_assets a JOIN record_projects rp ON rp.record_type='asset' AND rp.record_id=a.asset_id LEFT JOIN agent_asset_classifications c ON c.asset_id=a.asset_id WHERE rp.project_id=?`
	args := []any{strings.TrimSpace(projectID)}
	if strings.TrimSpace(status) != "" {
		query += ` AND a.status=?`
		args = append(args, strings.TrimSpace(status))
	}
	if strings.TrimSpace(assetType) != "" {
		query += ` AND a.asset_type=?`
		args = append(args, strings.ToLower(strings.TrimSpace(assetType)))
	}
	query += ` ORDER BY a.updated_at DESC,a.asset_id LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := e.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentAsset{}
	for rows.Next() {
		var item AgentAsset
		var ids, scores, reasons string
		if err := rows.Scan(&item.AssetID, &item.AssetType, &item.Title, &item.Summary, &item.Status, &item.Version, &item.RiskLevel, &ids, &item.ValidationStatus, &item.CreatedAt, &item.UpdatedAt, &item.ClassificationStatus, &scores, &reasons); err != nil {
			return nil, err
		}
		hydrateAgentAsset(&item, ids, scores, reasons)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (e *Engine) AssetDetail(ctx context.Context, assetID string) (AssetDetail, error) {
	asset, err := e.asset(ctx, assetID)
	if err != nil {
		return AssetDetail{}, err
	}
	memories := make([]MemoryRecord, 0, len(asset.SourceMemoryIDs))
	for _, id := range asset.SourceMemoryIDs {
		item, err := e.Memory(ctx, id)
		if err == nil {
			memories = append(memories, item)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return AssetDetail{}, err
		}
	}
	return AssetDetail{Asset: asset, Memories: memories}, nil
}

func (e *Engine) Trace(ctx context.Context, memoryID string) (Trace, error) {
	memory, err := e.Memory(ctx, memoryID)
	if err != nil {
		return Trace{}, err
	}
	operations, err := e.operationsForMemory(ctx, memoryID)
	if err != nil {
		return Trace{}, err
	}
	unitIDs := []string{}
	for _, op := range operations {
		unitIDs = append(unitIDs, op.UnitID)
	}
	units, err := e.unitsByID(ctx, unique(unitIDs))
	if err != nil {
		return Trace{}, err
	}
	var episodes []Episode
	for _, episodeID := range memory.EpisodeIDs {
		item, err := e.episode(ctx, episodeID)
		if err == nil {
			episodes = append(episodes, item)
		} else if err != sql.ErrNoRows {
			return Trace{}, err
		}
	}
	evidence, err := e.evidenceRefs(ctx, memory.EvidenceIDs)
	if err != nil {
		return Trace{}, err
	}
	if episodes == nil {
		episodes = []Episode{}
	}
	return Trace{Memory: memory, Operations: operations, Units: units, Episodes: episodes, Evidence: evidence}, nil
}

func (e *Engine) operationsForMemory(ctx context.Context, memoryID string) ([]MemoryOperation, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT operation_id,operation_type,status,coalesce(target_memory_id,''),coalesce(unit_id,''),coalesce(episode_id,''),evidence_ids_json,reason_codes_json,risk_tier,confidence,patch_json,created_at,coalesce(decided_at,''),coalesce(applied_at,''),coalesce(reviewed_by,'') FROM memory_operations WHERE target_memory_id=? ORDER BY created_at DESC`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryOperation
	for rows.Next() {
		item, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []MemoryOperation{}
	}
	return out, rows.Err()
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func anyStrings(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func (e *Engine) unitsByID(ctx context.Context, ids []string) ([]KnowledgeUnit, error) {
	if len(ids) == 0 {
		return []KnowledgeUnit{}, nil
	}
	query := fmt.Sprintf(`SELECT unit_id,episode_id,evidence_id,unit_type,tier_hint,statement,normalized_key,confidence,risk_tier,status,scope_json,observed_at,created_at,coalesce(processed_at,'') FROM knowledge_units WHERE unit_id IN (%s) ORDER BY observed_at`, placeholders(len(ids)))
	rows, err := e.control.DB.QueryContext(ctx, query, anyStrings(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeUnit
	for rows.Next() {
		var item KnowledgeUnit
		var scopes string
		if err := rows.Scan(&item.UnitID, &item.EpisodeID, &item.EvidenceID, &item.UnitType, &item.TierHint, &item.Statement, &item.NormalizedKey, &item.Confidence, &item.RiskTier, &item.Status, &scopes, &item.ObservedAt, &item.CreatedAt, &item.ProcessedAt); err != nil {
			return nil, err
		}
		item.Scopes = decodeStrings(scopes)
		out = append(out, item)
	}
	if out == nil {
		out = []KnowledgeUnit{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := e.hydrateKnowledgeUnits(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) evidenceRefs(ctx context.Context, ids []string) ([]EvidenceRef, error) {
	if len(ids) == 0 {
		return []EvidenceRef{}, nil
	}
	query := fmt.Sprintf(`SELECT evidence_id,source_system,session_id,coalesce(role,''),observed_at,body FROM turns WHERE evidence_id IN (%s) ORDER BY observed_at`, placeholders(len(ids)))
	rows, err := e.search.DB.QueryContext(ctx, query, anyStrings(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvidenceRef
	for rows.Next() {
		var item EvidenceRef
		var body string
		if err := rows.Scan(&item.EvidenceID, &item.SourceSystem, &item.SessionID, &item.Role, &item.ObservedAt, &body); err != nil {
			return nil, err
		}
		item.Preview = compact(body, 220)
		out = append(out, item)
	}
	if out == nil {
		out = []EvidenceRef{}
	}
	return out, rows.Err()
}

func (e *Engine) Graph(ctx context.Context, limit int) (Graph, error) {
	limit = boundedLimit(limit)
	memories, err := e.ListMemories(ctx, "", "", limit)
	if err != nil {
		return Graph{}, err
	}
	return e.graph(ctx, memories, "", limit)
}

func (e *Engine) GraphForProject(ctx context.Context, projectID string, limit int) (Graph, error) {
	limit = boundedLimit(limit)
	memories, _, err := e.ListMemoriesForProject(ctx, strings.TrimSpace(projectID), "", "", limit, 0)
	if err != nil {
		return Graph{}, err
	}
	return e.graph(ctx, memories, strings.TrimSpace(projectID), limit)
}

func (e *Engine) graph(ctx context.Context, memories []MemoryRecord, projectID string, limit int) (Graph, error) {
	nodes := []GraphNode{}
	edges := []GraphEdge{}
	seenNodes := map[string]bool{}
	addNode := func(node GraphNode) {
		if node.ID == "" || seenNodes[node.ID] {
			return
		}
		seenNodes[node.ID] = true
		nodes = append(nodes, node)
	}
	for _, item := range memories {
		addNode(GraphNode{ID: item.MemoryID, Layer: "memory", Label: item.Summary, Status: item.Status})
		for _, episodeID := range item.EpisodeIDs {
			episode, err := e.episode(ctx, episodeID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return Graph{}, err
			}
			if err == nil {
				addNode(GraphNode{ID: episode.EpisodeID, Layer: "episode", Label: episode.Title, Status: episode.Status})
				edges = append(edges, GraphEdge{From: episode.EpisodeID, To: item.MemoryID, Kind: "consolidates_into"})
				for _, evidenceID := range episode.EvidenceIDs {
					addNode(GraphNode{ID: evidenceID, Layer: "evidence", Label: evidenceID, Status: "canonical"})
					edges = append(edges, GraphEdge{From: evidenceID, To: episode.EpisodeID, Kind: "observed_in"})
				}
			}
		}
		operations, err := e.operationsForMemory(ctx, item.MemoryID)
		if err != nil {
			return Graph{}, err
		}
		unitIDs := []string{}
		for _, operation := range operations {
			unitIDs = append(unitIDs, operation.UnitID)
		}
		units, err := e.unitsByID(ctx, unique(unitIDs))
		if err != nil {
			return Graph{}, err
		}
		for _, unit := range units {
			addNode(GraphNode{ID: unit.UnitID, Layer: "knowledge", Label: compact(unit.Statement, 80), Status: unit.Status})
			addNode(GraphNode{ID: unit.EvidenceID, Layer: "evidence", Label: unit.EvidenceID, Status: "canonical"})
			edges = append(edges, GraphEdge{From: unit.EvidenceID, To: unit.UnitID, Kind: "extracts"})
			edges = append(edges, GraphEdge{From: unit.UnitID, To: unit.EpisodeID, Kind: "belongs_to"})
		}
	}
	assets, err := e.ListAssets(ctx, "", limit)
	if projectID != "" {
		assets, err = e.ListAssetsForProject(ctx, projectID, "", limit)
	}
	if err != nil {
		return Graph{}, err
	}
	for _, asset := range assets {
		addNode(GraphNode{ID: asset.AssetID, Layer: "asset", Label: asset.Title, Status: asset.Status})
		for _, memoryID := range asset.SourceMemoryIDs {
			edges = append(edges, GraphEdge{From: memoryID, To: asset.AssetID, Kind: "proposes"})
		}
	}
	views, err := e.ListLivingViews(ctx)
	if projectID != "" {
		views, err = e.ListLivingViewsForProject(ctx, projectID)
	}
	if err != nil {
		return Graph{}, err
	}
	for _, view := range views {
		addNode(GraphNode{ID: view.ViewID, Layer: "living", Label: view.Title, Status: view.Status})
		for _, memoryID := range view.SourceMemoryIDs {
			edges = append(edges, GraphEdge{From: memoryID, To: view.ViewID, Kind: "projects_to"})
		}
	}
	return Graph{Nodes: nodes, Edges: edges}, nil
}
