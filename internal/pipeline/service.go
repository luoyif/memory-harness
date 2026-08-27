package pipeline

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control           *store.ControlStore
	harness           *harness.Service
	models            *modelconfig.Service
	blueprintResolver BlueprintResolver
	stageHandlers     map[string]StageHandler
	runMu             sync.Mutex
	runCancels        map[string]context.CancelFunc
}

func New(control *store.ControlStore, harnessService *harness.Service, modelServices ...*modelconfig.Service) *Service {
	var models *modelconfig.Service
	if len(modelServices) > 0 {
		models = modelServices[0]
	}
	return &Service{control: control, harness: harnessService, models: models, stageHandlers: map[string]StageHandler{}, runCancels: map[string]context.CancelFunc{}}
}

func (s *Service) registerRunCancel(runID string, cancel context.CancelFunc) {
	s.runMu.Lock()
	s.runCancels[runID] = cancel
	s.runMu.Unlock()
}

func (s *Service) unregisterRunCancel(runID string) {
	s.runMu.Lock()
	delete(s.runCancels, runID)
	s.runMu.Unlock()
}

func (s *Service) cancelActiveRun(runID string) {
	s.runMu.Lock()
	cancel := s.runCancels[runID]
	s.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RegisterStageHandler connects a catalogued business stage to the local host.
// Registration happens during application startup, before concurrent runs.
func (s *Service) RegisterStageHandler(stageType string, handler StageHandler) error {
	stageType = strings.TrimSpace(stageType)
	if _, ok := stageCatalog[stageType]; !ok {
		return fmt.Errorf("cannot register unknown stage type %q", stageType)
	}
	if handler == nil {
		return errors.New("stage handler is required")
	}
	if _, exists := s.stageHandlers[stageType]; exists {
		return fmt.Errorf("stage handler %q is already registered", stageType)
	}
	s.stageHandlers[stageType] = handler
	return nil
}

func (s *Service) ValidateDefinition(pluginID string, raw []byte) error {
	definition, _, err := parseDefinition(raw)
	if err != nil {
		return err
	}
	_, err = validateDefinition(strings.TrimSpace(pluginID), definition)
	return err
}

func (s *Service) ValidateStructured(pluginID string, definition Definition) (ValidationResult, error) {
	definition, _, err := normalizeStructuredDefinition(definition)
	if err != nil {
		return ValidationResult{}, err
	}
	ordered, err := validateDefinition(strings.TrimSpace(pluginID), definition)
	if err != nil {
		return ValidationResult{}, err
	}
	order := make([]string, 0, len(ordered))
	modelCalls := 0
	for _, node := range ordered {
		order = append(order, node.ID)
		if descriptor, ok := stageCatalog[node.StageType]; ok && descriptor.Class == "model" {
			modelCalls++
		}
	}
	return ValidationResult{Valid: true, ExecutionOrder: order, RequiredCapabilities: definition.RequiredCapabilities, ModelCalls: modelCalls, NodeCount: len(definition.Nodes)}, nil
}

func draftID(pipelineID string) string {
	sum := sha256.Sum256([]byte(pipelineID))
	return "draft-" + hex.EncodeToString(sum[:12])
}

func (s *Service) SaveDraft(ctx context.Context, input SaveDraftInput) (Draft, error) {
	input.PluginID = strings.TrimSpace(input.PluginID)
	input.BaseVersion = strings.TrimSpace(input.BaseVersion)
	definition, canonical, err := normalizeStructuredDefinition(input.Definition)
	if err != nil {
		return Draft{}, err
	}
	if !pipelineIDPattern.MatchString(definition.PipelineID) || !strings.HasPrefix(definition.PipelineID, input.PluginID+".") {
		return Draft{}, errors.New("draft pipeline_id must be a stable identifier namespaced by plugin_id")
	}
	if len(definition.Nodes) > 128 || len(canonical) > 1<<20 {
		return Draft{}, errors.New("draft exceeds the pipeline size or node limit")
	}
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Draft{}, err
	}
	defer tx.Rollback()
	var currentRevision int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM harness_pipeline_drafts WHERE pipeline_id=?`, definition.PipelineID).Scan(&currentRevision)
	now := nowString()
	if errors.Is(err, sql.ErrNoRows) {
		if input.ExpectedRevision != 0 {
			return Draft{}, errors.New("draft revision conflict")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO harness_pipeline_drafts(draft_id,pipeline_id,plugin_id,base_version,definition_json,revision,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?)`, draftID(definition.PipelineID), definition.PipelineID, input.PluginID, input.BaseVersion, string(canonical), now, now)
	} else if err == nil {
		if input.ExpectedRevision != currentRevision {
			return Draft{}, errors.New("draft revision conflict")
		}
		_, err = tx.ExecContext(ctx, `UPDATE harness_pipeline_drafts SET plugin_id=?,base_version=?,definition_json=?,revision=revision+1,updated_at=? WHERE pipeline_id=?`, input.PluginID, input.BaseVersion, string(canonical), now, definition.PipelineID)
	}
	if err != nil {
		return Draft{}, err
	}
	if err := tx.Commit(); err != nil {
		return Draft{}, err
	}
	return s.Draft(ctx, definition.PipelineID)
}

