package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/localembedding"
	_ "modernc.org/sqlite"
)

type Receipt struct {
	EvidenceID    string
	LineHash      string
	SourceSystem  string
	SessionID     string
	ObservedAt    string
	CapturedAt    string
	LedgerRelPath string
	Ordinal       int
}

type ControlStore struct{ DB *sql.DB }
type SearchStore struct{ DB *sql.DB }

func open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %q ADD COLUMN %q %s`, table, column, definition))
	return err
}

func OpenControl(path string) (*ControlStore, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	schema := `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	INSERT OR IGNORE INTO meta(key,value) VALUES ('schema_version','0.1');
CREATE TABLE IF NOT EXISTS evidence_receipts (
  evidence_id TEXT PRIMARY KEY,
  line_hash TEXT NOT NULL,
  source_system TEXT NOT NULL,
  session_id TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  ledger_rel_path TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  recorded_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
  job_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  idempotency_key TEXT UNIQUE,
  status TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS operation_receipts (
  operation_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  target_hash TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  applied_at TEXT
);

-- M2 durable memory growth loop. Evidence remains canonical in JSONL; every
-- table below is a traceable, rebuildable interpretation with explicit source
-- references and operation history.
CREATE TABLE IF NOT EXISTS episodes (
  episode_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL UNIQUE,
  source_system TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  evidence_ids_json TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL,
  compiler TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_episodes_ended ON episodes(ended_at DESC);

-- Explicit participant context is session-scoped and user-controlled. It is
-- never inferred from a generic chat role. A first_person_speaker binding is
-- an affirmative declaration that unlabeled first-person references in this
-- session belong to one known participant; ordinary participants only provide
-- names and aliases for entity resolution.
CREATE TABLE IF NOT EXISTS session_participants (
  session_id TEXT NOT NULL,
  participant_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL,
  aliases_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(session_id,participant_id)
);
CREATE INDEX IF NOT EXISTS idx_session_participants_session ON session_participants(session_id,role,display_name);

CREATE TABLE IF NOT EXISTS knowledge_units (
  unit_id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL REFERENCES episodes(episode_id) ON DELETE CASCADE,
  evidence_id TEXT NOT NULL,
  unit_type TEXT NOT NULL,
  tier_hint TEXT NOT NULL,
  statement TEXT NOT NULL,
  normalized_key TEXT NOT NULL,
  confidence REAL NOT NULL,
  risk_tier TEXT NOT NULL,
  status TEXT NOT NULL,
  scope_json TEXT NOT NULL DEFAULT '[]',
  observed_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  processed_at TEXT,
  UNIQUE(evidence_id, normalized_key)
);
CREATE INDEX IF NOT EXISTS idx_units_episode ON knowledge_units(episode_id, observed_at);
CREATE INDEX IF NOT EXISTS idx_units_type ON knowledge_units(unit_type, observed_at DESC);

-- Structured semantics are additive and rebuildable. Keeping them in a
-- one-to-one table lets older readers continue to consume knowledge_units
-- while v2 readers can inspect attribution, semantic roles, time and exact
-- provenance without parsing the human-readable statement.
CREATE TABLE IF NOT EXISTS knowledge_unit_semantics (
  unit_id TEXT PRIMARY KEY REFERENCES knowledge_units(unit_id) ON DELETE CASCADE,
  schema_version TEXT NOT NULL,
  structure_json TEXT NOT NULL,
  quality_status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_unit_semantics_quality ON knowledge_unit_semantics(quality_status,updated_at DESC);

-- Memory Harness' semantic graph is its own graph, separate from the
-- Evidence-to-memory lineage DAG. Entities are project-scoped by default and
-- assertions are stored once; incoming/outgoing traversal is an indexed query
-- projection rather than two independently mutable facts.
CREATE TABLE IF NOT EXISTS memory_entities (
  entity_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  normalized_name TEXT NOT NULL,
  aliases_json TEXT NOT NULL DEFAULT '[]',
  properties_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0 CHECK(confidence BETWEEN 0.0 AND 1.0),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id,entity_type,normalized_name)
);
CREATE INDEX IF NOT EXISTS idx_memory_entities_project ON memory_entities(project_id,entity_type,canonical_name);

CREATE TABLE IF NOT EXISTS memory_assertions (
  assertion_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  unit_id TEXT NOT NULL REFERENCES knowledge_units(unit_id) ON DELETE CASCADE,
  subject_entity_id TEXT NOT NULL REFERENCES memory_entities(entity_id) ON DELETE RESTRICT,
  predicate TEXT NOT NULL,
  inverse_label TEXT NOT NULL DEFAULT '',
  object_entity_id TEXT REFERENCES memory_entities(entity_id) ON DELETE RESTRICT,
  object_literal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  valid_from TEXT,
  valid_until TEXT,
  recorded_at TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0 CHECK(confidence BETWEEN 0.0 AND 1.0),
  source_evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  run_id TEXT,
  stage_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id,unit_id,subject_entity_id,predicate,object_entity_id,object_literal)
);
CREATE INDEX IF NOT EXISTS idx_assertions_subject ON memory_assertions(project_id,subject_entity_id,predicate,status);
CREATE INDEX IF NOT EXISTS idx_assertions_object ON memory_assertions(project_id,object_entity_id,predicate,status);
CREATE INDEX IF NOT EXISTS idx_assertions_time ON memory_assertions(project_id,status,valid_from,valid_until);

CREATE TABLE IF NOT EXISTS memory_records (
  memory_id TEXT PRIMARY KEY,
  tier TEXT NOT NULL,
  asset_form TEXT NOT NULL,
  domain TEXT NOT NULL,
  status TEXT NOT NULL,
  summary TEXT NOT NULL,
  body TEXT NOT NULL,
  canonical_key TEXT NOT NULL,
  confidence REAL NOT NULL,
  importance REAL NOT NULL,
  strength REAL NOT NULL,
  evidence_ids_json TEXT NOT NULL,
  episode_ids_json TEXT NOT NULL,
  scopes_json TEXT NOT NULL DEFAULT '[]',
  visibility TEXT NOT NULL DEFAULT 'private',
  observed_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_reinforced_at TEXT,
  UNIQUE(tier, canonical_key)
);
CREATE INDEX IF NOT EXISTS idx_memory_tier_status ON memory_records(tier, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS memory_operations (
  operation_id TEXT PRIMARY KEY,
  operation_type TEXT NOT NULL,
  status TEXT NOT NULL,
  target_memory_id TEXT,
  unit_id TEXT,
  episode_id TEXT,
  evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  reason_codes_json TEXT NOT NULL DEFAULT '[]',
  risk_tier TEXT NOT NULL,
  confidence REAL NOT NULL,
  patch_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  decided_at TEXT,
  applied_at TEXT,
  reviewed_by TEXT
);
CREATE INDEX IF NOT EXISTS idx_operations_status ON memory_operations(status, created_at DESC);

CREATE TABLE IF NOT EXISTS living_views (
  view_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL DEFAULT '',
  view_type TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  source_memory_ids_json TEXT NOT NULL DEFAULT '[]',
  canonical_path TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_assets (
  asset_id TEXT PRIMARY KEY,
  asset_type TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  version TEXT NOT NULL,
  risk_level TEXT NOT NULL,
  source_memory_ids_json TEXT NOT NULL DEFAULT '[]',
  validation_status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_assets_status ON agent_assets(status, updated_at DESC);
CREATE TABLE IF NOT EXISTS agent_asset_classifications (
  asset_id TEXT PRIMARY KEY REFERENCES agent_assets(asset_id) ON DELETE CASCADE,
  classifier_version TEXT NOT NULL,
  classification_status TEXT NOT NULL,
  scores_json TEXT NOT NULL DEFAULT '{}',
  reasons_json TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_asset_classification_status ON agent_asset_classifications(classification_status,updated_at DESC);

-- M3 complete-application layer. Project IDs are registry-owned identifiers;
-- paths are never derived from user input. record_projects lets one canonical
-- source be referenced by multiple projects without duplicating Evidence.
CREATE TABLE IF NOT EXISTS projects (
  project_id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  color TEXT NOT NULL,
  default_currency TEXT NOT NULL,
  budget_minor INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS project_aliases (
  alias TEXT PRIMARY KEY COLLATE NOCASE,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS record_projects (
  record_type TEXT NOT NULL,
  record_id TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  relation TEXT NOT NULL DEFAULT 'member',
  is_primary INTEGER NOT NULL DEFAULT 0 CHECK(is_primary IN (0,1)),
  created_at TEXT NOT NULL,
  PRIMARY KEY(record_type,record_id,project_id)
);
CREATE INDEX IF NOT EXISTS idx_record_projects_project ON record_projects(project_id,record_type,record_id);

-- Owner-controlled homepage curation. A pin is presentation state only: it
-- never changes the memory content, lifecycle, importance score or Evidence
-- lineage, and it is intentionally separate from rebuildable derivatives.
CREATE TABLE IF NOT EXISTS memory_pins (
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  memory_id TEXT NOT NULL REFERENCES memory_records(memory_id) ON DELETE CASCADE,
  pinned_at TEXT NOT NULL,
  PRIMARY KEY(project_id,memory_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_pins_project ON memory_pins(project_id,pinned_at DESC,memory_id);

CREATE TABLE IF NOT EXISTS temporal_facts (
  fact_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  subject TEXT NOT NULL,
  predicate TEXT NOT NULL,
  object TEXT NOT NULL,
  status TEXT NOT NULL,
  observed_at TEXT,
  recorded_at TEXT NOT NULL,
  valid_from TEXT NOT NULL,
  valid_until TEXT,
  supersedes_fact_id TEXT REFERENCES temporal_facts(fact_id),
  source_memory_id TEXT,
  source_evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  confidence REAL NOT NULL DEFAULT 1.0
);
CREATE INDEX IF NOT EXISTS idx_facts_asof ON temporal_facts(project_id,status,valid_from,valid_until);
CREATE INDEX IF NOT EXISTS idx_facts_subject ON temporal_facts(project_id,subject,predicate,recorded_at DESC);

CREATE TABLE IF NOT EXISTS context_blocks (
  block_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  budget_chars INTEGER NOT NULL,
  read_only INTEGER NOT NULL DEFAULT 0 CHECK(read_only IN (0,1)),
  status TEXT NOT NULL,
  source_refs_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id,label)
);

CREATE TABLE IF NOT EXISTS project_goals (
  goal_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  target_at TEXT,
  source_evidence_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_goals_project ON project_goals(project_id,status,priority DESC,updated_at DESC);

-- Project tasks are deliberately separate from goals. AI-derived items remain
-- suggestions until the local Owner accepts them; external Agents cannot
-- promote their own suggestions into the Owner's authoritative todo list.
CREATE TABLE IF NOT EXISTS project_tasks (
  task_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK(status IN ('suggested','todo','in_progress','done','dismissed')),
  priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
  due_at TEXT,
  source_kind TEXT NOT NULL CHECK(source_kind IN ('manual','ai_suggestion')),
  source_record_id TEXT,
  source_evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id,source_kind,source_record_id)
);
CREATE INDEX IF NOT EXISTS idx_project_tasks_project ON project_tasks(project_id,status,due_at,priority,updated_at DESC);

-- Simple project-level automation remains understandable in the basic UI.
-- Advanced Blueprint/Pipeline configuration stays in its existing governed
-- stores and is linked from the project settings surface.
CREATE TABLE IF NOT EXISTS project_automation (
  project_id TEXT PRIMARY KEY REFERENCES projects(project_id) ON DELETE CASCADE,
  import_mode TEXT NOT NULL DEFAULT 'auto_new' CHECK(import_mode IN ('auto_new','manual')),
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_milestones (
  milestone_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  goal_id TEXT REFERENCES project_goals(goal_id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  due_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_milestones_project ON project_milestones(project_id,status,due_at);

CREATE TABLE IF NOT EXISTS project_decisions (
  decision_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  decision TEXT NOT NULL,
  rationale TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  decided_at TEXT NOT NULL,
  supersedes_decision_id TEXT REFERENCES project_decisions(decision_id),
  source_evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_decisions_project ON project_decisions(project_id,status,decided_at DESC);

CREATE TABLE IF NOT EXISTS project_risks (
  risk_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  probability INTEGER NOT NULL CHECK(probability BETWEEN 1 AND 5),
  impact INTEGER NOT NULL CHECK(impact BETWEEN 1 AND 5),
  status TEXT NOT NULL,
  mitigation TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  source_evidence_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_risks_project ON project_risks(project_id,status,impact DESC,probability DESC);

CREATE TABLE IF NOT EXISTS finance_accounts (
  account_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  account_type TEXT NOT NULL,
  currency TEXT NOT NULL,
  opening_balance_minor INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id,name,currency)
);

CREATE TABLE IF NOT EXISTS finance_entries (
  entry_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  account_id TEXT REFERENCES finance_accounts(account_id) ON DELETE SET NULL,
  entry_type TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK(amount_minor <> 0),
  currency TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  status TEXT NOT NULL,
  source_evidence_id TEXT,
  idempotency_key TEXT UNIQUE,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_finance_project ON finance_entries(project_id,status,currency,occurred_at DESC);

CREATE TABLE IF NOT EXISTS connectors (
  connector_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  cursor TEXT,
  config_json TEXT NOT NULL DEFAULT '{}',
  last_sync_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS import_batches (
  batch_id TEXT PRIMARY KEY,
  connector_id TEXT REFERENCES connectors(connector_id) ON DELETE SET NULL,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL,
  item_count INTEGER NOT NULL DEFAULT 0,
  evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  error TEXT,
  created_at TEXT NOT NULL,
  completed_at TEXT
);

CREATE TABLE IF NOT EXISTS recall_feedback (
  feedback_id TEXT PRIMARY KEY,
  context_id TEXT NOT NULL,
  project_id TEXT REFERENCES projects(project_id) ON DELETE SET NULL,
  result_id TEXT,
  rating TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

-- Agent access is separate from the local user's UI session. Tokens are shown
-- once and only their SHA-256 hashes are stored. Project grants are explicit;
-- all-project access must be opted into per principal.
CREATE TABLE IF NOT EXISTS agent_principals (
  agent_id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  permissions_json TEXT NOT NULL DEFAULT '[]',
  all_projects INTEGER NOT NULL DEFAULT 0 CHECK(all_projects IN (0,1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS agent_project_grants (
  agent_id TEXT NOT NULL REFERENCES agent_principals(agent_id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY(agent_id,project_id)
);

CREATE TABLE IF NOT EXISTS agent_audit_log (
  event_id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL REFERENCES agent_principals(agent_id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  project_id TEXT,
  status TEXT NOT NULL,
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_audit_principal ON agent_audit_log(agent_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_audit_project ON agent_audit_log(project_id,created_at DESC);

-- Owner sessions are short-lived process credentials and are deliberately not
-- persisted. This table stores only the audit trail for the privileged desktop
-- control plane; it never stores owner tokens or CSRF secrets.
CREATE TABLE IF NOT EXISTS owner_audit_log (
  event_id TEXT PRIMARY KEY,
  owner_session_id TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  status TEXT NOT NULL,
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_owner_audit_created ON owner_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_owner_audit_session ON owner_audit_log(owner_session_id,created_at DESC);

-- Memory Harness generic type/object plane. Existing six-layer tables remain
-- readable through the compatibility layer while these envelopes become the
-- extension boundary for new memory types.
CREATE TABLE IF NOT EXISTS harness_memory_types (
  type_id TEXT PRIMARY KEY,
  plugin_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  schema_version TEXT NOT NULL,
  schema_json TEXT NOT NULL,
  lifecycle_json TEXT NOT NULL,
  protection_class TEXT NOT NULL,
  renderer_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness_types_plugin ON harness_memory_types(plugin_id,status,type_id);

CREATE TABLE IF NOT EXISTS harness_objects (
  object_id TEXT PRIMARY KEY,
  type_id TEXT NOT NULL REFERENCES harness_memory_types(type_id),
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
  status TEXT NOT NULL,
  protection_class TEXT NOT NULL,
  current_revision INTEGER NOT NULL CHECK(current_revision >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness_objects_project ON harness_objects(project_id,type_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS harness_object_revisions (
  object_id TEXT NOT NULL REFERENCES harness_objects(object_id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK(revision >= 1),
  status TEXT NOT NULL,
  schema_version TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0 CHECK(confidence BETWEEN 0.0 AND 1.0),
  importance REAL NOT NULL DEFAULT 0.5 CHECK(importance BETWEEN 0.0 AND 1.0),
  valid_from TEXT NOT NULL,
  valid_until TEXT,
  source_evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  source_object_ids_json TEXT NOT NULL DEFAULT '[]',
  run_id TEXT,
  stage_id TEXT,
  plugin_id TEXT NOT NULL,
  plugin_version TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  living_asset_path TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY(object_id,revision),
  UNIQUE(plugin_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_harness_revisions_run ON harness_object_revisions(run_id,stage_id);

CREATE TABLE IF NOT EXISTS harness_object_relations (
  relation_id TEXT PRIMARY KEY,
  source_object_id TEXT NOT NULL REFERENCES harness_objects(object_id) ON DELETE CASCADE,
  target_object_id TEXT NOT NULL REFERENCES harness_objects(object_id) ON DELETE CASCADE,
  relation_type TEXT NOT NULL,
  run_id TEXT,
  plugin_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(source_object_id,target_object_id,relation_type)
);
CREATE INDEX IF NOT EXISTS idx_harness_relations_target ON harness_object_relations(target_object_id,relation_type);

-- Versioned pipeline and universal run/trace plane.
CREATE TABLE IF NOT EXISTS harness_pipeline_versions (
  pipeline_id TEXT NOT NULL,
  version TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  name TEXT NOT NULL,
  definition_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(pipeline_id,version),
  UNIQUE(pipeline_id,content_hash)
);

CREATE TABLE IF NOT EXISTS harness_pipeline_drafts (
  draft_id TEXT PRIMARY KEY,
  pipeline_id TEXT NOT NULL UNIQUE,
  plugin_id TEXT NOT NULL,
  base_version TEXT NOT NULL DEFAULT '',
  definition_json TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness_pipeline_drafts_updated ON harness_pipeline_drafts(updated_at DESC);

-- A blueprint is the project-level composition of memory growth,
-- organization and recall strategies. Published versions are immutable and
-- project assignments pin the exact content hash used by future runs.
CREATE TABLE IF NOT EXISTS harness_blueprint_versions (
  blueprint_id TEXT NOT NULL,
  version TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  definition_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(blueprint_id,version),
  UNIQUE(blueprint_id,content_hash)
);
CREATE INDEX IF NOT EXISTS idx_harness_blueprints_status ON harness_blueprint_versions(status,name,created_at DESC);

CREATE TABLE IF NOT EXISTS harness_project_blueprints (
  project_id TEXT PRIMARY KEY REFERENCES projects(project_id) ON DELETE CASCADE,
  blueprint_id TEXT NOT NULL,
  blueprint_version TEXT NOT NULL,
  blueprint_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  activated_by TEXT NOT NULL,
  activated_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(blueprint_id,blueprint_version) REFERENCES harness_blueprint_versions(blueprint_id,version) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_harness_project_blueprints_version ON harness_project_blueprints(blueprint_id,blueprint_version,status);

CREATE TABLE IF NOT EXISTS harness_runs (
  run_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
  caller_type TEXT NOT NULL,
  caller_id TEXT NOT NULL,
  channel TEXT NOT NULL,
  pipeline_id TEXT NOT NULL,
  pipeline_version TEXT NOT NULL,
  pipeline_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  retry_of_run_id TEXT,
  forked_from_run_id TEXT,
  created_at TEXT NOT NULL,
  started_at TEXT,
  ended_at TEXT,
  UNIQUE(project_id,pipeline_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_harness_runs_project ON harness_runs(project_id,status,created_at DESC);

CREATE TABLE IF NOT EXISTS harness_spans (
  span_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  parent_span_id TEXT REFERENCES harness_spans(span_id) ON DELETE SET NULL,
  node_id TEXT NOT NULL,
  stage_type TEXT NOT NULL,
  stage_version TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  attempt INTEGER NOT NULL CHECK(attempt >= 1),
  status TEXT NOT NULL,
  input_hash TEXT,
  output_hash TEXT,
  detail_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT NOT NULL,
  ended_at TEXT,
  UNIQUE(run_id,node_id,attempt)
);
CREATE INDEX IF NOT EXISTS idx_harness_spans_run ON harness_spans(run_id,started_at,span_id);

-- Immutable bounded stage outputs make completed prefixes replayable without
-- re-running earlier side effects. Historical runs created before this table
-- remain readable but can only be retried from their original input.
CREATE TABLE IF NOT EXISTS harness_stage_outputs (
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  output_hash TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(run_id,node_id)
);
CREATE INDEX IF NOT EXISTS idx_harness_stage_outputs_run ON harness_stage_outputs(run_id,created_at,node_id);

CREATE TABLE IF NOT EXISTS harness_events (
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  event_type TEXT NOT NULL,
  producer TEXT NOT NULL,
  schema_version TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  PRIMARY KEY(run_id,sequence)
);

CREATE TABLE IF NOT EXISTS harness_artifacts (
  artifact_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  span_id TEXT REFERENCES harness_spans(span_id) ON DELETE SET NULL,
  media_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  content_hash TEXT NOT NULL,
  retention_class TEXT NOT NULL,
  redaction_state TEXT NOT NULL,
  storage_ref TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_harness_artifacts_run ON harness_artifacts(run_id,created_at);

CREATE TABLE IF NOT EXISTS harness_object_links (
  link_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  span_id TEXT REFERENCES harness_spans(span_id) ON DELETE SET NULL,
  object_kind TEXT NOT NULL,
  object_id TEXT NOT NULL,
  relation TEXT NOT NULL,
  content_hash TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness_links_object ON harness_object_links(object_kind,object_id,created_at DESC);

CREATE TABLE IF NOT EXISTS harness_effects (
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  effect_key TEXT NOT NULL,
  provider_idempotency_key TEXT,
  status TEXT NOT NULL,
  outcome TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  result_hash TEXT,
  receipt_json TEXT NOT NULL DEFAULT '{}',
  intent_at TEXT NOT NULL,
  dispatched_at TEXT,
  received_at TEXT,
  materialized_at TEXT,
  PRIMARY KEY(run_id,node_id,effect_key)
);
CREATE INDEX IF NOT EXISTS idx_harness_effects_status ON harness_effects(status,outcome,intent_at);

-- Durable pipeline checkpoints and owner review decisions. A paused run can be
-- resumed after a process restart without re-running already completed stages.
CREATE TABLE IF NOT EXISTS harness_run_checkpoints (
  run_id TEXT PRIMARY KEY REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  next_node_index INTEGER NOT NULL CHECK(next_node_index >= 0),
  input_json TEXT NOT NULL,
  outputs_json TEXT NOT NULL DEFAULT '{}',
  effective_capabilities_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS harness_reviews (
  review_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES harness_runs(run_id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL,
  requested_by TEXT NOT NULL,
  request_json TEXT NOT NULL DEFAULT '{}',
  decision_by TEXT,
  decision_note TEXT,
  created_at TEXT NOT NULL,
  decided_at TEXT,
  UNIQUE(run_id,node_id)
);
CREATE INDEX IF NOT EXISTS idx_harness_reviews_status ON harness_reviews(status,created_at DESC);

-- Immutable object-revision proposals. A proposal appends a revision without
-- moving harness_objects.current_revision; only an Owner decision activates it.
CREATE TABLE IF NOT EXISTS harness_revision_reviews (
  review_id TEXT PRIMARY KEY,
  object_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  base_revision INTEGER NOT NULL CHECK(base_revision >= 1),
  edit_reason TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  target_status TEXT NOT NULL,
  requested_by TEXT NOT NULL,
  decision_by TEXT,
  decision_note TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  validation_json TEXT NOT NULL DEFAULT '{}',
  rollback_from_revision INTEGER,
  created_at TEXT NOT NULL,
  decided_at TEXT,
  activated_at TEXT,
  UNIQUE(object_id,revision),
  FOREIGN KEY(object_id,revision) REFERENCES harness_object_revisions(object_id,revision) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_harness_revision_reviews_status ON harness_revision_reviews(status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_harness_revision_reviews_object ON harness_revision_reviews(object_id,revision DESC);

-- Installed plugin packages and signer trust. Package payloads are stored as
-- immutable, content-addressed snapshots so historical runs stay inspectable
-- even after a plugin is disabled or superseded.
CREATE TABLE IF NOT EXISTS harness_trusted_signers (
  signer_id TEXT PRIMARY KEY,
  publisher TEXT NOT NULL,
  public_key_base64 TEXT NOT NULL,
  fingerprint TEXT NOT NULL UNIQUE,
  scope_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  approved_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS harness_plugin_versions (
  plugin_id TEXT NOT NULL,
  version TEXT NOT NULL,
  name TEXT NOT NULL,
  publisher TEXT NOT NULL,
  trust_class TEXT NOT NULL,
  signer_id TEXT REFERENCES harness_trusted_signers(signer_id) ON DELETE SET NULL,
  signature_status TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  manifest_yaml TEXT NOT NULL,
  package_blob BLOB,
  package_size INTEGER NOT NULL CHECK(package_size >= 0),
  permissions_json TEXT NOT NULL DEFAULT '[]',
  contributions_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  installed_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(plugin_id,version),
  UNIQUE(content_hash)
);
CREATE INDEX IF NOT EXISTS idx_harness_plugins_status ON harness_plugin_versions(plugin_id,status,installed_at DESC);

CREATE TABLE IF NOT EXISTS harness_plugin_project_state (
  plugin_id TEXT NOT NULL,
  version TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  granted_capabilities_json TEXT NOT NULL DEFAULT '[]',
  config_json TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL,
  PRIMARY KEY(plugin_id,version,project_id),
  FOREIGN KEY(plugin_id,version) REFERENCES harness_plugin_versions(plugin_id,version) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS harness_plugin_audit (
  event_id TEXT PRIMARY KEY,
  plugin_id TEXT NOT NULL,
  version TEXT,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_harness_plugin_audit ON harness_plugin_audit(plugin_id,created_at DESC);

-- Model profiles never contain API keys. The active secret lives in the
-- operating-system credential store and is addressed by provider_id.
CREATE TABLE IF NOT EXISTS model_providers (
  provider_id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
	protocol TEXT NOT NULL DEFAULT 'openai_chat',
  base_url TEXT NOT NULL,
  model TEXT NOT NULL,
  status TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
  has_secret INTEGER NOT NULL DEFAULT 0 CHECK(has_secret IN (0,1)),
  last_test_status TEXT,
  last_test_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_pricing (
  provider_id TEXT PRIMARY KEY REFERENCES model_providers(provider_id) ON DELETE CASCADE,
  currency TEXT NOT NULL DEFAULT '',
  input_per_million_minor INTEGER NOT NULL DEFAULT 0 CHECK(input_per_million_minor>=0),
  output_per_million_minor INTEGER NOT NULL DEFAULT 0 CHECK(output_per_million_minor>=0),
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_call_observations (
  call_id TEXT PRIMARY KEY, run_id TEXT NOT NULL DEFAULT '', node_id TEXT NOT NULL DEFAULT '',
  project_id TEXT NOT NULL DEFAULT '', stage_type TEXT NOT NULL DEFAULT '',
  provider_id TEXT NOT NULL, provider_kind TEXT NOT NULL, model TEXT NOT NULL, status TEXT NOT NULL,
  usage_source TEXT NOT NULL, prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0, reasoning_tokens INTEGER NOT NULL DEFAULT 0, cached_prompt_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0, currency TEXT NOT NULL DEFAULT '', estimated_cost_microminor INTEGER NOT NULL DEFAULT 0,
  pricing_source TEXT NOT NULL, error_code TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_model_calls_run ON model_call_observations(run_id,created_at);
CREATE INDEX IF NOT EXISTS idx_model_calls_project ON model_call_observations(project_id,created_at);
CREATE INDEX IF NOT EXISTS idx_model_calls_provider ON model_call_observations(provider_id,created_at);

-- Knowledge Product block locks are Owner governance metadata. They never replace
-- the canonical Object/Revision payload and can be rebuilt/removed independently.
CREATE TABLE IF NOT EXISTS knowledge_product_block_locks (
  object_id TEXT NOT NULL,
  block_id TEXT NOT NULL,
  block_label TEXT NOT NULL DEFAULT '',
  base_revision INTEGER NOT NULL,
  base_content_hash TEXT NOT NULL,
  base_content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(object_id,block_id)
);
CREATE INDEX IF NOT EXISTS idx_product_block_locks_object ON knowledge_product_block_locks(object_id);

CREATE TABLE IF NOT EXISTS model_runtime (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  mode TEXT NOT NULL,
  active_provider_id TEXT REFERENCES model_providers(provider_id) ON DELETE SET NULL,
  fallback_to_rules INTEGER NOT NULL DEFAULT 1 CHECK(fallback_to_rules IN (0,1)),
  updated_at TEXT NOT NULL
);
INSERT OR IGNORE INTO model_runtime(singleton,mode,active_provider_id,fallback_to_rules,updated_at)
VALUES(1,'rules',NULL,1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));

INSERT OR IGNORE INTO projects(project_id,slug,name,description,status,color,default_currency,budget_minor,created_at,updated_at)
VALUES
  ('project-inbox','inbox','收件箱','尚未明确归属的来源与记忆','active','#6B746E','CNY',0,strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  ('project-personal','personal','个人','跨项目的个人身份、偏好与长期目标','active','#A45B47','CNY',0,strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now'));
INSERT OR IGNORE INTO project_aliases(alias,project_id,created_at) VALUES
  ('inbox','project-inbox',strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  ('收件箱','project-inbox',strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  ('personal','project-personal',strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  ('个人','project-personal',strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- Existing M2 records remain immutable and are made visible through Inbox.
INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at)
SELECT 'evidence',evidence_id,'project-inbox','fallback',1,strftime('%Y-%m-%dT%H:%M:%fZ','now') FROM evidence_receipts;
INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at)
SELECT 'episode',episode_id,'project-inbox','fallback',1,strftime('%Y-%m-%dT%H:%M:%fZ','now') FROM episodes;
INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at)
SELECT 'memory',memory_id,'project-inbox','fallback',1,strftime('%Y-%m-%dT%H:%M:%fZ','now') FROM memory_records;

-- Preserve existing automatically derived goals while exposing them as
-- reviewable todo suggestions. This migration is additive and idempotent.
INSERT OR IGNORE INTO project_tasks(task_id,project_id,title,description,status,priority,due_at,source_kind,source_record_id,source_evidence_ids_json,created_at,updated_at)
SELECT 'suggested-' || goal_id,project_id,title,description,'suggested',
       CASE WHEN priority BETWEEN 1 AND 5 THEN priority ELSE 3 END,target_at,
       'ai_suggestion',goal_id,
       CASE WHEN source_evidence_id IS NULL OR source_evidence_id='' THEN '[]' ELSE json_array(source_evidence_id) END,
       created_at,updated_at
FROM project_goals WHERE goal_id LIKE 'auto-goal-%';

INSERT INTO meta(key,value) VALUES ('schema_version','1.0')
ON CONFLICT(key) DO UPDATE SET value=excluded.value;`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "harness_revision_reviews", "edit_reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "living_views", "project_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "model_providers", "protocol", "TEXT NOT NULL DEFAULT 'openai_chat'"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_living_views_project ON living_views(project_id,status,updated_at DESC)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`DELETE FROM record_projects WHERE record_type='living' AND record_id IN (SELECT view_id FROM living_views WHERE project_id='')`); err != nil {
		db.Close()
		return nil, err
	}
	return &ControlStore{DB: db}, nil
}

func OpenSearch(path string) (*SearchStore, error) {
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	schema := `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT OR IGNORE INTO meta(key,value) VALUES ('schema_version','0.1');
CREATE TABLE IF NOT EXISTS turns (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  evidence_id TEXT NOT NULL UNIQUE,
  line_hash TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source_system TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  role TEXT,
  scope_json TEXT NOT NULL DEFAULT '[]',
  ordinal INTEGER NOT NULL,
  body TEXT NOT NULL,
  ledger_rel_path TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_turns_session_ord ON turns(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_turns_observed ON turns(observed_at);
CREATE INDEX IF NOT EXISTS idx_turns_source ON turns(source_system);
CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(body, tokenize='unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS turns_tri USING fts5(body, tokenize='trigram');

CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  doc_key TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  source_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  valid_from TEXT,
  valid_until TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_documents_project ON documents(project_id,kind,status,observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_documents_source ON documents(kind,source_id);
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(title,body, tokenize='unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS documents_tri USING fts5(title,body, tokenize='trigram');
CREATE TABLE IF NOT EXISTS document_embeddings (
  document_id INTEGER PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
  algorithm TEXT NOT NULL,
  dimensions INTEGER NOT NULL,
  vector BLOB NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_document_embeddings_algorithm ON document_embeddings(algorithm,dimensions);
INSERT INTO meta(key,value) VALUES ('schema_version','0.4')
ON CONFLICT(key) DO UPDATE SET value=excluded.value;`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := backfillDocumentEmbeddings(db); err != nil {
		db.Close()
		return nil, err
	}
	return &SearchStore{DB: db}, nil
}

func backfillDocumentEmbeddings(db *sql.DB) error {
	rows, err := db.Query(`SELECT d.id,d.title,d.body FROM documents d LEFT JOIN document_embeddings e ON e.document_id=d.id WHERE e.document_id IS NULL`)
	if err != nil {
		return err
	}
	type missing struct {
		id          int64
		title, body string
	}
	items := []missing{}
	for rows.Next() {
		var item missing
		if err := rows.Scan(&item.id, &item.title, &item.body); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		if err := upsertDocumentEmbedding(context.Background(), tx, item.id, item.title, item.body); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertDocumentEmbedding(ctx context.Context, tx *sql.Tx, documentID int64, title, body string) error {
	vector := localembedding.Encode(strings.TrimSpace(title + "\n" + body))
	_, err := tx.ExecContext(ctx, `INSERT INTO document_embeddings(document_id,algorithm,dimensions,vector,updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(document_id) DO UPDATE SET algorithm=excluded.algorithm,dimensions=excluded.dimensions,vector=excluded.vector,updated_at=excluded.updated_at`,
		documentID, localembedding.Algorithm, localembedding.Dimensions, localembedding.Marshal(vector), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *ControlStore) Close() error { return s.DB.Close() }
func (s *SearchStore) Close() error  { return s.DB.Close() }

func (s *ControlStore) Receipt(ctx context.Context, id string) (Receipt, bool, error) {
	var r Receipt
	err := s.DB.QueryRowContext(ctx, `SELECT evidence_id,line_hash,source_system,session_id,observed_at,captured_at,ledger_rel_path,ordinal FROM evidence_receipts WHERE evidence_id=?`, id).
		Scan(&r.EvidenceID, &r.LineHash, &r.SourceSystem, &r.SessionID, &r.ObservedAt, &r.CapturedAt, &r.LedgerRelPath, &r.Ordinal)
	if err == sql.ErrNoRows {
		return Receipt{}, false, nil
	}
	return r, err == nil, err
}

func (s *ControlStore) UpsertReceipt(ctx context.Context, r Receipt) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO evidence_receipts(evidence_id,line_hash,source_system,session_id,observed_at,captured_at,ledger_rel_path,ordinal,recorded_at)
VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(evidence_id) DO UPDATE SET line_hash=excluded.line_hash,source_system=excluded.source_system,session_id=excluded.session_id,observed_at=excluded.observed_at,captured_at=excluded.captured_at,ledger_rel_path=excluded.ledger_rel_path,ordinal=excluded.ordinal`,
		r.EvidenceID, r.LineHash, r.SourceSystem, r.SessionID, r.ObservedAt, r.CapturedAt, r.LedgerRelPath, r.Ordinal, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES('evidence',?,'project-inbox','fallback',1,?)`, r.EvidenceID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ControlStore) CountReceipts(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM evidence_receipts`).Scan(&n)
	return n, err
}

func (s *SearchStore) Reset(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM turns_fts`, `DELETE FROM turns_tri`, `DELETE FROM turns`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Rebuild replaces the derived search index in one transaction. The producer is
// called while the transaction is open and may stream turns from the canonical
// JSONL ledger without materializing the whole corpus in memory.
func (s *SearchStore) Rebuild(ctx context.Context, producer func(add func(IndexedTurn) error) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM turns_fts`, `DELETE FROM turns_tri`, `DELETE FROM turns`, `DELETE FROM sqlite_sequence WHERE name='turns'`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	turnStmt, err := tx.PrepareContext(ctx, `INSERT INTO turns(evidence_id,line_hash,session_id,source_system,observed_at,captured_at,role,scope_json,ordinal,body,ledger_rel_path) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer turnStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `INSERT INTO turns_fts(rowid,body) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()
	triStmt, err := tx.PrepareContext(ctx, `INSERT INTO turns_tri(rowid,body) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer triStmt.Close()
	add := func(t IndexedTurn) error {
		res, err := turnStmt.ExecContext(ctx, t.EvidenceID, t.LineHash, t.SessionID, t.SourceSystem, t.ObservedAt, t.CapturedAt, t.Role, t.ScopeJSON, t.Ordinal, t.Body, t.LedgerRelPath)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = ftsStmt.ExecContext(ctx, id, t.Body); err != nil {
			return err
		}
		if _, err = triStmt.ExecContext(ctx, id, t.Body); err != nil {
			return err
		}
		return nil
	}
	if err := producer(add); err != nil {
		return err
	}
	return tx.Commit()
}

type IndexedTurn struct {
	EvidenceID    string
	LineHash      string
	SessionID     string
	SourceSystem  string
	ObservedAt    string
	CapturedAt    string
	Role          string
	ScopeJSON     string
	Ordinal       int
	Body          string
	LedgerRelPath string
}

type IndexedDocument struct {
	DocKey       string
	Kind         string
	SourceID     string
	ProjectID    string
	Title        string
	Body         string
	Status       string
	ObservedAt   string
	ValidFrom    string
	ValidUntil   string
	MetadataJSON string
}

func (s *SearchStore) UpsertDocument(ctx context.Context, d IndexedDocument) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM documents WHERE doc_key=?`, d.DocKey).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_fts WHERE rowid=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_tri WHERE rowid=?`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE documents SET kind=?,source_id=?,project_id=?,title=?,body=?,status=?,observed_at=?,valid_from=nullif(?,''),valid_until=nullif(?,''),metadata_json=? WHERE id=?`,
			d.Kind, d.SourceID, d.ProjectID, d.Title, d.Body, d.Status, d.ObservedAt, d.ValidFrom, d.ValidUntil, d.MetadataJSON, id)
		if err != nil {
			return err
		}
	} else {
		result, err := tx.ExecContext(ctx, `INSERT INTO documents(doc_key,kind,source_id,project_id,title,body,status,observed_at,valid_from,valid_until,metadata_json) VALUES(?,?,?,?,?,?,?,?,nullif(?,''),nullif(?,''),?)`,
			d.DocKey, d.Kind, d.SourceID, d.ProjectID, d.Title, d.Body, d.Status, d.ObservedAt, d.ValidFrom, d.ValidUntil, d.MetadataJSON)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO documents_fts(rowid,title,body) VALUES(?,?,?)`, id, d.Title, d.Body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO documents_tri(rowid,title,body) VALUES(?,?,?)`, id, d.Title, d.Body); err != nil {
		return err
	}
	if err := upsertDocumentEmbedding(ctx, tx, id, d.Title, d.Body); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SearchStore) DeleteDocumentsByKindAndProject(ctx context.Context, projectID, kind string) error {
	projectID = strings.TrimSpace(projectID)
	kind = strings.TrimSpace(kind)
	if projectID == "" || kind == "" {
		return errors.New("project_id and kind are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM documents WHERE project_id=? AND kind=?`, projectID, kind)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_fts WHERE rowid=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_tri WHERE rowid=?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE project_id=? AND kind=?`, projectID, kind); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SearchStore) DeleteDocumentsForProject(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("project_id is required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM documents WHERE project_id=?`, projectID)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_fts WHERE rowid=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM documents_tri WHERE rowid=?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE project_id=?`, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SearchStore) DeleteDocuments(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM documents_fts`, `DELETE FROM documents_tri`, `DELETE FROM documents`, `DELETE FROM sqlite_sequence WHERE name='documents'`} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SearchStore) UpsertTurn(ctx context.Context, t IndexedTurn) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM turns WHERE evidence_id=?`, t.EvidenceID).Scan(&existingID)
	if err == nil {
		var h string
		if err := tx.QueryRowContext(ctx, `SELECT line_hash FROM turns WHERE id=?`, existingID).Scan(&h); err != nil {
			return err
		}
		if h != t.LineHash {
			return fmt.Errorf("search index evidence conflict for %s", t.EvidenceID)
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO turns(evidence_id,line_hash,session_id,source_system,observed_at,captured_at,role,scope_json,ordinal,body,ledger_rel_path) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		t.EvidenceID, t.LineHash, t.SessionID, t.SourceSystem, t.ObservedAt, t.CapturedAt, t.Role, t.ScopeJSON, t.Ordinal, t.Body, t.LedgerRelPath)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO turns_fts(rowid,body) VALUES(?,?)`, id, t.Body); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO turns_tri(rowid,body) VALUES(?,?)`, id, t.Body); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SearchStore) CountTurns(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns`).Scan(&n)
	return n, err
}
