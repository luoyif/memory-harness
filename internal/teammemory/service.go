package teammemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

type Service struct {
	harness   *harness.Service
	agents    *agentauth.Service
	portfolio *portfolio.Service
}

func New(h *harness.Service, agents *agentauth.Service, p *portfolio.Service) *Service {
	return &Service{harness: h, agents: agents, portfolio: p}
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func stableID(prefix string, values ...string) string {
	return prefix + contracts.HashBytes([]byte(strings.Join(values, "\x00")))[:24]
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validEpistemic(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "observed", "inferred", "hypothesis", "disputed":
		return true
	default:
		return false
	}
}

func expiry(ttl int64) (string, error) {
	if ttl < 60 || ttl > 7*24*3600 {
		return "", errors.New("ttl_seconds must be between 60 and 604800")
	}
	return time.Now().UTC().Add(time.Duration(ttl) * time.Second).Format(time.RFC3339Nano), nil
}

func activeAt(expiresAt string, at time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	return err == nil && at.UTC().Before(t)
}

func decodeTask(object harness.Object) (Task, error) {
	var value Task
	if object.TypeID != TaskTypeV1 {
		return value, errors.New("object is not a Team Task")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}
func decodePrivate(object harness.Object) (PrivateScratch, error) {
	var value PrivateScratch
	if object.TypeID != PrivateScratchTypeV1 {
		return value, errors.New("object is not Private Scratchpad")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}
func decodeBlackboard(object harness.Object) (BlackboardEntry, error) {
	var value BlackboardEntry
	if object.TypeID != BlackboardEntryTypeV1 {
		return value, errors.New("object is not Blackboard Entry")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}
func decodeConflict(object harness.Object) (Conflict, error) {
	var value Conflict
	if object.TypeID != ConflictTypeV1 {
		return value, errors.New("object is not Team Conflict")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}
func decodeDurable(object harness.Object) (ProjectDurable, error) {
	var value ProjectDurable
	if object.TypeID != ProjectDurableTypeV1 {
		return value, errors.New("object is not Project Durable Team Memory")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func hasMember(task Task, agentID string) bool {
	for _, id := range task.MemberAgentIDs {
		if id == agentID {
			return true
		}
	}
	return false
}

func (s *Service) Task(ctx context.Context, taskID string) (harness.Object, Task, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return harness.Object{}, Task{}, err
	}
	value, err := decodeTask(object)
	return object, value, err
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (harness.Object, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Title = strings.TrimSpace(input.Title)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.MemberAgentIDs = unique(input.MemberAgentIDs)
	if input.ProjectID == "" || input.Title == "" || input.IdempotencyKey == "" || len(input.MemberAgentIDs) == 0 {
		return harness.Object{}, errors.New("project_id, title, member_agent_ids and idempotency_key are required")
	}
	if _, err := s.portfolio.Project(ctx, input.ProjectID); err != nil {
		return harness.Object{}, err
	}
	expiresAt, err := expiry(input.TTLSeconds)
	if err != nil {
		return harness.Object{}, err
	}
	for _, agentID := range input.MemberAgentIDs {
		principal, err := s.agents.Get(ctx, agentID)
		if err != nil {
			return harness.Object{}, fmt.Errorf("member %s: %w", agentID, err)
		}
		if principal.Status != "active" || !agentauth.CanAccessProject(principal, input.ProjectID) {
			return harness.Object{}, fmt.Errorf("member %s is not active/project-granted", agentID)
		}
	}
	taskID := stableID("team-task-", input.ProjectID, input.IdempotencyKey)
	value := Task{TaskID: taskID, ProjectID: input.ProjectID, Title: input.Title, MemberAgentIDs: input.MemberAgentIDs, Status: "active", CreatedAt: nowString(), ExpiresAt: expiresAt}
	raw, _ := json.Marshal(value)
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: taskID, TypeID: TaskTypeV1, ProjectID: input.ProjectID, Status: "active", Payload: raw,
		Confidence: 1, Importance: .7, ValidUntil: expiresAt,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-task:" + input.IdempotencyKey,
	})
}

func (s *Service) ListTasks(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), TaskTypeV1, strings.TrimSpace(status), limit)
}

func (s *Service) requireTaskAgent(ctx context.Context, principal agentauth.Principal, taskID, permission string, at time.Time) (harness.Object, Task, error) {
	object, task, err := s.Task(ctx, taskID)
	if err != nil {
		return harness.Object{}, Task{}, err
	}
	if !agentauth.HasPermission(principal, permission) {
		return harness.Object{}, Task{}, fmt.Errorf("Agent lacks %s", permission)
	}
	if !agentauth.CanAccessProject(principal, task.ProjectID) || !hasMember(task, principal.AgentID) {
		return harness.Object{}, Task{}, errors.New("Agent is not a direct member of this task/project")
	}
	if object.Status != "active" || task.Status != "active" || !activeAt(task.ExpiresAt, at) {
		return harness.Object{}, Task{}, errors.New("team task is closed or expired")
	}
	return object, task, nil
}
func boundedExpiry(ttl int64, taskExpiresAt string) (string, error) {
	expiresAt, err := expiry(ttl)
	if err != nil {
		return "", err
	}
	child, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "", err
	}
	parent, err := time.Parse(time.RFC3339Nano, taskExpiresAt)
	if err != nil {
		return "", errors.New("task expiry is invalid")
	}
	if child.After(parent) {
		return parent.Format(time.RFC3339Nano), nil
	}
	return child.Format(time.RFC3339Nano), nil
}

func (s *Service) validateContributionSource(ctx context.Context, principal agentauth.Principal, task Task, input ContributionInput) error {
	if input.Confidence < 0 || input.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if !validEpistemic(input.EpistemicStatus) {
		return errors.New("epistemic_status must be observed, inferred, hypothesis or disputed")
	}
	for _, evidenceID := range unique(input.SourceEvidenceIDs) {
		allowed, err := s.agents.CanAccessRecord(ctx, principal, "evidence", evidenceID)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("source Evidence %s is outside Agent scope", evidenceID)
		}
	}
	if strings.TrimSpace(input.RunID) != "" {
		run, err := s.harness.Run(ctx, input.RunID)
		if err != nil {
			return err
		}
		if run.ProjectID != task.ProjectID || run.CallerID != principal.AgentID {
			return errors.New("contribution run must belong to the same task project and Agent")
		}
	}
	return nil
}

func contributionMeta(principal agentauth.Principal, input ContributionInput, expiresAt string) ContributionMeta {
	return ContributionMeta{
		AgentID: principal.AgentID, RunID: strings.TrimSpace(input.RunID),
		SourceEvidenceIDs: unique(input.SourceEvidenceIDs), Confidence: input.Confidence,
		EpistemicStatus: strings.ToLower(strings.TrimSpace(input.EpistemicStatus)), CreatedAt: nowString(), ExpiresAt: expiresAt,
	}
}

func (s *Service) WritePrivate(ctx context.Context, principal agentauth.Principal, taskID string, input ContributionInput) (harness.Object, error) {
	_, task, err := s.requireTaskAgent(ctx, principal, taskID, agentauth.PermissionTeamPrivate, time.Now().UTC())
	if err != nil {
		return harness.Object{}, err
	}
	input.Content = strings.TrimSpace(input.Content)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Content == "" || input.IdempotencyKey == "" {
		return harness.Object{}, errors.New("content and idempotency_key are required")
	}
	if err := s.validateContributionSource(ctx, principal, task, input); err != nil {
		return harness.Object{}, err
	}
	expiresAt, err := boundedExpiry(input.TTLSeconds, task.ExpiresAt)
	if err != nil {
		return harness.Object{}, err
	}
	entryID := stableID("team-private-", task.TaskID, principal.AgentID, input.IdempotencyKey)
	value := PrivateScratch{EntryID: entryID, TaskID: task.TaskID, ProjectID: task.ProjectID, Content: input.Content, Meta: contributionMeta(principal, input, expiresAt)}
	raw, _ := json.Marshal(value)
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: entryID, TypeID: PrivateScratchTypeV1, ProjectID: task.ProjectID, Status: "active", Payload: raw,
		Confidence: max(.01, input.Confidence), Importance: .25, ValidUntil: expiresAt,
		SourceEvidenceIDs: value.Meta.SourceEvidenceIDs, RunID: value.Meta.RunID, StageID: "team.private",
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-private:" + task.TaskID + ":" + principal.AgentID + ":" + input.IdempotencyKey,
	})
}

