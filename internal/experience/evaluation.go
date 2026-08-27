package experience

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
)

func validVerdict(value string) bool {
	return value == "pass" || value == "fail" || value == "unknown"
}

func normalizeEvaluationInput(input EvaluateInput) (EvaluateInput, error) {
	input.TargetKind = strings.ToLower(strings.TrimSpace(input.TargetKind))
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	input.EvaluatorID = strings.TrimSpace(input.EvaluatorID)
	input.EvaluatorVersion = strings.TrimSpace(input.EvaluatorVersion)
	input.Verdict = strings.ToLower(strings.TrimSpace(input.Verdict))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.TargetKind != "case" && input.TargetKind != "pattern" && input.TargetKind != "change_proposal" {
		return input, errors.New("target_kind must be case, pattern or change_proposal")
	}
	if input.TargetID == "" || input.Protocol == "" || input.EvaluatorID == "" || input.EvaluatorVersion == "" || input.IdempotencyKey == "" {
		return input, errors.New("target, protocol, evaluator identity and idempotency_key are required")
	}
	if !validVerdict(input.Verdict) {
		return input, errors.New("verdict must be pass, fail or unknown")
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return input, errors.New("confidence must be between 0 and 1")
	}
	if input.SampleSize <= 0 {
		input.SampleSize = 1
	}
	if len(input.Dimensions) == 0 {
		return input, errors.New("at least one evaluation dimension is required")
	}
	for index := range input.Dimensions {
		dimension := &input.Dimensions[index]
		dimension.Name = strings.TrimSpace(dimension.Name)
		dimension.Verdict = strings.ToLower(strings.TrimSpace(dimension.Verdict))
		if dimension.Name == "" || !validVerdict(dimension.Verdict) {
			return input, fmt.Errorf("dimensions[%d] requires name and pass/fail/unknown verdict", index)
		}
		if dimension.Confidence < 0 || dimension.Confidence > 1 {
			return input, fmt.Errorf("dimensions[%d].confidence must be between 0 and 1", index)
		}
	}
	return input, nil
}

func aggregateEvaluations(values []Evaluation) (string, string, []string) {
	if len(values) == 0 {
		return "unknown", "", []string{}
	}
	verdict := "pass"
	failures := []string{}
	for _, value := range values {
		if value.Verdict == "fail" {
			verdict = "fail"
		} else if value.Verdict == "unknown" && verdict != "fail" {
			verdict = "unknown"
		}
		for _, dimension := range value.Dimensions {
			if dimension.Verdict == "fail" {
				failures = append(failures, dimension.Name)
			}
		}
	}
	failures = unique(failures)
	primary := ""
	secondary := []string{}
	if len(failures) > 0 {
		primary = failures[0]
		secondary = append(secondary, failures[1:]...)
	}
	return verdict, primary, secondary
}

func (s *Service) EvaluationsForTarget(ctx context.Context, projectID, targetID string) ([]harness.Object, []Evaluation, error) {
	objects, err := s.ListEvaluations(ctx, projectID, 500)
	if err != nil {
		return nil, nil, err
	}
	matchedObjects := []harness.Object{}
	values := []Evaluation{}
	for _, object := range objects {
		value, err := decodeEvaluation(object)
		if err != nil {
			return nil, nil, err
		}
		if value.TargetID != targetID {
			continue
		}
		matchedObjects = append(matchedObjects, object)
		values = append(values, value)
	}
	sort.SliceStable(values, func(i, j int) bool { return values[i].EvaluatedAt < values[j].EvaluatedAt })
	return matchedObjects, values, nil
}

