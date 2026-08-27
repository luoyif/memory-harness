package experience

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/harness"
)

func (s *Service) governedCases(ctx context.Context, projectID string, ids []string) ([]harness.Object, []Case, error) {
	ids = unique(ids)
	objects := make([]harness.Object, 0, len(ids))
	values := make([]Case, 0, len(ids))
	for _, id := range ids {
		object, value, err := s.Case(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if object.ProjectID != projectID {
			return nil, nil, errors.New("Experience Pattern cannot cross project boundaries")
		}
		if object.Status != "active" {
			return nil, nil, fmt.Errorf("case %s must be active before it can support a Pattern", id)
		}
		objects = append(objects, object)
		values = append(values, value)
	}
	return objects, values, nil
}
func (s *Service) CreatePattern(ctx context.Context, input PatternInput) (harness.Object, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.NormalizedPattern = strings.TrimSpace(input.NormalizedPattern)
	input.ExpectedEffect = strings.TrimSpace(input.ExpectedEffect)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SupportingCaseIDs = unique(input.SupportingCaseIDs)
	input.CounterexampleCaseIDs = unique(input.CounterexampleCaseIDs)
	input.TargetComponents = unique(input.TargetComponents)
	input.Conditions = unique(input.Conditions)
	input.KnownRegressions = unique(input.KnownRegressions)
	input.NegativeDomains = unique(input.NegativeDomains)
	if input.ProjectID == "" || input.NormalizedPattern == "" || input.ExpectedEffect == "" || input.IdempotencyKey == "" {
		return harness.Object{}, errors.New("project_id, normalized_pattern, expected_effect and idempotency_key are required")
	}
	if len(input.SupportingCaseIDs) < 2 {
		return harness.Object{}, errors.New("Pattern requires at least two active supporting Cases")
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return harness.Object{}, errors.New("confidence must be between 0 and 1")
	}
	supportObjects, _, err := s.governedCases(ctx, input.ProjectID, input.SupportingCaseIDs)
	if err != nil {
		return harness.Object{}, err
	}
	counterObjects, _, err := s.governedCases(ctx, input.ProjectID, input.CounterexampleCaseIDs)
	if err != nil {
		return harness.Object{}, err
	}
	supportSet := map[string]bool{}
	for _, id := range input.SupportingCaseIDs {
		supportSet[id] = true
	}
	for _, id := range input.CounterexampleCaseIDs {
		if supportSet[id] {
			return harness.Object{}, errors.New("a Case cannot be both support and counterexample for one Pattern")
		}
	}
	patternID := stableID("pattern-", input.ProjectID, input.IdempotencyKey)
	value := Pattern{
		PatternID: patternID, ProjectID: input.ProjectID, NormalizedPattern: input.NormalizedPattern,
		SupportingCaseIDs: input.SupportingCaseIDs, CounterexampleCaseIDs: input.CounterexampleCaseIDs,
		TargetComponents: input.TargetComponents, Conditions: input.Conditions, ExpectedEffect: input.ExpectedEffect,
		Confidence: input.Confidence, SampleSize: len(input.SupportingCaseIDs) + len(input.CounterexampleCaseIDs),
		EvaluationObjectIDs: []string{}, KnownRegressions: input.KnownRegressions, NegativeDomains: input.NegativeDomains,
		GeneratedAt: nowString(),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return harness.Object{}, err
	}
	sources := []string{}
	for _, object := range append(supportObjects, counterObjects...) {
		sources = append(sources, object.ObjectID)
	}
	object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: patternID, TypeID: PatternTypeV1, ProjectID: input.ProjectID, Status: "candidate",
		Payload: raw, Confidence: max(.01, input.Confidence), Importance: .75,
		SourceObjectIDs: unique(sources), PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: "pattern:" + input.IdempotencyKey,
	})
	if err != nil {
		return harness.Object{}, err
	}
	if err := s.refreshProjectionObject(ctx, object); err != nil {
		return harness.Object{}, err
	}
	return object, nil
}
