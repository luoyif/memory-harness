package reprojection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

type Service struct {
	memory    *memory.Engine
	portfolio *portfolio.Service
}

type Result struct {
	ProjectID       string                          `json:"project_id"`
	ObjectID        string                          `json:"object_id"`
	TypeID          string                          `json:"type_id"`
	KnowledgeUnitID string                          `json:"knowledge_unit_id,omitempty"`
	MemoryID        string                          `json:"memory_id,omitempty"`
	Semantic        *memory.SemanticMaterialization `json:"semantic,omitempty"`
	Project         *portfolio.DerivedProjection    `json:"project,omitempty"`
	Indexed         int                             `json:"indexed"`
}

func New(memoryEngine *memory.Engine, portfolioService *portfolio.Service) *Service {
	return &Service{memory: memoryEngine, portfolio: portfolioService}
}

// RefreshApprovedObject updates only rebuildable projections after the Object
// Store current pointer has already moved. It never edits Evidence or an Object
// Revision and therefore can be safely retried after a projection failure.
func (s *Service) RefreshApprovedObject(ctx context.Context, object harness.Object) (Result, error) {
	result := Result{ProjectID: object.ProjectID, ObjectID: object.ObjectID, TypeID: object.TypeID}
	switch object.TypeID {
	case memory.StructuredKnowledgeUnitTypeV2:
		unit, err := memory.DecodeStructuredKnowledgePayload(object.Revision.Payload)
		if err != nil {
			return result, fmt.Errorf("decode knowledge authority: %w", err)
		}
		semantic, err := s.memory.ReplaceKnowledgeUnitProjection(ctx, object.ProjectID, unit)
		if err != nil {
			return result, fmt.Errorf("semantic/temporal projection: %w", err)
		}
		project, err := s.portfolio.SyncDerivedFromEpisode(ctx, object.ProjectID, unit.EpisodeID)
		if err != nil {
			return result, fmt.Errorf("project projection: %w", err)
		}
		result.KnowledgeUnitID = unit.UnitID
		result.Semantic = &semantic
		result.Project = &project
	case memory.StructuredMemoryRecordTypeV1:
		item, err := memory.DecodeStructuredMemoryPayload(object.Revision.Payload)
		if err != nil {
			return result, fmt.Errorf("decode memory authority: %w", err)
		}
		result.MemoryID = item.MemoryID
		if _, err := s.memory.RebuildLivingViewsForProject(ctx, object.ProjectID); err != nil {
			return result, fmt.Errorf("living projection: %w", err)
		}
	default:
		// Other Object types may have their own projections. The common search
		// refresh below is still safe and keeps the generic object index current.
		if len(object.Revision.Payload) == 0 || !json.Valid(object.Revision.Payload) {
			return result, fmt.Errorf("current object payload is invalid JSON")
		}
	}
	indexed, err := s.portfolio.RebuildProjectIndex(ctx, object.ProjectID)
	if err != nil {
		return result, fmt.Errorf("project recall projection: %w", err)
	}
	result.Indexed = indexed
	return result, nil
}