func (s *Service) Draft(ctx context.Context, pipelineID string) (Draft, error) {
	var item Draft
	var raw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT draft_id,pipeline_id,plugin_id,base_version,definition_json,revision,created_at,updated_at FROM harness_pipeline_drafts WHERE pipeline_id=?`, strings.TrimSpace(pipelineID)).Scan(&item.DraftID, &item.PipelineID, &item.PluginID, &item.BaseVersion, &raw, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Draft{}, err
	}
	if err := json.Unmarshal([]byte(raw), &item.Definition); err != nil {
		return Draft{}, err
	}
	return item, nil
}

func (s *Service) ListDrafts(ctx context.Context) ([]Draft, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT pipeline_id FROM harness_pipeline_drafts ORDER BY updated_at DESC,pipeline_id`)
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
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]Draft, 0, len(ids))
	for _, id := range ids {
		item, err := s.Draft(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) DeleteDraft(ctx context.Context, pipelineID string) error {
	result, err := s.control.DB.ExecContext(ctx, `DELETE FROM harness_pipeline_drafts WHERE pipeline_id=?`, strings.TrimSpace(pipelineID))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Service) PublishDefinition(ctx context.Context, pluginID string, raw []byte) error {
	_, err := s.Publish(ctx, pluginID, raw)
	return err
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func hashBytes(raw []byte) string {
	hash := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func (s *Service) Publish(ctx context.Context, pluginID string, raw []byte) (Version, error) {
	pluginID = strings.TrimSpace(pluginID)
	definition, canonical, err := parseDefinition(raw)
	if err != nil {
		return Version{}, err
	}
	if _, err := validateDefinition(pluginID, definition); err != nil {
		return Version{}, err
	}
	hash := hashBytes(canonical)
	now := nowString()
	result, err := s.control.DB.ExecContext(ctx, `INSERT INTO harness_pipeline_versions(pipeline_id,version,plugin_id,name,definition_json,content_hash,status,created_at) VALUES(?,?,?,?,?,?,'published',?) ON CONFLICT(pipeline_id,version) DO NOTHING`, definition.PipelineID, definition.Version, pluginID, definition.Name, string(canonical), hash, now)
	if err != nil {
		return Version{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		existing, err := s.Version(ctx, definition.PipelineID, definition.Version)
		if err != nil {
			return Version{}, err
		}
		if existing.ContentHash != hash || existing.PluginID != pluginID {
			return Version{}, errors.New("published pipeline version is immutable")
		}
	}
	return s.Version(ctx, definition.PipelineID, definition.Version)
}

func (s *Service) Version(ctx context.Context, pipelineID, version string) (Version, error) {
	var item Version
	var raw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT pipeline_id,version,plugin_id,name,definition_json,content_hash,status,created_at FROM harness_pipeline_versions WHERE pipeline_id=? AND version=?`, strings.TrimSpace(pipelineID), strings.TrimSpace(version)).Scan(&item.PipelineID, &item.Version, &item.PluginID, &item.Name, &raw, &item.ContentHash, &item.Status, &item.CreatedAt)
	if err != nil {
		return Version{}, err
	}
	if err := json.Unmarshal([]byte(raw), &item.Definition); err != nil {
		return Version{}, err
	}
	return item, nil
}

func (s *Service) List(ctx context.Context) ([]Version, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT pipeline_id,version FROM harness_pipeline_versions ORDER BY pipeline_id,version`)
	if err != nil {
		return nil, err
	}
	type id struct{ pipeline, version string }
	ids := []id{}
	for rows.Next() {
		var item id
		if err := rows.Scan(&item.pipeline, &item.version); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	latest := map[string]id{}
	order := []string{}
	for _, candidate := range ids {
		current, exists := latest[candidate.pipeline]
		if !exists {
			latest[candidate.pipeline] = candidate
			order = append(order, candidate.pipeline)
			continue
		}
		if compareVersion(candidate.version, current.version) > 0 {
			latest[candidate.pipeline] = candidate
		}
	}
	items := make([]Version, 0, len(order))
	for _, pipelineID := range order {
		id := latest[pipelineID]
		item, err := s.Version(ctx, id.pipeline, id.version)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// compareVersion keeps the public catalog focused on one current immutable
// version per pipeline while older versions remain addressable by exact ID.
// Pipeline versions are validated as semantic versions before publication.
func compareVersion(left, right string) int {
	parse := func(value string) [3]int {
		var result [3]int
		for index, part := range strings.SplitN(strings.TrimPrefix(value, "v"), ".", 4) {
			if index >= len(result) {
				break
			}
			part = strings.SplitN(part, "-", 2)[0]
			result[index], _ = strconv.Atoi(part)
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := range a {
		if a[index] > b[index] {
			return 1
		}
		if a[index] < b[index] {
			return -1
		}
	}
	return strings.Compare(left, right)
}

func capabilitySet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[strings.TrimSpace(value)] = true
	}
	return result
}

func (s *Service) checkPluginProject(ctx context.Context, version Version, projectID string, effective []string) error {
	effectiveSet := capabilitySet(effective)
	for _, required := range version.Definition.RequiredCapabilities {
		if !effectiveSet[required] {
			return fmt.Errorf("effective capability %q is denied", required)
		}
	}
	if strings.HasPrefix(version.PluginID, "builtin.") {
		var projectStatus, grantedRaw string
		err := s.control.DB.QueryRowContext(ctx, `SELECT status,granted_capabilities_json FROM harness_plugin_project_state WHERE plugin_id=? AND version=(SELECT version FROM harness_plugin_versions WHERE plugin_id=? ORDER BY installed_at DESC LIMIT 1) AND project_id=?`, version.PluginID, version.PluginID, projectID).Scan(&projectStatus, &grantedRaw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if projectStatus != "enabled" {
			return errors.New("built-in plugin is disabled for this project")
		}
		var granted []string
		_ = json.Unmarshal([]byte(grantedRaw), &granted)
		grantSet := capabilitySet(granted)
		for _, required := range version.Definition.RequiredCapabilities {
			if !grantSet[required] {
				return fmt.Errorf("built-in plugin capability %q is denied for this project", required)
			}
		}
		return nil
	}
	var pluginStatus, projectStatus, grantedRaw, signerStatus string
	err := s.control.DB.QueryRowContext(ctx, `SELECT pv.status,pps.status,pps.granted_capabilities_json,coalesce(ts.status,'active') FROM harness_plugin_versions pv JOIN harness_plugin_project_state pps ON pps.plugin_id=pv.plugin_id AND pps.version=pv.version LEFT JOIN harness_trusted_signers ts ON ts.signer_id=pv.signer_id WHERE pv.plugin_id=? AND pv.version=? AND pps.project_id=?`, version.PluginID, version.Version, projectID).Scan(&pluginStatus, &projectStatus, &grantedRaw, &signerStatus)
	if err != nil {
		return fmt.Errorf("plugin project state: %w", err)
	}
	if (pluginStatus != "enabled" && pluginStatus != "installed") || projectStatus != "enabled" || signerStatus != "active" {
		return errors.New("plugin is disabled, quarantined or untrusted for this project")
	}
	var granted []string
	_ = json.Unmarshal([]byte(grantedRaw), &granted)
	grantSet := capabilitySet(granted)
	for _, required := range version.Definition.RequiredCapabilities {
		if !grantSet[required] {
			return fmt.Errorf("plugin capability %q is denied for this project", required)
		}
	}
	return nil
}

func dependencyInput(node Node, outputs map[string]json.RawMessage, initial json.RawMessage) json.RawMessage {
	if len(node.DependsOn) == 0 {
		return initial
	}
	if len(node.DependsOn) == 1 {
		return outputs[node.DependsOn[0]]
	}
	combined := map[string]json.RawMessage{}
	for _, dependency := range node.DependsOn {
		combined[dependency] = outputs[dependency]
	}
	raw, _ := json.Marshal(combined)
	return raw
}

type executionSnapshot struct {
	Pipeline              Version         `json:"pipeline"`
	Input                 json.RawMessage `json:"input"`
	EffectiveCapabilities []string        `json:"effective_capabilities"`
	Blueprint             json.RawMessage `json:"blueprint"`
}

type BlueprintResolver func(context.Context, string) (json.RawMessage, error)

func (s *Service) SetBlueprintResolver(resolver BlueprintResolver) { s.blueprintResolver = resolver }

func reviewID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "review-" + hex.EncodeToString(raw), nil
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (ExecutionResult, error) {
	version, err := s.Version(ctx, input.PipelineID, input.PipelineVersion)
	if err != nil {
		return ExecutionResult{}, err
	}
	ordered, err := validateDefinition(version.PluginID, version.Definition)
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := s.checkPluginProject(ctx, version, input.ProjectID, input.EffectiveCapabilities); err != nil {
		return ExecutionResult{}, err
	}
	if len(input.Input) == 0 || !json.Valid(input.Input) {
		return ExecutionResult{}, errors.New("input must be valid JSON")
	}
	timeoutSeconds := version.Definition.Policy.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	blueprintSnapshot := input.BlueprintSnapshot
	if len(blueprintSnapshot) == 0 && s.blueprintResolver != nil {
		blueprintSnapshot, err = s.blueprintResolver(ctx, input.ProjectID)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("resolve project blueprint: %w", err)
		}
	}
	input.BlueprintSnapshot = blueprintSnapshot
	snapshot, _ := json.Marshal(executionSnapshot{Pipeline: version, Input: input.Input, EffectiveCapabilities: input.EffectiveCapabilities, Blueprint: blueprintSnapshot})
	run, err := s.harness.StartRun(ctx, harness.StartRunInput{
		ProjectID: input.ProjectID, CallerType: input.CallerType, CallerID: input.CallerID, Channel: input.Channel,
		PipelineID: version.PipelineID, PipelineVersion: version.Version, PipelineHash: version.ContentHash,
		IdempotencyKey: input.IdempotencyKey, Snapshot: snapshot, RetryOfRunID: input.RetryOfRunID, ForkedFromRunID: input.ForkedFromRunID,
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	if run.Duplicate {
		return ExecutionResult{RunID: run.RunID, Status: run.Status, Outputs: map[string]json.RawMessage{}, Duplicate: true}, nil
	}
	s.registerRunCancel(run.RunID, cancel)
	defer s.unregisterRunCancel(run.RunID)
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.started", harness.CorePluginID, map[string]any{"pipeline_hash": version.ContentHash, "blueprint": json.RawMessage(blueprintSnapshot)}); err != nil {
		return ExecutionResult{}, err
	}
	return s.executeNodes(ctx, version, run, ordered, 0, input.Input, map[string]json.RawMessage{}, input)
}

func (s *Service) executeNodes(ctx context.Context, version Version, run harness.Run, ordered []Node, startIndex int, initial json.RawMessage, outputs map[string]json.RawMessage, input ExecuteInput) (ExecutionResult, error) {
	for index := startIndex; index < len(ordered); index++ {
		if err := ctx.Err(); err != nil {
			return s.finishContextEndedRun(run.RunID, "", outputs, err)
		}
		node := ordered[index]
		nodeInput := dependencyInput(node, outputs, initial)
		span, err := s.harness.StartSpan(ctx, run.RunID, "", node.ID, node.StageType, node.StageVersion, node.PluginID, hashBytes(nodeInput), map[string]any{"input_bytes": len(nodeInput)})
		if err != nil {
			s.failRun(ctx, run.RunID, node.ID, err)
			return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, err
		}
		output, waiting, executeErr := s.executeStage(ctx, version, run.RunID, span.SpanID, node, nodeInput, input)
		if err := ctx.Err(); err != nil {
			traceCtx := context.WithoutCancel(ctx)
			_, _ = s.harness.FinishSpan(traceCtx, span.SpanID, "cancelled", "", map[string]any{"error": err.Error()})
			return s.finishContextEndedRun(run.RunID, node.ID, outputs, err)
		}
		if executeErr != nil {
			_, _ = s.harness.FinishSpan(ctx, span.SpanID, "failed", "", map[string]any{"error": executeErr.Error()})
			s.failRun(ctx, run.RunID, node.ID, executeErr)
			return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, executeErr
		}
		snapshotOutput, snapshotErr := s.harness.RecordStageOutput(ctx, run.RunID, node.ID, output)
		if snapshotErr != nil {
			_, _ = s.harness.FinishSpan(ctx, span.SpanID, "failed", "", map[string]any{"error": snapshotErr.Error(), "phase": "persist_stage_output"})
			s.failRun(ctx, run.RunID, node.ID, snapshotErr)
			return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, snapshotErr
		}
		outputs[node.ID] = snapshotOutput.Payload
		detail := map[string]any{"output_bytes": len(output), "replay_snapshot": true}
		var outputEnvelope struct {
			Trace json.RawMessage `json:"trace"`
		}
		if json.Unmarshal(output, &outputEnvelope) == nil && len(outputEnvelope.Trace) > 0 && string(outputEnvelope.Trace) != "null" {
			detail["result"] = outputEnvelope.Trace
		}
		if _, err := s.harness.FinishSpan(ctx, span.SpanID, "completed", snapshotOutput.OutputHash, detail); err != nil {
			s.failRun(ctx, run.RunID, node.ID, err)
			return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, err
		}
		if waiting {
			review, err := s.persistReviewCheckpoint(ctx, run, node, index+1, initial, outputs, input.EffectiveCapabilities, nodeInput)
			if err != nil {
				s.failRun(ctx, run.RunID, node.ID, err)
				return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, err
			}
			if _, err = s.harness.AppendEvent(ctx, run.RunID, "review.requested", harness.CorePluginID, map[string]any{"review_id": review.ReviewID, "node_id": node.ID, "reason": review.Reason}); err != nil {
				return ExecutionResult{RunID: run.RunID, Status: "failed", Outputs: outputs}, err
			}
			_, err = s.harness.AppendEvent(ctx, run.RunID, "run.waiting_review", harness.CorePluginID, map[string]any{"node_id": node.ID, "review_id": review.ReviewID})
			return ExecutionResult{RunID: run.RunID, Status: "waiting_review", Outputs: outputs}, err
		}
	}
	resultOutputs := map[string]json.RawMessage{}
	for _, output := range version.Definition.Outputs {
		resultOutputs[output.Name] = outputs[output.NodeID]
	}
	_, err := s.harness.AppendEvent(ctx, run.RunID, "run.completed", harness.CorePluginID, map[string]any{"outputs": len(resultOutputs)})
	if err == nil {
		_, _ = s.control.DB.ExecContext(ctx, `DELETE FROM harness_run_checkpoints WHERE run_id=?`, run.RunID)
	}
	return ExecutionResult{RunID: run.RunID, Status: "completed", Outputs: resultOutputs}, err
}

func (s *Service) finishContextEndedRun(runID, nodeID string, outputs map[string]json.RawMessage, cause error) (ExecutionResult, error) {
	traceCtx := context.Background()
	run, err := s.harness.Run(traceCtx, runID)
	if err == nil && run.Status == "cancelled" {
		return ExecutionResult{RunID: runID, Status: "cancelled", Outputs: outputs}, cause
	}
	s.failRun(traceCtx, runID, nodeID, cause)
	return ExecutionResult{RunID: runID, Status: "failed", Outputs: outputs}, cause
}

func (s *Service) persistReviewCheckpoint(ctx context.Context, run harness.Run, node Node, nextIndex int, initial json.RawMessage, outputs map[string]json.RawMessage, capabilities []string, request json.RawMessage) (Review, error) {
	outputsRaw, err := json.Marshal(outputs)
	if err != nil {
		return Review{}, err
	}
	capabilitiesRaw, _ := json.Marshal(capabilities)
	reason := "owner_review_required"
	var config struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(node.Config, &config) == nil && strings.TrimSpace(config.Reason) != "" {
		reason = strings.TrimSpace(config.Reason)
	}
	id, err := reviewID()
	if err != nil {
		return Review{}, err
	}
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Review{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_run_checkpoints(run_id,next_node_index,input_json,outputs_json,effective_capabilities_json,status,updated_at) VALUES(?,?,?,?,?,'waiting_review',?) ON CONFLICT(run_id) DO UPDATE SET next_node_index=excluded.next_node_index,input_json=excluded.input_json,outputs_json=excluded.outputs_json,effective_capabilities_json=excluded.effective_capabilities_json,status=excluded.status,updated_at=excluded.updated_at`, run.RunID, nextIndex, string(initial), string(outputsRaw), string(capabilitiesRaw), now); err != nil {
		return Review{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_reviews(review_id,run_id,node_id,reason,status,requested_by,request_json,created_at) VALUES(?,?,?,?,'pending',?,?,?) ON CONFLICT(run_id,node_id) DO NOTHING`, id, run.RunID, node.ID, reason, run.CallerID, string(request), now); err != nil {
		return Review{}, err
	}
	if err = tx.Commit(); err != nil {
		return Review{}, err
	}
	return s.reviewForRunNode(ctx, run.RunID, node.ID)
}

func scanReview(scanner interface{ Scan(...any) error }) (Review, error) {
	var item Review
	var requestRaw string
	var decisionBy, decisionNote, decidedAt sql.NullString
	err := scanner.Scan(&item.ReviewID, &item.RunID, &item.NodeID, &item.ProjectID, &item.PipelineID, &item.Reason, &item.Status, &item.RequestedBy, &requestRaw, &decisionBy, &decisionNote, &item.CreatedAt, &decidedAt)
	item.Request = json.RawMessage(requestRaw)
	item.DecisionBy = decisionBy.String
	item.DecisionNote = decisionNote.String
	item.DecidedAt = decidedAt.String
	return item, err
}

func (s *Service) reviewForRunNode(ctx context.Context, runID, nodeID string) (Review, error) {
	return scanReview(s.control.DB.QueryRowContext(ctx, `SELECT rv.review_id,rv.run_id,rv.node_id,r.project_id,r.pipeline_id,rv.reason,rv.status,rv.requested_by,rv.request_json,rv.decision_by,rv.decision_note,rv.created_at,rv.decided_at FROM harness_reviews rv JOIN harness_runs r ON r.run_id=rv.run_id WHERE rv.run_id=? AND rv.node_id=?`, runID, nodeID))
}

func (s *Service) Review(ctx context.Context, reviewID string) (Review, error) {
	return scanReview(s.control.DB.QueryRowContext(ctx, `SELECT rv.review_id,rv.run_id,rv.node_id,r.project_id,r.pipeline_id,rv.reason,rv.status,rv.requested_by,rv.request_json,rv.decision_by,rv.decision_note,rv.created_at,rv.decided_at FROM harness_reviews rv JOIN harness_runs r ON r.run_id=rv.run_id WHERE rv.review_id=?`, strings.TrimSpace(reviewID)))
}

func (s *Service) ListReviews(ctx context.Context, projectID, status string, limit int) ([]Review, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT rv.review_id FROM harness_reviews rv JOIN harness_runs r ON r.run_id=rv.run_id WHERE (?='' OR r.project_id=?) AND (?='' OR rv.status=?) ORDER BY rv.created_at DESC LIMIT ?`, strings.TrimSpace(projectID), strings.TrimSpace(projectID), strings.TrimSpace(status), strings.TrimSpace(status), limit)
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
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]Review, 0, len(ids))
	for _, id := range ids {
		item, err := s.Review(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) DecideReview(ctx context.Context, reviewID string, decision ReviewDecisionInput) (ExecutionResult, error) {
	decision.Decision = strings.ToLower(strings.TrimSpace(decision.Decision))
	decision.OwnerID = strings.TrimSpace(decision.OwnerID)
	if decision.Decision != "approve" && decision.Decision != "reject" {
		return ExecutionResult{}, errors.New("decision must be approve or reject")
	}
	if decision.OwnerID == "" {
		return ExecutionResult{}, errors.New("owner identity is required")
	}
	review, err := s.Review(ctx, reviewID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if review.Status != "pending" {
		return ExecutionResult{}, fmt.Errorf("review is not pending: %s", review.Status)
	}
	now := nowString()
	decisionStatus := "approved"
	if decision.Decision == "reject" {
		decisionStatus = "rejected"
	}
	result, err := s.control.DB.ExecContext(ctx, `UPDATE harness_reviews SET status=?,decision_by=?,decision_note=?,decided_at=? WHERE review_id=? AND status='pending'`, decisionStatus, decision.OwnerID, strings.TrimSpace(decision.Note), now, reviewID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ExecutionResult{}, errors.New("review was already decided")
	}
	eventType := "review.approved"
	if decision.Decision == "reject" {
		eventType = "review.rejected"
	}
	if _, err = s.harness.AppendEvent(ctx, review.RunID, eventType, harness.CorePluginID, map[string]any{"review_id": reviewID, "node_id": review.NodeID, "owner_id": decision.OwnerID, "note": decision.Note}); err != nil {
		return ExecutionResult{}, err
	}
	if decision.Decision == "reject" {
		_, err = s.harness.AppendEvent(ctx, review.RunID, "run.denied", harness.CorePluginID, map[string]any{"review_id": reviewID, "node_id": review.NodeID})
		_, _ = s.control.DB.ExecContext(ctx, `DELETE FROM harness_run_checkpoints WHERE run_id=?`, review.RunID)
		return ExecutionResult{RunID: review.RunID, Status: "denied", Outputs: map[string]json.RawMessage{}}, err
	}
	return s.resumeApprovedReview(ctx, review)
}

func (s *Service) resumeApprovedReview(ctx context.Context, review Review) (ExecutionResult, error) {
	run, err := s.harness.Run(ctx, review.RunID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if run.Status != "waiting_review" {
		return ExecutionResult{}, fmt.Errorf("run is not waiting for review: %s", run.Status)
	}
	var nextIndex int
	var initialRaw, outputsRaw, capabilitiesRaw string
	if err := s.control.DB.QueryRowContext(ctx, `SELECT next_node_index,input_json,outputs_json,effective_capabilities_json FROM harness_run_checkpoints WHERE run_id=?`, run.RunID).Scan(&nextIndex, &initialRaw, &outputsRaw, &capabilitiesRaw); err != nil {
		return ExecutionResult{}, err
	}
	var outputs map[string]json.RawMessage
	var capabilities []string
	if err := json.Unmarshal([]byte(outputsRaw), &outputs); err != nil {
		return ExecutionResult{}, err
	}
	if err := json.Unmarshal([]byte(capabilitiesRaw), &capabilities); err != nil {
		return ExecutionResult{}, err
	}
	version, err := s.Version(ctx, run.PipelineID, run.PipelineVersion)
	if err != nil {
		return ExecutionResult{}, err
	}
	ordered, err := validateDefinition(version.PluginID, version.Definition)
	if err != nil {
		return ExecutionResult{}, err
	}
	if nextIndex > len(ordered) {
		return ExecutionResult{}, errors.New("stored checkpoint is outside the immutable pipeline")
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.resumed", harness.CorePluginID, map[string]any{"review_id": review.ReviewID, "next_node_index": nextIndex}); err != nil {
		return ExecutionResult{}, err
	}
	var savedSnapshot executionSnapshot
	if err := json.Unmarshal(run.Snapshot, &savedSnapshot); err != nil {
		return ExecutionResult{}, fmt.Errorf("decode run blueprint snapshot: %w", err)
	}
	input := ExecuteInput{ProjectID: run.ProjectID, CallerType: run.CallerType, CallerID: run.CallerID, Channel: run.Channel, PipelineID: run.PipelineID, PipelineVersion: run.PipelineVersion, IdempotencyKey: run.IdempotencyKey, Input: json.RawMessage(initialRaw), EffectiveCapabilities: capabilities, BlueprintSnapshot: savedSnapshot.Blueprint}
	return s.executeNodes(ctx, version, run, ordered, nextIndex, json.RawMessage(initialRaw), outputs, input)
}

func (s *Service) Cancel(ctx context.Context, runID, ownerID, reason string) (harness.Run, error) {
	run, err := s.harness.Run(ctx, runID)
	if err != nil {
		return harness.Run{}, err
	}
	if run.Status != "queued" && run.Status != "running" && run.Status != "paused" && run.Status != "waiting_review" {
		return harness.Run{}, fmt.Errorf("run cannot be cancelled from %s", run.Status)
	}
	if _, err := s.harness.AppendEvent(ctx, runID, "run.cancelled", harness.CorePluginID, map[string]any{"owner_id": ownerID, "reason": strings.TrimSpace(reason)}); err != nil {
		return harness.Run{}, err
	}
	s.cancelActiveRun(runID)
	_, _ = s.control.DB.ExecContext(ctx, `UPDATE harness_reviews SET status='cancelled',decision_by=?,decision_note=?,decided_at=? WHERE run_id=? AND status='pending'`, ownerID, strings.TrimSpace(reason), nowString(), runID)
	_, _ = s.control.DB.ExecContext(ctx, `DELETE FROM harness_run_checkpoints WHERE run_id=?`, runID)
	return s.harness.Run(ctx, runID)
}

func (s *Service) Retry(ctx context.Context, runID, ownerID string) (ExecutionResult, error) {
	run, err := s.harness.Run(ctx, runID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if run.Status != "failed" && run.Status != "cancelled" && run.Status != "denied" {
		return ExecutionResult{}, fmt.Errorf("run cannot be retried from %s", run.Status)
	}
	var snapshot executionSnapshot
	if err := json.Unmarshal(run.Snapshot, &snapshot); err != nil || len(snapshot.Input) == 0 {
		return ExecutionResult{}, errors.New("run snapshot does not contain replayable input")
	}
	return s.Execute(ctx, ExecuteInput{ProjectID: run.ProjectID, CallerType: "owner", CallerID: ownerID, Channel: "desktop-retry", PipelineID: run.PipelineID, PipelineVersion: run.PipelineVersion, IdempotencyKey: fmt.Sprintf("retry-%s-%d", run.RunID, time.Now().UTC().UnixNano()), Input: snapshot.Input, EffectiveCapabilities: snapshot.EffectiveCapabilities, BlueprintSnapshot: snapshot.Blueprint, RetryOfRunID: run.RunID})
}

func (s *Service) ForkFromNode(ctx context.Context, runID, nodeID, ownerID string) (ExecutionResult, error) {
	run, err := s.harness.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return ExecutionResult{}, err
	}
	if run.Status == "queued" || run.Status == "running" || run.Status == "paused" || run.Status == "waiting_review" {
		return ExecutionResult{}, fmt.Errorf("run cannot be forked while status=%s", run.Status)
	}
	var snapshot executionSnapshot
	if err := json.Unmarshal(run.Snapshot, &snapshot); err != nil || len(snapshot.Input) == 0 {
		return ExecutionResult{}, errors.New("run snapshot does not contain replayable input")
	}
	version, err := s.Version(ctx, run.PipelineID, run.PipelineVersion)
	if err != nil {
		return ExecutionResult{}, err
	}
	ordered, err := validateDefinition(version.PluginID, version.Definition)
	if err != nil {
		return ExecutionResult{}, err
	}
	startIndex := -1
	for index, node := range ordered {
		if node.ID == strings.TrimSpace(nodeID) {
			startIndex = index
			break
		}
	}
	if startIndex < 0 {
		return ExecutionResult{}, fmt.Errorf("node %q is not part of immutable pipeline %s@%s", nodeID, run.PipelineID, run.PipelineVersion)
	}
	if err := s.checkPluginProject(ctx, version, run.ProjectID, snapshot.EffectiveCapabilities); err != nil {
		return ExecutionResult{}, err
	}
	outputs := map[string]json.RawMessage{}
	for index := 0; index < startIndex; index++ {
		prior := ordered[index]
		stageOutput, err := s.harness.StageOutput(ctx, run.RunID, prior.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ExecutionResult{}, fmt.Errorf("run predates durable stage snapshots or stage %s never completed; retry the whole run instead", prior.ID)
			}
			return ExecutionResult{}, err
		}
		outputs[prior.ID] = stageOutput.Payload
	}
	if strings.TrimSpace(ownerID) == "" {
		ownerID = "local-owner"
	}
	timeoutSeconds := version.Definition.Policy.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	forkKey := fmt.Sprintf("fork-%s-%s-%d", run.RunID, ordered[startIndex].ID, time.Now().UTC().UnixNano())
	newSnapshot, _ := json.Marshal(executionSnapshot{Pipeline: version, Input: snapshot.Input, EffectiveCapabilities: snapshot.EffectiveCapabilities, Blueprint: snapshot.Blueprint})
	forked, err := s.harness.StartRun(ctx, harness.StartRunInput{
		ProjectID: run.ProjectID, CallerType: "owner", CallerID: ownerID, Channel: "desktop-fork",
		PipelineID: run.PipelineID, PipelineVersion: run.PipelineVersion, PipelineHash: run.PipelineHash,
		IdempotencyKey: forkKey, Snapshot: newSnapshot, ForkedFromRunID: run.RunID,
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	s.registerRunCancel(forked.RunID, cancel)
	defer s.unregisterRunCancel(forked.RunID)
	if _, err := s.harness.AppendEvent(ctx, forked.RunID, "run.started", harness.CorePluginID, map[string]any{
		"pipeline_hash": version.ContentHash, "forked_from_run_id": run.RunID, "fork_from_node": ordered[startIndex].ID,
		"reused_stage_outputs": startIndex, "blueprint": snapshot.Blueprint,
	}); err != nil {
		return ExecutionResult{}, err
	}
	input := ExecuteInput{ProjectID: run.ProjectID, CallerType: "owner", CallerID: ownerID, Channel: "desktop-fork", PipelineID: run.PipelineID, PipelineVersion: run.PipelineVersion, IdempotencyKey: forkKey, Input: snapshot.Input, EffectiveCapabilities: snapshot.EffectiveCapabilities, BlueprintSnapshot: snapshot.Blueprint, ForkedFromRunID: run.RunID}
	return s.executeNodes(ctx, version, forked, ordered, startIndex, snapshot.Input, outputs, input)
}

func (s *Service) RecoverInterrupted(ctx context.Context) error {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT run_id FROM harness_runs WHERE status IN ('queued','running','paused')`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.harness.AppendEvent(ctx, id, "run.failed", harness.CorePluginID, map[string]any{"error": "desktop process stopped before the run reached a durable boundary", "recoverable": true}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) failRun(ctx context.Context, runID, nodeID string, err error) {
	_, _ = s.harness.AppendEvent(ctx, runID, "run.failed", harness.CorePluginID, map[string]any{"node_id": nodeID, "error": err.Error()})
}

func (s *Service) executeStage(ctx context.Context, version Version, runID, spanID string, node Node, inputRaw json.RawMessage, execution ExecuteInput) (json.RawMessage, bool, error) {
	if handler, ok := s.stageHandlers[node.StageType]; ok {
		output, err := handler(ctx, StageInvocation{Pipeline: version, RunID: runID, SpanID: spanID, Node: node, Input: inputRaw, Execute: execution})
		if err != nil {
			return nil, false, err
		}
		if len(output) == 0 || !json.Valid(output) {
			return nil, false, fmt.Errorf("stage handler %q returned invalid JSON", node.StageType)
		}
		return output, false, nil
	}
	switch node.StageType {
	case "trigger.manual":
		return inputRaw, false, nil
	case "transform.map":
		var config struct {
			Value json.RawMessage `json:"value"`
			Merge json.RawMessage `json:"merge"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, false, err
		}
		if len(config.Value) > 0 {
			return config.Value, false, nil
		}
		if len(config.Merge) > 0 {
			var base, addition map[string]any
			if err := json.Unmarshal(inputRaw, &base); err != nil {
				return nil, false, errors.New("transform.map merge requires object input")
			}
			if err := json.Unmarshal(config.Merge, &addition); err != nil {
				return nil, false, errors.New("transform.map merge must be an object")
			}
			for key, value := range addition {
				base[key] = value
			}
			out, _ := json.Marshal(base)
			return out, false, nil
		}
		return inputRaw, false, nil
	case "transform.filter":
		var config struct {
			Field  string `json:"field"`
			Equals any    `json:"equals"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, false, err
		}
		var value map[string]any
		if err := json.Unmarshal(inputRaw, &value); err != nil {
			return nil, false, errors.New("transform.filter requires object input")
		}
		actual, _ := json.Marshal(value[config.Field])
		expected, _ := json.Marshal(config.Equals)
		if string(actual) != string(expected) {
			return json.RawMessage(`null`), false, nil
		}
		return inputRaw, false, nil
	case "policy.require_review":
		return inputRaw, true, nil
	case "llm.extract":
		if s.models == nil {
			return nil, false, errors.New("model runtime is unavailable")
		}
		var config struct {
			Prompt       string          `json:"prompt"`
			OutputSchema json.RawMessage `json:"output_schema"`
			MaxTokens    int             `json:"max_tokens"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, false, err
		}
		requestHash := hashBytes(append(append([]byte{}, node.Config...), inputRaw...))
		effectKey := "model.invoke"
		if _, err := s.harness.RecordEffectIntent(ctx, runID, node.ID, effectKey, requestHash); err != nil {
			return nil, false, err
		}
		providerKey := runID + ":" + node.ID
		if _, err := s.harness.MarkEffectDispatched(ctx, runID, node.ID, effectKey, providerKey); err != nil {
			return nil, false, err
		}
		modelCtx := modelusage.WithContext(ctx, modelusage.ContextInfo{RunID: runID, NodeID: node.ID, ProjectID: execution.ProjectID, StageType: node.StageType})
		generated, err := s.models.GenerateJSON(modelCtx, modelconfig.JSONGenerationRequest{SystemPrompt: config.Prompt, Input: inputRaw, OutputSchema: config.OutputSchema, MaxTokens: config.MaxTokens})
		if err != nil {
			_, _ = s.harness.RecordEffectReceipt(ctx, runID, node.ID, effectKey, "confirmed_failed", "", map[string]any{"error": err.Error()})
			return nil, false, err
		}
		canonical, err := harness.ValidateAgainstSchema(config.OutputSchema, generated.Output)
		if err != nil {
			_, _ = s.harness.RecordEffectReceipt(ctx, runID, node.ID, effectKey, "confirmed_failed", hashBytes(generated.Output), map[string]any{"provider_id": generated.ProviderID, "model": generated.Model, "error": "output schema validation failed"})
			return nil, false, fmt.Errorf("model output schema validation: %w", err)
		}
		if _, err := s.harness.RecordEffectReceipt(ctx, runID, node.ID, effectKey, "confirmed", hashBytes(canonical), map[string]any{"provider_id": generated.ProviderID, "provider": generated.Provider, "model": generated.Model}); err != nil {
			return nil, false, err
		}
		if _, err := s.harness.MarkEffectMaterialized(ctx, runID, node.ID, effectKey); err != nil {
			return nil, false, err
		}
		return canonical, false, nil
	case "object.materialize":
		var config struct {
			TypeID         string  `json:"type_id"`
			Status         string  `json:"status"`
			PluginID       string  `json:"plugin_id"`
			PluginVersion  string  `json:"plugin_version"`
			Confidence     float64 `json:"confidence"`
			Importance     float64 `json:"importance"`
			IdempotencyKey string  `json:"idempotency_key"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			return nil, false, err
		}
		if config.PluginID == "" {
			config.PluginID = version.PluginID
		}
		if config.PluginVersion == "" {
			config.PluginVersion = version.Version
		}
		if config.IdempotencyKey == "" {
			config.IdempotencyKey = execution.IdempotencyKey + ":" + node.ID
		}
		object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
			TypeID: config.TypeID, ProjectID: execution.ProjectID, Status: config.Status, Payload: inputRaw,
			Confidence: config.Confidence, Importance: config.Importance, RunID: runID, StageID: spanID,
			PluginID: config.PluginID, PluginVersion: config.PluginVersion, IdempotencyKey: config.IdempotencyKey,
		})
		if err != nil {
			return nil, false, err
		}
		if _, err := s.harness.AppendEvent(ctx, runID, "object.materialized", config.PluginID, map[string]any{"object_id": object.ObjectID, "type_id": object.TypeID, "revision": object.CurrentRevision, "content_hash": object.Revision.ContentHash}); err != nil {
			return nil, false, err
		}
		out, _ := json.Marshal(object)
		return out, false, nil
	default:
		return nil, false, fmt.Errorf("unsupported stage type %q", node.StageType)
	}
}
