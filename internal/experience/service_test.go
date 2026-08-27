package experience_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func richCapability(id string) contextbridge.ContextCapabilitySet {
	return contextbridge.ContextCapabilitySet{
		SchemaVersion: contextbridge.CapabilitySchemaVersion, CapabilitySetID: "caps-" + id,
		AdapterID: "test-rich-adapter", Runtime: "test-harness", ProtocolVersion: "1", Transport: "http",
		Capabilities:    []string{"recall", "pre_turn_injection", "context_receipt", "outcome_feedback"},
		MaxContextItems: 8, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 2,
		SupportsIdempotency: true, Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"},
	}
}

func receiptFor(plan contextbridge.ContextPlan, id string) contextbridge.ContextReceipt {
	items := make([]contextbridge.ReceiptItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		items = append(items, contextbridge.ReceiptItem{ItemID: item.ItemID, Status: "delivered", Revision: item.Revision, ContentHash: item.ContentHash, Presentation: item.Presentation})
	}
	return contextbridge.ContextReceipt{
		SchemaVersion: contextbridge.ReceiptSchemaVersion, ReceiptID: "receipt-" + id, PlanID: plan.PlanID,
		EvidenceLevel: "harness_observed", Completeness: "complete", Items: items,
		Retention:      contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"},
		IdempotencyKey: "receipt-" + id,
	}
}

