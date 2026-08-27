package server_test

import (
	"bytes"
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

func TestEntityRenameCreatesReviewGatedKURevisionsAndSupersedesOrphan(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "entity-rename", Name: "Entity Rename", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-entity-rename", "meeting", "session-entity-rename", "user", "2026-08-22T08:00:00Z", "项目目标是九月完成安全上线。"))
	if err != nil {
		t.Fatal(err)
	}
	beforeEvidence, err := a.Ledger.ReadEvidence(ctx, captured.EvidenceID)
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
	base := units[0]
	resolved := resolvedCorrection(base, "goal", "MemoryOS 的发布目标是完成安全上线。", "targets_safe_release", "安全上线", "2026-09-01T00:00:00Z")

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var first struct {
		Review harness.RevisionReview `json:"review"`
	}
	postJSON(t, ts.URL+"/v1/knowledge-units/"+base.UnitID+"/revision-proposals", kuProposalBody(t, project.ProjectID, 1, "确认主体为 MemoryOS", "entity-rename-seed", resolved), http.StatusCreated, &first)
	postJSON(t, ts.URL+"/v1/harness/revision-reviews/"+first.Review.ReviewID+"/decision", `{"decision":"approve","note":"seed entity"}`, http.StatusOK, &map[string]any{})
	var sourceEntityID string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT entity_id FROM memory_entities WHERE project_id=? AND canonical_name='MemoryOS' AND status='active'`, project.ProjectID).Scan(&sourceEntityID); err != nil {
		t.Fatal(err)
	}

	var batch struct {
		BatchID string                   `json:"batch_id"`
		Reviews []harness.RevisionReview `json:"reviews"`
		Impact  memory.CorrectionImpact  `json:"impact"`
	}
	postJSON(t, ts.URL+"/v1/entities/"+sourceEntityID+"/revision-proposals", `{"project_id":"`+project.ProjectID+`","action":"rename","canonical_name":"Memory Harness","entity_type":"system","edit_reason":"统一项目的规范名称","idempotency_key":"entity-rename-batch"}`, http.StatusCreated, &batch)
	if batch.BatchID == "" || len(batch.Reviews) != 1 || batch.Reviews[0].Status != "pending" || len(batch.Impact.UnitIDs) != 1 {
		t.Fatalf("batch=%#v", batch)
	}
	var oldStatus string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT status FROM memory_entities WHERE entity_id=?`, sourceEntityID).Scan(&oldStatus); err != nil || oldStatus != "active" {
		t.Fatalf("old before approval=%q err=%v", oldStatus, err)
	}
	current, err := a.Memory.KnowledgeUnitForProject(ctx, project.ProjectID, base.UnitID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Structure.Frame.Subject.CanonicalName != "MemoryOS" {
		t.Fatalf("pending proposal changed current: %#v", current.Structure.Frame.Subject)
	}

	postJSON(t, ts.URL+"/v1/harness/revision-reviews/"+batch.Reviews[0].ReviewID+"/decision", `{"decision":"approve","note":"rename verified"}`, http.StatusOK, &map[string]any{})
	current, err = a.Memory.KnowledgeUnitForProject(ctx, project.ProjectID, base.UnitID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Structure.Frame.Subject.CanonicalName != "Memory Harness" {
		t.Fatalf("rename not current: %#v", current.Structure.Frame.Subject)
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT status FROM memory_entities WHERE entity_id=?`, sourceEntityID).Scan(&oldStatus); err != nil || oldStatus != "superseded" {
		t.Fatalf("old entity=%q err=%v", oldStatus, err)
	}
	var newCount int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_entities WHERE project_id=? AND canonical_name='Memory Harness' AND status='active'`, project.ProjectID).Scan(&newCount); err != nil || newCount != 1 {
		t.Fatalf("new entity count=%d err=%v", newCount, err)
	}
	afterEvidence, err := a.Ledger.ReadEvidence(ctx, captured.EvidenceID)
	if err != nil || !bytes.Equal(beforeEvidence, afterEvidence) {
		t.Fatalf("Evidence changed err=%v", err)
	}
}
