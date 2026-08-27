package portfolio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/store"
)

func metadata(value any) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func harnessObjectSearchIdentity(typeID, objectID string) (string, string) {
	if strings.HasPrefix(strings.TrimSpace(typeID), "builtin.experience-bank.") {
		return "experience:" + objectID, "experience"
	}
	return "object:" + objectID, "object"
}

func skipGenericHarnessIndex(typeID, status string) bool {
	typeID = strings.TrimSpace(typeID)
	status = strings.TrimSpace(status)
	if typeID == "builtin.living-asset-vault.profile-projection.v1" {
		return true
	}
	for _, prefix := range []string{"builtin.adaptation-lab.", "builtin.portable-bundle."} {
		if strings.HasPrefix(typeID, prefix) {
			return true
		}
	}
	if strings.HasPrefix(typeID, "builtin.team-memory.") {
		return typeID != "builtin.team-memory.project-durable.v1" || status != "active"
	}
	return false
}

func genericObjectText(payloadRaw string) (string, string) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		return "Memory object", payloadRaw
	}
	title := ""
	for _, key := range []string{"title", "name", "subject", "topic", "summary", "normalized_pattern", "primary_failure_dimension", "verdict", "result"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			title = strings.TrimSpace(value)
			break
		}
	}
	if title == "" {
		title = "Memory object"
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, _ := json.Marshal(payload[key])
		parts = append(parts, key+" "+string(value))
	}
	return title, strings.Join(parts, "\n")
}

func (s *Service) indexProject(ctx context.Context, item Project) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{
		DocKey: "project:" + item.ProjectID, Kind: "project", SourceID: item.ProjectID, ProjectID: item.ProjectID,
		Title: item.Name, Body: strings.TrimSpace(item.Description + "\n" + strings.Join(item.Aliases, " ")), Status: item.Status,
		ObservedAt: item.UpdatedAt, MetadataJSON: metadata(map[string]any{"slug": item.Slug, "currency": item.DefaultCurrency}),
	})
}

func (s *Service) indexFact(ctx context.Context, item TemporalFact) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{
		DocKey: "fact:" + item.FactID, Kind: "fact", SourceID: item.FactID, ProjectID: item.ProjectID,
		Title: item.Subject + " · " + item.Predicate, Body: item.Subject + " " + item.Predicate + " " + item.Object, Status: item.Status,
		ObservedAt: item.RecordedAt, ValidFrom: item.ValidFrom, ValidUntil: item.ValidUntil,
		MetadataJSON: metadata(map[string]any{"source_memory_id": item.SourceMemoryID, "source_evidence_ids": item.SourceEvidenceIDs, "confidence": item.Confidence}),
	})
}

func (s *Service) indexGoal(ctx context.Context, item Goal) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{DocKey: "goal:" + item.GoalID, Kind: "goal", SourceID: item.GoalID, ProjectID: item.ProjectID, Title: item.Title, Body: item.Description, Status: item.Status, ObservedAt: item.UpdatedAt, ValidUntil: item.TargetAt, MetadataJSON: metadata(map[string]any{"priority": item.Priority, "source_evidence_id": item.SourceEvidenceID})})
}

func (s *Service) indexDecision(ctx context.Context, item Decision) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{DocKey: "decision:" + item.DecisionID, Kind: "decision", SourceID: item.DecisionID, ProjectID: item.ProjectID, Title: item.Title, Body: strings.TrimSpace(item.Decision + "\n" + item.Rationale), Status: item.Status, ObservedAt: item.DecidedAt, ValidFrom: item.DecidedAt, MetadataJSON: metadata(map[string]any{"source_evidence_ids": item.SourceEvidenceIDs, "supersedes": item.SupersedesDecisionID})})
}

func (s *Service) indexRisk(ctx context.Context, item Risk) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{DocKey: "risk:" + item.RiskID, Kind: "risk", SourceID: item.RiskID, ProjectID: item.ProjectID, Title: item.Title, Body: strings.TrimSpace(item.Description + "\n" + item.Mitigation), Status: item.Status, ObservedAt: item.UpdatedAt, MetadataJSON: metadata(map[string]any{"probability": item.Probability, "impact": item.Impact, "owner": item.Owner, "source_evidence_id": item.SourceEvidenceID})})
}

func (s *Service) indexFinanceEntry(ctx context.Context, item FinanceEntry) error {
	return s.search.UpsertDocument(ctx, store.IndexedDocument{DocKey: "finance:" + item.EntryID, Kind: "finance", SourceID: item.EntryID, ProjectID: item.ProjectID, Title: item.Description, Body: strings.TrimSpace(item.Category + " " + item.EntryType + " " + item.Currency), Status: item.Status, ObservedAt: item.OccurredAt, MetadataJSON: metadata(map[string]any{"amount_minor": item.AmountMinor, "currency": item.Currency, "account_id": item.AccountID, "source_evidence_id": item.SourceEvidenceID})})
}

