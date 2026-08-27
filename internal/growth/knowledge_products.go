package growth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

func productRefs(values ...[]string) []string {
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
	return out
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func lockedFieldSet(payload map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, field := range stringSlice(payload["locked_fields"]) {
		out[field] = true
	}
	return out
}

func productTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, value)
	}
	return parsed, err == nil
}

func staleProductTime(value string, now time.Time, age time.Duration) bool {
	parsed, ok := productTime(value)
	return ok && now.Sub(parsed) > age
}

func (s *Service) projectBriefPayload(ctx context.Context, projectID string, preserveLocks bool) (string, string, []string, json.RawMessage, error) {
	project, err := s.portfolio.Project(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	blocks, err := s.portfolio.ListContextBlocks(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	goals, err := s.portfolio.ListGoals(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	milestones, err := s.portfolio.ListMilestones(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	decisions, err := s.portfolio.ListDecisions(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	risks, err := s.portfolio.ListRisks(ctx, projectID)
	if err != nil {
		return "", "", nil, nil, err
	}
	timeline, err := s.portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: projectID, Kinds: []string{"fact", "goal", "milestone", "decision"}, Limit: 100})
	if err != nil {
		return "", "", nil, nil, err
	}
	units, _, err := s.memory.ListKnowledgeUnitsForProject(ctx, projectID, "", 500, 0)
	if err != nil {
		return "", "", nil, nil, err
	}
	pendingTime := make([]string, 0)
	pendingRefs := []string{}
	for _, unit := range units {
		text := strings.TrimSpace(unit.Structure.Temporal.EventTimeText)
		resolution := strings.TrimSpace(unit.Structure.Temporal.Resolution)
		if text != "" && resolution != "resolved" && resolution != "not_applicable" {
			pendingTime = append(pendingTime, fmt.Sprintf("- 待确认 `%s`：%s", text, unit.Statement))
			pendingRefs = append(pendingRefs, unit.EvidenceID)
		}
	}
	memories, _, err := s.memory.ListMemoriesForProject(ctx, projectID, "", "", 500, 0)
	if err != nil {
		return "", "", nil, nil, err
	}
	now := time.Now().UTC()
	staleGoals := []string{}
	for _, goal := range goals {
		status := strings.ToLower(strings.TrimSpace(goal.Status))
		if status != "completed" && status != "cancelled" && staleProductTime(goal.UpdatedAt, now, 30*24*time.Hour) {
			staleGoals = append(staleGoals, goal.Title)
		}
	}
	staleRisks := []string{}
	for _, risk := range risks {
		if strings.EqualFold(strings.TrimSpace(risk.Status), "open") && staleProductTime(risk.UpdatedAt, now, 30*24*time.Hour) {
			staleRisks = append(staleRisks, risk.Title)
		}
	}
	staleMemories := []string{}
	for _, item := range memories {
		if item.Status != "active" && item.Status != "corroborated" {
			continue
		}
		stamp := item.LastReinforcedAt
		if strings.TrimSpace(stamp) == "" {
			stamp = item.UpdatedAt
		}
		if staleProductTime(stamp, now, 90*24*time.Hour) {
			staleMemories = append(staleMemories, item.Summary)
		}
	}

	productID := stableID("product-", projectID, "project_brief")
	objectID := stableID("obj-product-", projectID, "project_brief")
	title := project.Name + " · 项目简报"
	summary := fmt.Sprintf("%d 个目标 · %d 个里程碑 · %d 项决策 · %d 个风险", len(goals), len(milestones), len(decisions), len(risks))
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n", title)
	if strings.TrimSpace(project.Description) != "" {
		fmt.Fprintf(&body, "%s\n\n", project.Description)
	}
	body.WriteString("## 当前上下文\n")
	if len(blocks) == 0 {
		body.WriteString("- 暂无已确认上下文。\n")
	}
	blockRefs := []string{}
	for _, block := range blocks {
		fmt.Fprintf(&body, "- **%s**：%s\n", block.Label, strings.TrimSpace(block.Content))
		blockRefs = append(blockRefs, block.SourceRefs...)
	}
	body.WriteString("\n## 目标与里程碑\n")
	goalRefs := []string{}
	for _, goal := range goals {
		fmt.Fprintf(&body, "- [%s] %s", goal.Status, goal.Title)
		if goal.TargetAt != "" {
			fmt.Fprintf(&body, " · %s", goal.TargetAt)
		}
		body.WriteString("\n")
		if goal.SourceEvidenceID != "" {
			goalRefs = append(goalRefs, goal.SourceEvidenceID)
		}
	}
	for _, milestone := range milestones {
		fmt.Fprintf(&body, "  - [%s] %s · %s\n", milestone.Status, milestone.Title, milestone.DueAt)
	}
	body.WriteString("\n## 时间状态\n")
	timeCounts := map[string]int{}
	for _, event := range timeline.Events {
		timeCounts[event.TemporalRelation]++
	}
	fmt.Fprintf(&body, "- 当前有效 %d · 即将发生 %d · 已逾期 %d\n", timeCounts["active"], timeCounts["upcoming"], timeCounts["overdue"])
	fmt.Fprintf(&body, "- 待确认相对时间 %d 条\n", len(pendingTime))
	summary = fmt.Sprintf("%d 个目标 · %d 个里程碑 · %d 项决策 · %d 个风险 · %d 项逾期 · %d 条时间待确认", len(goals), len(milestones), len(decisions), len(risks), timeCounts["overdue"], len(pendingTime))
	for i, line := range pendingTime {
		if i >= 6 {
			body.WriteString("- … 其余待确认时间请在时间脉络中处理。\n")
			break
		}
		body.WriteString(line + "\n")
	}
	body.WriteString("\n## 项目演化（按时间分段，不代表因果）\n")
	monthEvents := map[string][]portfolio.TemporalEvent{}
	monthKeys := []string{}
	for _, event := range timeline.Events {
		key := event.AnchorAt
		if len(key) >= 7 {
			key = key[:7]
		}
		if _, exists := monthEvents[key]; !exists {
			monthKeys = append(monthKeys, key)
		}
		monthEvents[key] = append(monthEvents[key], event)
	}
	sort.Strings(monthKeys)
	if len(monthKeys) > 6 {
		monthKeys = monthKeys[len(monthKeys)-6:]
	}
	if len(monthKeys) == 0 {
		body.WriteString("- 暂无可分段的时间事件。\n")
	}
	for _, key := range monthKeys {
		items := monthEvents[key]
		counts := map[string]int{}
		for _, event := range items {
			counts[event.Kind]++
		}
		parts := []string{}
		for _, kind := range []string{"fact", "goal", "milestone", "decision"} {
			if counts[kind] > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", kind, counts[kind]))
			}
		}
		titles := []string{}
		for i, event := range items {
			if i >= 3 {
				break
			}
			titles = append(titles, event.Title)
		}
		fmt.Fprintf(&body, "- **%s**：%s；%s\n", key, strings.Join(parts, " · "), strings.Join(titles, " / "))
	}
	recent := append([]portfolio.TemporalEvent(nil), timeline.Events...)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].AnchorAt > recent[j].AnchorAt })
	body.WriteString("\n## 最近变化\n")
	if len(recent) == 0 {
		body.WriteString("- 暂无已解析的项目时间事件。\n")
	}
	for i, event := range recent {
		if i >= 6 {
			break
		}
		when := event.AnchorAt
		if len(when) >= 10 {
			when = when[:10]
		}
		fmt.Fprintf(&body, "- %s · [%s/%s] %s\n", when, event.Kind, event.TemporalRelation, event.Title)
	}

	body.WriteString("\n## 维护提醒（Freshness 规则信号，不代表内容错误）\n")
	if len(staleGoals)+len(staleRisks)+len(staleMemories) == 0 {
		body.WriteString("- 当前没有命中长期未更新规则的核心记录。\n")
	}
	for i, title := range staleGoals {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&body, "- 目标超过 30 天未更新：%s\n", title)
	}
	for i, title := range staleRisks {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&body, "- 风险超过 30 天未更新：%s\n", title)
	}
	for i, title := range staleMemories {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&body, "- 长期记忆超过 90 天未强化：%s\n", title)
	}

	body.WriteString("\n## 关键决策\n")
	decisionRefs := []string{}
	for _, decision := range decisions {
		fmt.Fprintf(&body, "- **%s**：%s\n", decision.Title, decision.Decision)
		decisionRefs = append(decisionRefs, decision.SourceEvidenceIDs...)
	}
	body.WriteString("\n## 风险\n")
	riskRefs := []string{}
	for _, risk := range risks {
		fmt.Fprintf(&body, "- [%s · %d/25] **%s**：%s", risk.Status, risk.Score, risk.Title, risk.Description)
		if risk.Mitigation != "" {
			fmt.Fprintf(&body, "；应对：%s", risk.Mitigation)
		}
		body.WriteString("\n")
		if risk.SourceEvidenceID != "" {
			riskRefs = append(riskRefs, risk.SourceEvidenceID)
		}
	}
	timelineRefs := []string{}
	for _, event := range timeline.Events {
		timelineRefs = append(timelineRefs, event.SourceEvidenceIDs...)
	}
	refs := productRefs(blockRefs, goalRefs, decisionRefs, riskRefs, pendingRefs, timelineRefs)
	body.WriteString("\n## 来源覆盖\n")
	fmt.Fprintf(&body, "- 直接关联 %d 条唯一 Evidence；当前项目有 %d 条 Knowledge Unit、%d 条 Memory。\n", len(refs), len(units), len(memories))
	body.WriteString("- Object Revision 还会保存 source_object_ids 与内容哈希；本节只统计直接 Evidence 引用。\n")
	payload := map[string]any{
		"product_id": productID, "product_type": "project_brief", "title": title, "summary": summary,
		"body": body.String(), "format": "markdown", "source_refs": refs, "locked_fields": []string{}, "generation_status": "auto",
	}
	if current, currentErr := s.harness.Object(ctx, objectID); currentErr == nil {
		if preserveLocks {
			var prior map[string]any
			bodyFieldLocked := false
			if json.Unmarshal(current.Revision.Payload, &prior) == nil {
				locked := lockedFieldSet(prior)
				bodyFieldLocked = locked["body"]
				lockedFields := []string{}
				for _, field := range []string{"title", "summary", "body"} {
					if locked[field] {
						payload[field] = prior[field]
						lockedFields = append(lockedFields, field)
					}
				}
				payload["locked_fields"] = lockedFields
				if len(lockedFields) > 0 {
					payload["generation_status"] = "human_mixed"
				}
			}
			if candidateBody, ok := payload["body"].(string); ok && !bodyFieldLocked {
				preview, mergeErr := s.mergeLockedProductBody(ctx, current, candidateBody)
				if mergeErr != nil {
					return "", "", nil, nil, mergeErr
				}
				payload["body"] = preview.MergedBody
				for _, block := range preview.Blocks {
					if block.Locked {
						payload["generation_status"] = "human_mixed"
						break
					}
				}
			}
		}
	} else if currentErr != sql.ErrNoRows {
		return "", "", nil, nil, currentErr
	}
	raw, err := json.Marshal(payload)
	return productID, objectID, refs, raw, err
}

