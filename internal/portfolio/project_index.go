package portfolio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/store"
)

// RebuildProjectIndex refreshes one project's disposable retrieval projection.
// Canonical Evidence/Object/Revision records are never changed. Full
// RebuildIndex remains the recovery path; this method is for correction/growth.
func (s *Service) RebuildProjectIndex(ctx context.Context, projectID string) (int, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, fmt.Errorf("project_id is required")
	}
	project, err := s.Project(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if err := s.search.DeleteDocumentsForProject(ctx, projectID); err != nil {
		return 0, err
	}
	count := 0
	if err := s.indexProject(ctx, project); err != nil {
		return count, err
	}
	count++

	evidenceRows, err := s.control.DB.QueryContext(ctx, `SELECT record_id FROM record_projects WHERE record_type='evidence' AND project_id=? ORDER BY record_id`, projectID)
	if err != nil {
		return count, err
	}
	evidenceIDs := []string{}
	for evidenceRows.Next() {
		var id string
		if err := evidenceRows.Scan(&id); err != nil {
			evidenceRows.Close()
			return count, err
		}
		evidenceIDs = append(evidenceIDs, id)
	}
	if err := evidenceRows.Err(); err != nil {
		evidenceRows.Close()
		return count, err
	}
	evidenceRows.Close()
	for _, id := range evidenceIDs {
		var source, session, observed, body, scope, path string
		if err := s.search.DB.QueryRowContext(ctx, `SELECT source_system,session_id,observed_at,body,scope_json,ledger_rel_path FROM turns WHERE evidence_id=?`, id).Scan(&source, &session, &observed, &body, &scope, &path); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return count, err
		}
		doc := store.IndexedDocument{DocKey: "evidence:" + id, Kind: "evidence", SourceID: id, ProjectID: projectID, Title: source + " · " + session, Body: body, Status: "canonical", ObservedAt: observed, MetadataJSON: metadata(map[string]any{"source_system": source, "session_id": session, "scope_hints": json.RawMessage(scope), "ledger_path": path})}
		if err := s.search.UpsertDocument(ctx, doc); err != nil {
			return count, err
		}
		count++
	}

	if n, err := s.indexProjectLegacyRecords(ctx, projectID); err != nil {
		return count, err
	} else {
		count += n
	}
	if n, err := s.indexProjectObjects(ctx, projectID); err != nil {
		return count, err
	} else {
		count += n
	}
	if n, err := s.indexProjectOperationalRecords(ctx, projectID); err != nil {
		return count, err
	} else {
		count += n
	}
	return count, nil
}

func (s *Service) indexProjectLegacyRecords(ctx context.Context, projectID string) (int, error) {
	type recordDoc struct{ id, title, body, status, observed, validFrom, subtype, sources string }
	specs := []struct{ kind, query string }{
		{"memory", `SELECT m.memory_id,m.summary,m.body,m.status,m.updated_at,m.observed_at,m.tier,m.evidence_ids_json FROM memory_records m JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=m.memory_id WHERE rp.project_id=?`},
		{"episode", `SELECT e.episode_id,e.title,e.summary,e.status,e.updated_at,e.started_at,e.source_system,e.evidence_ids_json FROM episodes e JOIN record_projects rp ON rp.record_type='episode' AND rp.record_id=e.episode_id WHERE rp.project_id=?`},
		{"asset", `SELECT a.asset_id,a.title,a.summary,a.status,a.updated_at,a.created_at,a.asset_type,a.source_memory_ids_json FROM agent_assets a JOIN record_projects rp ON rp.record_type='asset' AND rp.record_id=a.asset_id WHERE rp.project_id=?`},
	}
	count := 0
	for _, spec := range specs {
		rows, err := s.control.DB.QueryContext(ctx, spec.query, projectID)
		if err != nil {
			return count, err
		}
		for rows.Next() {
			var item recordDoc
			if err := rows.Scan(&item.id, &item.title, &item.body, &item.status, &item.observed, &item.validFrom, &item.subtype, &item.sources); err != nil {
				rows.Close()
				return count, err
			}
			doc := store.IndexedDocument{DocKey: spec.kind + ":" + item.id, Kind: spec.kind, SourceID: item.id, ProjectID: projectID, Title: item.title, Body: item.body, Status: item.status, ObservedAt: item.observed, ValidFrom: item.validFrom, MetadataJSON: metadata(map[string]any{"subtype": item.subtype, "source_ids": json.RawMessage(item.sources)})}
			if err := s.search.UpsertDocument(ctx, doc); err != nil {
				rows.Close()
				return count, err
			}
			count++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return count, err
		}
		rows.Close()
	}
	return count, nil
}

