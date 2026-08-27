package harness

import (
	"context"
	"database/sql"
	"encoding/json"

	"errors"
	"fmt"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"strings"
)

var terminalRunStatuses = map[string]bool{
	"completed": true, "completed_with_warnings": true, "failed": true,
	"denied": true, "cancelled": true,
}

func (s *Service) StartRun(ctx context.Context, input StartRunInput) (Run, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.CallerType = strings.TrimSpace(input.CallerType)
	input.CallerID = strings.TrimSpace(input.CallerID)
	input.Channel = strings.TrimSpace(input.Channel)
	input.PipelineID = strings.TrimSpace(input.PipelineID)
	input.PipelineVersion = strings.TrimSpace(input.PipelineVersion)
	input.PipelineHash = strings.TrimSpace(input.PipelineHash)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID == "" || input.CallerType == "" || input.CallerID == "" || input.Channel == "" || input.PipelineID == "" || input.PipelineVersion == "" || input.PipelineHash == "" || input.IdempotencyKey == "" {
		return Run{}, errors.New("project, caller, channel, pipeline identity/hash and idempotency key are required")
	}
	_, snapshot, err := decodeJSON(input.Snapshot, maxPayloadBytes)
	if err != nil {
		return Run{}, fmt.Errorf("snapshot: %w", err)
	}
	var existingID string
	err = s.control.DB.QueryRowContext(ctx, `SELECT run_id FROM harness_runs WHERE project_id=? AND pipeline_id=? AND idempotency_key=?`, input.ProjectID, input.PipelineID, input.IdempotencyKey).Scan(&existingID)
	if err == nil {
		run, getErr := s.Run(ctx, existingID)
		run.Duplicate = true
		return run, getErr
	}
	if err != sql.ErrNoRows {
		return Run{}, err
	}
	runID, err := randomID("run-")
	if err != nil {
		return Run{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_runs(run_id,project_id,caller_type,caller_id,channel,pipeline_id,pipeline_version,pipeline_hash,status,snapshot_json,idempotency_key,retry_of_run_id,forked_from_run_id,created_at) VALUES(?,?,?,?,?,?,?,?, 'queued',?,?,nullif(?,''),nullif(?,''),?)`, runID, input.ProjectID, input.CallerType, input.CallerID, input.Channel, input.PipelineID, input.PipelineVersion, input.PipelineHash, string(snapshot), input.IdempotencyKey, input.RetryOfRunID, input.ForkedFromRunID, now)
	if err != nil {
		return Run{}, err
	}
	data, _ := json.Marshal(map[string]any{"pipeline_id": input.PipelineID, "pipeline_version": input.PipelineVersion, "channel": input.Channel})
	if _, err := appendEventTx(ctx, tx, runID, "run.queued", CorePluginID, data, now); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.Run(ctx, runID)
}

func (s *Service) Run(ctx context.Context, runID string) (Run, error) {
	var item Run
	var snapshot string
	var retryID, forkID, startedAt, endedAt sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT run_id,project_id,caller_type,caller_id,channel,pipeline_id,pipeline_version,pipeline_hash,status,snapshot_json,idempotency_key,retry_of_run_id,forked_from_run_id,created_at,started_at,ended_at FROM harness_runs WHERE run_id=?`, strings.TrimSpace(runID)).Scan(&item.RunID, &item.ProjectID, &item.CallerType, &item.CallerID, &item.Channel, &item.PipelineID, &item.PipelineVersion, &item.PipelineHash, &item.Status, &snapshot, &item.IdempotencyKey, &retryID, &forkID, &item.CreatedAt, &startedAt, &endedAt)
	if err != nil {
		return Run{}, err
	}
	item.Snapshot = json.RawMessage(snapshot)
	item.RetryOfRunID = retryID.String
	item.ForkedFromRunID = forkID.String
	item.StartedAt = startedAt.String
	item.EndedAt = endedAt.String
	return item, nil
}

func (s *Service) ListRuns(ctx context.Context, projectID, status string, limit int) ([]Run, error) {
	items, _, err := s.ListRunsPage(ctx, projectID, status, limit, 0)
	return items, err
}

func (s *Service) ListRunsPage(ctx context.Context, projectID, status string, limit, offset int) ([]Run, PageInfo, error) {
	projectID = strings.TrimSpace(projectID)
	status = strings.TrimSpace(status)
	if projectID == "" {
		return nil, PageInfo{}, errors.New("project_id is required")
	}
	limit, offset = normalizePage(limit, offset)
	var total int
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_runs WHERE project_id=? AND (?='' OR status=?)`, projectID, status, status).Scan(&total); err != nil {
		return nil, PageInfo{}, err
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT run_id FROM harness_runs WHERE project_id=? AND (?='' OR status=?) ORDER BY created_at DESC,run_id LIMIT ? OFFSET ?`, projectID, status, status, limit, offset)
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
	items := make([]Run, 0, len(ids))
	for _, id := range ids {
		item, err := s.Run(ctx, id)
		if err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, item)
	}
	page := PageInfo{Total: total, Limit: limit, Offset: offset, HasMore: offset+len(items) < total}
	return items, page, nil
}