func (s *Service) targetForEvaluation(ctx context.Context, input EvaluateInput) (harness.Object, []string, []string, error) {
	object, err := s.harness.Object(ctx, input.TargetID)
	if err != nil {
		return harness.Object{}, nil, nil, err
	}
	expectedType := CaseTypeV1
	if input.TargetKind == "pattern" {
		expectedType = PatternTypeV1
	}
	if object.TypeID != expectedType {
		return harness.Object{}, nil, nil, errors.New("evaluation target kind does not match object type")
	}
	runIDs, outcomeIDs := []string{}, []string{}
	if input.TargetKind == "case" {
		value, err := decodeCase(object)
		if err != nil {
			return harness.Object{}, nil, nil, err
		}
		runIDs = append(runIDs, value.SourceRunID)
		outcomeIDs = append(outcomeIDs, value.OutcomeRunIDs...)
	}
	return object, unique(runIDs), unique(outcomeIDs), nil
}
func (s *Service) Evaluate(ctx context.Context, input EvaluateInput) (harness.Object, error) {
	input, err := normalizeEvaluationInput(input)
	if err != nil {
		return harness.Object{}, err
	}
	target, sourceRuns, sourceOutcomes, err := s.targetForEvaluation(ctx, input)
	if err != nil {
		return harness.Object{}, err
	}
	evaluatedAt := nowString()
	evaluationID := stableID("evaluation-", target.ProjectID, input.TargetKind, target.ObjectID, input.IdempotencyKey)
	value := Evaluation{
		EvaluationID: evaluationID, ProjectID: target.ProjectID, TargetKind: input.TargetKind, TargetID: target.ObjectID,
		Protocol: input.Protocol, EvaluatorID: input.EvaluatorID, EvaluatorVersion: input.EvaluatorVersion,
		Verdict: input.Verdict, Dimensions: input.Dimensions, Expected: strings.TrimSpace(input.Expected), Observed: strings.TrimSpace(input.Observed),
		Confidence: input.Confidence, SampleSize: input.SampleSize, BaselineRef: strings.TrimSpace(input.BaselineRef), ChallengerRef: strings.TrimSpace(input.ChallengerRef),
		SourceRunIDs: sourceRuns, SourceOutcomeRunIDs: sourceOutcomes, Notes: strings.TrimSpace(input.Notes), EvaluatedAt: evaluatedAt,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return harness.Object{}, err
	}
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: evaluationID, TypeID: EvaluationTypeV1, ProjectID: target.ProjectID, Status: "active",
		Payload: raw, Confidence: max(.01, input.Confidence), Importance: .75,
		SourceObjectIDs: []string{target.ObjectID}, PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: "evaluation:" + target.ObjectID + ":" + input.IdempotencyKey,
	})
	if err != nil {
		return harness.Object{}, err
	}
	if err := s.refreshProjectionObject(ctx, object); err != nil {
		return harness.Object{}, err
	}
	if target.Status == "candidate" {
		if err := s.attachEvaluationToCandidate(ctx, target, object.ObjectID); err != nil {
			return harness.Object{}, err
		}
	}
	return object, nil
}

// EvaluateGovernedTarget records an independent Evaluation for a governed object
// owned by another subsystem. It never mutates the target object; the owning
// subsystem decides how the Evaluation participates in its review lifecycle.
func (s *Service) EvaluateGovernedTarget(ctx context.Context, input EvaluateInput, expectedType string, sourceRunIDs, sourceOutcomeRunIDs []string) (harness.Object, error) {
	input, err := normalizeEvaluationInput(input)
	if err != nil {
		return harness.Object{}, err
	}
	if input.TargetKind != "change_proposal" {
		return harness.Object{}, errors.New("external Evaluation target_kind is not supported")
	}
	target, err := s.harness.Object(ctx, input.TargetID)
	if err != nil {
		return harness.Object{}, err
	}
	if target.TypeID != strings.TrimSpace(expectedType) {
		return harness.Object{}, errors.New("evaluation target kind does not match governed object type")
	}
	evaluatedAt := nowString()
	evaluationID := stableID("evaluation-", target.ProjectID, input.TargetKind, target.ObjectID, input.IdempotencyKey)
	value := Evaluation{
		EvaluationID: evaluationID, ProjectID: target.ProjectID, TargetKind: input.TargetKind, TargetID: target.ObjectID,
		Protocol: input.Protocol, EvaluatorID: input.EvaluatorID, EvaluatorVersion: input.EvaluatorVersion,
		Verdict: input.Verdict, Dimensions: input.Dimensions, Expected: strings.TrimSpace(input.Expected), Observed: strings.TrimSpace(input.Observed),
		Confidence: input.Confidence, SampleSize: input.SampleSize, BaselineRef: strings.TrimSpace(input.BaselineRef), ChallengerRef: strings.TrimSpace(input.ChallengerRef),
		SourceRunIDs: unique(sourceRunIDs), SourceOutcomeRunIDs: unique(sourceOutcomeRunIDs), Notes: strings.TrimSpace(input.Notes), EvaluatedAt: evaluatedAt,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return harness.Object{}, err
	}
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: evaluationID, TypeID: EvaluationTypeV2, ProjectID: target.ProjectID, Status: "active",
		Payload: raw, Confidence: max(.01, input.Confidence), Importance: .75,
		SourceObjectIDs: []string{target.ObjectID}, PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: "evaluation:" + target.ObjectID + ":" + input.IdempotencyKey,
	})
	if err != nil {
		return harness.Object{}, err
	}
	if err := s.refreshProjectionObject(ctx, object); err != nil {
		return harness.Object{}, err
	}
	return object, nil
}

