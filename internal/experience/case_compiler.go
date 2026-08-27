package experience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
)

type contextSnapshot struct {
	CapabilitySet contextbridge.ContextCapabilitySet `json:"capability_set"`
}

func deliverySummary(plan contextbridge.ContextPlan, receipt *contextbridge.ContextReceipt) DeliverySummary {
	out := DeliverySummary{Total: len(plan.Items)}
	if receipt == nil {
		out.DeliveryUnverified = len(plan.Items)
		out.Completeness = "missing"
		return out
	}
	out.EvidenceLevel = receipt.EvidenceLevel
	out.Completeness = receipt.Completeness
	statuses := contextbridge.EffectiveDeliveryStatus(plan, receipt)
	for _, status := range statuses {
		switch status {
		case "delivered":
			out.Delivered++
		case "trimmed":
			out.Trimmed++
		case "denied":
			out.Denied++
		case "failed":
			out.Failed++
		default:
			out.DeliveryUnverified++
		}
	}
	return out
}

func (s *Service) outcomesForRun(ctx context.Context, projectID, targetRunID string) ([]harness.Run, []contextbridge.OutcomeFeedback, []string, error) {
	runs, err := s.harness.ListRuns(ctx, projectID, "", 500)
	if err != nil {
		return nil, nil, nil, err
	}
	matchedRuns := []harness.Run{}
	outcomes := []contextbridge.OutcomeFeedback{}
	hashes := []string{}
	for _, run := range runs {
		if run.PipelineID != contextbridge.OutcomeRunPipelineID {
			continue
		}
		output, err := s.harness.StageOutput(ctx, run.RunID, "context.outcome")
		if err != nil {
			continue
		}
		var value contextbridge.OutcomeFeedback
		if json.Unmarshal(output.Payload, &value) != nil || value.RunID != targetRunID {
			continue
		}
		matchedRuns = append(matchedRuns, run)
		outcomes = append(outcomes, value)
		hashes = append(hashes, output.OutputHash)
	}
	return matchedRuns, outcomes, hashes, nil
}

func costAndMetrics(values []contextbridge.OutcomeFeedback) (CostSummary, []OutcomeObservation) {
	cost := CostSummary{}
	metrics := []OutcomeObservation{}
	for _, outcome := range values {
		cost.Tokens += outcome.Cost.Tokens
		cost.LatencyMS += outcome.Cost.LatencyMS
		cost.MoneyMinor += outcome.Cost.MoneyMinor
		cost.SafetyEvents += outcome.Cost.SafetyEvents
		for _, metric := range outcome.Metrics {
			metrics = append(metrics, OutcomeObservation{Name: metric.Name, Value: metric.Value, Confidence: metric.Confidence})
		}
	}
	return cost, metrics
}

func sourceRefs(plan contextbridge.ContextPlan) ([]string, []string) {
	evidenceIDs, objectIDs := []string{}, []string{}
	for _, item := range plan.Items {
		if item.SourceKind == "evidence" {
			evidenceIDs = append(evidenceIDs, item.SourceID)
		} else if item.SourceKind == "object" {
			objectIDs = append(objectIDs, item.SourceID)
		}
		evidenceIDs = append(evidenceIDs, item.SourceRefs...)
	}
	return unique(evidenceIDs), unique(objectIDs)
}
func initialDiagnosis(run harness.Run, delivery DeliverySummary, cost CostSummary) (string, string) {
	if run.Status == "failed" || run.Status == "denied" || run.Status == "cancelled" {
		return "run_execution", "Run ended before a successful completion; causal diagnosis requires Evaluation."
	}
	if delivery.Failed > 0 || delivery.Denied > 0 || delivery.DeliveryUnverified > 0 {
		return "context_delivery", "At least one planned context item was not verified as delivered; task impact remains unknown until Evaluation."
	}
	if cost.SafetyEvents > 0 {
		return "safety", "The reported outcome contains one or more safety events; task impact requires independent Evaluation."
	}
	return "", "Context delivery and runtime outcome are observations only; correctness remains unknown until an independent Evaluation is attached."
}

func expiryFromRun(run harness.Run) string {
	base := time.Now().UTC()
	if run.EndedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, run.EndedAt); err == nil {
			base = parsed
		}
	}
	return base.Add(90 * 24 * time.Hour).Format(time.RFC3339Nano)
}