func appendEventTx(ctx context.Context, tx *sql.Tx, runID, eventType, producer string, data []byte, now string) (Event, error) {
	if !json.Valid(data) {
		return Event{}, errors.New("event data must be valid JSON")
	}
	var sequence int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(sequence),0)+1 FROM harness_events WHERE run_id=?`, runID).Scan(&sequence); err != nil {
		return Event{}, err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO harness_events(run_id,sequence,event_type,producer,schema_version,data_json,created_at) VALUES(?,?,?,?,?,?,?)`, runID, sequence, eventType, producer, TraceSchemaVersion, string(data), now)
	return Event{RunID: runID, Sequence: sequence, EventType: eventType, Producer: producer, SchemaVersion: TraceSchemaVersion, Data: json.RawMessage(data), CreatedAt: now}, err
}

func runStatusForEvent(eventType string) (status string, terminal bool, ok bool) {
	switch eventType {
	case "run.started":
		return "running", false, true
	case "run.paused":
		return "paused", false, true
	case "run.resumed":
		return "running", false, true
	case "run.waiting_review":
		return "waiting_review", false, true
	case "run.completed":
		return "completed", true, true
	case "run.completed_with_warnings":
		return "completed_with_warnings", true, true
	case "run.failed":
		return "failed", true, true
	case "run.denied":
		return "denied", true, true
	case "run.cancelled":
		return "cancelled", true, true
	default:
		return "", false, false
	}
}

func (s *Service) AppendEvent(ctx context.Context, runID, eventType, producer string, data any) (Event, error) {
	runID = strings.TrimSpace(runID)
	eventType = strings.TrimSpace(eventType)
	producer = strings.TrimSpace(producer)
	if runID == "" || eventType == "" || producer == "" {
		return Event{}, errors.New("run_id, event_type and producer are required")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM harness_runs WHERE run_id=?`, runID).Scan(&current); err != nil {
		return Event{}, err
	}
	if terminalRunStatuses[current] {
		return Event{}, fmt.Errorf("terminal run %s cannot accept new events", current)
	}
	event, err := appendEventTx(ctx, tx, runID, eventType, producer, raw, now)
	if err != nil {
		return Event{}, err
	}
	if status, terminal, ok := runStatusForEvent(eventType); ok {
		if terminal {
			_, err = tx.ExecContext(ctx, `UPDATE harness_runs SET status=?,started_at=coalesce(started_at,?),ended_at=? WHERE run_id=?`, status, now, now, runID)
		} else if eventType == "run.started" || eventType == "run.resumed" {
			_, err = tx.ExecContext(ctx, `UPDATE harness_runs SET status=?,started_at=coalesce(started_at,?) WHERE run_id=?`, status, now, runID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE harness_runs SET status=? WHERE run_id=?`, status, runID)
		}
		if err != nil {
			return Event{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) StartSpan(ctx context.Context, runID, parentSpanID, nodeID, stageType, stageVersion, pluginID, inputHash string, detail any) (Span, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" || strings.TrimSpace(stageType) == "" || strings.TrimSpace(stageVersion) == "" || strings.TrimSpace(pluginID) == "" {
		return Span{}, errors.New("run, node, stage identity and plugin are required")
	}
	detailRaw, err := json.Marshal(detail)
	if err != nil {
		return Span{}, err
	}
	spanID, err := randomID("span-")
	if err != nil {
		return Span{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Span{}, err
	}
	defer tx.Rollback()
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(attempt),0)+1 FROM harness_spans WHERE run_id=? AND node_id=?`, runID, nodeID).Scan(&attempt); err != nil {
		return Span{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_spans(span_id,run_id,parent_span_id,node_id,stage_type,stage_version,plugin_id,attempt,status,input_hash,detail_json,started_at) VALUES(?,?,nullif(?,''),?,?,?,?,?,'running',nullif(?,''),?,?)`, spanID, runID, parentSpanID, nodeID, stageType, stageVersion, pluginID, attempt, inputHash, string(detailRaw), now)
	if err != nil {
		return Span{}, err
	}
	eventRaw, _ := json.Marshal(map[string]any{"span_id": spanID, "node_id": nodeID, "attempt": attempt, "stage_type": stageType})
	if _, err := appendEventTx(ctx, tx, runID, "stage.started", pluginID, eventRaw, now); err != nil {
		return Span{}, err
	}
	if err := tx.Commit(); err != nil {
		return Span{}, err
	}
	return Span{SpanID: spanID, RunID: runID, ParentSpanID: parentSpanID, NodeID: nodeID, StageType: stageType, StageVersion: stageVersion, PluginID: pluginID, Attempt: attempt, Status: "running", InputHash: inputHash, Detail: detailRaw, StartedAt: now}, nil
}

