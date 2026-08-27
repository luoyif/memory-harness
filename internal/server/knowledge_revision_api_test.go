package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func kuProposalBody(t *testing.T, projectID string, expected int, reason, key string, unit memory.KnowledgeUnit) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"project_id":        projectID,
		"expected_revision": expected,
		"edit_reason":       reason,
		"idempotency_key":   key,
		"knowledge_unit":    unit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func resolvedCorrection(base memory.KnowledgeUnit, unitType, statement, predicate, object, validFrom string) memory.KnowledgeUnit {
	candidate := base
	candidate.UnitType = unitType
	candidate.TierHint = "semantic"
	candidate.RiskTier = "B"
	candidate.Statement = statement
	candidate.Confidence = .93
	candidate.Structure.Attribution.SubjectRef = "project-local:system:memoryos"
	candidate.Structure.Attribution.SubjectSurface = "MemoryOS"
	candidate.Structure.Attribution.Resolution = "resolved"
	candidate.Structure.Attribution.OwnerMapping = "not_assumed"
	candidate.Structure.Frame.Subject = memory.EntityRef{
		EntityID: "project-local:system:memoryos", EntityType: "system", Surface: "MemoryOS",
		CanonicalName: "MemoryOS", Resolution: "resolved",
	}
	candidate.Structure.Frame.Predicate = predicate
	candidate.Structure.Frame.InverseLabel = "is_" + predicate + "_of"
	candidate.Structure.Frame.Object = memory.SemanticObject{Kind: "literal", Value: object}
	candidate.Structure.Temporal.ValidFrom = validFrom
	candidate.Structure.Temporal.ValidUntil = ""
	candidate.Structure.Temporal.Resolution = "resolved"
	candidate.Structure.Temporal.Precision = "day"
	candidate.Structure.Epistemic.Polarity = "positive"
	candidate.Structure.Epistemic.Modality = "asserted"
	candidate.Structure.Epistemic.Confidence = candidate.Confidence
	return candidate
}

