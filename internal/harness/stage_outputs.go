package harness

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/contracts"
)

const maxStageOutputBytes = 2 << 20

func (s *Service) RecordStageOutput(ctx context.Context, runID, nodeID string, payload json.RawMessage) (StageOutputSnapshot, error) {
	runID, nodeID = strings.TrimSpace(runID), strings.TrimSpace(nodeID)
	if runID == "" || nodeID == "" {
		return StageOutputSnapshot{}, errors.New("run_id and node_id are required")
	}
	if len(payload) == 0 || len(payload) > maxStageOutputBytes || !json.Valid(payload) {
		return StageOutputSnapshot{}, fmt.Errorf("stage output must be valid JSON up to %d bytes", maxStageOutputBytes)
	}
	var canonical any
	if err := json.Unmarshal(payload, &canonical); err != nil {
		return StageOutputSnapshot{}, err
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return StageOutputSnapshot{}, err
	}
	hash := contracts.HashBytes(raw)
	var existingHash string
	err = s.control.DB.QueryRowContext(ctx, `SELECT output_hash FROM harness_stage_outputs WHERE run_id=? AND node_id=?`, runID, nodeID).Scan(&existingHash)
	if err == nil {
		if existingHash != hash {
			return StageOutputSnapshot{}, errors.New("completed stage output is immutable within a run")
		}
		return s.StageOutput(ctx, runID, nodeID)
	}
	if err != sql.ErrNoRows {
		return StageOutputSnapshot{}, err
	}
	now := nowString()
	if _, err := s.control.DB.ExecContext(ctx, `INSERT INTO harness_stage_outputs(run_id,node_id,output_hash,payload_json,created_at) VALUES(?,?,?,?,?)`, runID, nodeID, hash, string(raw), now); err != nil {
		return StageOutputSnapshot{}, err
	}
	return StageOutputSnapshot{RunID: runID, NodeID: nodeID, OutputHash: hash, Payload: raw, CreatedAt: now}, nil
}

func (s *Service) StageOutput(ctx context.Context, runID, nodeID string) (StageOutputSnapshot, error) {
	var item StageOutputSnapshot
	var raw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT run_id,node_id,output_hash,payload_json,created_at FROM harness_stage_outputs WHERE run_id=? AND node_id=?`, strings.TrimSpace(runID), strings.TrimSpace(nodeID)).Scan(&item.RunID, &item.NodeID, &item.OutputHash, &raw, &item.CreatedAt)
	if err != nil {
		return StageOutputSnapshot{}, err
	}
	item.Payload = json.RawMessage(raw)
	return item, nil
}

func (s *Service) ListStageOutputs(ctx context.Context, runID string) ([]StageOutputSnapshot, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT node_id FROM harness_stage_outputs WHERE run_id=? ORDER BY created_at,node_id`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]StageOutputSnapshot, 0, len(ids))
	for _, id := range ids {
		item, err := s.StageOutput(ctx, runID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
