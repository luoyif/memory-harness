// Package growth binds Memory Harness business stages to the generic pipeline
// host. The pipeline engine remains reusable; this package owns the default
// semantics for compiling Evidence, routing projects and materializing typed
// memory objects.
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

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/store"
)

const (
	DefaultPipelineID      = "builtin.core-memory-growth.default-import"
	DefaultPipelineVersion = "3.0.0"
)

type Service struct {
	control    *store.ControlStore
	memory     *memory.Engine
	portfolio  *portfolio.Service
	harness    *harness.Service
	pipelines  *pipeline.Service
	blueprints *blueprint.Service
	models     *modelconfig.Service
}

type ProcessInput struct {
	ProjectID             string
	SessionID             string
	EvidenceIDs           []string
	Primary               bool
	Force                 bool
	CallerType            string
	CallerID              string
	Channel               string
	IdempotencyKey        string
	EffectiveCapabilities []string
}

type ProcessResult struct {
	Compilation memory.RunResult         `json:"compilation"`
	Execution   pipeline.ExecutionResult `json:"execution"`
}

type stagePayload struct {
	ProjectID   string                         `json:"project_id"`
	SessionID   string                         `json:"session_id"`
	EvidenceIDs []string                       `json:"evidence_ids"`
	Primary     bool                           `json:"primary"`
	Force       bool                           `json:"force,omitempty"`
	Compilation memory.RunResult               `json:"compilation"`
	Projection  portfolio.DerivedProjection    `json:"projection"`
	Semantic    memory.SemanticMaterialization `json:"semantic"`
	Objects     map[string]int                 `json:"objects"`
	Indexed     int                            `json:"indexed"`
	Trace       map[string]any                 `json:"trace"`
}

func New(control *store.ControlStore, memoryEngine *memory.Engine, portfolioService *portfolio.Service, harnessService *harness.Service, pipelineService *pipeline.Service, blueprintService *blueprint.Service, models *modelconfig.Service) *Service {
	return &Service{control: control, memory: memoryEngine, portfolio: portfolioService, harness: harnessService, pipelines: pipelineService, blueprints: blueprintService, models: models}
}

func (s *Service) RegisterStages() error {
	handlers := map[string]pipeline.StageHandler{
		"memory.compile":        s.compile,
		"project.derive":        s.deriveProject,
		"memory.semantic_graph": s.semanticGraph,
		"memory.materialize":    s.materialize,
		"search.refresh":        s.refreshSearch,
	}
	for stageType, handler := range handlers {
		if err := s.pipelines.RegisterStageHandler(stageType, handler); err != nil {
			return err
		}
	}
	return nil
}

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func decodePayload(raw json.RawMessage) (stagePayload, error) {
	var payload stagePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, errors.New("memory growth stage requires an object input")
	}
	return payload, nil
}

func encodePayload(payload stagePayload) (json.RawMessage, error) {
	return json.Marshal(payload)
}

func (s *Service) Process(ctx context.Context, input ProcessInput) (ProcessResult, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	if input.ProjectID == "" || input.SessionID == "" {
		return ProcessResult{}, errors.New("project_id and session_id are required")
	}
	if input.CallerType == "" {
		input.CallerType = "system"
	}
	if input.CallerID == "" {
		input.CallerID = "memory-growth"
	}
	if input.Channel == "" {
		input.Channel = "default-import"
	}
	if len(input.EffectiveCapabilities) == 0 {
		input.EffectiveCapabilities = pipeline.OwnerCapabilities()
	}
	if input.IdempotencyKey == "" {
		parts := []string{input.ProjectID, input.SessionID, strings.Join(input.EvidenceIDs, ","), fmt.Sprint(input.Force)}
		if input.Force {
			parts = append(parts, time.Now().UTC().Format(time.RFC3339Nano))
		}
		input.IdempotencyKey = stableID("growth-", parts...)
	}
	payload, _ := json.Marshal(stagePayload{ProjectID: input.ProjectID, SessionID: input.SessionID, EvidenceIDs: input.EvidenceIDs, Primary: input.Primary, Force: input.Force, Objects: map[string]int{}})
	execution, err := s.pipelines.Execute(ctx, pipeline.ExecuteInput{
		ProjectID: input.ProjectID, CallerType: input.CallerType, CallerID: input.CallerID, Channel: input.Channel,
		PipelineID: DefaultPipelineID, PipelineVersion: DefaultPipelineVersion, IdempotencyKey: input.IdempotencyKey,
		Input: payload, EffectiveCapabilities: input.EffectiveCapabilities,
	})
	if err != nil {
		return ProcessResult{Execution: execution}, err
	}
	var final stagePayload
	if raw := execution.Outputs["result"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &final); err != nil {
			return ProcessResult{Execution: execution}, err
		}
	}
	if execution.Duplicate && final.Compilation.EpisodeID == "" {
		// Pipeline outputs are not replayed into the duplicate response. The
		// compiler is independently idempotent, so reconstruct the public result.
		final.Compilation, err = s.memory.EnqueueAndProcess(ctx, input.SessionID)
		if err != nil {
			return ProcessResult{Execution: execution}, err
		}
	}
	return ProcessResult{Compilation: final.Compilation, Execution: execution}, nil
}

