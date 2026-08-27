package portfolio

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

type TimelineQuery struct {
	ProjectID string
	AnchorAt  string
	From      string
	Until     string
	Kinds     []string
	Limit     int
}

type TemporalEvent struct {
	EventID           string   `json:"event_id"`
	ProjectID         string   `json:"project_id"`
	Kind              string   `json:"kind"`
	Title             string   `json:"title"`
	Detail            string   `json:"detail,omitempty"`
	Status            string   `json:"status"`
	AnchorAt          string   `json:"anchor_at"`
	StartAt           string   `json:"start_at,omitempty"`
	EndAt             string   `json:"end_at,omitempty"`
	ObservedAt        string   `json:"observed_at,omitempty"`
	RecordedAt        string   `json:"recorded_at,omitempty"`
	TemporalRelation  string   `json:"temporal_relation"`
	TemporalRelevance float64  `json:"temporal_relevance"`
	SourceID          string   `json:"source_id"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
}

type TemporalRelation struct {
	FromID       string  `json:"from_id"`
	ToID         string  `json:"to_id"`
	Kind         string  `json:"kind"`
	DeltaSeconds float64 `json:"delta_seconds,omitempty"`
}

type TemporalCorrelation struct {
	LeftID       string   `json:"left_id"`
	RightID      string   `json:"right_id"`
	Kind         string   `json:"kind"`
	Strength     float64  `json:"strength"`
	DeltaSeconds float64  `json:"delta_seconds,omitempty"`
	Reasons      []string `json:"reasons"`
}

type Timeline struct {
	ProjectID    string                `json:"project_id"`
	AnchorAt     string                `json:"anchor_at"`
	From         string                `json:"from,omitempty"`
	Until        string                `json:"until,omitempty"`
	Events       []TemporalEvent       `json:"events"`
	Relations    []TemporalRelation    `json:"relations"`
	Correlations []TemporalCorrelation `json:"correlations"`
	Counts       map[string]int        `json:"counts"`
}

func parseTimelineTime(value string) (time.Time, bool) {
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

func temporalRelation(anchor, start time.Time, end time.Time, hasEnd bool, kind, status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if start.After(anchor) {
		return "upcoming"
	}
	if (kind == "goal" || kind == "milestone") && (status == "active" || status == "open" || status == "pending" || status == "planned" || status == "in_progress") {
		return "overdue"
	}
	if hasEnd && !end.After(anchor) {
		return "historical"
	}
	if hasEnd && !start.After(anchor) && end.After(anchor) {
		return "active"
	}
	if !hasEnd && !start.After(anchor) && (kind == "fact" || status == "active" || status == "running" || status == "open" || status == "in_progress") {
		return "active"
	}
	if strings.Contains(status, "complete") || strings.Contains(status, "closed") {
		return "completed"
	}
	return "past"
}

func temporalRelevance(anchor, start time.Time, end time.Time, hasEnd bool, kind, status string) float64 {
	relation := temporalRelation(anchor, start, end, hasEnd, kind, status)
	if relation == "active" {
		return 1
	}
	days := math.Abs(anchor.Sub(start).Hours()) / 24
	halfLife := 45.0
	if kind == "goal" || kind == "milestone" {
		halfLife = 21
	}
	if kind == "run" || kind == "episode" {
		halfLife = 14
	}
	if kind == "fact" {
		halfLife = 60
	}
	score := math.Exp(-math.Ln2 * days / halfLife)
	if relation == "upcoming" && (kind == "goal" || kind == "milestone") {
		score = math.Min(1, score*1.2)
	}
	if status == "active" || status == "open" || status == "in_progress" {
		score = math.Min(1, score*1.08)
	}
	return math.Round(score*10000) / 10000
}

func sharedTemporalEvidence(left, right []string) bool {
	seen := map[string]bool{}
	for _, value := range left {
		if strings.TrimSpace(value) != "" {
			seen[value] = true
		}
	}
	for _, value := range right {
		if seen[value] {
			return true
		}
	}
	return false
}

func eventInterval(event TemporalEvent) (time.Time, time.Time, bool) {
	start, ok := parseTimelineTime(event.StartAt)
	if !ok {
		start, ok = parseTimelineTime(event.AnchorAt)
	}
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, hasEnd := parseTimelineTime(event.EndAt)
	if !hasEnd || end.Before(start) {
		end = start
	}
	return start, end, true
}

func correlationForPair(left, right TemporalEvent) (TemporalCorrelation, bool) {
	leftStart, leftEnd, leftOK := eventInterval(left)
	rightStart, rightEnd, rightOK := eventInterval(right)
	if !leftOK || !rightOK {
		return TemporalCorrelation{}, false
	}
	kind := "near_in_time"
	reasons := []string{"timestamp_proximity"}
	gap := math.Abs(leftStart.Sub(rightStart).Seconds())
	strength := math.Exp(-math.Ln2 * gap / (7 * 24 * 3600))
	if !leftEnd.Before(rightStart) && !rightEnd.Before(leftStart) {
		intersectionStart := leftStart
		if rightStart.After(intersectionStart) {
			intersectionStart = rightStart
		}
		intersectionEnd := leftEnd
		if rightEnd.Before(intersectionEnd) {
			intersectionEnd = rightEnd
		}
		unionStart := leftStart
		if rightStart.Before(unionStart) {
			unionStart = rightStart
		}
		unionEnd := leftEnd
		if rightEnd.After(unionEnd) {
			unionEnd = rightEnd
		}
		intersection := math.Max(0, intersectionEnd.Sub(intersectionStart).Seconds())
		union := math.Max(1, unionEnd.Sub(unionStart).Seconds())
		kind = "overlaps"
		if (leftStart.Before(rightStart) || leftStart.Equal(rightStart)) && (leftEnd.After(rightEnd) || leftEnd.Equal(rightEnd)) && leftEnd.After(leftStart) {
			kind = "contains"
		} else if (rightStart.Before(leftStart) || rightStart.Equal(leftStart)) && (rightEnd.After(leftEnd) || rightEnd.Equal(leftEnd)) && rightEnd.After(rightStart) {
			kind = "during"
		}
		strength = math.Max(strength, .82+.18*(intersection/union))
		reasons = []string{"time_interval_overlap"}
	} else {
		if leftEnd.Before(rightStart) {
			gap = rightStart.Sub(leftEnd).Seconds()
		}
		if rightEnd.Before(leftStart) {
			gap = leftStart.Sub(rightEnd).Seconds()
		}
		strength = math.Exp(-math.Ln2 * gap / (7 * 24 * 3600))
	}
	if left.Kind == right.Kind {
		strength += .04
		reasons = append(reasons, "same_kind")
	}
	if sharedTemporalEvidence(left.SourceEvidenceIDs, right.SourceEvidenceIDs) {
		strength += .16
		reasons = append(reasons, "shared_evidence")
	}
	if (left.Kind == "goal" || left.Kind == "milestone") && (right.Kind == "goal" || right.Kind == "milestone") {
		strength += .06
		reasons = append(reasons, "task_time_context")
	}
	strength = math.Min(1, strength)
	if strength < .35 || gap > 30*24*3600 {
		return TemporalCorrelation{}, false
	}
	return TemporalCorrelation{LeftID: left.EventID, RightID: right.EventID, Kind: kind, Strength: math.Round(strength*10000) / 10000, DeltaSeconds: gap, Reasons: reasons}, true
}

func buildTemporalCorrelations(events []TemporalEvent) []TemporalCorrelation {
	bounded := events
	if len(bounded) > 200 {
		bounded = bounded[:200]
	}
	out := []TemporalCorrelation{}
	for i := 0; i < len(bounded); i++ {
		for j := i + 1; j < len(bounded); j++ {
			if correlation, ok := correlationForPair(bounded[i], bounded[j]); ok {
				out = append(out, correlation)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Strength == out[j].Strength {
			return out[i].DeltaSeconds < out[j].DeltaSeconds
		}
		return out[i].Strength > out[j].Strength
	})
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

type temporalRow struct {
	id, kind, title, detail, status, anchorAt, startAt, endAt, observedAt, recordedAt, sourceID, evidenceJSON, supersedesID string
}

func (s *Service) Timeline(ctx context.Context, input TimelineQuery) (Timeline, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	if input.ProjectID == "" {
		return Timeline{}, errors.New("project_id is required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Timeline{}, err
	}
	anchor := time.Now().UTC()
	if strings.TrimSpace(input.AnchorAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.AnchorAt))
		if err != nil {
			return Timeline{}, errors.New("invalid anchor_at")
		}
		anchor = parsed.UTC()
	}
	from, hasFrom := parseTimelineTime(input.From)
	until, hasUntil := parseTimelineTime(input.Until)
	if hasFrom && hasUntil && !until.After(from) {
		return Timeline{}, errors.New("until must be after from")
	}
	if input.Limit <= 0 || input.Limit > 500 {
		input.Limit = 200
	}
	allowed := map[string]bool{"fact": true, "goal": true, "milestone": true, "decision": true, "episode": true, "memory": true, "run": true, "finance": true, "evidence": true}
	kindFilter := map[string]bool{}
	for _, kind := range input.Kinds {
		kind = strings.TrimSpace(kind)
		if !allowed[kind] {
			return Timeline{}, errors.New("unsupported timeline kind: " + kind)
		}
		kindFilter[kind] = true
	}
	queries := []string{
		`SELECT fact_id,'fact',subject||' · '||predicate,object,status,valid_from,valid_from,coalesce(valid_until,''),coalesce(observed_at,''),recorded_at,fact_id,source_evidence_ids_json,coalesce(supersedes_fact_id,'') FROM temporal_facts WHERE project_id=?`,
		`SELECT goal_id,'goal',title,description,status,coalesce(target_at,updated_at),coalesce(target_at,updated_at),'',created_at,updated_at,goal_id,CASE WHEN source_evidence_id IS NULL THEN '[]' ELSE json_array(source_evidence_id) END,'' FROM project_goals WHERE project_id=?`,
		`SELECT milestone_id,'milestone',title,coalesce(goal_id,''),status,coalesce(due_at,completed_at,updated_at),coalesce(due_at,created_at),coalesce(completed_at,''),created_at,updated_at,milestone_id,'[]','' FROM project_milestones WHERE project_id=?`,
		`SELECT decision_id,'decision',title,decision,status,decided_at,decided_at,'',decided_at,created_at,decision_id,source_evidence_ids_json,'' FROM project_decisions WHERE project_id=?`,
		`SELECT e.episode_id,'episode',e.title,e.summary,e.status,e.ended_at,e.started_at,e.ended_at,e.started_at,e.updated_at,e.episode_id,e.evidence_ids_json,'' FROM episodes e JOIN record_projects rp ON rp.record_type='episode' AND rp.record_id=e.episode_id WHERE rp.project_id=?`,
		`SELECT m.memory_id,'memory',m.summary,m.body,m.status,m.observed_at,m.observed_at,'',m.observed_at,m.updated_at,m.memory_id,m.evidence_ids_json,'' FROM memory_records m JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=m.memory_id WHERE rp.project_id=?`,
		`SELECT run_id,'run',pipeline_id,channel,status,coalesce(started_at,created_at),coalesce(started_at,created_at),coalesce(ended_at,''),created_at,created_at,run_id,'[]','' FROM harness_runs WHERE project_id=?`,
		`SELECT entry_id,'finance',description,category,status,occurred_at,occurred_at,'',occurred_at,created_at,entry_id,CASE WHEN source_evidence_id IS NULL THEN '[]' ELSE json_array(source_evidence_id) END,'' FROM finance_entries WHERE project_id=?`,
		`SELECT r.evidence_id,'evidence',r.source_system,r.session_id,'captured',r.observed_at,r.observed_at,'',r.observed_at,r.captured_at,r.evidence_id,json_array(r.evidence_id),'' FROM evidence_receipts r JOIN record_projects rp ON rp.record_type='evidence' AND rp.record_id=r.evidence_id WHERE rp.project_id=?`,
	}
	rowsOut := []temporalRow{}
	for _, query := range queries {
		rows, err := s.control.DB.QueryContext(ctx, query, input.ProjectID)
		if err != nil {
			return Timeline{}, err
		}
		for rows.Next() {
			var row temporalRow
			if err := rows.Scan(&row.id, &row.kind, &row.title, &row.detail, &row.status, &row.anchorAt, &row.startAt, &row.endAt, &row.observedAt, &row.recordedAt, &row.sourceID, &row.evidenceJSON, &row.supersedesID); err != nil {
				rows.Close()
				return Timeline{}, err
			}
			rowsOut = append(rowsOut, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return Timeline{}, err
		}
		rows.Close()
	}
	events := make([]TemporalEvent, 0, len(rowsOut))
	supersedes := map[string]string{}
	for _, row := range rowsOut {
		if len(kindFilter) > 0 && !kindFilter[row.kind] {
			continue
		}
		at, ok := parseTimelineTime(row.anchorAt)
		if !ok {
			continue
		}
		if hasFrom && at.Before(from) {
			continue
		}
		if hasUntil && !at.Before(until) {
			continue
		}
		start, ok := parseTimelineTime(row.startAt)
		if !ok {
			start = at
		}
		end, hasEnd := parseTimelineTime(row.endAt)
		eventID := row.kind + ":" + row.id
		events = append(events, TemporalEvent{EventID: eventID, ProjectID: input.ProjectID, Kind: row.kind, Title: row.title, Detail: row.detail, Status: row.status, AnchorAt: at.UTC().Format(time.RFC3339Nano), StartAt: start.UTC().Format(time.RFC3339Nano), EndAt: row.endAt, ObservedAt: row.observedAt, RecordedAt: row.recordedAt, TemporalRelation: temporalRelation(anchor, start, end, hasEnd, row.kind, row.status), TemporalRelevance: temporalRelevance(anchor, start, end, hasEnd, row.kind, row.status), SourceID: row.sourceID, SourceEvidenceIDs: decodeStrings(row.evidenceJSON)})
		if row.kind == "fact" && row.supersedesID != "" {
			supersedes[eventID] = "fact:" + row.supersedesID
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].TemporalRelevance == events[j].TemporalRelevance {
			return events[i].AnchorAt > events[j].AnchorAt
		}
		return events[i].TemporalRelevance > events[j].TemporalRelevance
	})
	if len(events) > input.Limit {
		events = events[:input.Limit]
	}
	chronological := append([]TemporalEvent(nil), events...)
	sort.SliceStable(chronological, func(i, j int) bool { return chronological[i].AnchorAt < chronological[j].AnchorAt })
	relations := []TemporalRelation{}
	for i := 1; i < len(chronological); i++ {
		a, _ := parseTimelineTime(chronological[i-1].AnchorAt)
		b, _ := parseTimelineTime(chronological[i].AnchorAt)
		relations = append(relations, TemporalRelation{FromID: chronological[i-1].EventID, ToID: chronological[i].EventID, Kind: "precedes", DeltaSeconds: b.Sub(a).Seconds()})
	}
	included := map[string]bool{}
	for _, event := range events {
		included[event.EventID] = true
	}
	for fromID, toID := range supersedes {
		if included[fromID] && included[toID] {
			relations = append(relations, TemporalRelation{FromID: fromID, ToID: toID, Kind: "supersedes"})
		}
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Kind]++
	}
	correlations := buildTemporalCorrelations(events)
	return Timeline{ProjectID: input.ProjectID, AnchorAt: anchor.Format(time.RFC3339Nano), From: input.From, Until: input.Until, Events: events, Relations: relations, Correlations: correlations, Counts: counts}, nil
}