func prepareProject(t *testing.T) (*app.App, string) {
	t.Helper()
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "experience-ft3", Name: "Experience FT3", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(t.Context(), portfolio.GoalInput{ProjectID: project.ProjectID, Title: "验证 Experience Bank", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(t.Context(), project.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(t.Context(), project.ProjectID, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}
	return a, project.ProjectID
}
func createObservedCase(t *testing.T, a *app.App, projectID string, sequence int) (contextbridge.PlanResult, harnessCase) {
	t.Helper()
	ctx := t.Context()
	id := fmt.Sprintf("ft3-%d", sequence)
	query := "private-eval-query-" + id
	planned, err := a.Context.CompilePlan(ctx, contextbridge.PlanRequest{
		ProjectID: projectID, AgentID: "agent-ft3", Query: query,
		CapabilitySet: richCapability(id), IdempotencyKey: "plan-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := a.Context.RecordReceipt(ctx, contextbridge.ReceiptRequest{
		RunID: planned.RunID, AgentID: "agent-ft3", Receipt: receiptFor(planned.Plan, id),
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := a.Context.RecordOutcome(ctx, "agent-ft3", contextbridge.OutcomeFeedback{
		SchemaVersion: contextbridge.OutcomeSchemaVersion, OutcomeID: "outcome-" + id,
		ProjectID: projectID, RunID: planned.RunID, PlanID: planned.Plan.PlanID, ReceiptID: receipt.Receipt.ReceiptID,
		Source: "test-rich-adapter", Metrics: []contextbridge.OutcomeMetric{{Name: "turn_completed", Value: json.RawMessage(`1`), Confidence: 1}},
		Cost: contextbridge.OutcomeCost{Tokens: 123, LatencyMS: 450}, IdempotencyKey: "outcome-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	object, err := a.Experience.CompileCaseFromRun(ctx, planned.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, value, err := a.Experience.Case(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	return planned, harnessCase{ObjectID: object.ObjectID, Revision: object.CurrentRevision, Status: object.Status, Value: value, OutcomeRunID: outcome.RunID, Query: query}
}

type harnessCase struct {
	ObjectID     string
	Revision     int
	Status       string
	Value        experience.Case
	OutcomeRunID string
	Query        string
}

func evaluateCase(t *testing.T, a *app.App, item harnessCase, verdict, observed, id string) harnessCase {
	t.Helper()
	ctx := t.Context()
	_, err := a.Experience.Evaluate(ctx, experience.EvaluateInput{
		TargetKind: "case", TargetID: item.ObjectID, Protocol: "fixture", EvaluatorID: "owner-fixture", EvaluatorVersion: "1.0.0",
		Verdict: verdict, Dimensions: []experience.EvaluationDimension{{Name: "task_correctness", Verdict: verdict, Confidence: 1}},
		Expected: "answer from delivered governed context", Observed: observed, Confidence: 1, SampleSize: 1, IdempotencyKey: "evaluation-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	object, value, err := a.Experience.Case(ctx, item.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	item.Revision, item.Status, item.Value = object.CurrentRevision, object.Status, value
	return item
}

func activateCase(t *testing.T, a *app.App, item harnessCase, id string) harnessCase {
	t.Helper()
	review, err := a.Experience.ProposeActivation(t.Context(), item.ObjectID, experience.ActivationInput{ExpectedRevision: item.Revision, EditReason: "independent evaluation reviewed", IdempotencyKey: "activate-" + id})
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != "pending" {
		t.Fatalf("activation review=%#v", review)
	}
	before, _, err := a.Experience.Case(t.Context(), item.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != "candidate" {
		t.Fatalf("activation proposal moved current pointer: %#v", before)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), review.ReviewID, "approve", "owner-test", "evaluated Experience accepted"); err != nil {
		t.Fatal(err)
	}
	object, value, err := a.Experience.Case(t.Context(), item.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	item.Revision, item.Status, item.Value = object.CurrentRevision, object.Status, value
	return item
}

func TestDeliveredCompletedCaseRemainsUnknownUntilIndependentEvaluation(t *testing.T) {
	a, projectID := prepareProject(t)
	planned, item := createObservedCase(t, a, projectID, 1)
	if item.Status != "candidate" || item.Value.Result != "unknown" {
		t.Fatalf("raw observations became truth: %#v", item)
	}
	if item.Value.Delivery.Delivered != len(planned.Plan.Items) || item.Value.Delivery.DeliveryUnverified != 0 {
		t.Fatalf("delivery=%#v", item.Value.Delivery)
	}
	if !strings.Contains(fmt.Sprint(item.Value.OutcomeMetrics), "turn_completed") {
		t.Fatalf("outcome missing: %#v", item.Value.OutcomeMetrics)
	}
	raw, _ := json.Marshal(item.Value)
	if strings.Contains(string(raw), item.Query) {
		t.Fatalf("raw query leaked into Experience Case payload: %s", raw)
	}
	if _, err := a.Experience.ProposeActivation(t.Context(), item.ObjectID, experience.ActivationInput{ExpectedRevision: item.Revision, EditReason: "must fail", IdempotencyKey: "no-eval"}); err == nil || !strings.Contains(err.Error(), "Evaluation") {
		t.Fatalf("unevaluated case activation should fail, err=%v", err)
	}

	item = evaluateCase(t, a, item, "fail", "NO_CONTEXT", "negative-1")
	if item.Value.Result != "fail" || item.Value.PrimaryFailureDimension != "task_correctness" || len(item.Value.EvaluationObjectIDs) != 1 {
		t.Fatalf("negative evaluation lost: %#v", item.Value)
	}
	searchResult, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "task_correctness", ProjectID: projectID, Kinds: []string{experience.SearchKind}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	foundCase := false
	for _, hit := range searchResult.Hits {
		if hit.SourceID == item.ObjectID && hit.Kind == experience.SearchKind {
			foundCase = true
			break
		}
	}
	if !foundCase {
		t.Fatalf("failed Case not searchable: %#v", searchResult)
	}
	if err := a.SearchStore.DeleteDocumentsByKindAndProject(t.Context(), projectID, experience.SearchKind); err != nil {
		t.Fatal(err)
	}
	empty, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "task_correctness", ProjectID: projectID, Kinds: []string{experience.SearchKind}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Hits) != 0 {
		t.Fatalf("Experience projection clear failed: %#v", empty.Hits)
	}
	indexed, err := a.Experience.RebuildProjection(t.Context(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if indexed < 2 {
		t.Fatalf("projection rebuild indexed=%d", indexed)
	}
	rebuiltSearch, err := a.Unified.Search(t.Context(), unifiedsearch.Query{Text: "task_correctness", ProjectID: projectID, Kinds: []string{experience.SearchKind}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	foundCase = false
	for _, hit := range rebuiltSearch.Hits {
		if hit.SourceID == item.ObjectID && hit.Kind == experience.SearchKind {
			foundCase = true
			break
		}
	}
	if !foundCase {
		t.Fatalf("failed Case missing after projection rebuild: %#v", rebuiltSearch)
	}
	if _, err := a.Portfolio.RebuildIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	var kind string
	var docs int
	if err := a.SearchStore.DB.QueryRowContext(t.Context(), `SELECT count(*),max(kind) FROM documents WHERE source_id=?`, item.ObjectID).Scan(&docs, &kind); err != nil {
		t.Fatal(err)
	}
	if docs != 1 || kind != experience.SearchKind {
		t.Fatalf("full rebuild leaked Experience as generic object docs=%d kind=%s", docs, kind)
	}
	evaluatedRevision := item.Revision
	if _, err := a.Context.RecordOutcome(t.Context(), "agent-ft3", contextbridge.OutcomeFeedback{
		SchemaVersion: contextbridge.OutcomeSchemaVersion, OutcomeID: "outcome-ft3-delayed", ProjectID: projectID,
		RunID: planned.RunID, PlanID: item.Value.PlanID, ReceiptID: item.Value.ReceiptID, Source: "delayed-user-feedback",
		Metrics:        []contextbridge.OutcomeMetric{{Name: "user_rejected_answer", Value: json.RawMessage(`true`), Confidence: 1}},
		IdempotencyKey: "outcome-ft3-delayed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Experience.DiscoverCases(t.Context(), projectID, 100); err != nil {
		t.Fatal(err)
	}
	frozen, frozenValue, err := a.Experience.Case(t.Context(), item.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.CurrentRevision != evaluatedRevision || len(frozenValue.EvaluationObjectIDs) != 1 {
		t.Fatalf("late Outcome rewrote evaluated candidate: %#v %#v", frozen, frozenValue)
	}
	item = activateCase(t, a, item, "negative-1")
	if item.Status != "active" || item.Value.Result != "fail" {
		t.Fatalf("governed negative Case not retained: %#v", item)
	}
	activeRevision := item.Revision
	if _, err := a.Experience.DiscoverCases(t.Context(), projectID, 100); err != nil {
		t.Fatal(err)
	}
	rebuilt, rebuiltValue, err := a.Experience.Case(t.Context(), item.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.CurrentRevision != activeRevision || rebuiltValue.Result != "fail" {
		t.Fatalf("rebuild rewrote governed Case: %#v %#v", rebuilt, rebuiltValue)
	}
}
func TestPatternRequiresMultipleGovernedCasesAndKeepsCounterexamples(t *testing.T) {
	a, projectID := prepareProject(t)
	_, first := createObservedCase(t, a, projectID, 11)
	_, second := createObservedCase(t, a, projectID, 12)
	_, counter := createObservedCase(t, a, projectID, 13)
	first = activateCase(t, a, evaluateCase(t, a, first, "fail", "NO_CONTEXT", "pattern-support-1"), "pattern-support-1")
	second = activateCase(t, a, evaluateCase(t, a, second, "fail", "wrong field interpretation", "pattern-support-2"), "pattern-support-2")
	counter = activateCase(t, a, evaluateCase(t, a, counter, "pass", "expected goal title", "pattern-counter-1"), "pattern-counter-1")

	if _, err := a.Experience.CreatePattern(t.Context(), experience.PatternInput{
		ProjectID: projectID, NormalizedPattern: "insufficient support", SupportingCaseIDs: []string{first.ObjectID},
		ExpectedEffect: "none", Confidence: .5, IdempotencyKey: "pattern-too-small",
	}); err == nil || !strings.Contains(err.Error(), "two") {
		t.Fatalf("single-case Pattern should fail, err=%v", err)
	}

	patternObject, err := a.Experience.CreatePattern(t.Context(), experience.PatternInput{
		ProjectID:         projectID,
		NormalizedPattern: "Context can be delivered but semantically under-used when exact-field wording is interpreted too literally.",
		SupportingCaseIDs: []string{first.ObjectID, second.ObjectID}, CounterexampleCaseIDs: []string{counter.ObjectID},
		TargetComponents: []string{"context.presentation-policy"}, Conditions: []string{"governed profile delivered", "exact-answer task"},
		ExpectedEffect: "Make the Profile presentation preserve semantic field labels without claiming task success.",
		Confidence:     .75, KnownRegressions: []string{"extra context may increase tokens"}, NegativeDomains: []string{"tasks without profile context"},
		IdempotencyKey: "pattern-delivered-underused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if patternObject.Status != "candidate" {
		t.Fatalf("new Pattern bypassed candidate gate: %#v", patternObject)
	}
	_, pattern, err := a.Experience.Pattern(t.Context(), patternObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pattern.SupportingCaseIDs) != 2 || len(pattern.CounterexampleCaseIDs) != 1 || pattern.CounterexampleCaseIDs[0] != counter.ObjectID {
		t.Fatalf("Pattern evidence set=%#v", pattern)
	}
	if pattern.SampleSize != 3 {
		t.Fatalf("Pattern sample size=%d", pattern.SampleSize)
	}

	if _, err := a.Experience.Evaluate(t.Context(), experience.EvaluateInput{
		TargetKind: "pattern", TargetID: patternObject.ObjectID, Protocol: "case-set-review", EvaluatorID: "owner-fixture", EvaluatorVersion: "1.0.0",
		Verdict: "pass", Dimensions: []experience.EvaluationDimension{{Name: "evidence_support", Verdict: "pass", Confidence: .9}},
		Expected: "support cases and counterexample are all traceable", Observed: "2 support + 1 counterexample retained", Confidence: .9, SampleSize: 3,
		IdempotencyKey: "evaluate-pattern-1",
	}); err != nil {
		t.Fatal(err)
	}
	candidate, pattern, err := a.Experience.Pattern(t.Context(), patternObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != "candidate" || len(pattern.EvaluationObjectIDs) != 1 || pattern.LastValidated == "" {
		t.Fatalf("evaluated Pattern=%#v %#v", candidate, pattern)
	}
	review, err := a.Experience.ProposeActivation(t.Context(), candidate.ObjectID, experience.ActivationInput{ExpectedRevision: candidate.CurrentRevision, EditReason: "retain evaluated diagnostic Pattern", IdempotencyKey: "activate-pattern-1"})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _ := a.Experience.Pattern(t.Context(), candidate.ObjectID)
	if before.Status != "candidate" {
		t.Fatalf("Pattern activated before review: %#v", before)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), review.ReviewID, "approve", "owner-test", "diagnostic Pattern accepted"); err != nil {
		t.Fatal(err)
	}
	after, activePattern, err := a.Experience.Pattern(t.Context(), candidate.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "active" || len(activePattern.CounterexampleCaseIDs) != 1 {
		t.Fatalf("active Pattern lost counterexample: %#v %#v", after, activePattern)
	}
	discovered, err := a.Experience.DiscoverCases(t.Context(), projectID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 3 {
		t.Fatalf("eligible Context Run / Case reconstruction mismatch: cases=%d", len(discovered))
	}

	foreignProject, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "experience-ft3-foreign", Name: "Foreign Experience", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(t.Context(), portfolio.GoalInput{ProjectID: foreignProject.ProjectID, Title: "foreign goal", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(t.Context(), foreignProject.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(t.Context(), foreignProject.ProjectID, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}
	_, foreign := createObservedCase(t, a, foreignProject.ProjectID, 99)
	foreign = activateCase(t, a, evaluateCase(t, a, foreign, "fail", "foreign failure", "foreign"), "foreign")
	if _, err := a.Experience.CreatePattern(t.Context(), experience.PatternInput{
		ProjectID: projectID, NormalizedPattern: "must not cross projects", SupportingCaseIDs: []string{first.ObjectID, foreign.ObjectID},
		ExpectedEffect: "none", Confidence: .5, IdempotencyKey: "cross-project-pattern",
	}); err == nil || !strings.Contains(err.Error(), "cross project") {
		t.Fatalf("cross-project Pattern should fail closed, err=%v", err)
	}
}
