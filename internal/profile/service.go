package profile

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control   *store.ControlStore
	memory    *memory.Engine
	portfolio *portfolio.Service
	harness   *harness.Service
}

func New(control *store.ControlStore, memoryEngine *memory.Engine, portfolioService *portfolio.Service, harnessService *harness.Service) *Service {
	return &Service{control: control, memory: memoryEngine, portfolio: portfolioService, harness: harnessService}
}

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(strings.TrimSpace(value)))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func unique(values ...[]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, group := range values {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	sort.Strings(out)
	return out
}

func latest(values ...string) string {
	out := ""
	for _, value := range values {
		if value = strings.TrimSpace(value); value > out {
			out = value
		}
	}
	return out
}

func blockHash(value any) string {
	raw, _ := json.Marshal(value)
	return contracts.HashBytes(raw)
}

func trimText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "…"
}

func (s *Service) currentMemoryObjectID(ctx context.Context, projectID, memoryID string) string {
	var objectID string
	err := s.control.DB.QueryRowContext(ctx, `SELECT o.object_id FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE o.project_id=? AND o.type_id=? AND json_extract(r.payload_json,'$.memory_id')=? LIMIT 1`, projectID, memory.StructuredMemoryRecordTypeV1, memoryID).Scan(&objectID)
	if err != nil {
		return ""
	}
	return objectID
}

