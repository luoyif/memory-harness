package growth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
)

type ProductBlock struct {
	BlockID          string `json:"block_id"`
	Label            string `json:"label"`
	Content          string `json:"content"`
	ContentHash      string `json:"content_hash"`
	Locked           bool   `json:"locked"`
	LockBaseRevision int    `json:"lock_base_revision,omitempty"`
	LockBaseHash     string `json:"lock_base_hash,omitempty"`
}
type ProductMergeBlock struct {
	BlockID       string `json:"block_id"`
	Label         string `json:"label"`
	Locked        bool   `json:"locked"`
	Base          string `json:"base"`
	Current       string `json:"current"`
	Candidate     string `json:"candidate"`
	Merged        string `json:"merged"`
	Status        string `json:"status"`
	RequiresOwner bool   `json:"requires_owner"`
}

type ProductMergePreview struct {
	ObjectID          string              `json:"object_id"`
	CurrentRevision   int                 `json:"current_revision"`
	CandidateBodyHash string              `json:"candidate_body_hash"`
	MergedBody        string              `json:"merged_body"`
	HasConflicts      bool                `json:"has_conflicts"`
	Blocks            []ProductMergeBlock `json:"blocks"`
}

type productBlockLock struct {
	BlockID      string
	Label        string
	BaseRevision int
	BaseHash     string
	BaseContent  string
}

func productBlockHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func productBlockID(label string, occurrence int) string {
	key := strings.ToLower(strings.TrimSpace(label))
	return "section-" + productBlockHash(fmt.Sprintf("%s#%d", key, occurrence))[:12]
}

func splitProductBlocks(body string) []ProductBlock {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	blocks := []ProductBlock{}
	start, label := 0, "Document header"
	counts := map[string]int{}
	flush := func(end int) {
		content := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if content == "" {
			return
		}
		counts[label]++
		blocks = append(blocks, ProductBlock{BlockID: productBlockID(label, counts[label]), Label: label, Content: content, ContentHash: productBlockHash(content)})
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			flush(i)
			start = i
			label = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## "))
		}
	}
	flush(len(lines))
	return blocks
}
func (s *Service) productBlockLocks(ctx context.Context, objectID string) (map[string]productBlockLock, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT block_id,block_label,base_revision,base_content_hash,base_content FROM knowledge_product_block_locks WHERE object_id=? ORDER BY block_id`, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]productBlockLock{}
	for rows.Next() {
		var item productBlockLock
		if err := rows.Scan(&item.BlockID, &item.Label, &item.BaseRevision, &item.BaseHash, &item.BaseContent); err != nil {
			return nil, err
		}
		out[item.BlockID] = item
	}
	return out, rows.Err()
}

func productPayloadBody(object harness.Object) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(object.Revision.Payload, &payload); err != nil {
		return "", err
	}
	body, _ := payload["body"].(string)
	if strings.TrimSpace(body) == "" {
		return "", errors.New("knowledge product body is empty")
	}
	return body, nil
}

func (s *Service) ProductBlocks(ctx context.Context, objectID string) ([]ProductBlock, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}
	if object.TypeID != harness.KnowledgeProductTypeV1 {
		return nil, errors.New("object is not a knowledge product")
	}
	body, err := productPayloadBody(object)
	if err != nil {
		return nil, err
	}
	locks, err := s.productBlockLocks(ctx, object.ObjectID)
	if err != nil {
		return nil, err
	}
	blocks := splitProductBlocks(body)
	for i := range blocks {
		if lock, ok := locks[blocks[i].BlockID]; ok {
			blocks[i].Locked = true
			blocks[i].LockBaseRevision = lock.BaseRevision
			blocks[i].LockBaseHash = lock.BaseHash
		}
	}
	return blocks, nil
}
func (s *Service) SetProductBlockLocks(ctx context.Context, objectID string, expectedRevision int, blockIDs []string) ([]ProductBlock, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}
	if object.TypeID != harness.KnowledgeProductTypeV1 {
		return nil, errors.New("object is not a knowledge product")
	}
	if expectedRevision <= 0 || object.CurrentRevision != expectedRevision {
		return nil, errors.New("knowledge product revision changed; reload before editing block locks")
	}
	body, err := productPayloadBody(object)
	if err != nil {
		return nil, err
	}
	blocks := splitProductBlocks(body)
	byID := map[string]ProductBlock{}
	for _, block := range blocks {
		byID[block.BlockID] = block
	}
	wanted := map[string]bool{}
	for _, id := range blockIDs {
		id = strings.TrimSpace(id)
		if id == "" || wanted[id] {
			continue
		}
		if _, ok := byID[id]; !ok {
			return nil, fmt.Errorf("unknown product block %q", id)
		}
		wanted[id] = true
	}
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for id := range wanted {
		block := byID[id]
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_product_block_locks(object_id,block_id,block_label,base_revision,base_content_hash,base_content,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(object_id,block_id) DO UPDATE SET block_label=excluded.block_label,updated_at=excluded.updated_at`, object.ObjectID, id, block.Label, object.CurrentRevision, block.ContentHash, block.Content, now, now)
		if err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT block_id FROM knowledge_product_block_locks WHERE object_id=?`, object.ObjectID)
	if err != nil {
		return nil, err
	}
	toDelete := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if !wanted[id] {
			toDelete = append(toDelete, id)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, id := range toDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_product_block_locks WHERE object_id=? AND block_id=?`, object.ObjectID, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ProductBlocks(ctx, object.ObjectID)
}