func (s *Service) PrivateForAgentAt(ctx context.Context, principal agentauth.Principal, taskID string, at time.Time) ([]harness.Object, error) {
	_, task, err := s.requireTaskAgent(ctx, principal, taskID, agentauth.PermissionTeamPrivate, at)
	if err != nil {
		return nil, err
	}
	items, err := s.harness.ListObjects(ctx, task.ProjectID, PrivateScratchTypeV1, "active", 500)
	if err != nil {
		return nil, err
	}
	out := []harness.Object{}
	for _, object := range items {
		value, err := decodePrivate(object)
		if err != nil {
			return nil, err
		}
		if value.TaskID == task.TaskID && value.Meta.AgentID == principal.AgentID && activeAt(value.Meta.ExpiresAt, at) {
			out = append(out, object)
		}
	}
	return out, nil
}
func directRecipients(task Task, author string, values []string) ([]string, error) {
	values = unique(values)
	out := []string{}
	for _, id := range values {
		if id == author {
			continue
		}
		if !hasMember(task, id) {
			return nil, fmt.Errorf("direct share target %s is not a task member", id)
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Service) WriteBlackboard(ctx context.Context, principal agentauth.Principal, taskID string, input BlackboardInput) (BlackboardResult, error) {
	_, task, err := s.requireTaskAgent(ctx, principal, taskID, agentauth.PermissionTeamBlackboardWrite, time.Now().UTC())
	if err != nil {
		return BlackboardResult{}, err
	}
	input.Content = strings.TrimSpace(input.Content)
	input.Topic = strings.TrimSpace(input.Topic)
	input.ClaimKey = strings.TrimSpace(input.ClaimKey)
	input.ClaimValue = strings.TrimSpace(input.ClaimValue)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Content == "" || input.Topic == "" || input.ClaimKey == "" || input.ClaimValue == "" || input.IdempotencyKey == "" {
		return BlackboardResult{}, errors.New("content, topic, claim_key, claim_value and idempotency_key are required")
	}
	if err := s.validateContributionSource(ctx, principal, task, input.ContributionInput); err != nil {
		return BlackboardResult{}, err
	}
	expiresAt, err := boundedExpiry(input.TTLSeconds, task.ExpiresAt)
	if err != nil {
		return BlackboardResult{}, err
	}
	recipients, err := directRecipients(task, principal.AgentID, input.DirectShareAgentIDs)
	if err != nil {
		return BlackboardResult{}, err
	}
	if len(recipients) > 0 && !agentauth.HasPermission(principal, agentauth.PermissionTeamBlackboardShare) {
		return BlackboardResult{}, errors.New("Agent lacks team.blackboard.share for direct recipients")
	}
	entryID := stableID("team-blackboard-", task.TaskID, principal.AgentID, input.IdempotencyKey)
	value := BlackboardEntry{
		EntryID: entryID, TaskID: task.TaskID, ProjectID: task.ProjectID, Topic: input.Topic,
		ClaimKey: input.ClaimKey, ClaimValue: input.ClaimValue, Content: input.Content,
		DirectShareAgentIDs: recipients, Meta: contributionMeta(principal, input.ContributionInput, expiresAt),
	}
	raw, _ := json.Marshal(value)
	_, err = s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: entryID, TypeID: BlackboardEntryTypeV1, ProjectID: task.ProjectID, Status: "active", Payload: raw,
		Confidence: max(.01, input.Confidence), Importance: .45, ValidUntil: expiresAt,
		SourceEvidenceIDs: value.Meta.SourceEvidenceIDs, RunID: value.Meta.RunID, StageID: "team.blackboard",
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-blackboard:" + task.TaskID + ":" + principal.AgentID + ":" + input.IdempotencyKey,
	})
	if err != nil {
		return BlackboardResult{}, err
	}
	conflicts, err := s.detectConflicts(ctx, task, value)
	if err != nil {
		return BlackboardResult{}, err
	}
	return BlackboardResult{Entry: value, Conflicts: conflicts}, nil
}
func (s *Service) detectConflicts(ctx context.Context, task Task, current BlackboardEntry) ([]ConflictResult, error) {
	items, err := s.harness.ListObjects(ctx, task.ProjectID, BlackboardEntryTypeV1, "active", 500)
	if err != nil {
		return nil, err
	}
	entryIDs := []string{current.EntryID}
	agentIDs := []string{current.Meta.AgentID}
	for _, object := range items {
		value, err := decodeBlackboard(object)
		if err != nil {
			return nil, err
		}
		if value.EntryID == current.EntryID || value.TaskID != task.TaskID || value.ClaimKey != current.ClaimKey || value.ClaimValue == current.ClaimValue {
			continue
		}
		if value.Meta.AgentID == current.Meta.AgentID || !activeAt(value.Meta.ExpiresAt, time.Now().UTC()) {
			continue
		}
		entryIDs = append(entryIDs, value.EntryID)
		agentIDs = append(agentIDs, value.Meta.AgentID)
	}
	entryIDs, agentIDs = unique(entryIDs), unique(agentIDs)
	if len(entryIDs) < 2 || len(agentIDs) < 2 {
		return []ConflictResult{}, nil
	}
	conflictID := stableID("team-conflict-", task.TaskID, current.ClaimKey, strings.Join(entryIDs, "|"))
	value := Conflict{ConflictID: conflictID, TaskID: task.TaskID, ProjectID: task.ProjectID, Topic: current.Topic, ClaimKey: current.ClaimKey, EntryIDs: entryIDs, AgentIDs: agentIDs, Status: "needs_review", CreatedAt: nowString()}
	raw, _ := json.Marshal(value)
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: conflictID, TypeID: ConflictTypeV1, ProjectID: task.ProjectID, Status: "candidate", Payload: raw,
		Confidence: 1, Importance: .9, SourceObjectIDs: entryIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-conflict:" + conflictID,
	})
	if err != nil {
		return nil, err
	}
	reviews, err := s.harness.ListRevisionReviews(ctx, object.ObjectID, "", 20)
	if err != nil {
		return nil, err
	}
	if len(reviews) > 0 {
		return []ConflictResult{{ObjectID: object.ObjectID, ReviewID: reviews[0].ReviewID}}, nil
	}
	validation, _ := json.Marshal(map[string]any{
		"status": "passed", "kind": "team_claim_conflict", "review_required": true, "automatic_resolution": false,
		"majority_vote": false, "last_write_wins": false,
	})
	review, err := s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: object.CurrentRevision, EditReason: "Conflicting task claims require explicit Owner review",
		TargetStatus: "active", Confidence: 1, Importance: .9, SourceObjectIDs: entryIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-conflict-review:" + conflictID,
		RequestedBy: "team-memory", Validation: validation,
	})
	if err != nil {
		return nil, err
	}
	return []ConflictResult{{ObjectID: object.ObjectID, ReviewID: review.ReviewID}}, nil
}
func sharedWith(value BlackboardEntry, agentID string) bool {
	if value.Meta.AgentID == agentID {
		return true
	}
	for _, id := range value.DirectShareAgentIDs {
		if id == agentID {
			return true
		}
	}
	return false
}