func (s *Service) projectAuthorityObjects(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT object_id FROM harness_objects WHERE project_id=? AND type_id IN (?,?) AND status='active' ORDER BY updated_at DESC,object_id LIMIT 300`, projectID, memory.StructuredKnowledgeUnitTypeV2, memory.StructuredMemoryRecordTypeV1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Service) maxSourceRevision(ctx context.Context, objectIDs []string) int {
	maxRevision := 0
	for _, objectID := range unique(objectIDs) {
		var revision int
		if err := s.control.DB.QueryRowContext(ctx, `SELECT current_revision FROM harness_objects WHERE object_id=?`, objectID).Scan(&revision); err == nil && revision > maxRevision {
			maxRevision = revision
		}
	}
	return maxRevision
}

func (s *Service) currentProjection(ctx context.Context, objectID string) (Projection, harness.Object, error) {
	object, err := s.harness.Object(ctx, objectID)
	if err != nil {
		return Projection{}, harness.Object{}, err
	}
	var projection Projection
	if err := json.Unmarshal(object.Revision.Payload, &projection); err != nil {
		return Projection{}, harness.Object{}, err
	}
	return projection, object, nil
}

func mergeLocked(prior Projection, generated Projection) Projection {
	priorByID := map[string]Block{}
	for _, block := range prior.Blocks {
		priorByID[block.BlockID] = block
	}
	seen := map[string]bool{}
	merged := make([]Block, 0, len(generated.Blocks)+len(prior.Blocks))
	for _, block := range generated.Blocks {
		seen[block.BlockID] = true
		if old, ok := priorByID[block.BlockID]; ok && old.Locked {
			old.CandidateSourceHash = ""
			old.ReviewStatus = "current"
			if old.SourceHash != block.SourceHash {
				old.CandidateSourceHash = block.SourceHash
				old.ReviewStatus = "stale_locked"
			}
			merged = append(merged, old)
			continue
		}
		merged = append(merged, block)
	}
	for _, old := range prior.Blocks {
		if old.Locked && !seen[old.BlockID] {
			old.CandidateSourceHash = ""
			old.ReviewStatus = "stale_locked"
			merged = append(merged, old)
		}
	}
	generated.Blocks = merged
	generated.LockedBlockIDs = []string{}
	for _, block := range merged {
		if block.Locked {
			generated.LockedBlockIDs = append(generated.LockedBlockIDs, block.BlockID)
		}
	}
	if len(generated.LockedBlockIDs) > 0 {
		generated.GenerationStatus = "human_mixed"
	}
	return generated
}

func projectionObjectID(projectID, viewKind string) string {
	return stableID("profile-", projectID, viewKind)
}

func (s *Service) materialize(ctx context.Context, projectID string, projection Projection) (harness.Object, error) {
	objectID := projectionObjectID(projectID, projection.ViewKind)
	if prior, _, err := s.currentProjection(ctx, objectID); err == nil {
		projection = mergeLocked(prior, projection)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return harness.Object{}, err
	}
	sourceRefs := []string{}
	sourceObjects := []string{}
	for _, block := range projection.Blocks {
		sourceRefs = append(sourceRefs, block.SourceRefs...)
		sourceObjects = append(sourceObjects, block.SourceObjectIDs...)
	}
	projection.GeneratedFromRevision = s.maxSourceRevision(ctx, sourceObjects)
	raw, err := json.Marshal(projection)
	if err != nil {
		return harness.Object{}, err
	}
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: objectID, TypeID: harness.ProfileProjectionTypeV1, ProjectID: projectID, Status: "active",
		Payload: raw, Confidence: 1, Importance: .75, SourceEvidenceIDs: unique(sourceRefs), SourceObjectIDs: unique(sourceObjects),
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: projectID + ":profile:" + projection.ViewKind + ":" + contracts.HashBytes(raw),
	})
}

func (s *Service) compileOwnerIdentity(ctx context.Context, projectID string) (Projection, error) {
	project, err := s.portfolio.Project(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	memories, _, err := s.memory.ListMemoriesForProject(ctx, projectID, "identity_core", "", 100, 0)
	if err != nil {
		return Projection{}, err
	}
	blocks := []Block{}
	generatedAt := project.UpdatedAt
	for _, item := range memories {
		if item.Status != "active" && item.Status != "corroborated" {
			continue
		}
		objectID := s.currentMemoryObjectID(ctx, projectID, item.MemoryID)
		verified := latest(item.UpdatedAt, item.LastReinforcedAt, item.ObservedAt)
		generatedAt = latest(generatedAt, verified)
		content := strings.TrimSpace(item.Body)
		if content == "" {
			content = item.Summary
		}
		block := Block{
			BlockID: "identity:" + item.MemoryID, Label: trimText(item.Summary, 180), Content: trimText(content, 2400),
			SourceRefs: unique(item.EvidenceIDs), SourceObjectIDs: unique([]string{objectID}), ValidFrom: item.ObservedAt,
			LastVerifiedAt: verified, Confidence: item.Confidence, Locked: false, ReviewStatus: "current",
		}
		block.SourceHash = blockHash(map[string]any{"memory_id": item.MemoryID, "summary": item.Summary, "body": item.Body, "refs": block.SourceRefs, "object_id": objectID})
		blocks = append(blocks, block)
		if len(blocks) >= 12 {
			break
		}
	}
	return Projection{ProfileID: stableID("profile-id-", projectID, ViewOwnerIdentity), ViewKind: ViewOwnerIdentity, ProfileClass: "static", Title: "Owner Identity", Summary: fmt.Sprintf("%d 个经当前记忆权威支持的稳定身份/原则块。", len(blocks)), Blocks: blocks, LockedBlockIDs: []string{}, GenerationStatus: "auto", GeneratedAt: generatedAt}, nil
}

func linesForGoals(items []portfolio.Goal, max int) (string, []string, string) {
	lines, refs, updated := []string{}, []string{}, ""
	for _, item := range items {
		if item.Status == "completed" || item.Status == "cancelled" {
			continue
		}
		line := fmt.Sprintf("- [%s] %s", item.Status, item.Title)
		if item.TargetAt != "" {
			line += " · target " + item.TargetAt
		}
		lines = append(lines, line)
		if item.SourceEvidenceID != "" {
			refs = append(refs, item.SourceEvidenceID)
		}
		updated = latest(updated, item.UpdatedAt)
		if len(lines) >= max {
			break
		}
	}
	return strings.Join(lines, "\n"), unique(refs), updated
}

func linesForRisks(items []portfolio.Risk, max int) (string, []string, string) {
	lines, refs, updated := []string{}, []string{}, ""
	for _, item := range items {
		if item.Status == "closed" || item.Status == "mitigated" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s · %d] %s", item.Status, item.Score, item.Title))
		if item.SourceEvidenceID != "" {
			refs = append(refs, item.SourceEvidenceID)
		}
		updated = latest(updated, item.UpdatedAt)
		if len(lines) >= max {
			break
		}
	}
	return strings.Join(lines, "\n"), unique(refs), updated
}

func linesForDecisions(items []portfolio.Decision, max int) (string, []string, string) {
	lines, refs, updated := []string{}, []string{}, ""
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s：%s", item.Title, trimText(item.Decision, 320)))
		refs = append(refs, item.SourceEvidenceIDs...)
		updated = latest(updated, item.DecidedAt, item.CreatedAt)
		if len(lines) >= max {
			break
		}
	}
	return strings.Join(lines, "\n"), unique(refs), updated
}

func linesForFacts(items []portfolio.TemporalFact, max int) (string, []string, string) {
	lines, refs, updated := []string{}, []string{}, ""
	for _, item := range items {
		if item.Status != "active" {
			continue
		}
		line := fmt.Sprintf("- %s · %s · %s", item.Subject, item.Predicate, item.Object)
		if item.ValidFrom != "" {
			line += " · from " + item.ValidFrom
		}
		lines = append(lines, line)
		refs = append(refs, item.SourceEvidenceIDs...)
		updated = latest(updated, item.RecordedAt, item.ValidFrom)
		if len(lines) >= max {
			break
		}
	}
	return strings.Join(lines, "\n"), unique(refs), updated
}

func makeBlock(id, label, content string, refs, objects []string, verified string, confidence float64) Block {
	block := Block{
		BlockID: id, Label: label, Content: trimText(content, 6000), SourceRefs: unique(refs), SourceObjectIDs: unique(objects),
		LastVerifiedAt: verified, Confidence: confidence, Locked: false, ReviewStatus: "current",
	}
	block.SourceHash = blockHash(map[string]any{"id": id, "content": block.Content, "refs": block.SourceRefs, "objects": block.SourceObjectIDs, "verified": verified})
	return block
}

func (s *Service) compileDynamicProject(ctx context.Context, projectID string) (Projection, error) {
	summary, err := s.portfolio.ProjectSummary(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	goals, err := s.portfolio.ListGoals(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	decisions, err := s.portfolio.ListDecisions(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	risks, err := s.portfolio.ListRisks(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	facts, err := s.portfolio.ListFacts(ctx, projectID, "", false, 50)
	if err != nil {
		return Projection{}, err
	}
	authorityObjects, err := s.projectAuthorityObjects(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	if len(authorityObjects) > 50 {
		authorityObjects = authorityObjects[:50]
	}
	blocks := []Block{}
	stateText := fmt.Sprintf("Project: %s\nStatus: %s\nOpen goals: %d\nOpen risks: %d\nActive temporal facts: %d", summary.Project.Name, summary.Project.Status, summary.Metrics.OpenGoals, summary.Metrics.OpenRisks, summary.Metrics.Facts)
	blocks = append(blocks, makeBlock("project:state", "当前项目状态", stateText, nil, authorityObjects, summary.Project.UpdatedAt, 1))
	generatedAt := summary.Project.UpdatedAt
	if content, refs, updated := linesForGoals(goals, 8); content != "" {
		blocks = append(blocks, makeBlock("project:goals", "当前目标", content, refs, nil, updated, 1))
		generatedAt = latest(generatedAt, updated)
	}
	if content, refs, updated := linesForDecisions(decisions, 8); content != "" {
		blocks = append(blocks, makeBlock("project:decisions", "有效决策", content, refs, nil, updated, 1))
		generatedAt = latest(generatedAt, updated)
	}
	if content, refs, updated := linesForRisks(risks, 8); content != "" {
		blocks = append(blocks, makeBlock("project:risks", "当前风险", content, refs, nil, updated, 1))
		generatedAt = latest(generatedAt, updated)
	}
	if content, refs, updated := linesForFacts(facts, 10); content != "" {
		blocks = append(blocks, makeBlock("project:temporal", "当前有效时间事实", content, refs, nil, updated, .95))
		generatedAt = latest(generatedAt, updated)
	}
	return Projection{ProfileID: stableID("profile-id-", projectID, ViewDynamicProject), ViewKind: ViewDynamicProject, ProfileClass: "dynamic", Title: "Dynamic Project", Summary: fmt.Sprintf("%d 个项目动态块，来自当前受治理投影。", len(blocks)), Blocks: blocks, LockedBlockIDs: []string{}, GenerationStatus: "auto", GeneratedAt: generatedAt}, nil
}

func (s *Service) compileSessionResume(ctx context.Context, projectID string) (Projection, error) {
	project, err := s.portfolio.Project(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	goals, err := s.portfolio.ListGoals(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	milestones, err := s.portfolio.ListMilestones(ctx, projectID)
	if err != nil {
		return Projection{}, err
	}
	memories, _, err := s.memory.ListMemoriesForProject(ctx, projectID, "", "", 60, 0)
	if err != nil {
		return Projection{}, err
	}
	blocks := []Block{}
	generatedAt := project.UpdatedAt
	if content, refs, updated := linesForGoals(goals, 6); content != "" {
		blocks = append(blocks, makeBlock("resume:goals", "未完成目标", content, refs, nil, updated, 1))
		generatedAt = latest(generatedAt, updated)
	}
	pending := []string{}
	pendingUpdated := ""
	for _, item := range milestones {
		if item.Status == "completed" || item.Status == "cancelled" {
			continue
		}
		line := fmt.Sprintf("- [%s] %s", item.Status, item.Title)
		if item.DueAt != "" {
			line += " · due " + item.DueAt
		}
		pending = append(pending, line)
		pendingUpdated = latest(pendingUpdated, item.UpdatedAt)
		if len(pending) >= 8 {
			break
		}
	}
	if len(pending) > 0 {
		blocks = append(blocks, makeBlock("resume:milestones", "未完成里程碑", strings.Join(pending, "\n"), nil, nil, pendingUpdated, 1))
		generatedAt = latest(generatedAt, pendingUpdated)
	}
	recent := []string{}
	recentRefs := []string{}
	recentObjects := []string{}
	recentUpdated := ""
	for _, item := range memories {
		if item.Status != "active" && item.Status != "corroborated" {
			continue
		}
		recent = append(recent, fmt.Sprintf("- %s：%s", item.Summary, trimText(item.Body, 360)))
		recentRefs = append(recentRefs, item.EvidenceIDs...)
		if objectID := s.currentMemoryObjectID(ctx, projectID, item.MemoryID); objectID != "" {
			recentObjects = append(recentObjects, objectID)
		}
		recentUpdated = latest(recentUpdated, item.UpdatedAt, item.LastReinforcedAt)
		if len(recent) >= 6 {
			break
		}
	}
	if len(recent) > 0 {
		blocks = append(blocks, makeBlock("resume:recent-memory", "最近可恢复记忆", strings.Join(recent, "\n"), recentRefs, recentObjects, recentUpdated, .9))
		generatedAt = latest(generatedAt, recentUpdated)
	}
	return Projection{ProfileID: stableID("profile-id-", projectID, ViewSessionResume), ViewKind: ViewSessionResume, ProfileClass: "dynamic", Title: "Session Resume", Summary: fmt.Sprintf("%d 个用于恢复未完成上下文的块。", len(blocks)), Blocks: blocks, LockedBlockIDs: []string{}, GenerationStatus: "auto", GeneratedAt: generatedAt}, nil
}

func shouldMaterialize(projection Projection) bool {
	return projection.ViewKind == ViewDynamicProject || len(projection.Blocks) > 0
}

func (s *Service) ReconcileProject(ctx context.Context, projectID string) ([]harness.Object, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}
	if _, err := s.portfolio.Project(ctx, projectID); err != nil {
		return nil, err
	}
	compilers := []func(context.Context, string) (Projection, error){s.compileOwnerIdentity, s.compileDynamicProject, s.compileSessionResume}
	out := []harness.Object{}
	for _, compile := range compilers {
		projection, err := compile(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if !shouldMaterialize(projection) {
			continue
		}
		object, err := s.materialize(ctx, projectID, projection)
		if err != nil {
			return nil, err
		}
		out = append(out, object)
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, projectID string) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), harness.ProfileProjectionTypeV1, "active", 20)
}

func (s *Service) RefreshProject(ctx context.Context, projectID string) ([]harness.Object, error) {
	return s.ReconcileProject(ctx, projectID)
}

func (s *Service) SetLockedBlocks(ctx context.Context, projectID, viewKind string, blockIDs []string) (harness.Object, error) {
	objectID := projectionObjectID(strings.TrimSpace(projectID), strings.TrimSpace(viewKind))
	projection, object, err := s.currentProjection(ctx, objectID)
	if err != nil {
		return harness.Object{}, err
	}
	locks := map[string]bool{}
	for _, id := range unique(blockIDs) {
		locks[id] = true
	}
	projection.LockedBlockIDs = nil
	for i := range projection.Blocks {
		projection.Blocks[i].Locked = locks[projection.Blocks[i].BlockID]
		if projection.Blocks[i].Locked {
			projection.LockedBlockIDs = append(projection.LockedBlockIDs, projection.Blocks[i].BlockID)
		}
	}
	if len(projection.LockedBlockIDs) > 0 {
		projection.GenerationStatus = "human_mixed"
	} else {
		projection.GenerationStatus = "auto"
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return harness.Object{}, err
	}
	refs, sources := []string{}, []string{}
	for _, block := range projection.Blocks {
		refs = append(refs, block.SourceRefs...)
		sources = append(sources, block.SourceObjectIDs...)
	}
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: object.ObjectID, TypeID: harness.ProfileProjectionTypeV1, ProjectID: object.ProjectID, Status: "active",
		Payload: raw, Confidence: 1, Importance: .75, SourceEvidenceIDs: unique(refs), SourceObjectIDs: unique(sources),
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: object.ProjectID + ":profile-lock:" + viewKind + ":" + contracts.HashBytes(raw),
	})
}

func appendBlocksWithBudget(dst []Block, source []Block, maxChars int) []Block {
	used := 0
	for _, block := range dst {
		used += len([]rune(block.Content))
	}
	for _, block := range source {
		cost := len([]rune(block.Content))
		if used+cost > maxChars {
			break
		}
		dst = append(dst, block)
		used += cost
	}
	return dst
}

func (s *Service) AgentView(ctx context.Context, principal agentauth.Principal, projectID string) (AgentView, error) {
	projectID = strings.TrimSpace(projectID)
	if !agentauth.CanAccessProject(principal, projectID) {
		return AgentView{}, errors.New("agent is not granted to this project")
	}
	objects, err := s.List(ctx, projectID)
	if err != nil {
		return AgentView{}, err
	}
	view := AgentView{AgentID: principal.AgentID, ProjectID: projectID, Blocks: []Block{}, SourceProjectionIDs: []string{}, DeliveryStatus: "not_delivered"}
	for _, object := range objects {
		var projection Projection
		if err := json.Unmarshal(object.Revision.Payload, &projection); err != nil {
			return AgentView{}, err
		}
		allowed := false
		switch projection.ViewKind {
		case ViewDynamicProject:
			allowed = agentauth.HasPermission(principal, agentauth.PermissionProjectRead)
		case ViewSessionResume:
			allowed = agentauth.HasPermission(principal, agentauth.PermissionMemoryRead)
		}
		if !allowed {
			continue
		}
		blocks := append([]Block(nil), projection.Blocks...)
		if !agentauth.HasPermission(principal, agentauth.PermissionMemoryRead) {
			for i := range blocks {
				blocks[i].SourceRefs = []string{}
				blocks[i].SourceObjectIDs = []string{}
			}
		}
		view.Blocks = appendBlocksWithBudget(view.Blocks, blocks, 12000)
		view.SourceProjectionIDs = append(view.SourceProjectionIDs, object.ObjectID)
		view.GeneratedAt = latest(view.GeneratedAt, projection.GeneratedAt, object.UpdatedAt)
	}
	view.SourceProjectionIDs = unique(view.SourceProjectionIDs)
	return view, nil
}

func (s *Service) Projection(ctx context.Context, projectID, viewKind string) (Projection, harness.Object, error) {
	objectID := projectionObjectID(strings.TrimSpace(projectID), strings.TrimSpace(viewKind))
	projection, object, err := s.currentProjection(ctx, objectID)
	if err != nil {
		return Projection{}, harness.Object{}, err
	}
	if object.ProjectID != strings.TrimSpace(projectID) {
		return Projection{}, harness.Object{}, sql.ErrNoRows
	}
	return projection, object, nil
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }
