package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s *Service) CreateConnector(ctx context.Context, input ConnectorInput) (Connector, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.Name = strings.TrimSpace(input.Name)
	allowed := []string{"manual", "chatgpt", "codex", "claude", "gemini", "deepseek", "file", "browser", "llm-wiki", "generic-json"}
	if input.Name == "" || !inSet(input.Kind, allowed...) {
		return Connector{}, errors.New("valid connector kind and name are required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Connector{}, err
	}
	now := nowString()
	id := stableID("connector-", input.ProjectID, input.Kind, input.Name)
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO connectors(connector_id,kind,name,status,project_id,config_json,created_at,updated_at) VALUES(?,?,?,'active',?,'{}',?,?)`, id, input.Kind, input.Name, input.ProjectID, now, now)
	if err != nil {
		return Connector{}, err
	}
	return s.connector(ctx, id)
}

func (s *Service) connector(ctx context.Context, id string) (Connector, error) {
	var item Connector
	var cursor, syncAt, lastError sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT connector_id,kind,name,status,project_id,cursor,last_sync_at,last_error,created_at,updated_at FROM connectors WHERE connector_id=?`, id).
		Scan(&item.ConnectorID, &item.Kind, &item.Name, &item.Status, &item.ProjectID, &cursor, &syncAt, &lastError, &item.CreatedAt, &item.UpdatedAt)
	item.Cursor = cursor.String
	item.LastSyncAt = syncAt.String
	item.LastError = lastError.String
	return item, err
}

func (s *Service) ListConnectors(ctx context.Context, projectID string) ([]Connector, error) {
	query := `SELECT connector_id FROM connectors`
	args := []any{}
	if projectID != "" {
		query += ` WHERE project_id=?`
		args = append(args, projectID)
	}
	query += ` ORDER BY name`
	rows, err := s.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, rowErr
	}
	items := make([]Connector, 0, len(ids))
	for _, id := range ids {
		item, err := s.connector(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) BeginImportBatch(ctx context.Context, connectorID, projectID, idempotencyKey string) (ImportBatch, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return ImportBatch{}, false, errors.New("idempotency_key is required")
	}
	if err := s.projectExists(ctx, projectID); err != nil {
		return ImportBatch{}, false, err
	}
	if connectorID != "" {
		connector, err := s.connector(ctx, connectorID)
		if err != nil || connector.ProjectID != projectID {
			return ImportBatch{}, false, errors.New("connector must belong to the same project")
		}
	}
	var priorID string
	err := s.control.DB.QueryRowContext(ctx, `SELECT batch_id FROM import_batches WHERE idempotency_key=?`, idempotencyKey).Scan(&priorID)
	if err == nil {
		batch, getErr := s.importBatch(ctx, priorID)
		return batch, true, getErr
	}
	if err != sql.ErrNoRows {
		return ImportBatch{}, false, err
	}
	now := nowString()
	id := stableID("batch-", projectID, idempotencyKey)
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO import_batches(batch_id,connector_id,project_id,idempotency_key,status,created_at) VALUES(?,nullif(?,''),?,?,'running',?)`, id, connectorID, projectID, idempotencyKey, now)
	if err != nil {
		return ImportBatch{}, false, err
	}
	batch, err := s.importBatch(ctx, id)
	return batch, false, err
}

func (s *Service) CompleteImportBatch(ctx context.Context, batchID string, evidenceIDs []string, failure string) (ImportBatch, error) {
	status := "completed"
	if strings.TrimSpace(failure) != "" {
		status = "failed"
	}
	_, err := s.control.DB.ExecContext(ctx, `UPDATE import_batches SET status=?,item_count=?,evidence_ids_json=?,error=nullif(?,''),completed_at=? WHERE batch_id=? AND status='running'`, status, len(evidenceIDs), encodeStrings(evidenceIDs), strings.TrimSpace(failure), nowString(), batchID)
	if err != nil {
		return ImportBatch{}, err
	}
	return s.importBatch(ctx, batchID)
}

func (s *Service) importBatch(ctx context.Context, id string) (ImportBatch, error) {
	var item ImportBatch
	var connector, failure, completed sql.NullString
	var evidence string
	err := s.control.DB.QueryRowContext(ctx, `SELECT batch_id,connector_id,project_id,idempotency_key,status,item_count,evidence_ids_json,error,created_at,completed_at FROM import_batches WHERE batch_id=?`, id).
		Scan(&item.BatchID, &connector, &item.ProjectID, &item.IdempotencyKey, &item.Status, &item.ItemCount, &evidence, &failure, &item.CreatedAt, &completed)
	item.ConnectorID = connector.String
	item.EvidenceIDs = decodeStrings(evidence)
	item.Error = failure.String
	item.CompletedAt = completed.String
	return item, err
}

func (s *Service) RecordRecallFeedback(ctx context.Context, projectID, contextID, resultID, rating, note string) error {
	if !inSet(rating, "helpful", "irrelevant", "missed", "overloaded") || strings.TrimSpace(contextID) == "" {
		return errors.New("valid context_id and rating are required")
	}
	if projectID != "" {
		if err := s.projectExists(ctx, projectID); err != nil {
			return err
		}
	}
	id := stableID("feedback-", contextID, resultID, rating, nowString())
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO recall_feedback(feedback_id,context_id,project_id,result_id,rating,note,created_at) VALUES(?,?,nullif(?,''),?,?,?,?)`, id, contextID, projectID, strings.TrimSpace(resultID), rating, strings.TrimSpace(note), nowString())
	return err
}
