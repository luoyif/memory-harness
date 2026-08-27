package teammemory_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/teammemory"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func createAgent(t *testing.T, a *agentauth.Service, name, projectID string) agentauth.Credential {
	t.Helper()
	credential, err := a.Create(t.Context(), agentauth.CreateInput{
		Name: name, Kind: "team-test", ProjectIDs: []string{projectID},
		Permissions: []string{agentauth.PermissionTeamPrivate, agentauth.PermissionTeamBlackboardRead, agentauth.PermissionTeamBlackboardWrite, agentauth.PermissionTeamBlackboardShare},
	})
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func TestTeamMemoryE8PrivateBlackboardTTLSharingConflictAndDurablePromotion(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "team-e8", Name: "Team E8", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	aAgent := createAgent(t, a.Agents, "Agent A", project.ProjectID)
	bAgent := createAgent(t, a.Agents, "Agent B", project.ProjectID)
	cAgent := createAgent(t, a.Agents, "Agent C", project.ProjectID)
	taskObject, err := a.TeamMemory.CreateTask(t.Context(), teammemory.CreateTaskInput{
		ProjectID: project.ProjectID, Title: "E8 shared task",
		MemberAgentIDs: []string{aAgent.Agent.AgentID, bAgent.Agent.AgentID, cAgent.Agent.AgentID},
		TTLSeconds:     3600, IdempotencyKey: "task-e8",
	})
	if err != nil {
		t.Fatal(err)
	}

	privateA, err := a.TeamMemory.WritePrivate(t.Context(), aAgent.Agent, taskObject.ObjectID, teammemory.ContributionInput{
		Content: "A-PRIVATE-MARKER", Confidence: .9, EpistemicStatus: "hypothesis", TTLSeconds: 600, IdempotencyKey: "private-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	privateForA, err := a.TeamMemory.PrivateForAgent(t.Context(), aAgent.Agent, taskObject.ObjectID)
	if err != nil || len(privateForA) != 1 || privateForA[0].ObjectID != privateA.ObjectID {
		t.Fatalf("A private=%#v err=%v", privateForA, err)
	}
	privateForB, err := a.TeamMemory.PrivateForAgent(t.Context(), bAgent.Agent, taskObject.ObjectID)
	if err != nil || len(privateForB) != 0 {
		t.Fatalf("B saw A private=%#v err=%v", privateForB, err)
	}

	shared, err := a.TeamMemory.WriteBlackboard(t.Context(), aAgent.Agent, taskObject.ObjectID, teammemory.BlackboardInput{
		ContributionInput: teammemory.ContributionInput{Content: "A shares only with B", Confidence: .95, EpistemicStatus: "observed", TTLSeconds: 600, IdempotencyKey: "share-a-b"},
		Topic:             "status", ClaimKey: "release.status", ClaimValue: "ready", DirectShareAgentIDs: []string{bAgent.Agent.AgentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	visibleB, err := a.TeamMemory.BlackboardForAgent(t.Context(), bAgent.Agent, taskObject.ObjectID)
	if err != nil || len(visibleB) != 1 {
		t.Fatalf("B visible=%#v err=%v", visibleB, err)
	}
	visibleC, err := a.TeamMemory.BlackboardForAgent(t.Context(), cAgent.Agent, taskObject.ObjectID)
	if err != nil || len(visibleC) != 0 {
		t.Fatalf("C saw non-shared entry=%#v err=%v", visibleC, err)
	}
	if _, err := a.TeamMemory.SetShare(t.Context(), bAgent.Agent, shared.Entry.EntryID, teammemory.ShareInput{DirectShareAgentIDs: []string{cAgent.Agent.AgentID}, IdempotencyKey: "forward-b-c"}); err == nil || !strings.Contains(err.Error(), "recipients cannot forward") {
		t.Fatalf("B forwarded A share err=%v", err)
	}
	if _, err := a.TeamMemory.SetShare(t.Context(), aAgent.Agent, shared.Entry.EntryID, teammemory.ShareInput{DirectShareAgentIDs: []string{}, IdempotencyKey: "revoke-a-b"}); err != nil {
		t.Fatal(err)
	}
	visibleB, err = a.TeamMemory.BlackboardForAgent(t.Context(), bAgent.Agent, taskObject.ObjectID)
	if err != nil || len(visibleB) != 0 {
		t.Fatalf("revoked B still sees entry=%#v err=%v", visibleB, err)
	}

	ttlEntry, err := a.TeamMemory.WriteBlackboard(t.Context(), aAgent.Agent, taskObject.ObjectID, teammemory.BlackboardInput{
		ContributionInput: teammemory.ContributionInput{Content: "short lived", Confidence: .8, EpistemicStatus: "observed", TTLSeconds: 60, IdempotencyKey: "ttl-a-c"},
		Topic:             "ttl", ClaimKey: "ttl.marker", ClaimValue: "temporary", DirectShareAgentIDs: []string{cAgent.Agent.AgentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, ttlEntry.Entry.Meta.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	visibleC, err = a.TeamMemory.BlackboardForAgentAt(t.Context(), cAgent.Agent, taskObject.ObjectID, expiresAt.Add(time.Second))
	if err != nil || len(visibleC) != 0 {
		t.Fatalf("expired Blackboard remained visible=%#v err=%v", visibleC, err)
	}

	leaveReview, err := a.TeamMemory.ProposeSelfLeave(t.Context(), cAgent.Agent, taskObject.ObjectID, teammemory.ActivationInput{
		ExpectedRevision: taskObject.CurrentRevision, EditReason: "Agent C leaves the task", IdempotencyKey: "leave-c-e8",
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeLeave, err := a.TeamMemory.TasksForAgent(t.Context(), cAgent.Agent)
	if err != nil || len(beforeLeave) != 1 {
		t.Fatalf("pending leave changed membership=%#v err=%v", beforeLeave, err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), leaveReview.ReviewID, "approve", "owner-e8", "approve voluntary leave"); err != nil {
		t.Fatal(err)
	}
	afterLeave, err := a.TeamMemory.TasksForAgent(t.Context(), cAgent.Agent)
	if err != nil || len(afterLeave) != 0 {
		t.Fatalf("approved leave kept C in task=%#v err=%v", afterLeave, err)
	}
	if _, err := a.TeamMemory.BlackboardForAgent(t.Context(), cAgent.Agent, taskObject.ObjectID); err == nil || !strings.Contains(err.Error(), "direct member") {
		t.Fatalf("departed Agent C retained Blackboard access err=%v", err)
	}

	firstConflict, err := a.TeamMemory.WriteBlackboard(t.Context(), aAgent.Agent, taskObject.ObjectID, teammemory.BlackboardInput{
		ContributionInput: teammemory.ContributionInput{Content: "A says red", Confidence: .7, EpistemicStatus: "inferred", TTLSeconds: 600, IdempotencyKey: "conflict-a"},
		Topic:             "decision", ClaimKey: "deployment.color", ClaimValue: "red", DirectShareAgentIDs: []string{bAgent.Agent.AgentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondConflict, err := a.TeamMemory.WriteBlackboard(t.Context(), bAgent.Agent, taskObject.ObjectID, teammemory.BlackboardInput{
		ContributionInput: teammemory.ContributionInput{Content: "B says blue", Confidence: .9, EpistemicStatus: "observed", TTLSeconds: 600, IdempotencyKey: "conflict-b"},
		Topic:             "decision", ClaimKey: "deployment.color", ClaimValue: "blue", DirectShareAgentIDs: []string{aAgent.Agent.AgentID},
	})
	if err != nil || len(secondConflict.Conflicts) != 1 {
		t.Fatalf("conflict=%#v err=%v", secondConflict, err)
	}
	review, err := a.Harness.RevisionReview(t.Context(), secondConflict.Conflicts[0].ReviewID)
	if err != nil || review.Status != "pending" {
		t.Fatalf("conflict review=%#v err=%v", review, err)
	}
	var validation map[string]any
	if err := json.Unmarshal(review.Validation, &validation); err != nil {
		t.Fatal(err)
	}
	if validation["majority_vote"] != false || validation["last_write_wins"] != false {
		t.Fatalf("conflict review permits automatic truth selection: %#v", validation)
	}
	ownerEntries, err := a.TeamMemory.BlackboardForOwner(t.Context(), taskObject.ObjectID, true)
	if err != nil {
		t.Fatal(err)
	}
	seenConflict := 0
	for _, object := range ownerEntries {
		if object.ObjectID == firstConflict.Entry.EntryID || object.ObjectID == secondConflict.Entry.EntryID {
			seenConflict++
		}
	}
	if seenConflict != 2 {
		t.Fatalf("conflicting claims were overwritten ownerEntries=%#v", ownerEntries)
	}
	if durables, err := a.TeamMemory.ListDurables(t.Context(), project.ProjectID, "", 20); err != nil || len(durables) != 0 {
		t.Fatalf("conflict auto-promoted durable=%#v err=%v", durables, err)
	}

	if _, err := a.TeamMemory.CreateDurable(t.Context(), teammemory.DurableInput{TaskID: taskObject.ObjectID, EntryIDs: []string{firstConflict.Entry.EntryID}, Title: "too early", Body: "must fail", IdempotencyKey: "before-close"}); err == nil {
		t.Fatal("open task allowed Project Durable promotion")
	}
	currentTaskObject, _, err := a.TeamMemory.Task(t.Context(), taskObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	closeReview, err := a.TeamMemory.ProposeTaskClosure(t.Context(), taskObject.ObjectID, teammemory.ActivationInput{ExpectedRevision: currentTaskObject.CurrentRevision, EditReason: "task complete", IdempotencyKey: "close-e8"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), closeReview.ReviewID, "approve", "owner-e8", "close task before promotion"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.TeamMemory.BlackboardForAgent(t.Context(), aAgent.Agent, taskObject.ObjectID); err == nil || !strings.Contains(err.Error(), "closed or expired") {
		t.Fatalf("closed task remained Agent-readable err=%v", err)
	}
	if _, err := a.TeamMemory.CreateDurable(t.Context(), teammemory.DurableInput{
		TaskID: taskObject.ObjectID, EntryIDs: []string{firstConflict.Entry.EntryID},
		Title: "unreviewed conflict choice", Body: "must wait for conflict review", IdempotencyKey: "durable-unreviewed-conflict",
	}); err == nil || !strings.Contains(err.Error(), "unresolved conflict") {
		t.Fatalf("pending Conflict Review was bypassed by selecting one branch err=%v", err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), review.ReviewID, "approve", "owner-e8", "acknowledge conflict before selecting durable branch"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TeamMemory.CreateDurable(t.Context(), teammemory.DurableInput{
		TaskID: taskObject.ObjectID, EntryIDs: []string{firstConflict.Entry.EntryID, secondConflict.Entry.EntryID},
		Title: "conflicting durable", Body: "must not majority-vote", IdempotencyKey: "durable-conflict",
	}); err == nil || !strings.Contains(err.Error(), "unresolved conflict") {
		t.Fatalf("conflicting entries auto-promoted err=%v", err)
	}
	resolvedBranch, err := a.TeamMemory.CreateDurable(t.Context(), teammemory.DurableInput{
		TaskID: taskObject.ObjectID, EntryIDs: []string{firstConflict.Entry.EntryID}, Title: "Reviewed conflict branch",
		Body: "Owner explicitly selected red after Conflict Review", IdempotencyKey: "durable-reviewed-branch",
	})
	if err != nil || resolvedBranch.Status != "candidate" {
		t.Fatalf("reviewed single conflict branch could not become candidate=%#v err=%v", resolvedBranch, err)
	}
	durable, err := a.TeamMemory.CreateDurable(t.Context(), teammemory.DurableInput{
		TaskID: taskObject.ObjectID, EntryIDs: []string{shared.Entry.EntryID}, Title: "Approved team result",
		Summary: "Owner selected one non-conflicting task result", Body: "APPROVED-DURABLE-MARKER release is ready",
		IdempotencyKey: "durable-approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if durable.Status != "candidate" {
		t.Fatalf("durable was directly active %#v", durable)
	}
	if _, err := a.Portfolio.RebuildProjectIndex(t.Context(), project.ProjectID); err != nil {
		t.Fatal(err)
	}
	before, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "APPROVED-DURABLE-MARKER", ProjectID: project.ProjectID, Limit: 20})
	if err != nil || len(before.Hits) != 0 {
		t.Fatalf("candidate durable leaked to recall=%#v err=%v", before.Hits, err)
	}
	durableReview, err := a.TeamMemory.ProposeDurableActivation(t.Context(), durable.ObjectID, teammemory.ActivationInput{ExpectedRevision: durable.CurrentRevision, EditReason: "Owner selected task result", IdempotencyKey: "activate-durable-e8"})
	if err != nil {
		t.Fatal(err)
	}
	stillCandidate, err := a.Harness.Object(t.Context(), durable.ObjectID)
	if err != nil || stillCandidate.Status != "candidate" {
		t.Fatalf("pending durable review moved current=%#v err=%v", stillCandidate, err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), durableReview.ReviewID, "approve", "owner-e8", "promote selected team result"); err != nil {
		t.Fatal(err)
	}
	activeDurable, err := a.Harness.Object(t.Context(), durable.ObjectID)
	if err != nil || activeDurable.Status != "active" {
		t.Fatalf("active durable=%#v err=%v", activeDurable, err)
	}
	if _, err := a.Reprojection.RefreshApprovedObject(t.Context(), activeDurable); err != nil {
		t.Fatal(err)
	}
	after, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "APPROVED-DURABLE-MARKER", ProjectID: project.ProjectID, Limit: 20})
	if err != nil || len(after.Hits) != 1 || after.Hits[0].SourceID != durable.ObjectID {
		t.Fatalf("active durable missing from recall=%#v err=%v", after.Hits, err)
	}
	privateLeak, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "A-PRIVATE-MARKER", ProjectID: project.ProjectID, Limit: 20})
	if err != nil || len(privateLeak.Hits) != 0 {
		t.Fatalf("private scratch leaked to project recall=%#v err=%v", privateLeak.Hits, err)
	}
	blackboardLeak, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "A says red", ProjectID: project.ProjectID, Limit: 20})
	if err != nil || len(blackboardLeak.Hits) != 0 {
		t.Fatalf("blackboard leaked to project recall=%#v err=%v", blackboardLeak.Hits, err)
	}
}
