package harness

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/store"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,159}$`)

type Service struct{ control *store.ControlStore }

func New(control *store.ControlStore) *Service { return &Service{control: control} }

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func randomID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func canonicalJSON(value any) ([]byte, error) { return json.Marshal(value) }

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func validateLifecycle(lifecycle Lifecycle) (Lifecycle, error) {
	lifecycle.Initial = strings.TrimSpace(lifecycle.Initial)
	lifecycle.States = normalizeStrings(lifecycle.States)
	if lifecycle.Initial == "" || len(lifecycle.States) == 0 {
		return lifecycle, errors.New("lifecycle initial and states are required")
	}
	states := map[string]bool{}
	for _, state := range lifecycle.States {
		if !identifierPattern.MatchString(state) {
			return lifecycle, fmt.Errorf("invalid lifecycle state %q", state)
		}
		states[state] = true
	}
	if !states[lifecycle.Initial] {
		return lifecycle, errors.New("lifecycle initial must be one of states")
	}
	if lifecycle.Transitions == nil {
		lifecycle.Transitions = map[string][]string{}
	}
	for from, targets := range lifecycle.Transitions {
		if !states[from] {
			return lifecycle, fmt.Errorf("transition source %q is not a state", from)
		}
		targets = normalizeStrings(targets)
		for _, target := range targets {
			if !states[target] {
				return lifecycle, fmt.Errorf("transition target %q is not a state", target)
			}
		}
		lifecycle.Transitions[from] = targets
	}
	return lifecycle, nil
}

func (s *Service) RegisterType(ctx context.Context, input RegisterTypeInput) (MemoryType, error) {
	input.TypeID = strings.ToLower(strings.TrimSpace(input.TypeID))
	input.PluginID = strings.ToLower(strings.TrimSpace(input.PluginID))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	input.ProtectionClass = strings.ToLower(strings.TrimSpace(input.ProtectionClass))
	if !identifierPattern.MatchString(input.TypeID) || !identifierPattern.MatchString(input.PluginID) {
		return MemoryType{}, errors.New("type_id and plugin_id must be stable namespaced identifiers")
	}
	if input.DisplayName == "" || input.SchemaVersion == "" {
		return MemoryType{}, errors.New("display_name and schema_version are required")
	}
	if input.ProtectionClass == "" {
		input.ProtectionClass = "standard"
	}
	_, schemaCanonical, err := validateSchema(input.Schema)
	if err != nil {
		return MemoryType{}, fmt.Errorf("schema: %w", err)
	}
	lifecycle, err := validateLifecycle(input.Lifecycle)
	if err != nil {
		return MemoryType{}, err
	}
	lifecycleRaw, _ := canonicalJSON(lifecycle)
	renderer := input.Renderer
	if len(renderer) == 0 {
		renderer = json.RawMessage(`{}`)
	}
	_, rendererCanonical, err := decodeJSON(renderer, maxSchemaBytes)
	if err != nil {
		return MemoryType{}, fmt.Errorf("renderer: %w", err)
	}
	now := nowString()
	result, err := s.control.DB.ExecContext(ctx, `INSERT INTO harness_memory_types(type_id,plugin_id,display_name,schema_version,schema_json,lifecycle_json,protection_class,renderer_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,'enabled',?,?) ON CONFLICT(type_id) DO NOTHING`, input.TypeID, input.PluginID, input.DisplayName, input.SchemaVersion, string(schemaCanonical), string(lifecycleRaw), input.ProtectionClass, string(rendererCanonical), now, now)
	if err != nil {
		return MemoryType{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		existing, err := s.Type(ctx, input.TypeID)
		if err != nil {
			return MemoryType{}, err
		}
		if existing.PluginID != input.PluginID || existing.SchemaVersion != input.SchemaVersion || string(existing.Schema) != string(schemaCanonical) || string(mustJSON(existing.Lifecycle)) != string(lifecycleRaw) {
			return MemoryType{}, errors.New("type_id already exists with a different immutable contract")
		}
		if existing.Status != "enabled" {
			if _, err := s.control.DB.ExecContext(ctx, `UPDATE harness_memory_types SET status='enabled',updated_at=? WHERE type_id=?`, now, input.TypeID); err != nil {
				return MemoryType{}, err
			}
		}
	}
	return s.Type(ctx, input.TypeID)
}

func mustJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }

func (s *Service) Type(ctx context.Context, typeID string) (MemoryType, error) {
	var item MemoryType
	var schemaRaw, lifecycleRaw, rendererRaw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT type_id,plugin_id,display_name,schema_version,schema_json,lifecycle_json,protection_class,renderer_json,status,created_at,updated_at FROM harness_memory_types WHERE type_id=?`, strings.TrimSpace(typeID)).Scan(&item.TypeID, &item.PluginID, &item.DisplayName, &item.SchemaVersion, &schemaRaw, &lifecycleRaw, &item.ProtectionClass, &rendererRaw, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return MemoryType{}, err
	}
	item.Schema = json.RawMessage(schemaRaw)
	item.Renderer = json.RawMessage(rendererRaw)
	if err := json.Unmarshal([]byte(lifecycleRaw), &item.Lifecycle); err != nil {
		return MemoryType{}, err
	}
	return item, nil
}

func (s *Service) ListTypes(ctx context.Context) ([]MemoryType, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT type_id FROM harness_memory_types WHERE status='enabled' ORDER BY display_name,type_id`)
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
	items := make([]MemoryType, 0, len(ids))
	for _, id := range ids {
		item, err := s.Type(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func allowedTransition(lifecycle Lifecycle, from, to string) bool {
	if from == to {
		return true
	}
	for _, candidate := range lifecycle.Transitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func (s *Service) Materialize(ctx context.Context, input MaterializeInput) (Object, error) {
	input.TypeID = strings.TrimSpace(input.TypeID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.PluginID = strings.TrimSpace(input.PluginID)
	input.PluginVersion = strings.TrimSpace(input.PluginVersion)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.TypeID == "" || input.ProjectID == "" || input.PluginID == "" || input.PluginVersion == "" || input.IdempotencyKey == "" {
		return Object{}, errors.New("type_id, project_id, plugin_id, plugin_version and idempotency_key are required")
	}
	typeDef, err := s.Type(ctx, input.TypeID)
	if err != nil {
		return Object{}, err
	}
	if typeDef.Status != "enabled" || typeDef.PluginID != input.PluginID {
		return Object{}, errors.New("memory type is not enabled for the materializing plugin")
	}
	schema, _, err := validateSchema(typeDef.Schema)
	if err != nil {
		return Object{}, err
	}
	payload, err := validatePayload(schema, input.Payload)
	if err != nil {
		return Object{}, fmt.Errorf("payload: %w", err)
	}
	if input.Confidence == 0 {
		input.Confidence = 1
	}
	if input.Importance == 0 {
		input.Importance = .5
	}
	if input.Confidence < 0 || input.Confidence > 1 || input.Importance < 0 || input.Importance > 1 {
		return Object{}, errors.New("confidence and importance must be between 0 and 1")
	}
	if input.ValidFrom == "" {
		input.ValidFrom = nowString()
	} else if _, err := time.Parse(time.RFC3339, input.ValidFrom); err != nil {
		return Object{}, errors.New("valid_from must be RFC3339")
	}
	if input.ValidUntil != "" {
		if _, err := time.Parse(time.RFC3339, input.ValidUntil); err != nil {
			return Object{}, errors.New("valid_until must be RFC3339")
		}
	}
	if input.Status == "" {
		input.Status = typeDef.Lifecycle.Initial
	}
	stateKnown := false
	for _, state := range typeDef.Lifecycle.States {
		stateKnown = stateKnown || state == input.Status
	}
	if !stateKnown {
		return Object{}, fmt.Errorf("unknown lifecycle status %q", input.Status)
	}
	var duplicateObjectID string
	err = s.control.DB.QueryRowContext(ctx, `SELECT object_id FROM harness_object_revisions WHERE plugin_id=? AND idempotency_key=?`, input.PluginID, input.IdempotencyKey).Scan(&duplicateObjectID)
	if err == nil {
		item, getErr := s.Object(ctx, duplicateObjectID)
		item.Duplicate = true
		return item, getErr
	}
	if err != sql.ErrNoRows {
		return Object{}, err
	}
	if input.ObjectID == "" {
		input.ObjectID, err = randomID("obj-")
		if err != nil {
			return Object{}, err
		}
	}
	now := nowString()
	evidenceRaw, _ := json.Marshal(normalizeStrings(input.SourceEvidenceIDs))
	objectsRaw, _ := json.Marshal(normalizeStrings(input.SourceObjectIDs))
	hash := contracts.HashBytes(payload)
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Object{}, err
	}
	defer tx.Rollback()
	var currentRevision int
	var currentStatus, currentType, currentProject string
	err = tx.QueryRowContext(ctx, `SELECT type_id,project_id,status,current_revision FROM harness_objects WHERE object_id=?`, input.ObjectID).Scan(&currentType, &currentProject, &currentStatus, &currentRevision)
	if err == sql.ErrNoRows {
		currentRevision = 0
		if input.Status != typeDef.Lifecycle.Initial {
			return Object{}, errors.New("new object must start at the lifecycle initial state")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO harness_objects(object_id,type_id,project_id,status,protection_class,current_revision,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?)`, input.ObjectID, input.TypeID, input.ProjectID, input.Status, typeDef.ProtectionClass, now, now)
	} else if err == nil {
		if currentType != input.TypeID || currentProject != input.ProjectID {
			return Object{}, errors.New("object type and project are immutable")
		}
		if typeDef.ProtectionClass == "protected" && (currentStatus == "active" || input.Status == "active") {
			return Object{}, errors.New("protected active objects must be revised and activated through owner revision review")
		}
		if !allowedTransition(typeDef.Lifecycle, currentStatus, input.Status) {
			return Object{}, fmt.Errorf("lifecycle transition %s -> %s is not allowed", currentStatus, input.Status)
		}
		_, err = tx.ExecContext(ctx, `UPDATE harness_objects SET status=?,current_revision=?,updated_at=? WHERE object_id=?`, input.Status, currentRevision+1, now, input.ObjectID)
	} else {
		return Object{}, err
	}
	if err != nil {
		return Object{}, err
	}
	revision := currentRevision + 1
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_object_revisions(object_id,revision,status,schema_version,payload_json,content_hash,confidence,importance,valid_from,valid_until,source_evidence_ids_json,source_object_ids_json,run_id,stage_id,plugin_id,plugin_version,idempotency_key,living_asset_path,created_at) VALUES(?,?,?,?,?,?,?,?,?,nullif(?,''),?,?,nullif(?,''),nullif(?,''),?,?,?,nullif(?,''),?)`, input.ObjectID, revision, input.Status, typeDef.SchemaVersion, string(payload), hash, input.Confidence, input.Importance, input.ValidFrom, input.ValidUntil, string(evidenceRaw), string(objectsRaw), input.RunID, input.StageID, input.PluginID, input.PluginVersion, input.IdempotencyKey, input.LivingAssetPath, now)
	if err != nil {
		return Object{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES('harness_object',?,?, 'member',1,?)`, input.ObjectID, input.ProjectID, now); err != nil {
		return Object{}, err
	}
	if err := tx.Commit(); err != nil {
		return Object{}, err
	}
	return s.Object(ctx, input.ObjectID)
}

func (s *Service) Object(ctx context.Context, objectID string) (Object, error) {
	var item Object
	var payloadRaw, evidenceRaw, objectsRaw string
	var validUntil, runID, stageID, livingPath sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT o.object_id,o.type_id,o.project_id,o.status,o.protection_class,o.current_revision,o.created_at,o.updated_at,r.revision,r.status,r.schema_version,r.payload_json,r.content_hash,r.confidence,r.importance,r.valid_from,r.valid_until,r.source_evidence_ids_json,r.source_object_ids_json,r.run_id,r.stage_id,r.plugin_id,r.plugin_version,r.idempotency_key,r.living_asset_path,r.created_at FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE o.object_id=?`, strings.TrimSpace(objectID)).Scan(&item.ObjectID, &item.TypeID, &item.ProjectID, &item.Status, &item.ProtectionClass, &item.CurrentRevision, &item.CreatedAt, &item.UpdatedAt, &item.Revision.Revision, &item.Revision.Status, &item.Revision.SchemaVersion, &payloadRaw, &item.Revision.ContentHash, &item.Revision.Confidence, &item.Revision.Importance, &item.Revision.ValidFrom, &validUntil, &evidenceRaw, &objectsRaw, &runID, &stageID, &item.Revision.PluginID, &item.Revision.PluginVersion, &item.Revision.IdempotencyKey, &livingPath, &item.Revision.CreatedAt)
	if err != nil {
		return Object{}, err
	}
	item.Revision.ObjectID = item.ObjectID
	item.Revision.Payload = json.RawMessage(payloadRaw)
	_ = json.Unmarshal([]byte(evidenceRaw), &item.Revision.SourceEvidenceIDs)
	_ = json.Unmarshal([]byte(objectsRaw), &item.Revision.SourceObjectIDs)
	item.Revision.ValidUntil = validUntil.String
	item.Revision.RunID = runID.String
	item.Revision.StageID = stageID.String
	item.Revision.LivingAssetPath = livingPath.String
	return item, nil
}

func (s *Service) ListObjects(ctx context.Context, projectID, typeID, status string, limit int) ([]Object, error) {
	items, _, err := s.ListObjectsPage(ctx, ObjectListOptions{ProjectID: projectID, TypeID: typeID, Status: status, Limit: limit})
	return items, err
}

func (s *Service) ListObjectsPage(ctx context.Context, options ObjectListOptions) ([]Object, PageInfo, error) {
	projectID := strings.TrimSpace(options.ProjectID)
	typeID := strings.TrimSpace(options.TypeID)
	status := strings.TrimSpace(options.Status)
	if projectID == "" {
		return nil, PageInfo{}, errors.New("project_id is required")
	}
	limit, offset := normalizePage(options.Limit, options.Offset)
	clauses := []string{"project_id=?"}
	args := []any{projectID}
	if typeID != "" {
		clauses = append(clauses, "type_id=?")
		args = append(args, typeID)
	}
	if status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, status)
	}
	for _, excluded := range normalizeStrings(options.ExcludedTypeIDs) {
		clauses = append(clauses, "type_id<>?")
		args = append(args, excluded)
	}
	for _, prefix := range normalizeStrings(options.ExcludedTypePrefixes) {
		if prefix == "" {
			continue
		}
		clauses = append(clauses, "type_id NOT LIKE ?")
		args = append(args, prefix+"%")
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_objects WHERE `+where, args...).Scan(&total); err != nil {
		return nil, PageInfo{}, err
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.control.DB.QueryContext(ctx, `SELECT object_id FROM harness_objects WHERE `+where+` ORDER BY updated_at DESC,object_id LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, PageInfo{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, PageInfo{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, PageInfo{}, err
	}
	if err := rows.Close(); err != nil {
		return nil, PageInfo{}, err
	}
	items := make([]Object, 0, len(ids))
	for _, id := range ids {
		item, err := s.Object(ctx, id)
		if err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, item)
	}
	page := PageInfo{Total: total, Limit: limit, Offset: offset, HasMore: offset+len(items) < total}
	return items, page, nil
}