func (s *Service) CompileCaseFromRun(ctx context.Context, runID string) (harness.Object, error) {
	run, err := s.harness.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return harness.Object{}, err
	}
	if !terminal(run.Status) {
		return harness.Object{}, errors.New("experience case requires a terminal source run")
	}
	if run.PipelineID != contextbridge.ContextRunPipelineID {
		return harness.Object{}, errors.New("FT3 v1 case compiler currently accepts Context exchange runs only")
	}
	planOutput, err := s.harness.StageOutput(ctx, run.RunID, "context.plan")
	if err != nil {
		return harness.Object{}, fmt.Errorf("context plan is required: %w", err)
	}
	var plan contextbridge.ContextPlan
	if err := json.Unmarshal(planOutput.Payload, &plan); err != nil {
		return harness.Object{}, err
	}
	var receipt *contextbridge.ContextReceipt
	receiptHash := "missing"
	if output, getErr := s.harness.StageOutput(ctx, run.RunID, "context.receipt"); getErr == nil {
		var value contextbridge.ContextReceipt
		if err := json.Unmarshal(output.Payload, &value); err != nil {
			return harness.Object{}, err
		}
		receipt = &value
		receiptHash = output.OutputHash
	} else if !errors.Is(getErr, sql.ErrNoRows) {
		return harness.Object{}, getErr
	}
	outcomeRuns, outcomes, outcomeHashes, err := s.outcomesForRun(ctx, run.ProjectID, run.RunID)
	if err != nil {
		return harness.Object{}, err
	}
	if receipt == nil && len(outcomes) == 0 && run.Status == "completed" {
		return harness.Object{}, errors.New("completed Context run has neither Receipt nor Outcome; not eligible for Experience auto-promotion")
	}
	cost, metrics := costAndMetrics(outcomes)
	delivery := deliverySummary(plan, receipt)
	failure, diagnosis := initialDiagnosis(run, delivery, cost)
	var snapshot contextSnapshot
	_ = json.Unmarshal(run.Snapshot, &snapshot)
	outcomeRunIDs := []string{}
	for _, item := range outcomeRuns {
		outcomeRunIDs = append(outcomeRunIDs, item.RunID)
	}
	receiptID := ""
	if receipt != nil {
		receiptID = receipt.ReceiptID
	}
	sourceParts := append([]string{run.RunID, planOutput.OutputHash, receiptHash}, outcomeHashes...)
	sourceHash := contracts.HashBytes([]byte(strings.Join(sourceParts, "\x00")))
	caseID := stableID("case-", run.ProjectID, run.RunID)
	value := Case{
		CaseID: caseID, ProjectID: run.ProjectID, SourceRunID: run.RunID,
		PlanID: plan.PlanID, ReceiptID: receiptID, RequestFingerprint: plan.RequestFingerprint,
		BlueprintID: plan.BlueprintID, BlueprintVersion: plan.BlueprintVersion, BlueprintHash: plan.BlueprintHash,
		AdapterID: snapshot.CapabilitySet.AdapterID, Runtime: snapshot.CapabilitySet.Runtime, ProtocolVersion: snapshot.CapabilitySet.ProtocolVersion,
		TaskFeatures: map[string]string{"pipeline_id": run.PipelineID, "pipeline_version": run.PipelineVersion, "channel": run.Channel, "caller_type": run.CallerType, "run_status": run.Status},
		Delivery:     delivery, OutcomeRunIDs: unique(outcomeRunIDs), OutcomeMetrics: metrics, Cost: cost,
		EvaluationObjectIDs: []string{}, Result: "unknown", PrimaryFailureDimension: failure, SecondaryFailureDimensions: []string{},
		Diagnosis: diagnosis, TransferScope: unique([]string{snapshot.CapabilitySet.Runtime, plan.BlueprintID}),
		ExpiresAt: expiryFromRun(run), Sensitivity: "standard", SourceArtifactRefs: unique(append([]string{"run:" + run.RunID, "plan:" + plan.PlanID}, outcomeRunIDs...)),
		SourceHash: sourceHash, GeneratedAt: nowString(),
	}
	if receiptID != "" {
		value.SourceArtifactRefs = unique(append(value.SourceArtifactRefs, "receipt:"+receiptID))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return harness.Object{}, err
	}
	evidenceIDs, objectIDs := sourceRefs(plan)
	objectID := stableID("experience-case-", run.ProjectID, run.RunID)
	if current, getErr := s.harness.Object(ctx, objectID); getErr == nil {
		existing, decodeErr := decodeCase(current)
		if decodeErr != nil {
			return harness.Object{}, decodeErr
		}
		// Rebuildable only while the Case is still unevaluated. Once an
		// independent Evaluation exists, automatic discovery may not move the
		// evaluated candidate or an active governed Case.
		if existing.SourceHash == sourceHash || len(existing.EvaluationObjectIDs) > 0 || current.Status != "candidate" {
			return current, nil
		}
	} else if !errors.Is(getErr, sql.ErrNoRows) {
		return harness.Object{}, getErr
	}
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: objectID, TypeID: CaseTypeV1, ProjectID: run.ProjectID, Status: "candidate",
		Payload: raw, Confidence: 1, Importance: .7, ValidUntil: value.ExpiresAt,
		SourceEvidenceIDs: evidenceIDs, SourceObjectIDs: objectIDs, RunID: run.RunID, StageID: "experience.case.compile",
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "case:" + run.RunID + ":" + sourceHash,
	})
	if err != nil {
		return harness.Object{}, err
	}
	if err := s.refreshProjectionObject(ctx, object); err != nil {
		return harness.Object{}, err
	}
	return object, nil
}

func (s *Service) DiscoverCases(ctx context.Context, projectID string, limit int) ([]harness.Object, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}
	runs, err := s.harness.ListRuns(ctx, projectID, "", limit)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []harness.Object{}
	for _, run := range runs {
		if run.PipelineID != contextbridge.ContextRunPipelineID || !terminal(run.Status) {
			continue
		}
		object, err := s.CompileCaseFromRun(ctx, run.RunID)
		if err != nil {
			if strings.Contains(err.Error(), "not eligible for Experience auto-promotion") || errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, object)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
