package adaptation_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/adaptation"
	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func richCaps(id string) contextbridge.ContextCapabilitySet {
	return contextbridge.ContextCapabilitySet{SchemaVersion: contextbridge.CapabilitySchemaVersion, CapabilitySetID: "caps-" + id, AdapterID: "ft4-test", Runtime: "test-harness", ProtocolVersion: "1", Transport: "http", Capabilities: []string{"recall", "pre_turn_injection", "context_receipt", "outcome_feedback"}, MaxContextItems: 8, MaxItemBytes: 65536, MaxTotalBytes: 1048576, MaxConcurrent: 2, SupportsIdempotency: true, Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"}}
}
func prepareFT4(t *testing.T) (*app.App, string) {
	t.Helper()
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "adaptation-ft4", Name: "Adaptation FT4", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(t.Context(), portfolio.GoalInput{ProjectID: project.ProjectID, Title: "验证 Governed Adaptation", Priority: 5}); err != nil {
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

func failedCase(t *testing.T, a *app.App, projectID string, seq int) string {
	t.Helper()
	ctx := t.Context()
	id := fmt.Sprintf("ft4-%d", seq)
	planned, err := a.Context.CompilePlan(ctx, contextbridge.PlanRequest{ProjectID: projectID, AgentID: "agent-ft4", Query: "isolated-" + id, CapabilitySet: richCaps(id), IdempotencyKey: "plan-" + id})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]contextbridge.ReceiptItem, 0, len(planned.Plan.Items))
	for _, item := range planned.Plan.Items {
		items = append(items, contextbridge.ReceiptItem{ItemID: item.ItemID, Status: "delivered", Revision: item.Revision, ContentHash: item.ContentHash, Presentation: item.Presentation})
	}
	receipt, err := a.Context.RecordReceipt(ctx, contextbridge.ReceiptRequest{RunID: planned.RunID, AgentID: "agent-ft4", Receipt: contextbridge.ContextReceipt{
		SchemaVersion: contextbridge.ReceiptSchemaVersion, ReceiptID: "receipt-" + id, PlanID: planned.Plan.PlanID,
		EvidenceLevel: "harness_observed", Completeness: "complete", Items: items,
		Retention: contextbridge.RetentionPolicy{Mode: "session", Redaction: "supported"}, IdempotencyKey: "receipt-" + id,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Context.RecordOutcome(ctx, "agent-ft4", contextbridge.OutcomeFeedback{
		SchemaVersion: contextbridge.OutcomeSchemaVersion, OutcomeID: "outcome-" + id, ProjectID: projectID,
		RunID: planned.RunID, PlanID: planned.Plan.PlanID, ReceiptID: receipt.Receipt.ReceiptID, Source: "ft4-test",
		Metrics:        []contextbridge.OutcomeMetric{{Name: "turn_completed", Value: json.RawMessage(`1`), Confidence: 1}},
		IdempotencyKey: "outcome-" + id,
	}); err != nil {
		t.Fatal(err)
	}
	object, err := a.Experience.CompileCaseFromRun(ctx, planned.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Experience.Evaluate(ctx, experience.EvaluateInput{
		TargetKind: "case", TargetID: object.ObjectID, Protocol: "fixture", EvaluatorID: "case-evaluator", EvaluatorVersion: "1",
		Verdict: "fail", Dimensions: []experience.EvaluationDimension{{Name: "task_correctness", Verdict: "fail", Confidence: 1}},
		Expected: "correct", Observed: "wrong", Confidence: 1, SampleSize: 1, IdempotencyKey: "case-eval-" + id,
	}); err != nil {
		t.Fatal(err)
	}
	object, _, err = a.Experience.Case(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	review, err := a.Experience.ProposeActivation(ctx, object.ObjectID, experience.ActivationInput{
		ExpectedRevision: object.CurrentRevision, EditReason: "retain reproducible failure", IdempotencyKey: "case-active-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(ctx, review.ReviewID, "approve", "owner-case-reviewer", "retain negative Case"); err != nil {
		t.Fatal(err)
	}
	active, value, err := a.Experience.Case(ctx, object.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != "active" || value.Result != "fail" {
		t.Fatalf("failed Case not governed: %#v %#v", active, value)
	}
	return active.ObjectID
}

func proposalInput(projectID, caseID string) adaptation.ProposalInput {
	return adaptation.ProposalInput{
		ProjectID: projectID, SourceCaseIDs: []string{caseID}, Patch: adaptation.Patch{Role: "context.presentation-policy", Config: json.RawMessage(`{"object":"verbatim"}`)},
		PredictedFix:         "Give the external harness a less lossy object presentation for this scoped failure mode.",
		PredictedRegressions: []string{"larger context payload"}, EvaluationSuite: []string{"fixture-context-answer", "regression-check"}, MinimumSample: 2,
		StopConditions: adaptation.StopConditions{MaxRegressionRate: .25, StopOnSafetyFailure: true}, PrivacyImpact: "no scope expansion", CostImpact: "bounded context increase",
		ProposerID: "agent-evolver", CanaryScope: "case-only", OverlayTTLSeconds: 3600, IdempotencyKey: "proposal-1",
	}
}
func TestGovernedOverlayCanaryStopsOnRegressionWithoutChangingBase(t *testing.T) {
	a, projectID := prepareFT4(t)
	caseID := failedCase(t, a, projectID, 1)
	base, err := a.Blueprints.Current(t.Context(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	input := proposalInput(projectID, caseID)
	before, err := a.Adaptation.ListProposals(t.Context(), projectID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := a.Adaptation.DryRunProposal(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NoWritesPerformed || preview.BaseBlueprintHash != base.Assignment.BlueprintHash || preview.EffectiveBlueprintHash == preview.BaseBlueprintHash || len(preview.PermissionDelta) != 0 {
		t.Fatalf("dry run=%#v", preview)
	}
	afterDry, _ := a.Adaptation.ListProposals(t.Context(), projectID, "", 20)
	if len(afterDry) != len(before) {
		t.Fatal("Dry Run wrote a Change Proposal")
	}
	proposalObject, err := a.Adaptation.CreateProposal(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if proposalObject.Status != "candidate" {
		t.Fatalf("proposal=%#v", proposalObject)
	}
	if _, err := a.Adaptation.EvaluateProposal(t.Context(), proposalObject.ObjectID, experience.EvaluateInput{
		Protocol: "self-eval", EvaluatorID: "agent-evolver", EvaluatorVersion: "1", Verdict: "pass",
		Dimensions: []experience.EvaluationDimension{{Name: "conformance", Verdict: "pass", Confidence: 1}}, Confidence: 1, SampleSize: 2, IdempotencyKey: "self-eval",
	}); err == nil || !strings.Contains(err.Error(), "cannot evaluate") {
		t.Fatalf("proposer self-evaluation should fail, err=%v", err)
	}
	preEval, err := a.Adaptation.EvaluateProposal(t.Context(), proposalObject.ObjectID, experience.EvaluateInput{
		Protocol: "pre-canary", EvaluatorID: "evaluator-pre", EvaluatorVersion: "1", Verdict: "pass",
		Dimensions: []experience.EvaluationDimension{{Name: "conformance", Verdict: "pass", Confidence: 1}},
		Expected:   "overlay remains scoped", Observed: "dry-run passed", Confidence: 1, SampleSize: 2, IdempotencyKey: "pre-eval",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preEval.TypeID != experience.EvaluationTypeV2 {
		t.Fatalf("adaptation evaluation must use v2: %#v", preEval)
	}
	proposalObject, proposal, err := a.Adaptation.Proposal(t.Context(), proposalObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := a.Adaptation.ProposeApproval(t.Context(), proposalObject.ObjectID, adaptation.ApprovalInput{
		ExpectedRevision: proposalObject.CurrentRevision, VerifierID: "verifier-ft4", EditReason: "independent pre-canary evaluation passed", IdempotencyKey: "proposal-approve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), approval.ReviewID, "approve", "owner-reviewer", "allow scoped canary only"); err != nil {
		t.Fatal(err)
	}
	proposalObject, proposal, err = a.Adaptation.Proposal(t.Context(), proposalObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if proposalObject.Status != "active" || proposal.VerifierID != "verifier-ft4" {
		t.Fatalf("approved proposal=%#v %#v", proposalObject, proposal)
	}
	current, _ := a.Blueprints.Current(t.Context(), projectID)
	if current.Assignment.BlueprintHash != base.Assignment.BlueprintHash {
		t.Fatal("Proposal approval mutated Global Blueprint")
	}
	overlayObject, err := a.Adaptation.CreateOverlay(t.Context(), adaptation.OverlayInput{ProposalID: proposal.ProposalID, IdempotencyKey: "overlay-1"})
	if err != nil {
		t.Fatal(err)
	}
	if overlayObject.Status != "candidate" {
		t.Fatalf("overlay=%#v", overlayObject)
	}
	overlayReview, err := a.Adaptation.ProposeOverlayActivation(t.Context(), overlayObject.ObjectID, experience.ActivationInput{
		ExpectedRevision: overlayObject.CurrentRevision, EditReason: "activate bounded canary overlay", IdempotencyKey: "overlay-activate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.DecideRevisionReview(t.Context(), overlayReview.ReviewID, "approve", "owner-overlay-reviewer", "bounded canary only"); err != nil {
		t.Fatal(err)
	}
	overlayObject, overlay, err := a.Adaptation.Overlay(t.Context(), overlayObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if overlayObject.Status != "active" || len(overlay.PermissionDelta) != 0 {
		t.Fatalf("active overlay=%#v %#v", overlayObject, overlay)
	}
	snapshot, err := a.Adaptation.OverlaySnapshot(t.Context(), overlayObject.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	var effective blueprint.Current
	if err := json.Unmarshal(snapshot, &effective); err != nil {
		t.Fatal(err)
	}
	if effective.Blueprint.ContentHash != proposal.EffectiveBlueprintHash || effective.Assignment.Status != "overlay" {
		t.Fatalf("effective snapshot=%#v", effective)
	}
	current, _ = a.Blueprints.Current(t.Context(), projectID)
	if current.Assignment.BlueprintHash != base.Assignment.BlueprintHash {
		t.Fatal("Overlay activation mutated Global Blueprint")
	}
	good, err := a.Adaptation.EvaluateProposal(t.Context(), proposalObject.ObjectID, experience.EvaluateInput{
		Protocol: "canary", EvaluatorID: "evaluator-good", EvaluatorVersion: "1", Verdict: "pass",
		Dimensions: []experience.EvaluationDimension{{Name: "improvement", Verdict: "pass", Confidence: 1}, {Name: "regression", Verdict: "pass", Confidence: 1}},
		Confidence: 1, SampleSize: 1, IdempotencyKey: "canary-good",
	})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := a.Adaptation.EvaluateProposal(t.Context(), proposalObject.ObjectID, experience.EvaluateInput{
		Protocol: "canary", EvaluatorID: "evaluator-bad", EvaluatorVersion: "1", Verdict: "fail",
		Dimensions: []experience.EvaluationDimension{{Name: "improvement", Verdict: "unknown", Confidence: 1}, {Name: "regression", Verdict: "fail", Confidence: 1}},
		Confidence: 1, SampleSize: 1, IdempotencyKey: "canary-bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	canary, err := a.Adaptation.RunCanary(t.Context(), adaptation.CanaryInput{
		OverlayID: overlayObject.ObjectID, VerifierID: "verifier-ft4", EvaluationObjectIDs: []string{good.ObjectID, bad.ObjectID}, IdempotencyKey: "canary-run-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if canary.Status != "stopped_fallback_base" || canary.RegressedSamples != 1 || canary.Samples != 2 || canary.RegressionRate != .5 || !canary.GlobalBlueprintUnchanged {
		t.Fatalf("canary=%#v", canary)
	}
	detail, err := a.Harness.RunDetail(t.Context(), canary.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(detail.Run.Snapshot), overlayObject.ObjectID) || !strings.Contains(string(detail.Run.Snapshot), proposal.BaseBlueprintHash) || !strings.Contains(string(detail.Run.Snapshot), proposal.EffectiveBlueprintHash) {
		t.Fatalf("Canary Run cannot explain overlay: %s", detail.Run.Snapshot)
	}
	output, err := a.Harness.StageOutput(t.Context(), canary.RunID, "adaptation.canary")
	if err != nil || !strings.Contains(string(output.Payload), "stopped_fallback_base") {
		t.Fatalf("canary output=%s err=%v", output.Payload, err)
	}
	rollback, err := a.Adaptation.RollbackToBase(t.Context(), overlayObject.ObjectID, "owner-reviewer", "rollback-1")
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Status != "rolled_back_to_base" || !rollback.GlobalBlueprintUnchanged {
		t.Fatalf("rollback=%#v", rollback)
	}
	current, _ = a.Blueprints.Current(t.Context(), projectID)
	if current.Assignment.BlueprintHash != base.Assignment.BlueprintHash {
		t.Fatal("rollback path rewrote Global Blueprint")
	}
	if _, err := a.Blueprints.Activate(t.Context(), projectID, blueprint.DefaultBlueprintID, blueprint.DefaultBlueprintVersion, "owner-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Adaptation.OverlaySnapshot(t.Context(), overlayObject.ObjectID); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("overlay should fail closed after base drift, err=%v", err)
	}
}
func TestAdaptationPatchFailsClosed(t *testing.T) {
	a, projectID := prepareFT4(t)
	caseID := failedCase(t, a, projectID, 2)
	input := proposalInput(projectID, caseID)
	input.Patch = adaptation.Patch{Role: "context.write-policy", Config: json.RawMessage(`{"mode":"implicit"}`)}
	if _, err := a.Adaptation.DryRunProposal(t.Context(), input); err == nil || !strings.Contains(err.Error(), "low-risk") {
		t.Fatalf("unsafe role should fail, err=%v", err)
	}
	input = proposalInput(projectID, caseID)
	input.Patch = adaptation.Patch{Role: "context.budget-policy", Config: json.RawMessage(`{"candidate_multiplier":100}`)}
	if _, err := a.Adaptation.DryRunProposal(t.Context(), input); err == nil || !strings.Contains(err.Error(), "safe bounds") {
		t.Fatalf("unsafe budget should fail, err=%v", err)
	}
}
