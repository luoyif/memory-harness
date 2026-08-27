package experience

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control *store.ControlStore
	harness *harness.Service
	search  *store.SearchStore
}

func New(control *store.ControlStore, harnessService *harness.Service, searchStore *store.SearchStore) *Service {
	return &Service{control: control, harness: harnessService, search: searchStore}
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func stableID(prefix string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strings.TrimSpace(value))
	}
	return prefix + contracts.HashBytes([]byte(strings.Join(parts, "\x00")))[:24]
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

func terminal(status string) bool {
	switch status {
	case "completed", "completed_with_warnings", "failed", "denied", "cancelled":
		return true
	default:
		return false
	}
}
func (s *Service) listObjects(ctx context.Context, projectID, typeID, status string, limit int) ([]harness.Object, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}
	return s.harness.ListObjects(ctx, projectID, typeID, strings.TrimSpace(status), limit)
}

func (s *Service) ListCases(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.listObjects(ctx, projectID, CaseTypeV1, status, limit)
}

func (s *Service) ListEvaluations(ctx context.Context, projectID string, limit int) ([]harness.Object, error) {
	v1, err := s.listObjects(ctx, projectID, EvaluationTypeV1, "active", limit)
	if err != nil {
		return nil, err
	}
	v2, err := s.listObjects(ctx, projectID, EvaluationTypeV2, "active", limit)
	if err != nil {
		return nil, err
	}
	items := append(v1, v2...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *Service) ListPatterns(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.listObjects(ctx, projectID, PatternTypeV1, status, limit)
}

func decodeCase(object harness.Object) (Case, error) {
	var value Case
	if object.TypeID != CaseTypeV1 {
		return value, errors.New("object is not an Experience Case")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}
func decodeEvaluation(object harness.Object) (Evaluation, error) {
	var value Evaluation
	if object.TypeID != EvaluationTypeV1 && object.TypeID != EvaluationTypeV2 {
		return value, errors.New("object is not an Experience Evaluation")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func decodePattern(object harness.Object) (Pattern, error) {
	var value Pattern
	if object.TypeID != PatternTypeV1 {
		return value, errors.New("object is not an Experience Pattern")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func (s *Service) Case(ctx context.Context, objectID string) (harness.Object, Case, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.Object{}, Case{}, err
	}
	value, err := decodeCase(object)
	return object, value, err
}

func (s *Service) Pattern(ctx context.Context, objectID string) (harness.Object, Pattern, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.Object{}, Pattern{}, err
	}
	value, err := decodePattern(object)
	return object, value, err
}