func (s *Service) FinishSpan(ctx context.Context, spanID, status, outputHash string, detail any) (Span, error) {
	if status != "completed" && status != "failed" && status != "cancelled" {
		return Span{}, errors.New("span terminal status must be completed, failed or cancelled")
	}
	detailRaw, err := json.Marshal(detail)
	if err != nil {
		return Span{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Span{}, err
	}
	defer tx.Rollback()
	var runID, pluginID, current string
	if err := tx.QueryRowContext(ctx, `SELECT run_id,plugin_id,status FROM harness_spans WHERE span_id=?`, spanID).Scan(&runID, &pluginID, &current); err != nil {
		return Span{}, err
	}
	if current != "running" {
		return Span{}, fmt.Errorf("span is not running: %s", current)
	}
	_, err = tx.ExecContext(ctx, `UPDATE harness_spans SET status=?,output_hash=nullif(?,''),detail_json=?,ended_at=? WHERE span_id=?`, status, outputHash, string(detailRaw), now, spanID)
	if err != nil {
		return Span{}, err
	}
	eventRaw, _ := json.Marshal(map[string]any{"span_id": spanID, "status": status, "output_hash": outputHash})
	if _, err := appendEventTx(ctx, tx, runID, "stage."+status, pluginID, eventRaw, now); err != nil {
		return Span{}, err
	}
	if err := tx.Commit(); err != nil {
		return Span{}, err
	}
	return s.span(ctx, spanID)
}

func (s *Service) span(ctx context.Context, spanID string) (Span, error) {
	var item Span
	var parent, input, output, ended sql.NullString
	var detail string
	err := s.control.DB.QueryRowContext(ctx, `SELECT span_id,run_id,parent_span_id,node_id,stage_type,stage_version,plugin_id,attempt,status,input_hash,output_hash,detail_json,started_at,ended_at FROM harness_spans WHERE span_id=?`, spanID).Scan(&item.SpanID, &item.RunID, &parent, &item.NodeID, &item.StageType, &item.StageVersion, &item.PluginID, &item.Attempt, &item.Status, &input, &output, &detail, &item.StartedAt, &ended)
	if err != nil {
		return Span{}, err
	}
	item.ParentSpanID = parent.String
	item.InputHash = input.String
	item.OutputHash = output.String
	item.Detail = json.RawMessage(detail)
	item.EndedAt = ended.String
	return item, nil
}

func (s *Service) RecordEffectIntent(ctx context.Context, runID, nodeID, effectKey, requestHash string) (Effect, error) {
	if runID == "" || nodeID == "" || effectKey == "" || requestHash == "" {
		return Effect{}, errors.New("run, node, effect key and request hash are required")
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Effect{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO harness_effects(run_id,node_id,effect_key,status,outcome,request_hash,receipt_json,intent_at) VALUES(?,?,?,'intent_recorded','unknown',?,'{}',?) ON CONFLICT(run_id,node_id,effect_key) DO NOTHING`, runID, nodeID, effectKey, requestHash, now)
	if err != nil {
		return Effect{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		existing, getErr := effectTx(ctx, tx, runID, nodeID, effectKey)
		if getErr != nil {
			return Effect{}, getErr
		}
		if existing.RequestHash != requestHash {
			return Effect{}, errors.New("effect key reused for a different request")
		}
		return existing, tx.Commit()
	}
	eventRaw, _ := json.Marshal(map[string]any{"node_id": nodeID, "effect_key": effectKey, "request_hash": requestHash})
	if _, err := appendEventTx(ctx, tx, runID, "effect.intent_recorded", CorePluginID, eventRaw, now); err != nil {
		return Effect{}, err
	}
	if err := tx.Commit(); err != nil {
		return Effect{}, err
	}
	return s.Effect(ctx, runID, nodeID, effectKey)
}

func (s *Service) MarkEffectDispatched(ctx context.Context, runID, nodeID, effectKey, providerKey string) (Effect, error) {
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Effect{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE harness_effects SET status='dispatched',provider_idempotency_key=nullif(?,''),dispatched_at=? WHERE run_id=? AND node_id=? AND effect_key=? AND status='intent_recorded'`, providerKey, now, runID, nodeID, effectKey)
	if err != nil {
		return Effect{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Effect{}, errors.New("effect is not awaiting dispatch")
	}
	eventRaw, _ := json.Marshal(map[string]any{"node_id": nodeID, "effect_key": effectKey, "provider_idempotency_key": providerKey})
	if _, err = appendEventTx(ctx, tx, runID, "effect.dispatched", CorePluginID, eventRaw, now); err != nil {
		return Effect{}, err
	}
	if err = tx.Commit(); err != nil {
		return Effect{}, err
	}
	return s.Effect(ctx, runID, nodeID, effectKey)
}

func (s *Service) RecordEffectReceipt(ctx context.Context, runID, nodeID, effectKey, outcome, resultHash string, receipt any) (Effect, error) {
	if outcome != "confirmed" && outcome != "confirmed_failed" && outcome != "unknown" {
		return Effect{}, errors.New("effect outcome must be confirmed, confirmed_failed or unknown")
	}
	receiptRaw, err := json.Marshal(receipt)
	if err != nil {
		return Effect{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Effect{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE harness_effects SET status='receipt_recorded',outcome=?,result_hash=nullif(?,''),receipt_json=?,received_at=? WHERE run_id=? AND node_id=? AND effect_key=? AND status='dispatched'`, outcome, resultHash, string(receiptRaw), now, runID, nodeID, effectKey)
	if err != nil {
		return Effect{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Effect{}, errors.New("effect is not awaiting a receipt")
	}
	eventRaw, _ := json.Marshal(map[string]any{"node_id": nodeID, "effect_key": effectKey, "outcome": outcome, "result_hash": resultHash})
	if _, err = appendEventTx(ctx, tx, runID, "effect.receipt_recorded", CorePluginID, eventRaw, now); err != nil {
		return Effect{}, err
	}
	if err = tx.Commit(); err != nil {
		return Effect{}, err
	}
	return s.Effect(ctx, runID, nodeID, effectKey)
}

func (s *Service) MarkEffectMaterialized(ctx context.Context, runID, nodeID, effectKey string) (Effect, error) {
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Effect{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE harness_effects SET status='materialized',materialized_at=? WHERE run_id=? AND node_id=? AND effect_key=? AND status='receipt_recorded' AND outcome='confirmed'`, now, runID, nodeID, effectKey)
	if err != nil {
		return Effect{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Effect{}, errors.New("only a confirmed receipt can be materialized")
	}
	eventRaw, _ := json.Marshal(map[string]any{"node_id": nodeID, "effect_key": effectKey, "status": "materialized"})
	if _, err = appendEventTx(ctx, tx, runID, "effect.reconciled", CorePluginID, eventRaw, now); err != nil {
		return Effect{}, err
	}
	if err = tx.Commit(); err != nil {
		return Effect{}, err
	}
	return s.Effect(ctx, runID, nodeID, effectKey)
}

func effectTx(ctx context.Context, tx *sql.Tx, runID, nodeID, effectKey string) (Effect, error) {
	var item Effect
	var provider, result, dispatched, received, materialized sql.NullString
	var receipt string
	err := tx.QueryRowContext(ctx, `SELECT run_id,node_id,effect_key,provider_idempotency_key,status,outcome,request_hash,result_hash,receipt_json,intent_at,dispatched_at,received_at,materialized_at FROM harness_effects WHERE run_id=? AND node_id=? AND effect_key=?`, runID, nodeID, effectKey).Scan(&item.RunID, &item.NodeID, &item.EffectKey, &provider, &item.Status, &item.Outcome, &item.RequestHash, &result, &receipt, &item.IntentAt, &dispatched, &received, &materialized)
	if err != nil {
		return Effect{}, err
	}
	item.ProviderIdempotencyKey = provider.String
	item.ResultHash = result.String
	item.Receipt = json.RawMessage(receipt)
	item.DispatchedAt = dispatched.String
	item.ReceivedAt = received.String
	item.MaterializedAt = materialized.String
	return item, nil
}

func (s *Service) Effect(ctx context.Context, runID, nodeID, effectKey string) (Effect, error) {
	tx, err := s.control.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Effect{}, err
	}
	defer tx.Rollback()
	item, err := effectTx(ctx, tx, runID, nodeID, effectKey)
	if err != nil {
		return Effect{}, err
	}
	return item, tx.Commit()
}

func (s *Service) RunDetail(ctx context.Context, runID string) (RunDetail, error) {
	run, err := s.Run(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{Run: run, Spans: []Span{}, Events: []Event{}, Effects: []Effect{}, StageOutputs: []StageOutputSnapshot{}}
	spanRows, err := s.control.DB.QueryContext(ctx, `SELECT span_id FROM harness_spans WHERE run_id=? ORDER BY started_at,span_id`, runID)
	if err != nil {
		return RunDetail{}, err
	}
	spanIDs := []string{}
	for spanRows.Next() {
		var id string
		if err := spanRows.Scan(&id); err != nil {
			spanRows.Close()
			return RunDetail{}, err
		}
		spanIDs = append(spanIDs, id)
	}
	if err := spanRows.Err(); err != nil {
		spanRows.Close()
		return RunDetail{}, err
	}
	if err := spanRows.Close(); err != nil {
		return RunDetail{}, err
	}
	for _, id := range spanIDs {
		item, err := s.span(ctx, id)
		if err != nil {
			return RunDetail{}, err
		}
		detail.Spans = append(detail.Spans, item)
	}
	eventRows, err := s.control.DB.QueryContext(ctx, `SELECT sequence,event_type,producer,schema_version,data_json,created_at FROM harness_events WHERE run_id=? ORDER BY sequence`, runID)
	if err != nil {
		return RunDetail{}, err
	}
	for eventRows.Next() {
		var item Event
		var data string
		item.RunID = runID
		if err := eventRows.Scan(&item.Sequence, &item.EventType, &item.Producer, &item.SchemaVersion, &data, &item.CreatedAt); err != nil {
			eventRows.Close()
			return RunDetail{}, err
		}
		item.Data = json.RawMessage(data)
		detail.Events = append(detail.Events, item)
	}
	if err := eventRows.Err(); err != nil {
		eventRows.Close()
		return RunDetail{}, err
	}
	if err := eventRows.Close(); err != nil {
		return RunDetail{}, err
	}
	effectRows, err := s.control.DB.QueryContext(ctx, `SELECT node_id,effect_key FROM harness_effects WHERE run_id=? ORDER BY intent_at,node_id,effect_key`, runID)
	if err != nil {
		return RunDetail{}, err
	}
	type effectID struct{ nodeID, effectKey string }
	effectIDs := []effectID{}
	for effectRows.Next() {
		var nodeID, effectKey string
		if err := effectRows.Scan(&nodeID, &effectKey); err != nil {
			effectRows.Close()
			return RunDetail{}, err
		}
		effectIDs = append(effectIDs, effectID{nodeID: nodeID, effectKey: effectKey})
	}
	if err := effectRows.Err(); err != nil {
		effectRows.Close()
		return RunDetail{}, err
	}
	if err := effectRows.Close(); err != nil {
		return RunDetail{}, err
	}
	for _, id := range effectIDs {
		item, err := s.Effect(ctx, runID, id.nodeID, id.effectKey)
		if err != nil {
			return RunDetail{}, err
		}
		detail.Effects = append(detail.Effects, item)
	}
	detail.StageOutputs, err = s.ListStageOutputs(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail.ModelCalls, err = modelusage.ListByRun(ctx, s.control.DB, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail.ModelHealth = modelusage.Aggregate(detail.ModelCalls)
	return detail, nil
}