func (s *Service) BlackboardForAgentAt(ctx context.Context, principal agentauth.Principal, taskID string, at time.Time) ([]harness.Object, error) {
	_, task, err := s.requireTaskAgent(ctx, principal, taskID, agentauth.PermissionTeamBlackboardRead, at)
	if err != nil {
		return nil, err
	}
	items, err := s.harness.ListObjects(ctx, task.ProjectID, BlackboardEntryTypeV1, "active", 500)
	if err != nil {
		return nil, err
	}
	out := []harness.Object{}
	for _, object := range items {
		value, err := decodeBlackboard(object)
		if err != nil {
			return nil, err
		}
		if value.TaskID == task.TaskID && activeAt(value.Meta.ExpiresAt, at) && sharedWith(value, principal.AgentID) {
			out = append(out, object)
		}
	}
	return out, nil
}

func (s *Service) BlackboardForOwner(ctx context.Context, taskID string, includeExpired bool) ([]harness.Object, error) {
	_, task, err := s.Task(ctx, taskID)
	if err != nil {
		return nil, err
	}
	items, err := s.harness.ListObjects(ctx, task.ProjectID, BlackboardEntryTypeV1, "active", 500)
	if err != nil {
		return nil, err
	}
	out := []harness.Object{}
	for _, object := range items {
		value, err := decodeBlackboard(object)
		if err != nil {
			return nil, err
		}
		if value.TaskID == task.TaskID && (includeExpired || activeAt(value.Meta.ExpiresAt, time.Now().UTC())) {
			out = append(out, object)
		}
	}
	return out, nil
}
func (s *Service) SetShare(ctx context.Context, principal agentauth.Principal, entryID string, input ShareInput) (harness.Object, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(entryID))
	if err != nil {
		return harness.Object{}, err
	}
	value, err := decodeBlackboard(object)
	if err != nil {
		return harness.Object{}, err
	}
	_, task, err := s.requireTaskAgent(ctx, principal, value.TaskID, agentauth.PermissionTeamBlackboardShare, time.Now().UTC())
	if err != nil {
		return harness.Object{}, err
	}
	if value.Meta.AgentID != principal.AgentID {
		return harness.Object{}, errors.New("only the original author may change direct sharing; recipients cannot forward")
	}
	if !activeAt(value.Meta.ExpiresAt, time.Now().UTC()) {
		return harness.Object{}, errors.New("expired Blackboard entry cannot change sharing")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return harness.Object{}, errors.New("idempotency_key is required")
	}
	recipients, err := directRecipients(task, principal.AgentID, input.DirectShareAgentIDs)
	if err != nil {
		return harness.Object{}, err
	}
	value.DirectShareAgentIDs = recipients
	raw, _ := json.Marshal(value)
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: object.ObjectID, TypeID: BlackboardEntryTypeV1, ProjectID: task.ProjectID, Status: "active", Payload: raw,
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance,
		ValidFrom: object.Revision.ValidFrom, ValidUntil: value.Meta.ExpiresAt,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		RunID: object.Revision.RunID, StageID: "team.blackboard.share", PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: "team-share:" + object.ObjectID + ":" + input.IdempotencyKey,
	})
}
func (s *Service) ProposeSelfLeave(ctx context.Context, principal agentauth.Principal, taskID string, input ActivationInput) (harness.RevisionReview, error) {
	object, task, err := s.Task(ctx, taskID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.Status != "active" || task.Status != "active" || !activeAt(task.ExpiresAt, time.Now().UTC()) {
		return harness.RevisionReview{}, errors.New("team task is closed or expired")
	}
	if !agentauth.CanAccessProject(principal, task.ProjectID) || !hasMember(task, principal.AgentID) {
		return harness.RevisionReview{}, errors.New("Agent is not a direct member of this task/project")
	}
	if len(task.MemberAgentIDs) <= 1 {
		return harness.RevisionReview{}, errors.New("last task member cannot leave; close the task instead")
	}
	if input.ExpectedRevision <= 0 || strings.TrimSpace(input.EditReason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return harness.RevisionReview{}, errors.New("expected_revision, edit_reason and idempotency_key are required")
	}
	members := []string{}
	for _, id := range task.MemberAgentIDs {
		if id != principal.AgentID {
			members = append(members, id)
		}
	}
	task.MemberAgentIDs = unique(members)
	raw, _ := json.Marshal(task)
	validation, _ := json.Marshal(map[string]any{"status": "passed", "kind": "team_member_self_leave", "removed_agent_id": principal.AgentID, "non_transitive_sharing": true})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "active",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance, ValidFrom: object.Revision.ValidFrom, ValidUntil: object.Revision.ValidUntil,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: input.IdempotencyKey,
		RequestedBy: "agent:" + principal.AgentID, Validation: validation,
	})
}