func (s *Service) compile(ctx context.Context, invocation pipeline.StageInvocation) (json.RawMessage, error) {
	payload, err := decodePayload(invocation.Input)
	if err != nil {
		return nil, err
	}
	if payload.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	modelCtx := modelusage.WithContext(ctx, modelusage.ContextInfo{RunID: invocation.RunID, NodeID: invocation.Node.ID, ProjectID: invocation.Execute.ProjectID, StageType: invocation.Node.StageType})
	if payload.Force {
		payload.Compilation, err = s.memory.ReprocessSession(modelCtx, payload.SessionID)
	} else {
		payload.Compilation, err = s.memory.EnqueueAndProcess(modelCtx, payload.SessionID)
	}
	if err != nil {
		return nil, err
	}
	payload.Trace = map[string]any{
		"operation": "Evidence 提炼与受控长期记忆合并", "episode_id": payload.Compilation.EpisodeID,
		"evidence": payload.Compilation.Evidence, "knowledge_units": payload.Compilation.KnowledgeUnits,
		"memory_operations": payload.Compilation.Operations, "compiler": payload.Compilation.Compiler,
		"quality_status": payload.Compilation.QualityStatus,
	}
	return encodePayload(payload)
}

type record struct{ kind, id string }

func (s *Service) recordsForEpisode(ctx context.Context, episodeID string) ([]record, error) {
	queries := []struct{ kind, query string }{
		{"knowledge_unit", `SELECT unit_id FROM knowledge_units WHERE episode_id=?`},
		{"memory", `SELECT DISTINCT memory_id FROM memory_records,json_each(memory_records.episode_ids_json) WHERE json_each.value=?`},
	}
	records := []record{}
	for _, query := range queries {
		rows, err := s.control.DB.QueryContext(ctx, query.query, episodeID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			records = append(records, record{query.kind, id})
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (s *Service) deriveProject(ctx context.Context, invocation pipeline.StageInvocation) (json.RawMessage, error) {
	payload, err := decodePayload(invocation.Input)
	if err != nil {
		return nil, err
	}
	episodeID := payload.Compilation.EpisodeID
	for _, evidenceID := range payload.EvidenceIDs {
		if err := s.portfolio.LinkRecord(ctx, "evidence", evidenceID, invocation.Execute.ProjectID, payload.Primary); err != nil {
			return nil, err
		}
	}
	if err := s.portfolio.LinkRecord(ctx, "episode", episodeID, invocation.Execute.ProjectID, payload.Primary); err != nil {
		return nil, err
	}
	records, err := s.recordsForEpisode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	memoryIDs := []string{}
	for _, item := range records {
		if item.kind == "memory" {
			memoryIDs = append(memoryIDs, item.id)
		}
	}
	for _, table := range []struct{ kind, table, id string }{{"asset", "agent_assets", "asset_id"}} {
		for _, memoryID := range memoryIDs {
			query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s,json_each(%s.source_memory_ids_json) WHERE json_each.value=?`, table.id, table.table, table.table)
			rows, err := s.control.DB.QueryContext(ctx, query, memoryID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, err
				}
				records = append(records, record{table.kind, id})
			}
			if err := rows.Close(); err != nil {
				return nil, err
			}
		}
	}
	for _, item := range records {
		if err := s.portfolio.LinkRecord(ctx, item.kind, item.id, invocation.Execute.ProjectID, payload.Primary); err != nil {
			return nil, err
		}
	}
	policy := growthPolicyFromSnapshot(invocation.Execute.BlueprintSnapshot)
	livingViews := []memory.LivingView{}
	if policy.roleEnabled("growth.living") {
		livingViews, err = s.memory.RebuildLivingViewsForProject(ctx, invocation.Execute.ProjectID)
	} else {
		err = s.memory.ClearLivingViewsForProject(ctx, invocation.Execute.ProjectID)
	}
	if err != nil {
		return nil, err
	}
	payload.Projection, err = s.portfolio.SyncDerivedFromEpisode(ctx, invocation.Execute.ProjectID, episodeID)
	if err != nil {
		return nil, err
	}
	payload.Trace = map[string]any{
		"operation": "项目归属与工作台投影", "linked_records": len(records) + len(payload.EvidenceIDs) + 1 + len(livingViews), "living_views": len(livingViews),
		"context_blocks": payload.Projection.ContextBlocks, "goals": payload.Projection.Goals,
		"decisions": payload.Projection.Decisions, "risks": payload.Projection.Risks,
	}
	return encodePayload(payload)
}

func (s *Service) semanticGraph(ctx context.Context, invocation pipeline.StageInvocation) (json.RawMessage, error) {
	payload, err := decodePayload(invocation.Input)
	if err != nil {
		return nil, err
	}
	payload.Semantic, err = s.memory.MaterializeSemantics(ctx, invocation.Execute.ProjectID, payload.Compilation.EpisodeID, invocation.RunID, invocation.SpanID)
	if err != nil {
		return nil, err
	}
	payload.Trace = map[string]any{
		"operation": "归因安全的实体、双向关系与时间投影", "units": payload.Semantic.Units,
		"entities": payload.Semantic.Entities, "assertions": payload.Semantic.Assertions,
		"temporal_facts": payload.Semantic.TemporalFacts, "review_required": payload.Semantic.AmbiguousUnits,
	}
	return encodePayload(payload)
}

func objectPayloadForMemory(item memory.MemoryRecord) (typeID, pluginID string, payload json.RawMessage) {
	pluginID = "builtin.core-memory-growth"
	switch item.Tier {
	case "episodic":
		typeID = "builtin.core-memory-growth.episodic"
		payload, _ = json.Marshal(map[string]any{"summary": item.Summary, "outcome": item.Body, "participants": []string{}})
	case "procedural":
		typeID = "builtin.core-memory-growth.procedural"
		payload, _ = json.Marshal(map[string]any{"procedure": item.Body, "trigger": item.Summary})
	case "identity_core":
		typeID = "builtin.core-memory-growth.identity"
		payload, _ = json.Marshal(map[string]any{"statement": item.Body, "stability": "review_required"})
	default:
		typeID = "builtin.core-memory-growth.semantic"
		payload, _ = json.Marshal(map[string]any{"statement": item.Body, "domain": item.Domain})
	}
	return typeID, pluginID, payload
}

func (s *Service) materializeOne(ctx context.Context, input harness.MaterializeInput) (bool, error) {
	object, err := s.harness.Materialize(ctx, input)
	if err != nil {
		return false, err
	}
	return !object.Duplicate, nil
}

func (s *Service) materialize(ctx context.Context, invocation pipeline.StageInvocation) (json.RawMessage, error) {
	payload, err := decodePayload(invocation.Input)
	if err != nil {
		return nil, err
	}
	projectID, episodeID := invocation.Execute.ProjectID, payload.Compilation.EpisodeID
	counts := map[string]int{"knowledge": 0, "episode": 0, "long_term": 0, "living": 0, "knowledge_product": 0, "agent_asset": 0}
	policy := growthPolicyFromSnapshot(invocation.Execute.BlueprintSnapshot)
	units, err := s.memory.ListKnowledgeUnits(ctx, episodeID, "", 500)
	if err != nil {
		return nil, err
	}
	if policy.roleEnabled("growth.knowledge") {
		for _, unit := range units {
			objectID := memory.StructuredKnowledgeObjectID(projectID, unit.UnitID)
			// Once an Owner has corrected a structured KU, automatic growth must
			// not append a new current revision from the compatibility tables.
			// Future automated improvements must be explicit revision proposals.
			if _, objectErr := s.harness.Object(ctx, objectID); objectErr == nil {
				continue
			} else if !errors.Is(objectErr, sql.ErrNoRows) {
				return nil, objectErr
			}
			raw, err := memory.StructuredKnowledgePayload(unit)
			if err != nil {
				return nil, err
			}
			created, err := s.materializeOne(ctx, harness.MaterializeInput{ObjectID: objectID, TypeID: memory.StructuredKnowledgeUnitTypeV2, ProjectID: projectID, Status: "active", Payload: raw, Confidence: unit.Confidence, Importance: unit.Structure.Epistemic.Importance, ValidFrom: unit.ObservedAt, ValidUntil: unit.Structure.Temporal.ValidUntil, SourceEvidenceIDs: []string{unit.EvidenceID}, RunID: invocation.RunID, StageID: invocation.SpanID, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: projectID + ":" + unit.UnitID + ":structured-ku-v2"})
			if err != nil {
				return nil, err
			}
			if created {
				counts["knowledge"]++
			}
		}
	}
	memories, _, err := s.memory.ListMemoriesForProject(ctx, projectID, "", "", 500, 0)
	if err != nil {
		return nil, err
	}
	for _, item := range memories {
		linked := false
		for _, sourceEpisodeID := range item.EpisodeIDs {
			linked = linked || sourceEpisodeID == episodeID
		}
		if !linked {
			continue
		}
		if item.Tier == "episodic" && !policy.roleEnabled("growth.episode") {
			continue
		}
		if item.Tier != "episodic" && !policy.roleEnabled("growth.long-term") {
			continue
		}
		objectID := memory.StructuredMemoryObjectID(projectID, item.MemoryID)
		// The current structured Memory Revision is authoritative once it
		// exists. Automatic growth must not overwrite a human correction.
		if _, objectErr := s.harness.Object(ctx, objectID); objectErr == nil {
			continue
		} else if !errors.Is(objectErr, sql.ErrNoRows) {
			return nil, objectErr
		}
		raw, err := memory.StructuredMemoryPayload(item)
		if err != nil {
			return nil, err
		}
		created, err := s.materializeOne(ctx, harness.MaterializeInput{ObjectID: objectID, TypeID: memory.StructuredMemoryRecordTypeV1, ProjectID: projectID, Status: "active", Payload: raw, Confidence: item.Confidence, Importance: item.Importance, ValidFrom: item.ObservedAt, SourceEvidenceIDs: item.EvidenceIDs, RunID: invocation.RunID, StageID: invocation.SpanID, PluginID: "builtin.core-memory-growth", PluginVersion: "2.0.0", IdempotencyKey: projectID + ":" + item.MemoryID + ":structured-memory-v1"})
		if err != nil {
			return nil, err
		}
		if created {
			if item.Tier == "episodic" {
				counts["episode"]++
			} else {
				counts["long_term"]++
			}
		}
	}
	views, err := s.memory.ListLivingViewsForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if policy.roleEnabled("growth.living") {
		for _, item := range views {
			raw, _ := json.Marshal(map[string]any{"title": item.Title, "summary": item.Summary, "format": "markdown"})
			created, err := s.materializeOne(ctx, harness.MaterializeInput{ObjectID: stableID("obj-live-", projectID, item.ViewID), TypeID: "builtin.living-asset-vault.document", ProjectID: projectID, Status: "candidate", Payload: raw, Confidence: 1, Importance: .7, SourceObjectIDs: item.SourceMemoryIDs, RunID: invocation.RunID, StageID: invocation.SpanID, PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: projectID + ":" + item.ViewID + ":living-v2", LivingAssetPath: item.CanonicalPath})
			if err != nil {
				return nil, err
			}
			if created {
				counts["living"]++
			}
		}
		createdProduct, err := s.materializeProjectBrief(ctx, projectID, invocation.RunID, invocation.SpanID)
		if err != nil {
			return nil, err
		}
		if createdProduct {
			counts["knowledge_product"]++
		}
	}
	assets, err := s.memory.ListAssetsForProject(ctx, projectID, "", 500)
	if err != nil {
		return nil, err
	}
	if policy.roleEnabled("growth.agent-asset") {
		for _, item := range assets {
			if !memory.IsGovernedAgentAssetType(item.AssetType) {
				// Ambiguous assets remain visible in the owner review queue and must not
				// become executable Agent objects until a human resolves their type.
				continue
			}
			raw, _ := json.Marshal(map[string]any{"asset_id": item.AssetID, "asset_type": item.AssetType, "title": item.Title, "body": item.Summary, "source_memory_ids": item.SourceMemoryIDs, "validation_status": item.ValidationStatus})
			revisionKey := stableID("typed-asset-", item.AssetType, item.Title, item.Summary, item.Version)
			created, err := s.materializeOne(ctx, harness.MaterializeInput{ObjectID: stableID("obj-governed-asset-v3-", projectID, item.AssetID), TypeID: harness.GovernedAgentAssetTypeV3, ProjectID: projectID, Status: "candidate", Payload: raw, Confidence: 1, Importance: .8, SourceObjectIDs: item.SourceMemoryIDs, RunID: invocation.RunID, StageID: invocation.SpanID, PluginID: "builtin.agent-assets", PluginVersion: "2.0.0", IdempotencyKey: projectID + ":" + item.AssetID + ":" + revisionKey})
			if err != nil {
				return nil, err
			}
			if created {
				counts["agent_asset"]++
			}
		}
	}
	payload.Objects = counts
	disabledRoles := []string{}
	for _, role := range []string{"growth.knowledge", "growth.episode", "growth.long-term", "growth.living", "growth.agent-asset"} {
		if !policy.roleEnabled(role) {
			disabledRoles = append(disabledRoles, role)
		}
	}
	payload.Trace = map[string]any{"operation": "六层类型化对象物化", "created": counts, "source_bound": true, "run_bound": true, "blueprint_id": policy.Blueprint, "blueprint_version": policy.Version, "disabled_roles": disabledRoles}
	return encodePayload(payload)
}

func (s *Service) refreshSearch(ctx context.Context, invocation pipeline.StageInvocation) (json.RawMessage, error) {
	payload, err := decodePayload(invocation.Input)
	if err != nil {
		return nil, err
	}
	payload.Indexed, err = s.portfolio.RebuildProjectIndex(ctx, invocation.Execute.ProjectID)
	if err != nil {
		return nil, err
	}
	payload.Trace = map[string]any{"operation": "刷新项目检索投影", "indexed_records": payload.Indexed, "project_id": invocation.Execute.ProjectID}
	return encodePayload(payload)
}

// EnsureGovernedAsset materializes one manually resolved typed asset as a
// non-executable candidate. Repeated calls are content-idempotent; a changed
// manual classification may append a candidate revision but never activates it.
func (s *Service) EnsureGovernedAsset(ctx context.Context, projectID string, item memory.AgentAsset) (harness.Object, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return harness.Object{}, errors.New("project_id is required")
	}
	if !memory.IsGovernedAgentAssetType(item.AssetType) {
		return harness.Object{}, errors.New("asset must be manually classified before governance")
	}
	raw, _ := json.Marshal(map[string]any{
		"asset_id": item.AssetID, "asset_type": item.AssetType, "title": item.Title, "body": item.Summary,
		"source_memory_ids": item.SourceMemoryIDs, "validation_status": item.ValidationStatus,
	})
	revisionKey := stableID("typed-asset-", item.AssetType, item.Title, item.Summary, item.Version)
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: stableID("obj-governed-asset-v3-", projectID, item.AssetID), TypeID: harness.GovernedAgentAssetTypeV3,
		ProjectID: projectID, Status: "candidate", Payload: raw, Confidence: 1, Importance: .8,
		SourceObjectIDs: item.SourceMemoryIDs, PluginID: "builtin.agent-assets", PluginVersion: "2.0.0",
		IdempotencyKey: projectID + ":" + item.AssetID + ":" + revisionKey,
	})
}