func (s *Service) attachEvaluationToCandidate(ctx context.Context, target harness.Object, evaluationID string) error {
	evaluationObjects, evaluations, err := s.EvaluationsForTarget(ctx, target.ProjectID, target.ObjectID)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, object := range evaluationObjects {
		ids = append(ids, object.ObjectID)
	}
	ids = unique(append(ids, evaluationID))
	if target.TypeID == CaseTypeV1 {
		value, err := decodeCase(target)
		if err != nil {
			return err
		}
		value.EvaluationObjectIDs = ids
		value.Result, value.PrimaryFailureDimension, value.SecondaryFailureDimensions = aggregateEvaluations(evaluations)
		if value.Result == "fail" && strings.TrimSpace(value.Diagnosis) == "" {
			value.Diagnosis = "Independent Evaluation recorded a task failure; causal diagnosis remains a separate claim."
		}
		return s.rewriteCandidate(ctx, target, value, ids)
	}
	if target.TypeID == PatternTypeV1 {
		value, err := decodePattern(target)
		if err != nil {
			return err
		}
		value.EvaluationObjectIDs = ids
		if len(evaluations) > 0 {
			value.LastValidated = evaluations[len(evaluations)-1].EvaluatedAt
		}
		return s.rewriteCandidate(ctx, target, value, ids)
	}
	return errors.New("unsupported evaluation target")
}

func (s *Service) rewriteCandidate(ctx context.Context, target harness.Object, payload any, evaluationIDs []string) error {
	if target.Status != "candidate" {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: target.ObjectID, TypeID: target.TypeID, ProjectID: target.ProjectID, Status: "candidate",
		Payload: raw, Confidence: target.Revision.Confidence, Importance: target.Revision.Importance,
		ValidFrom: target.Revision.ValidFrom, ValidUntil: target.Revision.ValidUntil,
		SourceEvidenceIDs: target.Revision.SourceEvidenceIDs, SourceObjectIDs: unique(append(target.Revision.SourceObjectIDs, evaluationIDs...)),
		RunID: target.Revision.RunID, StageID: "experience.evaluation.attach", PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: "attach-evaluation:" + target.ObjectID + ":" + contracts.HashBytes(raw),
	})
	if err != nil {
		return err
	}
	return s.refreshProjectionObject(ctx, object)
}
func (s *Service) ProposeActivation(ctx context.Context, objectID string, input ActivationInput) (harness.RevisionReview, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.TypeID != CaseTypeV1 && object.TypeID != PatternTypeV1 {
		return harness.RevisionReview{}, errors.New("only Experience Case or Pattern can be activated here")
	}
	if object.Status != "candidate" {
		return harness.RevisionReview{}, errors.New("only candidate Experience objects can request activation")
	}
	input.EditReason = strings.TrimSpace(input.EditReason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ExpectedRevision <= 0 || input.EditReason == "" || input.IdempotencyKey == "" {
		return harness.RevisionReview{}, errors.New("expected_revision, edit_reason and idempotency_key are required")
	}
	evaluationObjects, evaluations, err := s.EvaluationsForTarget(ctx, object.ProjectID, object.ObjectID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if len(evaluations) == 0 {
		return harness.RevisionReview{}, errors.New("Experience activation requires at least one independent Evaluation")
	}
	evaluationIDs := []string{}
	for _, item := range evaluationObjects {
		evaluationIDs = append(evaluationIDs, item.ObjectID)
	}
	var payload any
	verdict, primary, secondary := aggregateEvaluations(evaluations)
	if object.TypeID == CaseTypeV1 {
		value, err := decodeCase(object)
		if err != nil {
			return harness.RevisionReview{}, err
		}
		value.EvaluationObjectIDs = unique(evaluationIDs)
		value.Result, value.PrimaryFailureDimension, value.SecondaryFailureDimensions = verdict, primary, secondary
		payload = value
	} else {
		value, err := decodePattern(object)
		if err != nil {
			return harness.RevisionReview{}, err
		}
		value.EvaluationObjectIDs = unique(evaluationIDs)
		value.LastValidated = evaluations[len(evaluations)-1].EvaluatedAt
		payload = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	validation, _ := json.Marshal(map[string]any{
		"status": "passed", "kind": "experience_evaluated", "evaluation_object_ids": unique(evaluationIDs), "aggregate_verdict": verdict,
	})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: raw, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "active",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance, ValidFrom: object.Revision.ValidFrom, ValidUntil: object.Revision.ValidUntil,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: unique(append(object.Revision.SourceObjectIDs, evaluationIDs...)),
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: input.IdempotencyKey, RequestedBy: "owner", Validation: validation,
	})
}