func TestOwnerKnowledgeUnitRevisionIsReviewGatedAndIncrementallyReprojects(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "ku-revision", Name: "KU Revision", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-ku-revision", "meeting", "session-ku-revision", "user", "2026-08-22T02:00:00Z", "当前风险是旧方案可能延迟。"))
	if err != nil {
		t.Fatal(err)
	}
	beforeEvidence, err := a.Ledger.ReadEvidence(ctx, captured.EvidenceID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Execution.Status != "completed" {
		t.Fatalf("growth=%#v", result)
	}
	units, total, err := a.Memory.ListKnowledgeUnitsForProject(ctx, project.ProjectID, "", 20, 0)
	if err != nil || total != 1 || len(units) != 1 {
		t.Fatalf("units=%#v total=%d err=%v", units, total, err)
	}
	base := units[0]
	objectID := memory.StructuredKnowledgeObjectID(project.ProjectID, base.UnitID)
	object, err := a.Harness.Object(ctx, objectID)
	if err != nil {
		t.Fatal(err)
	}
	if object.TypeID != memory.StructuredKnowledgeUnitTypeV2 || object.CurrentRevision != 1 {
		t.Fatalf("structured KU object=%#v", object)
	}
	baseHash := object.Revision.ContentHash

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	first := resolvedCorrection(base, "risk", "MemoryOS 的旧发布路径存在延期风险。", "has_release_risk", "旧发布路径", "2026-09-01T00:00:00Z")
	var proposed struct {
		Review harness.RevisionReview `json:"review"`
	}
	postJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"/revision-proposals", kuProposalBody(t, project.ProjectID, 1, "纠正主体并补充明确有效时间", "ku-revision-first", first), http.StatusCreated, &proposed)
	if proposed.Review.Status != "pending" || proposed.Review.BaseRevision != 1 || proposed.Review.Revision != 2 {
		t.Fatalf("proposal=%#v", proposed.Review)
	}
	stillCurrent, err := a.Harness.Object(ctx, objectID)
	if err != nil {
		t.Fatal(err)
	}
	if stillCurrent.CurrentRevision != 1 || stillCurrent.Revision.ContentHash != baseHash {
		t.Fatalf("pending proposal moved current pointer: %#v", stillCurrent)
	}

	postJSON(t, ts.URL+"/v1/harness/revision-reviews/"+proposed.Review.ReviewID+"/decision", `{"decision":"approve","note":"source verified"}`, http.StatusOK, &map[string]any{})
	approved, err := a.Harness.Object(ctx, objectID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.CurrentRevision != 2 {
		t.Fatalf("approved current revision=%d", approved.CurrentRevision)
	}
	current, err := a.Memory.KnowledgeUnitForProject(ctx, project.ProjectID, base.UnitID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Statement != first.Statement || current.UnitType != "risk" {
		t.Fatalf("current KU did not overlay approved revision: %#v", current)
	}
	var firstAssertion string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT assertion_id FROM memory_assertions WHERE project_id=? AND unit_id=? AND status='active'`, project.ProjectID, base.UnitID).Scan(&firstAssertion); err != nil {
		t.Fatalf("approved correction produced no active assertion: %v", err)
	}
	var firstFactStatus string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT status FROM temporal_facts WHERE project_id=? AND predicate='has_release_risk'`, project.ProjectID).Scan(&firstFactStatus); err != nil || firstFactStatus != "active" {
		t.Fatalf("first temporal projection status=%q err=%v", firstFactStatus, err)
	}
	var autoRisk int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_risks WHERE project_id=? AND risk_id LIKE 'auto-risk-%'`, project.ProjectID).Scan(&autoRisk); err != nil || autoRisk != 1 {
		t.Fatalf("risk projection count=%d err=%v", autoRisk, err)
	}

	second := resolvedCorrection(current, "goal", "MemoryOS 的发布目标改为完成安全发布。", "targets_safe_release", "安全发布", "2026-09-15T00:00:00Z")
	var proposedSecond struct {
		Review harness.RevisionReview `json:"review"`
	}
	postJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"/revision-proposals", kuProposalBody(t, project.ProjectID, 2, "风险已经转化为明确发布目标", "ku-revision-second", second), http.StatusCreated, &proposedSecond)
	postJSON(t, ts.URL+"/v1/harness/revision-reviews/"+proposedSecond.Review.ReviewID+"/decision", `{"decision":"approve","note":"goal verified"}`, http.StatusOK, &map[string]any{})

	finalObject, err := a.Harness.Object(ctx, objectID)
	if err != nil || finalObject.CurrentRevision != 3 {
		t.Fatalf("final object=%#v err=%v", finalObject, err)
	}
	finalUnit, err := a.Memory.KnowledgeUnitForProject(ctx, project.ProjectID, base.UnitID)
	if err != nil || finalUnit.Statement != second.Statement || finalUnit.UnitType != "goal" {
		t.Fatalf("final unit=%#v err=%v", finalUnit, err)
	}
	var oldAssertionStatus string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT status FROM memory_assertions WHERE assertion_id=?`, firstAssertion).Scan(&oldAssertionStatus); err != nil || oldAssertionStatus != "superseded" {
		t.Fatalf("old assertion status=%q err=%v", oldAssertionStatus, err)
	}
	var oldFactStatus string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT status FROM temporal_facts WHERE project_id=? AND predicate='has_release_risk'`, project.ProjectID).Scan(&oldFactStatus); err != nil || oldFactStatus != "superseded" {
		t.Fatalf("old temporal fact status=%q err=%v", oldFactStatus, err)
	}
	var activeGoal, remainingRisk int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_goals WHERE project_id=? AND goal_id LIKE 'auto-goal-%'`, project.ProjectID).Scan(&activeGoal); err != nil {
		t.Fatal(err)
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_risks WHERE project_id=? AND risk_id LIKE 'auto-risk-%'`, project.ProjectID).Scan(&remainingRisk); err != nil {
		t.Fatal(err)
	}
	if activeGoal != 1 || remainingRisk != 0 {
		t.Fatalf("project projection did not switch risk->goal: goals=%d risks=%d", activeGoal, remainingRisk)
	}
	var projectedTarget string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT coalesce(target_at,'') FROM project_goals WHERE project_id=? AND goal_id LIKE 'auto-goal-%'`, project.ProjectID).Scan(&projectedTarget); err != nil {
		t.Fatal(err)
	}
	if projectedTarget != "2026-09-15T00:00:00Z" {
		t.Fatalf("confirmed temporal value did not reach goal target_at: %q", projectedTarget)
	}

	var legacyStatement, legacyType string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT statement,unit_type FROM knowledge_units WHERE unit_id=?`, base.UnitID).Scan(&legacyStatement, &legacyType); err != nil {
		t.Fatal(err)
	}
	if legacyStatement != base.Statement || legacyType != base.UnitType {
		t.Fatalf("legacy compatibility row was dual-written: %q/%q want %q/%q", legacyStatement, legacyType, base.Statement, base.UnitType)
	}
	afterEvidence, err := a.Ledger.ReadEvidence(ctx, captured.EvidenceID)
	if err != nil || !bytes.Equal(beforeEvidence, afterEvidence) {
		t.Fatalf("canonical Evidence changed err=%v", err)
	}

	// A stale optimistic-lock proposal must fail after revision 3 became current.
	postJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"/revision-proposals", kuProposalBody(t, project.ProjectID, 2, "stale edit", "ku-revision-stale", second), http.StatusConflict, &map[string]any{})

	// Provenance and identity cannot be rewritten through semantic correction.
	tampered := finalUnit
	tampered.EvidenceID = "ev-other"
	postJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"/revision-proposals", kuProposalBody(t, project.ProjectID, 3, "attempt source relink", "ku-revision-tamper", tampered), http.StatusBadRequest, &map[string]any{})

	var detail struct {
		KnowledgeUnit        memory.KnowledgeUnit `json:"knowledge_unit"`
		ObjectID             string               `json:"object_id"`
		LegacyProjectionOnly bool                 `json:"legacy_projection_only"`
	}
	getJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"?project_id="+project.ProjectID, &detail)
	if detail.LegacyProjectionOnly || detail.ObjectID != objectID || detail.KnowledgeUnit.Statement != second.Statement {
		t.Fatalf("detail endpoint did not expose current authority: %#v", detail)
	}
}