func (s *Service) projectProductSourceObjects(ctx context.Context, projectID, excludeObjectID string) ([]string, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT object_id FROM harness_objects WHERE project_id=? AND object_id<>? ORDER BY updated_at DESC,object_id LIMIT 200`, projectID, excludeObjectID)
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

func (s *Service) projectBriefObject(ctx context.Context, projectID, runID, spanID string) (harness.Object, error) {
	productID, objectID, refs, raw, err := s.projectBriefPayload(ctx, projectID, true)
	if err != nil {
		return harness.Object{}, err
	}
	sourceObjects, err := s.projectProductSourceObjects(ctx, projectID, objectID)
	if err != nil {
		return harness.Object{}, err
	}
	revisionKey := stableID("product-revision-", string(raw))
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: objectID, TypeID: harness.KnowledgeProductTypeV1, ProjectID: projectID, Status: "active",
		Payload: raw, Confidence: 1, Importance: .85, SourceEvidenceIDs: refs, SourceObjectIDs: sourceObjects, RunID: runID, StageID: spanID,
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0",
		IdempotencyKey: projectID + ":" + productID + ":" + revisionKey,
	})
}

func (s *Service) materializeProjectBrief(ctx context.Context, projectID, runID, spanID string) (bool, error) {
	object, err := s.projectBriefObject(ctx, projectID, runID, spanID)
	return !object.Duplicate, err
}

// RefreshProjectBrief is a safe backfill for projects created before Knowledge
// Products existed. It reads current governed projections only; it does not
// re-extract or rewrite canonical Evidence. Locked human fields are preserved.
func (s *Service) RefreshProjectBrief(ctx context.Context, projectID string) (harness.Object, error) {
	object, err := s.projectBriefObject(ctx, strings.TrimSpace(projectID), "", "")
	if err != nil {
		return harness.Object{}, err
	}
	if _, err := s.portfolio.RebuildProjectIndex(ctx, object.ProjectID); err != nil {
		return harness.Object{}, err
	}
	return object, nil
}

// ReconcileProjectKnowledgeProducts rebuilds disposable Living projections and
// safely backfills the Project Brief for an existing project. Automatic work
// obeys the active Blueprint; an Owner can still call RefreshProjectBrief
// explicitly when they want a one-off refresh.
func (s *Service) ReconcileProjectKnowledgeProducts(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project_id is required")
	}
	snapshot, err := s.blueprints.Snapshot(ctx, projectID)
	if err != nil {
		return err
	}
	policy := growthPolicyFromSnapshot(snapshot)
	if !policy.roleEnabled("growth.living") {
		return s.memory.ClearLivingViewsForProject(ctx, projectID)
	}
	if _, err := s.memory.RebuildLivingViewsForProject(ctx, projectID); err != nil {
		return err
	}
	var hasData int
	err = s.control.DB.QueryRowContext(ctx, `SELECT CASE WHEN
		EXISTS(SELECT 1 FROM record_projects WHERE project_id=? AND record_type IN ('evidence','episode','knowledge_unit','memory')) OR
		EXISTS(SELECT 1 FROM project_goals WHERE project_id=?) OR EXISTS(SELECT 1 FROM project_milestones WHERE project_id=?) OR
		EXISTS(SELECT 1 FROM project_decisions WHERE project_id=?) OR EXISTS(SELECT 1 FROM project_risks WHERE project_id=?) OR
		EXISTS(SELECT 1 FROM context_blocks WHERE project_id=?) OR EXISTS(SELECT 1 FROM temporal_facts WHERE project_id=?)
		THEN 1 ELSE 0 END`, projectID, projectID, projectID, projectID, projectID, projectID, projectID).Scan(&hasData)
	if err != nil {
		return err
	}
	if hasData == 0 {
		return nil
	}
	_, err = s.projectBriefObject(ctx, projectID, "", "")
	return err
}