func (s *Service) ProposeTaskClosure(ctx context.Context, taskID string, input ActivationInput) (harness.RevisionReview, error) {
	object, value, err := s.Task(ctx, taskID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.Status != "active" || value.Status != "active" {
		return harness.RevisionReview{}, errors.New("only active task can be closed")
	}
	if input.ExpectedRevision <= 0 || strings.TrimSpace(input.EditReason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return harness.RevisionReview{}, errors.New("expected_revision, edit_reason and idempotency_key are required")
	}
	value.Status, value.ClosedAt = "closed", nowString()
	raw, _ := json.Marshal(value)
	validation, _ := json.Marshal(map[string]any{"status": "passed", "kind": "team_task_close", "durable_promotion": "owner_review_required"})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "closed",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance,
		ValidFrom: object.Revision.ValidFrom, ValidUntil: object.Revision.ValidUntil,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: input.IdempotencyKey, RequestedBy: "owner", Validation: validation,
	})
}

func (s *Service) ListConflicts(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), ConflictTypeV1, strings.TrimSpace(status), limit)
}

func epistemicSummary(values []string) string {
	values = unique(values)
	if len(values) == 1 {
		return values[0]
	}
	return "mixed"
}
func (s *Service) ensureSelectedConflictsReviewed(ctx context.Context, projectID string, entryIDs []string) error {
	selected := map[string]bool{}
	for _, id := range entryIDs {
		selected[id] = true
	}
	conflicts, err := s.ListConflicts(ctx, projectID, "", 500)
	if err != nil {
		return err
	}
	for _, object := range conflicts {
		value, err := decodeConflict(object)
		if err != nil {
			return err
		}
		involved := false
		for _, id := range value.EntryIDs {
			if selected[id] {
				involved = true
				break
			}
		}
		if involved {
			reviews, err := s.harness.ListRevisionReviews(ctx, object.ObjectID, "", 20)
			if err != nil {
				return err
			}
			decided := len(reviews) > 0 && (reviews[0].Status == "approved" || reviews[0].Status == "rejected")
			if !decided {
				return fmt.Errorf("selected entry belongs to unresolved conflict %s; decide its Owner Review before durable promotion", value.ConflictID)
			}
		}
	}
	return nil
}
func (s *Service) CreateDurable(ctx context.Context, input DurableInput) (harness.Object, error) {
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.EntryIDs = unique(input.EntryIDs)
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Body = strings.TrimSpace(input.Body)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.TaskID == "" || len(input.EntryIDs) == 0 || input.Title == "" || input.Body == "" || input.IdempotencyKey == "" {
		return harness.Object{}, errors.New("task_id, entry_ids, title, body and idempotency_key are required")
	}
	taskObject, task, err := s.Task(ctx, input.TaskID)
	if err != nil {
		return harness.Object{}, err
	}
	if taskObject.Status != "closed" || task.Status != "closed" || task.ClosedAt == "" {
		return harness.Object{}, errors.New("Project Durable promotion requires an explicitly closed task")
	}
	closedAt, err := time.Parse(time.RFC3339Nano, task.ClosedAt)
	if err != nil {
		return harness.Object{}, errors.New("task closed_at is invalid")
	}
	if err := s.ensureSelectedConflictsReviewed(ctx, task.ProjectID, input.EntryIDs); err != nil {
		return harness.Object{}, err
	}
	claims := map[string]string{}
	agents, runs, evidence, epistemic := []string{}, []string{}, []string{}, []string{}
	for _, entryID := range input.EntryIDs {
		object, err := s.harness.Object(ctx, entryID)
		if err != nil {
			return harness.Object{}, err
		}
		entry, err := decodeBlackboard(object)
		if err != nil {
			return harness.Object{}, err
		}
		if entry.TaskID != task.TaskID || entry.ProjectID != task.ProjectID {
			return harness.Object{}, errors.New("durable entry is outside the closed task")
		}
		if !activeAt(entry.Meta.ExpiresAt, closedAt) {
			return harness.Object{}, fmt.Errorf("entry %s expired before task closure", entryID)
		}
		if prior, ok := claims[entry.ClaimKey]; ok && prior != entry.ClaimValue {
			return harness.Object{}, fmt.Errorf("selected entries contain unresolved conflict for claim %s", entry.ClaimKey)
		}
		claims[entry.ClaimKey] = entry.ClaimValue
		agents = append(agents, entry.Meta.AgentID)
		if entry.Meta.RunID != "" {
			runs = append(runs, entry.Meta.RunID)
		}
		evidence = append(evidence, entry.Meta.SourceEvidenceIDs...)
		epistemic = append(epistemic, entry.Meta.EpistemicStatus)
	}
	durableID := stableID("team-durable-", task.ProjectID, task.TaskID, input.IdempotencyKey)
	value := ProjectDurable{
		DurableID: durableID, ProjectID: task.ProjectID, TaskID: task.TaskID, EntryIDs: input.EntryIDs,
		Title: input.Title, Summary: input.Summary, Body: input.Body,
		SourceAgentIDs: unique(agents), SourceRunIDs: unique(runs), SourceEvidenceIDs: unique(evidence),
		EpistemicStatus: epistemicSummary(epistemic), GenerationStatus: "owner_selected", CreatedAt: nowString(),
	}
	raw, _ := json.Marshal(value)
	return s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: durableID, TypeID: ProjectDurableTypeV1, ProjectID: task.ProjectID, Status: "candidate", Payload: raw,
		Confidence: 1, Importance: .85, SourceEvidenceIDs: value.SourceEvidenceIDs, SourceObjectIDs: input.EntryIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "team-durable:" + input.IdempotencyKey,
	})
}

func (s *Service) ListDurables(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), ProjectDurableTypeV1, strings.TrimSpace(status), limit)
}
func (s *Service) ProposeDurableActivation(ctx context.Context, objectID string, input ActivationInput) (harness.RevisionReview, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.RevisionReview{}, err
	}
	value, err := decodeDurable(object)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.Status != "candidate" {
		return harness.RevisionReview{}, errors.New("only candidate Project Durable memory can request activation")
	}
	if input.ExpectedRevision <= 0 || strings.TrimSpace(input.EditReason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return harness.RevisionReview{}, errors.New("expected_revision, edit_reason and idempotency_key are required")
	}
	validation, _ := json.Marshal(map[string]any{
		"status": "passed", "kind": "team_durable_owner_selection", "entry_ids": value.EntryIDs,
		"automatic_promotion": false, "conflict_policy": "owner_selection_only",
	})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: object.Revision.Payload, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "active",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance,
		ValidFrom: object.Revision.ValidFrom, ValidUntil: object.Revision.ValidUntil,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: input.IdempotencyKey, RequestedBy: "owner", Validation: validation,
	})
}

func (s *Service) PrivateForAgent(ctx context.Context, principal agentauth.Principal, taskID string) ([]harness.Object, error) {
	return s.PrivateForAgentAt(ctx, principal, taskID, time.Now().UTC())
}

func (s *Service) BlackboardForAgent(ctx context.Context, principal agentauth.Principal, taskID string) ([]harness.Object, error) {
	return s.BlackboardForAgentAt(ctx, principal, taskID, time.Now().UTC())
}

func (s *Service) TasksForAgentAt(ctx context.Context, principal agentauth.Principal, at time.Time) ([]harness.Object, error) {
	projects, err := s.portfolio.ListProjects(ctx, false)
	if err != nil {
		return nil, err
	}
	out := []harness.Object{}
	for _, project := range projects {
		if !agentauth.CanAccessProject(principal, project.ProjectID) {
			continue
		}
		items, err := s.ListTasks(ctx, project.ProjectID, "active", 500)
		if err != nil {
			return nil, err
		}
		for _, object := range items {
			task, err := decodeTask(object)
			if err != nil {
				return nil, err
			}
			if hasMember(task, principal.AgentID) && activeAt(task.ExpiresAt, at) {
				out = append(out, object)
			}
		}
	}
	return out, nil
}

func (s *Service) TasksForAgent(ctx context.Context, principal agentauth.Principal) ([]harness.Object, error) {
	return s.TasksForAgentAt(ctx, principal, time.Now().UTC())
}