func blocksByID(blocks []ProductBlock) map[string]ProductBlock {
	out := map[string]ProductBlock{}
	for _, block := range blocks {
		out[block.BlockID] = block
	}
	return out
}

func joinProductBlocks(blocks []ProductBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Content) != "" {
			parts = append(parts, strings.TrimSpace(block.Content))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}
func (s *Service) mergeLockedProductBody(ctx context.Context, object harness.Object, candidateBody string) (ProductMergePreview, error) {
	currentBody, err := productPayloadBody(object)
	if err != nil {
		return ProductMergePreview{}, err
	}
	locks, err := s.productBlockLocks(ctx, object.ObjectID)
	if err != nil {
		return ProductMergePreview{}, err
	}
	currentBlocks, candidateBlocks := splitProductBlocks(currentBody), splitProductBlocks(candidateBody)
	currentByID := blocksByID(currentBlocks)
	mergedBlocks := make([]ProductBlock, 0, len(candidateBlocks)+len(locks))
	preview := ProductMergePreview{ObjectID: object.ObjectID, CurrentRevision: object.CurrentRevision, CandidateBodyHash: productBlockHash(candidateBody), Blocks: []ProductMergeBlock{}}
	seen := map[string]bool{}
	for _, candidate := range candidateBlocks {
		seen[candidate.BlockID] = true
		current := currentByID[candidate.BlockID]
		item := ProductMergeBlock{BlockID: candidate.BlockID, Label: candidate.Label, Current: current.Content, Candidate: candidate.Content, Merged: candidate.Content, Status: "candidate", RequiresOwner: false}
		if lock, ok := locks[candidate.BlockID]; ok {
			item.Locked, item.Base = true, lock.BaseContent
			chosen := current.Content
			if strings.TrimSpace(chosen) == "" {
				chosen = lock.BaseContent
			}
			item.Merged = chosen
			currentChanged := productBlockHash(chosen) != lock.BaseHash
			candidateChanged := candidate.ContentHash != lock.BaseHash
			if currentChanged && candidateChanged && strings.TrimSpace(chosen) != strings.TrimSpace(candidate.Content) {
				item.Status, item.RequiresOwner, preview.HasConflicts = "diverged_locked", true, true
			} else {
				item.Status = "locked_preserved"
			}
		}
		mergedBlocks = append(mergedBlocks, ProductBlock{BlockID: item.BlockID, Label: item.Label, Content: item.Merged, ContentHash: productBlockHash(item.Merged), Locked: item.Locked})
		preview.Blocks = append(preview.Blocks, item)
	}
	for blockID, lock := range locks {
		if seen[blockID] {
			continue
		}
		current := currentByID[blockID]
		chosen := current.Content
		if strings.TrimSpace(chosen) == "" {
			chosen = lock.BaseContent
		}
		item := ProductMergeBlock{BlockID: blockID, Label: lock.Label, Locked: true, Base: lock.BaseContent, Current: current.Content, Candidate: "", Merged: chosen, Status: "candidate_missing_locked", RequiresOwner: true}
		preview.HasConflicts = true
		preview.Blocks = append(preview.Blocks, item)
		mergedBlocks = append(mergedBlocks, ProductBlock{BlockID: blockID, Label: lock.Label, Content: chosen, ContentHash: productBlockHash(chosen), Locked: true})
	}
	preview.MergedBody = joinProductBlocks(mergedBlocks)
	return preview, nil
}

func (s *Service) ProductMergePreview(ctx context.Context, objectID string) (ProductMergePreview, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return ProductMergePreview{}, err
	}
	if object.TypeID != harness.KnowledgeProductTypeV1 {
		return ProductMergePreview{}, errors.New("object is not a knowledge product")
	}
	var current map[string]any
	if err := json.Unmarshal(object.Revision.Payload, &current); err != nil {
		return ProductMergePreview{}, err
	}
	if strings.TrimSpace(fmt.Sprint(current["product_type"])) != "project_brief" {
		return ProductMergePreview{}, errors.New("merge preview is available for auto-generated project briefs")
	}
	_, generatedObjectID, _, raw, err := s.projectBriefPayload(ctx, object.ProjectID, false)
	if err != nil {
		return ProductMergePreview{}, err
	}
	if generatedObjectID != object.ObjectID {
		return ProductMergePreview{}, errors.New("knowledge product is not the canonical project brief")
	}
	var candidate map[string]any
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return ProductMergePreview{}, err
	}
	candidateBody, _ := candidate["body"].(string)
	return s.mergeLockedProductBody(ctx, object, candidateBody)
}

var _ = sql.ErrNoRows
