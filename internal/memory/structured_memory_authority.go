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

func StructuredMemoryObjectID(projectID, memoryID string) string {
	return stableID("obj-memory-v1-", strings.TrimSpace(projectID), strings.TrimSpace(memoryID))
}

func normalizeMemoryRecord(item MemoryRecord) MemoryRecord {
	item.MemoryID = strings.TrimSpace(item.MemoryID)
	item.Tier = strings.ToLower(strings.TrimSpace(item.Tier))
	item.AssetForm = strings.TrimSpace(item.AssetForm)
	item.Domain = strings.TrimSpace(item.Domain)
	item.Status = strings.TrimSpace(item.Status)
	item.Summary = strings.TrimSpace(item.Summary)
	item.Body = strings.TrimSpace(item.Body)
	item.Visibility = strings.TrimSpace(item.Visibility)
	item.EvidenceIDs = unique(item.EvidenceIDs)
	item.EpisodeIDs = unique(item.EpisodeIDs)
	item.Scopes = unique(item.Scopes)
	if item.CanonicalKey == "" {
		item.CanonicalKey = normalizeStatement(item.Body)
	}
	return item
}

func StructuredMemoryPayload(item MemoryRecord) (json.RawMessage, error) {
	item = normalizeMemoryRecord(item)
	if err := ValidateStructuredMemoryRevision(item, item); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(item)
	return json.RawMessage(raw), err
}

func DecodeStructuredMemoryPayload(raw json.RawMessage) (MemoryRecord, error) {
	var item MemoryRecord
	if len(raw) == 0 || !json.Valid(raw) {
		return item, errors.New("structured memory payload must be valid JSON")
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return item, err
	}
	return normalizeMemoryRecord(item), nil
}

func PrepareStructuredMemoryRevision(base, candidate MemoryRecord) (MemoryRecord, error) {
	base = normalizeMemoryRecord(base)
	if strings.TrimSpace(candidate.MemoryID) != "" && strings.TrimSpace(candidate.MemoryID) != base.MemoryID {
		return MemoryRecord{}, errors.New("memory_id is immutable")
	}
	if len(candidate.EvidenceIDs) > 0 && !equalStringSets(candidate.EvidenceIDs, base.EvidenceIDs) {
		return MemoryRecord{}, errors.New("source_evidence_ids are immutable; append correction Evidence instead")
	}
	if len(candidate.EpisodeIDs) > 0 && !equalStringSets(candidate.EpisodeIDs, base.EpisodeIDs) {
		return MemoryRecord{}, errors.New("source_episode_ids are immutable")
	}
	if strings.TrimSpace(candidate.ObservedAt) != "" && strings.TrimSpace(candidate.ObservedAt) != base.ObservedAt {
		return MemoryRecord{}, errors.New("observed_at is anchored to source Evidence")
	}
	if strings.TrimSpace(candidate.CreatedAt) != "" && strings.TrimSpace(candidate.CreatedAt) != base.CreatedAt {
		return MemoryRecord{}, errors.New("created_at is immutable")
	}
	candidate = normalizeMemoryRecord(candidate)
	candidate.MemoryID = base.MemoryID
	candidate.EvidenceIDs = append([]string(nil), base.EvidenceIDs...)
	candidate.EpisodeIDs = append([]string(nil), base.EpisodeIDs...)
	candidate.ObservedAt = base.ObservedAt
	candidate.CreatedAt = base.CreatedAt
	candidate.LastReinforcedAt = base.LastReinforcedAt
	candidate.Status = base.Status
	candidate.CanonicalKey = normalizeStatement(candidate.Body)
	candidate.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := ValidateStructuredMemoryRevision(base, candidate); err != nil {
		return MemoryRecord{}, err
	}
	return candidate, nil
}

func equalStringSets(left, right []string) bool {
	a, b := unique(left), unique(right)
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range a {
		seen[value] = true
	}
	for _, value := range b {
		if !seen[value] {
			return false
		}
	}
	return true
}

func ValidateStructuredMemoryRevision(base, candidate MemoryRecord) error {
	if candidate.MemoryID == "" {
		return errors.New("memory_id is required")
	}
	if base.MemoryID != "" && candidate.MemoryID != base.MemoryID {
		return errors.New("memory_id is immutable")
	}
	allowedTier := map[string]bool{"episodic": true, "semantic": true, "procedural": true, "identity_core": true}
	if !allowedTier[candidate.Tier] {
		return fmt.Errorf("unsupported tier %q", candidate.Tier)
	}
	if candidate.Body == "" || len([]rune(candidate.Body)) > 12000 {
		return errors.New("body must contain 1-12000 characters")
	}
	if len([]rune(candidate.Summary)) > 1200 {
		return errors.New("summary exceeds 1200 characters")
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 || candidate.Importance < 0 || candidate.Importance > 1 || candidate.Strength < 0 {
		return errors.New("confidence/importance must be 0-1 and strength non-negative")
	}
	if !equalStringSets(candidate.EvidenceIDs, base.EvidenceIDs) || !equalStringSets(candidate.EpisodeIDs, base.EpisodeIDs) {
		return errors.New("memory source provenance is immutable")
	}
	for _, value := range []struct{ name, raw string }{{"observed_at", candidate.ObservedAt}, {"created_at", candidate.CreatedAt}, {"updated_at", candidate.UpdatedAt}, {"last_reinforced_at", candidate.LastReinforcedAt}} {
		if strings.TrimSpace(value.raw) == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value.raw); err != nil {
			return fmt.Errorf("%s must be RFC3339", value.name)
		}
	}
	return nil
}

func (e *Engine) overlayStructuredMemoryObjects(ctx context.Context, projectID string, items []MemoryRecord) error {
	if strings.TrimSpace(projectID) == "" || len(items) == 0 {
		return nil
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT r.payload_json FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE o.project_id=? AND o.type_id=? AND o.status='active'`, strings.TrimSpace(projectID), StructuredMemoryRecordTypeV1)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[string]MemoryRecord{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		item, err := DecodeStructuredMemoryPayload(json.RawMessage(raw))
		if err != nil {
			return err
		}
		byID[item.MemoryID] = item
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if current, ok := byID[items[i].MemoryID]; ok {
			items[i] = current
		}
	}
	return nil
}

func (e *Engine) MemoryForProject(ctx context.Context, projectID, memoryID string) (MemoryRecord, error) {
	projectID, memoryID = strings.TrimSpace(projectID), strings.TrimSpace(memoryID)
	if projectID == "" || memoryID == "" {
		return MemoryRecord{}, errors.New("project_id and memory_id are required")
	}
	var linked int
	if err := e.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE record_type='memory' AND record_id=? AND project_id=?`, memoryID, projectID).Scan(&linked); err != nil {
		return MemoryRecord{}, err
	}
	if linked == 0 {
		return MemoryRecord{}, sql.ErrNoRows
	}
	item, err := e.Memory(ctx, memoryID)
	if err != nil {
		return MemoryRecord{}, err
	}
	items := []MemoryRecord{item}
	if err := e.overlayStructuredMemoryObjects(ctx, projectID, items); err != nil {
		return MemoryRecord{}, err
	}
	return items[0], nil
}