// RebuildIndex recreates the complete disposable retrieval surface from the
// JSONL-derived turns and canonical control records.
func (s *Service) RebuildIndex(ctx context.Context) (int, error) {
	if err := s.search.DeleteDocuments(ctx); err != nil {
		return 0, err
	}
	count := 0
	projects, err := s.ListProjects(ctx, true)
	if err != nil {
		return count, err
	}
	for _, project := range projects {
		if err := s.indexProject(ctx, project); err != nil {
			return count, err
		}
		count++
	}

	rows, err := s.search.DB.QueryContext(ctx, `SELECT evidence_id,source_system,session_id,observed_at,body,scope_json,ledger_rel_path FROM turns ORDER BY id`)
	if err != nil {
		return count, err
	}
	type turnDoc struct{ id, source, session, observed, body, scope, path string }
	turns := []turnDoc{}
	for rows.Next() {
		var item turnDoc
		if err := rows.Scan(&item.id, &item.source, &item.session, &item.observed, &item.body, &item.scope, &item.path); err != nil {
			rows.Close()
			return count, err
		}
		turns = append(turns, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return count, err
	}
	for _, item := range turns {
		projectID, err := s.ProjectForRecord(ctx, "evidence", item.id)
		if err != nil {
			return count, err
		}
		doc := store.IndexedDocument{DocKey: "evidence:" + item.id, Kind: "evidence", SourceID: item.id, ProjectID: projectID, Title: item.source + " · " + item.session, Body: item.body, Status: "canonical", ObservedAt: item.observed, MetadataJSON: metadata(map[string]any{"source_system": item.source, "session_id": item.session, "scope_hints": json.RawMessage(item.scope), "ledger_path": item.path})}
		if err := s.search.UpsertDocument(ctx, doc); err != nil {
			return count, err
		}
		count++
	}

	type querySpec struct {
		query string
		kind  string
	}
	specs := []querySpec{
		{`SELECT m.memory_id,coalesce((SELECT project_id FROM record_projects rp WHERE rp.record_type='memory' AND rp.record_id=m.memory_id ORDER BY is_primary DESC LIMIT 1),'project-inbox'),m.summary,m.body,m.status,m.updated_at,m.observed_at,m.tier,m.evidence_ids_json FROM memory_records m`, "memory"},
		{`SELECT e.episode_id,coalesce((SELECT project_id FROM record_projects rp WHERE rp.record_type='episode' AND rp.record_id=e.episode_id ORDER BY is_primary DESC LIMIT 1),'project-inbox'),e.title,e.summary,e.status,e.updated_at,e.started_at,e.source_system,e.evidence_ids_json FROM episodes e`, "episode"},
		{`SELECT a.asset_id,coalesce((SELECT project_id FROM record_projects rp WHERE rp.record_type='asset' AND rp.record_id=a.asset_id ORDER BY is_primary DESC LIMIT 1),'project-inbox'),a.title,a.summary,a.status,a.updated_at,a.created_at,a.asset_type,a.source_memory_ids_json FROM agent_assets a`, "asset"},
	}
	for _, spec := range specs {
		rows, err := s.control.DB.QueryContext(ctx, spec.query)
		if err != nil {
			return count, err
		}
		type recordDoc struct{ id, project, title, body, status, observed, validFrom, subtype, sources string }
		items := []recordDoc{}
		for rows.Next() {
			var item recordDoc
			if err := rows.Scan(&item.id, &item.project, &item.title, &item.body, &item.status, &item.observed, &item.validFrom, &item.subtype, &item.sources); err != nil {
				rows.Close()
				return count, err
			}
			items = append(items, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return count, err
		}
		for _, item := range items {
			doc := store.IndexedDocument{DocKey: spec.kind + ":" + item.id, Kind: spec.kind, SourceID: item.id, ProjectID: item.project, Title: item.title, Body: item.body, Status: item.status, ObservedAt: item.observed, ValidFrom: item.validFrom, MetadataJSON: metadata(map[string]any{"subtype": item.subtype, "source_ids": json.RawMessage(item.sources)})}
			if err := s.search.UpsertDocument(ctx, doc); err != nil {
				return count, err
			}
			count++
		}
	}

	objectRows, err := s.control.DB.QueryContext(ctx, `SELECT o.object_id,o.project_id,o.type_id,o.status,o.updated_at,r.valid_from,r.valid_until,r.payload_json,r.schema_version,r.content_hash,r.confidence,r.importance,r.source_evidence_ids_json,r.source_object_ids_json,r.run_id,r.stage_id,r.plugin_id,r.plugin_version,r.living_asset_path FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision`)
	if err != nil {
		return count, err
	}
	type objectDoc struct {
		id, project, typeID, status, observed, validFrom, payload, schemaVersion, hash, evidenceIDs, objectIDs, pluginID, pluginVersion string
		validUntil, runID, stageID, livingPath                                                                                          sql.NullString
		confidence, importance                                                                                                          float64
	}
	objects := []objectDoc{}
	for objectRows.Next() {
		var item objectDoc
		if err := objectRows.Scan(&item.id, &item.project, &item.typeID, &item.status, &item.observed, &item.validFrom, &item.validUntil, &item.payload, &item.schemaVersion, &item.hash, &item.confidence, &item.importance, &item.evidenceIDs, &item.objectIDs, &item.runID, &item.stageID, &item.pluginID, &item.pluginVersion, &item.livingPath); err != nil {
			objectRows.Close()
			return count, err
		}
		objects = append(objects, item)
	}
	if err := objectRows.Err(); err != nil {
		objectRows.Close()
		return count, err
	}
	if err := objectRows.Close(); err != nil {
		return count, err
	}
	for _, item := range objects {
		if skipGenericHarnessIndex(item.typeID, item.status) {
			continue
		}
		if item.typeID == memory.StructuredMemoryRecordTypeV1 {
			current, decodeErr := memory.DecodeStructuredMemoryPayload(json.RawMessage(item.payload))
			if decodeErr != nil {
				return count, fmt.Errorf("decode authoritative memory %s: %w", item.id, decodeErr)
			}
			doc := store.IndexedDocument{
				DocKey: "memory:" + current.MemoryID, Kind: "memory", SourceID: current.MemoryID, ProjectID: item.project,
				Title: current.Summary, Body: current.Body, Status: current.Status, ObservedAt: current.UpdatedAt, ValidFrom: current.ObservedAt,
				MetadataJSON: metadata(map[string]any{
					"subtype": current.Tier, "asset_form": current.AssetForm, "source_ids": current.EvidenceIDs,
					"authority_object_id": item.id, "authority_content_hash": item.hash,
					"confidence": current.Confidence, "importance": current.Importance,
				}),
			}
			if err := s.search.UpsertDocument(ctx, doc); err != nil {
				return count, err
			}
			// This replaces the legacy memory:<id> fallback in-place. Do not index
			// a second generic object document and do not increment the unique count.
			continue
		}
		title, body := genericObjectText(item.payload)
		if title == "Memory object" {
			title = item.typeID + " · " + item.id
		}
		docKey, kind := harnessObjectSearchIdentity(item.typeID, item.id)
		doc := store.IndexedDocument{
			DocKey: docKey, Kind: kind, SourceID: item.id, ProjectID: item.project,
			Title: title, Body: body, Status: item.status, ObservedAt: item.observed, ValidFrom: item.validFrom, ValidUntil: item.validUntil.String,
			MetadataJSON: metadata(map[string]any{
				"type_id": item.typeID, "schema_version": item.schemaVersion, "content_hash": item.hash,
				"confidence": item.confidence, "importance": item.importance,
				"source_evidence_ids": json.RawMessage(item.evidenceIDs), "source_object_ids": json.RawMessage(item.objectIDs),
				"run_id": item.runID.String, "stage_id": item.stageID.String, "plugin_id": item.pluginID, "plugin_version": item.pluginVersion,
				"living_asset_path": item.livingPath.String,
			}),
		}
		if err := s.search.UpsertDocument(ctx, doc); err != nil {
			return count, err
		}
		count++
	}

	factRows, err := s.control.DB.QueryContext(ctx, `SELECT fact_id FROM temporal_facts ORDER BY recorded_at`)
	if err != nil {
		return count, err
	}
	factIDs := []string{}
	for factRows.Next() {
		var id string
		if err := factRows.Scan(&id); err != nil {
			factRows.Close()
			return count, err
		}
		factIDs = append(factIDs, id)
	}
	factRows.Close()
	for _, id := range factIDs {
		fact, err := s.Fact(ctx, id)
		if err != nil {
			return count, err
		}
		if err := s.indexFact(ctx, fact); err != nil {
			return count, err
		}
		count++
	}

	additional := []struct {
		query string
		load  func(context.Context, string) error
	}{
		{`SELECT goal_id FROM project_goals`, func(ctx context.Context, id string) error {
			item, err := s.Goal(ctx, id)
			if err != nil {
				return err
			}
			return s.indexGoal(ctx, item)
		}},
		{`SELECT decision_id FROM project_decisions`, func(ctx context.Context, id string) error {
			item, err := s.decision(ctx, id)
			if err != nil {
				return err
			}
			return s.indexDecision(ctx, item)
		}},
		{`SELECT risk_id FROM project_risks`, func(ctx context.Context, id string) error {
			item, err := s.risk(ctx, id)
			if err != nil {
				return err
			}
			return s.indexRisk(ctx, item)
		}},
		{`SELECT entry_id FROM finance_entries`, func(ctx context.Context, id string) error {
			item, err := s.financeEntry(ctx, id)
			if err != nil {
				return err
			}
			return s.indexFinanceEntry(ctx, item)
		}},
	}
	for _, group := range additional {
		rows, err := s.control.DB.QueryContext(ctx, group.query)
		if err != nil {
			return count, err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return count, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			if err := group.load(ctx, id); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func (s *Service) CountIndexedDocuments(ctx context.Context) (int, error) {
	var count int
	err := s.search.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents`).Scan(&count)
	return count, err
}

func scanNullString(value sql.NullString) string { return value.String }

func indexError(kind, id string, err error) error {
	return fmt.Errorf("index %s %s: %w", kind, id, err)
}