func (s *Service) indexProjectObjects(ctx context.Context, projectID string) (int, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT o.object_id,o.type_id,o.status,o.updated_at,r.valid_from,r.valid_until,r.payload_json,r.schema_version,r.content_hash,r.confidence,r.importance,r.source_evidence_ids_json,r.source_object_ids_json,r.run_id,r.stage_id,r.plugin_id,r.plugin_version,r.living_asset_path FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE o.project_id=?`, projectID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, typeID, status, observed, validFrom, payload, schemaVersion, hash, evidenceIDs, objectIDs, pluginID, pluginVersion string
		var validUntil, runID, stageID, livingPath sql.NullString
		var confidence, importance float64
		if err := rows.Scan(&id, &typeID, &status, &observed, &validFrom, &validUntil, &payload, &schemaVersion, &hash, &confidence, &importance, &evidenceIDs, &objectIDs, &runID, &stageID, &pluginID, &pluginVersion, &livingPath); err != nil {
			return count, err
		}
		if skipGenericHarnessIndex(typeID, status) {
			continue
		}
		if typeID == memory.StructuredMemoryRecordTypeV1 {
			current, err := memory.DecodeStructuredMemoryPayload(json.RawMessage(payload))
			if err != nil {
				return count, fmt.Errorf("decode authoritative memory %s: %w", id, err)
			}
			doc := store.IndexedDocument{DocKey: "memory:" + current.MemoryID, Kind: "memory", SourceID: current.MemoryID, ProjectID: projectID, Title: current.Summary, Body: current.Body, Status: current.Status, ObservedAt: current.UpdatedAt, ValidFrom: current.ObservedAt, MetadataJSON: metadata(map[string]any{"subtype": current.Tier, "asset_form": current.AssetForm, "source_ids": current.EvidenceIDs, "authority_object_id": id, "authority_content_hash": hash, "confidence": current.Confidence, "importance": current.Importance})}
			if err := s.search.UpsertDocument(ctx, doc); err != nil {
				return count, err
			}
			continue
		}
		title, body := genericObjectText(payload)
		if title == "Memory object" {
			title = typeID + " · " + id
		}
		docKey, kind := harnessObjectSearchIdentity(typeID, id)
		doc := store.IndexedDocument{DocKey: docKey, Kind: kind, SourceID: id, ProjectID: projectID, Title: title, Body: body, Status: status, ObservedAt: observed, ValidFrom: validFrom, ValidUntil: validUntil.String, MetadataJSON: metadata(map[string]any{"type_id": typeID, "schema_version": schemaVersion, "content_hash": hash, "confidence": confidence, "importance": importance, "source_evidence_ids": json.RawMessage(evidenceIDs), "source_object_ids": json.RawMessage(objectIDs), "run_id": runID.String, "stage_id": stageID.String, "plugin_id": pluginID, "plugin_version": pluginVersion, "living_asset_path": livingPath.String})}
		if err := s.search.UpsertDocument(ctx, doc); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func collectIDs(ctx context.Context, db *sql.DB, query, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) indexProjectOperationalRecords(ctx context.Context, projectID string) (int, error) {
	count := 0
	factIDs, err := collectIDs(ctx, s.control.DB, `SELECT fact_id FROM temporal_facts WHERE project_id=? ORDER BY recorded_at`, projectID)
	if err != nil {
		return count, err
	}
	for _, id := range factIDs {
		item, err := s.Fact(ctx, id)
		if err != nil {
			return count, err
		}
		if err := s.indexFact(ctx, item); err != nil {
			return count, err
		}
		count++
	}
	groups := []struct {
		query string
		load  func(context.Context, string) error
	}{
		{`SELECT goal_id FROM project_goals WHERE project_id=?`, func(ctx context.Context, id string) error {
			item, err := s.Goal(ctx, id)
			if err != nil {
				return err
			}
			return s.indexGoal(ctx, item)
		}},
		{`SELECT decision_id FROM project_decisions WHERE project_id=?`, func(ctx context.Context, id string) error {
			item, err := s.decision(ctx, id)
			if err != nil {
				return err
			}
			return s.indexDecision(ctx, item)
		}},
		{`SELECT risk_id FROM project_risks WHERE project_id=?`, func(ctx context.Context, id string) error {
			item, err := s.risk(ctx, id)
			if err != nil {
				return err
			}
			return s.indexRisk(ctx, item)
		}},
		{`SELECT entry_id FROM finance_entries WHERE project_id=?`, func(ctx context.Context, id string) error {
			item, err := s.financeEntry(ctx, id)
			if err != nil {
				return err
			}
			return s.indexFinanceEntry(ctx, item)
		}},
	}
	for _, group := range groups {
		ids, err := collectIDs(ctx, s.control.DB, group.query, projectID)
		if err != nil {
			return count, err
		}
		for _, id := range ids {
			if err := group.load(ctx, id); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
