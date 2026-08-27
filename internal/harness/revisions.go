package harness

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/assettemplate"
	"github.com/luoyif/memory-harness/internal/contracts"
)

var ErrStaleRevisionReview = errors.New("revision proposal base is no longer current")

func (s *Service) ObjectRevision(ctx context.Context, objectID string, revision int) (ObjectRevision, error) {
	var item ObjectRevision
	var payloadRaw, evidenceRaw, objectsRaw string
	var validUntil, runID, stageID, livingPath sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT object_id,revision,status,schema_version,payload_json,content_hash,confidence,importance,valid_from,valid_until,source_evidence_ids_json,source_object_ids_json,run_id,stage_id,plugin_id,plugin_version,idempotency_key,living_asset_path,created_at FROM harness_object_revisions WHERE object_id=? AND revision=?`, strings.TrimSpace(objectID), revision).Scan(
		&item.ObjectID, &item.Revision, &item.Status, &item.SchemaVersion, &payloadRaw, &item.ContentHash, &item.Confidence, &item.Importance, &item.ValidFrom, &validUntil, &evidenceRaw, &objectsRaw, &runID, &stageID, &item.PluginID, &item.PluginVersion, &item.IdempotencyKey, &livingPath, &item.CreatedAt)
	if err != nil {
		return ObjectRevision{}, err
	}
	item.Payload = json.RawMessage(payloadRaw)
	_ = json.Unmarshal([]byte(evidenceRaw), &item.SourceEvidenceIDs)
	_ = json.Unmarshal([]byte(objectsRaw), &item.SourceObjectIDs)
	item.ValidUntil, item.RunID, item.StageID, item.LivingAssetPath = validUntil.String, runID.String, stageID.String, livingPath.String
	return item, nil
}

func (s *Service) ListObjectRevisions(ctx context.Context, objectID string, limit int) ([]ObjectRevision, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT revision FROM harness_object_revisions WHERE object_id=? ORDER BY revision DESC LIMIT ?`, strings.TrimSpace(objectID), limit)
	if err != nil {
		return nil, err
	}
	ids := []int{}
	for rows.Next() {
		var revision int
		if err := rows.Scan(&revision); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, revision)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]ObjectRevision, 0, len(ids))
	for _, revision := range ids {
		item, err := s.ObjectRevision(ctx, objectID, revision)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func revisionDiff(before, after json.RawMessage) json.RawMessage {
	var left, right map[string]any
	if json.Unmarshal(before, &left) != nil {
		left = map[string]any{"value": string(before)}
	}
	if json.Unmarshal(after, &right) != nil {
		right = map[string]any{"value": string(after)}
	}
	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	changed := []string{}
	for key := range keys {
		a, _ := json.Marshal(left[key])
		b, _ := json.Marshal(right[key])
		if string(a) != string(b) {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	raw, _ := json.Marshal(map[string]any{"changed_fields": changed, "before": left, "after": right})
	return raw
}

func canonicalOptionalJSON(raw json.RawMessage, fallback string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(fallback), nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	return json.RawMessage(canonical), err
}

func (s *Service) ProposeRevision(ctx context.Context, objectID string, input ProposeRevisionInput) (RevisionReview, error) {
	current, err := s.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return RevisionReview{}, err
	}
	if input.ExpectedRevision <= 0 {
		return RevisionReview{}, errors.New("expected_revision is required")
	}
	if input.ExpectedRevision != current.CurrentRevision {
		return RevisionReview{}, fmt.Errorf("%w: expected %d, current %d", ErrStaleRevisionReview, input.ExpectedRevision, current.CurrentRevision)
	}
	input.EditReason = strings.TrimSpace(input.EditReason)
	if input.EditReason == "" {
		return RevisionReview{}, errors.New("edit_reason is required")
	}
	typeDef, err := s.Type(ctx, current.TypeID)
	if err != nil {
		return RevisionReview{}, err
	}
	schema, _, err := validateSchema(typeDef.Schema)
	if err != nil {
		return RevisionReview{}, err
	}
	payload, err := validatePayload(schema, input.Payload)
	if err != nil {
		return RevisionReview{}, fmt.Errorf("payload: %w", err)
	}
	input.PluginID = strings.TrimSpace(input.PluginID)
	if input.PluginID == "" {
		input.PluginID = current.Revision.PluginID
	}
	input.PluginVersion = strings.TrimSpace(input.PluginVersion)
	if input.PluginVersion == "" {
		input.PluginVersion = current.Revision.PluginVersion
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return RevisionReview{}, errors.New("idempotency_key is required")
	}
	input.RequestedBy = strings.TrimSpace(input.RequestedBy)
	if input.RequestedBy == "" {
		input.RequestedBy = "owner"
	}
	if input.PluginID != typeDef.PluginID {
		return RevisionReview{}, errors.New("revision plugin does not own object type")
	}
	input.TargetStatus = strings.TrimSpace(input.TargetStatus)
	if input.TargetStatus == "" {
		input.TargetStatus = current.Status
	}
	known := false
	for _, state := range typeDef.Lifecycle.States {
		known = known || state == input.TargetStatus
	}
	if !known {
		return RevisionReview{}, fmt.Errorf("unknown lifecycle status %q", input.TargetStatus)
	}
	if !allowedTransition(typeDef.Lifecycle, current.Status, input.TargetStatus) {
		return RevisionReview{}, fmt.Errorf("lifecycle transition %s -> %s is not allowed", current.Status, input.TargetStatus)
	}
	if input.Confidence == 0 {
		input.Confidence = current.Revision.Confidence
	}
	if input.Importance == 0 {
		input.Importance = current.Revision.Importance
	}
	if input.Confidence < 0 || input.Confidence > 1 || input.Importance < 0 || input.Importance > 1 {
		return RevisionReview{}, errors.New("confidence and importance must be between 0 and 1")
	}
	if input.ValidFrom == "" {
		input.ValidFrom = current.Revision.ValidFrom
	}
	if input.ValidFrom == "" {
		input.ValidFrom = nowString()
	}
	if _, err := time.Parse(time.RFC3339, input.ValidFrom); err != nil {
		return RevisionReview{}, errors.New("valid_from must be RFC3339")
	}
	if input.ValidUntil != "" {
		if _, err := time.Parse(time.RFC3339, input.ValidUntil); err != nil {
			return RevisionReview{}, errors.New("valid_until must be RFC3339")
		}
	}
	if len(input.SourceEvidenceIDs) == 0 {
		input.SourceEvidenceIDs = current.Revision.SourceEvidenceIDs
	}
	if len(input.SourceObjectIDs) == 0 {
		input.SourceObjectIDs = current.Revision.SourceObjectIDs
	}
	validation, err := canonicalOptionalJSON(input.Validation, `{"status":"not_run"}`)
	if err != nil {
		return RevisionReview{}, fmt.Errorf("validation: %w", err)
	}
	if current.TypeID == GovernedAgentAssetTypeV3 {
		payload, validation, err = validateGovernedAgentAssetPayload(payload)
		if err != nil {
			return RevisionReview{}, fmt.Errorf("asset validation: %w", err)
		}
		payload, validation, err = s.deepValidateGovernedAgentAsset(ctx, current, payload, validation)
		if err != nil {
			return RevisionReview{}, fmt.Errorf("deep asset validation: %w", err)
		}
	} else if current.TypeID == GovernedAgentAssetTypeV4 {
		payload, validation, err = assettemplate.ValidatePayload(payload)
		if err != nil {
			return RevisionReview{}, fmt.Errorf("asset template validation: %w", err)
		}
		payload, validation, err = s.deepValidateGovernedAgentAsset(ctx, current, payload, validation)
		if err != nil {
			return RevisionReview{}, fmt.Errorf("deep asset validation: %w", err)
		}
	}
	var duplicateObject string
	var duplicateRevision int
	err = s.control.DB.QueryRowContext(ctx, `SELECT object_id,revision FROM harness_object_revisions WHERE plugin_id=? AND idempotency_key=?`, input.PluginID, input.IdempotencyKey).Scan(&duplicateObject, &duplicateRevision)
	if err == nil {
		return s.RevisionReviewForRevision(ctx, duplicateObject, duplicateRevision)
	}
	if err != sql.ErrNoRows {
		return RevisionReview{}, err
	}
	now := nowString()
	reviewID, err := randomID("revreview-")
	if err != nil {
		return RevisionReview{}, err
	}
	evidenceRaw, _ := json.Marshal(normalizeStrings(input.SourceEvidenceIDs))
	objectsRaw, _ := json.Marshal(normalizeStrings(input.SourceObjectIDs))
	hash := contracts.HashBytes(payload)
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return RevisionReview{}, err
	}
	defer tx.Rollback()
	var maxRevision int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(revision),0) FROM harness_object_revisions WHERE object_id=?`, current.ObjectID).Scan(&maxRevision); err != nil {
		return RevisionReview{}, err
	}
	revision := maxRevision + 1
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_object_revisions(object_id,revision,status,schema_version,payload_json,content_hash,confidence,importance,valid_from,valid_until,source_evidence_ids_json,source_object_ids_json,run_id,stage_id,plugin_id,plugin_version,idempotency_key,living_asset_path,created_at) VALUES(?,?,?,?,?,?,?,?,?,nullif(?,''),?,?,NULL,NULL,?,?,?,NULL,?)`, current.ObjectID, revision, input.TargetStatus, typeDef.SchemaVersion, string(payload), hash, input.Confidence, input.Importance, input.ValidFrom, input.ValidUntil, string(evidenceRaw), string(objectsRaw), input.PluginID, input.PluginVersion, input.IdempotencyKey, now)
	if err != nil {
		return RevisionReview{}, err
	}
	diff := revisionDiff(current.Revision.Payload, payload)
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_revision_reviews(review_id,object_id,revision,base_revision,edit_reason,status,target_status,requested_by,diff_json,validation_json,rollback_from_revision,created_at) VALUES(?,?,?,?,?,'pending',?,?,?,?,nullif(?,0),?)`, reviewID, current.ObjectID, revision, current.CurrentRevision, input.EditReason, input.TargetStatus, input.RequestedBy, string(diff), string(validation), input.RollbackFrom, now)
	if err != nil {
		return RevisionReview{}, err
	}
	if err := tx.Commit(); err != nil {
		return RevisionReview{}, err
	}
	return s.RevisionReview(ctx, reviewID)
}

func (s *Service) RevisionReview(ctx context.Context, reviewID string) (RevisionReview, error) {
	var item RevisionReview
	var decisionBy, decidedAt, activatedAt sql.NullString
	var diffRaw, validationRaw string
	var rollback sql.NullInt64
	err := s.control.DB.QueryRowContext(ctx, `SELECT review_id,object_id,revision,base_revision,edit_reason,status,target_status,requested_by,decision_by,decision_note,diff_json,validation_json,rollback_from_revision,created_at,decided_at,activated_at FROM harness_revision_reviews WHERE review_id=?`, strings.TrimSpace(reviewID)).Scan(&item.ReviewID, &item.ObjectID, &item.Revision, &item.BaseRevision, &item.EditReason, &item.Status, &item.TargetStatus, &item.RequestedBy, &decisionBy, &item.DecisionNote, &diffRaw, &validationRaw, &rollback, &item.CreatedAt, &decidedAt, &activatedAt)
	if err != nil {
		return RevisionReview{}, err
	}
	item.DecisionBy, item.DecidedAt, item.ActivatedAt = decisionBy.String, decidedAt.String, activatedAt.String
	item.RollbackFromRevision = int(rollback.Int64)
	item.Diff = json.RawMessage(diffRaw)
	item.Validation = json.RawMessage(validationRaw)
	item.ProposedRevision, err = s.ObjectRevision(ctx, item.ObjectID, item.Revision)
	return item, err
}

func (s *Service) RevisionReviewForRevision(ctx context.Context, objectID string, revision int) (RevisionReview, error) {
	var id string
	if err := s.control.DB.QueryRowContext(ctx, `SELECT review_id FROM harness_revision_reviews WHERE object_id=? AND revision=?`, objectID, revision).Scan(&id); err != nil {
		return RevisionReview{}, err
	}
	return s.RevisionReview(ctx, id)
}

func (s *Service) ListRevisionReviews(ctx context.Context, objectID, status string, limit int) ([]RevisionReview, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT review_id FROM harness_revision_reviews WHERE (?='' OR object_id=?) AND (?='' OR status=?) ORDER BY created_at DESC LIMIT ?`, strings.TrimSpace(objectID), strings.TrimSpace(objectID), strings.TrimSpace(status), strings.TrimSpace(status), limit)
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
	rows.Close()
	out := make([]RevisionReview, 0, len(ids))
	for _, id := range ids {
		item, err := s.RevisionReview(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) DecideRevisionReview(ctx context.Context, reviewID, decision, by, note string) (RevisionReview, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approve" && decision != "reject" {
		return RevisionReview{}, errors.New("decision must be approve or reject")
	}
	by = strings.TrimSpace(by)
	if by == "" {
		by = "owner"
	}
	preview, err := s.RevisionReview(ctx, reviewID)
	if err != nil {
		return RevisionReview{}, err
	}
	currentObject, err := s.Object(ctx, preview.ObjectID)
	if err != nil {
		return RevisionReview{}, err
	}
	typeDef, err := s.Type(ctx, currentObject.TypeID)
	if err != nil {
		return RevisionReview{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return RevisionReview{}, err
	}
	defer tx.Rollback()
	var objectID, status, targetStatus string
	var revision, baseRevision int
	if err := tx.QueryRowContext(ctx, `SELECT object_id,revision,base_revision,status,target_status FROM harness_revision_reviews WHERE review_id=?`, strings.TrimSpace(reviewID)).Scan(&objectID, &revision, &baseRevision, &status, &targetStatus); err != nil {
		return RevisionReview{}, err
	}
	if status != "pending" {
		return RevisionReview{}, errors.New("revision review is not pending")
	}
	if decision == "reject" {
		_, err = tx.ExecContext(ctx, `UPDATE harness_revision_reviews SET status='rejected',decision_by=?,decision_note=?,decided_at=? WHERE review_id=?`, by, strings.TrimSpace(note), now, reviewID)
		if err != nil {
			return RevisionReview{}, err
		}
		if err = tx.Commit(); err != nil {
			return RevisionReview{}, err
		}
		return s.RevisionReview(ctx, reviewID)
	}
	var currentRevision int
	var currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision,status FROM harness_objects WHERE object_id=?`, objectID).Scan(&currentRevision, &currentStatus); err != nil {
		return RevisionReview{}, err
	}
	if currentRevision != baseRevision {
		_, _ = tx.ExecContext(ctx, `UPDATE harness_revision_reviews SET status='stale',decision_by=?,decision_note=?,decided_at=? WHERE review_id=?`, by, "base revision changed before approval", now, reviewID)
		_ = tx.Commit()
		return s.RevisionReview(ctx, reviewID)
	}
	if targetStatus == "active" && typeDef.ProtectionClass == "protected" {
		var validation map[string]any
		_ = json.Unmarshal(preview.Validation, &validation)
		validationStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(validation["status"])))
		if validationStatus != "passed" && validationStatus != "manual_passed" {
			return RevisionReview{}, errors.New("protected revision must pass validation before activation")
		}
	}
	if !allowedTransition(typeDef.Lifecycle, currentStatus, targetStatus) {
		return RevisionReview{}, fmt.Errorf("lifecycle transition %s -> %s is not allowed", currentStatus, targetStatus)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE harness_objects SET current_revision=?,status=?,updated_at=? WHERE object_id=?`, revision, targetStatus, now, objectID); err != nil {
		return RevisionReview{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE harness_revision_reviews SET status='approved',decision_by=?,decision_note=?,decided_at=?,activated_at=? WHERE review_id=?`, by, strings.TrimSpace(note), now, now, reviewID); err != nil {
		return RevisionReview{}, err
	}
	if targetStatus == "active" && typeDef.TypeID == GovernedAgentAssetTypeV4 {
		var payload struct {
			AssetID string `json:"asset_id"`
		}
		_ = json.Unmarshal(preview.ProposedRevision.Payload, &payload)
		if strings.TrimSpace(payload.AssetID) != "" {
			if _, err = tx.ExecContext(ctx, `UPDATE harness_objects SET status='superseded',updated_at=? WHERE object_id<>? AND status='active' AND type_id IN (?,?,?) AND EXISTS (SELECT 1 FROM harness_object_revisions r WHERE r.object_id=harness_objects.object_id AND r.revision=harness_objects.current_revision AND json_extract(r.payload_json,'$.asset_id')=?)`, now, objectID, GovernedAgentAssetTypeV2, GovernedAgentAssetTypeV3, GovernedAgentAssetTypeV4, payload.AssetID); err != nil {
				return RevisionReview{}, err
			}
		}
	}
	_, _ = tx.ExecContext(ctx, `UPDATE harness_revision_reviews SET status='stale',decision_note='another proposal was activated first',decided_at=? WHERE object_id=? AND status='pending' AND review_id<>? AND base_revision=?`, now, objectID, reviewID, baseRevision)
	if err = tx.Commit(); err != nil {
		return RevisionReview{}, err
	}
	return s.RevisionReview(ctx, reviewID)
}

func (s *Service) ProposeRollback(ctx context.Context, objectID string, targetRevision int, requestedBy string) (RevisionReview, error) {
	current, err := s.Object(ctx, objectID)
	if err != nil {
		return RevisionReview{}, err
	}
	target, err := s.ObjectRevision(ctx, objectID, targetRevision)
	if err != nil {
		return RevisionReview{}, err
	}
	if targetRevision == current.CurrentRevision {
		return RevisionReview{}, errors.New("target revision is already current")
	}
	validation, _ := json.Marshal(map[string]any{"status": "passed", "kind": "rollback", "restore_from_revision": targetRevision, "restore_from_hash": target.ContentHash})
	return s.ProposeRevision(ctx, objectID, ProposeRevisionInput{Payload: target.Payload, ExpectedRevision: current.CurrentRevision, EditReason: fmt.Sprintf("rollback to revision %d", targetRevision), TargetStatus: current.Status, Confidence: target.Confidence, Importance: target.Importance, ValidFrom: nowString(), SourceEvidenceIDs: target.SourceEvidenceIDs, SourceObjectIDs: target.SourceObjectIDs, PluginID: current.Revision.PluginID, PluginVersion: current.Revision.PluginVersion, IdempotencyKey: fmt.Sprintf("rollback:%s:%d:%d:%d", objectID, current.CurrentRevision, targetRevision, time.Now().UTC().UnixNano()), RequestedBy: requestedBy, Validation: validation, RollbackFrom: targetRevision})
}
