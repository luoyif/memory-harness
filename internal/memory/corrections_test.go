package memory_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestCorrectionImpactTracesEntityBackToAuthorityAndEvidence(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "correction-impact", Name: "Correction Impact", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-impact", "meeting", "session-impact", "user", "2026-08-22T08:00:00Z", "项目目标是九月完成安全上线。当前风险是旧接口可能阻塞发布。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	units, _, err := a.Memory.ListKnowledgeUnitsForProject(ctx, project.ProjectID, "", 20, 0)
	if err != nil || len(units) == 0 {
		t.Fatalf("units=%#v err=%v", units, err)
	}
	unit := units[0]
	unit.SchemaVersion = memory.KnowledgeUnitSchemaV2
	unit.Structure.Attribution.SubjectSurface = "MemoryOS"
	unit.Structure.Attribution.Resolution = "resolved"
	unit.Structure.Attribution.OwnerMapping = "not_assumed"
	unit.Structure.Frame.Subject = memory.EntityRef{EntityType: "project", Surface: "MemoryOS", CanonicalName: "MemoryOS", Resolution: "resolved"}
	unit.Structure.Frame.Predicate = "targets"
	unit.Structure.Frame.Object = memory.SemanticObject{Kind: "literal", Value: "时间能力"}
	unit.Structure.Temporal.Resolution = "relative_pending"
	if _, err := a.Memory.ReplaceKnowledgeUnitProjection(ctx, project.ProjectID, unit); err != nil {
		t.Fatal(err)
	}
	var entityID string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT entity_id FROM memory_entities WHERE project_id=? AND canonical_name='MemoryOS'`, project.ProjectID).Scan(&entityID); err != nil {
		t.Fatal(err)
	}
	impact, err := a.Memory.PreviewCorrection(ctx, project.ProjectID, "entity", entityID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.Label != "MemoryOS" || len(impact.Impact.UnitIDs) == 0 || len(impact.Impact.AssertionIDs) == 0 || len(impact.Impact.EvidenceIDs) == 0 {
		t.Fatalf("impact=%#v", impact)
	}
	if impact.Impact.EvidenceIDs[0] != captured.EvidenceID {
		t.Fatalf("evidence=%#v", impact.Impact.EvidenceIDs)
	}
}
