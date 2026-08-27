package memory

import (
	"context"
	"errors"
	"strings"
	"time"
)

type PinnedMemory struct {
	MemoryRecord
	PinnedAt string `json:"pinned_at"`
}

func (e *Engine) ListPinnedMemories(ctx context.Context, projectID string, limit int) ([]PinnedMemory, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT memory_id,pinned_at FROM memory_pins WHERE project_id=? ORDER BY pinned_at DESC,memory_id LIMIT ?`, projectID, boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pin struct{ memoryID, pinnedAt string }
	pins := []pin{}
	for rows.Next() {
		var item pin
		if err := rows.Scan(&item.memoryID, &item.pinnedAt); err != nil {
			return nil, err
		}
		pins = append(pins, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]PinnedMemory, 0, len(pins))
	for _, pin := range pins {
		item, err := e.MemoryForProject(ctx, projectID, pin.memoryID)
		if err != nil {
			return nil, err
		}
		out = append(out, PinnedMemory{MemoryRecord: item, PinnedAt: pin.pinnedAt})
	}
	return out, nil
}

func (e *Engine) SetMemoryPinned(ctx context.Context, projectID, memoryID string, pinned bool) (string, error) {
	projectID = strings.TrimSpace(projectID)
	memoryID = strings.TrimSpace(memoryID)
	if projectID == "" || memoryID == "" {
		return "", errors.New("project_id and memory_id are required")
	}
	if _, err := e.MemoryForProject(ctx, projectID, memoryID); err != nil {
		return "", err
	}
	if !pinned {
		_, err := e.control.DB.ExecContext(ctx, `DELETE FROM memory_pins WHERE project_id=? AND memory_id=?`, projectID, memoryID)
		return "", err
	}
	pinnedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_pins(project_id,memory_id,pinned_at) VALUES(?,?,?) ON CONFLICT(project_id,memory_id) DO UPDATE SET pinned_at=excluded.pinned_at`, projectID, memoryID, pinnedAt)
	if err != nil {
		return "", err
	}
	return pinnedAt, nil
}
