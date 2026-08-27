package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/store"
)

const SearchKind = "experience"

func marshalMetadata(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func experienceDocument(object harness.Object) (store.IndexedDocument, error) {
	title, body := "Experience", ""
	metadata := map[string]any{"type_id": object.TypeID, "content_hash": object.Revision.ContentHash, "revision": object.CurrentRevision}
	switch object.TypeID {
	case CaseTypeV1:
		value, err := decodeCase(object)
		if err != nil {
			return store.IndexedDocument{}, err
		}
		title = "Experience Case · " + value.Result
		if value.PrimaryFailureDimension != "" {
			title += " · " + value.PrimaryFailureDimension
		}
		body = strings.Join([]string{value.Diagnosis, value.CounterfactualHypothesis, strings.Join(value.SecondaryFailureDimensions, " "), strings.Join(value.TransferScope, " ")}, "\n")
		metadata["experience_kind"], metadata["result"], metadata["source_run_id"] = "case", value.Result, value.SourceRunID
	case PatternTypeV1:
		value, err := decodePattern(object)
		if err != nil {
			return store.IndexedDocument{}, err
		}
		title = value.NormalizedPattern
		body = strings.Join([]string{value.ExpectedEffect, strings.Join(value.Conditions, "\n"), strings.Join(value.TargetComponents, " "), strings.Join(value.KnownRegressions, "\n"), strings.Join(value.NegativeDomains, "\n")}, "\n")
		metadata["experience_kind"], metadata["support_count"], metadata["counterexample_count"] = "pattern", len(value.SupportingCaseIDs), len(value.CounterexampleCaseIDs)
	case EvaluationTypeV1, EvaluationTypeV2:
		value, err := decodeEvaluation(object)
		if err != nil {
			return store.IndexedDocument{}, err
		}
		title = fmt.Sprintf("Evaluation · %s · %s", value.TargetKind, value.Verdict)
		parts := []string{value.Expected, value.Observed, value.Notes}
		for _, dimension := range value.Dimensions {
			parts = append(parts, dimension.Name+" "+dimension.Verdict+" "+dimension.Note)
		}
		body = strings.Join(parts, "\n")
		metadata["experience_kind"], metadata["verdict"], metadata["target_id"] = "evaluation", value.Verdict, value.TargetID
	default:
		return store.IndexedDocument{}, fmt.Errorf("unsupported Experience type %s", object.TypeID)
	}
	return store.IndexedDocument{
		DocKey: "experience:" + object.ObjectID, Kind: SearchKind, SourceID: object.ObjectID, ProjectID: object.ProjectID,
		Title: strings.TrimSpace(title), Body: strings.TrimSpace(body), Status: object.Status, ObservedAt: object.UpdatedAt,
		ValidFrom: object.Revision.ValidFrom, ValidUntil: object.Revision.ValidUntil, MetadataJSON: marshalMetadata(metadata),
	}, nil
}

func (s *Service) refreshProjectionObject(ctx context.Context, object harness.Object) error {
	if s.search == nil {
		return nil
	}
	doc, err := experienceDocument(object)
	if err != nil {
		return err
	}
	return s.search.UpsertDocument(ctx, doc)
}

func (s *Service) RebuildProjection(ctx context.Context, projectID string) (int, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, fmt.Errorf("project_id is required")
	}
	if s.search == nil {
		return 0, fmt.Errorf("Experience search projection is unavailable")
	}
	if err := s.search.DeleteDocumentsByKindAndProject(ctx, projectID, SearchKind); err != nil {
		return 0, err
	}
	count := 0
	for _, typeID := range []string{CaseTypeV1, PatternTypeV1, EvaluationTypeV1, EvaluationTypeV2} {
		items, err := s.harness.ListObjects(ctx, projectID, typeID, "", 1000)
		if err != nil {
			return count, err
		}
		for _, object := range items {
			if err := s.refreshProjectionObject(ctx, object); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
