package server_test

import (
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

func memoryProposalBody(t *testing.T, projectID string, expected int, reason, key string, item memory.MemoryRecord) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"project_id": projectID, "expected_revision": expected, "edit_reason": reason,
		"idempotency_key": key, "memory": item,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestOwnerMemoryRevisionIsAuthorityAndSearchReplacesLegacyFallback(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "memory-authority", Name: "Memory Authority", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-memory-authority", "meeting", "session-memory-authority", "user", "2026-08-22T04:00:00Z", "MemoryOS 当前采用旧检索描述，后续需要人工纠正。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	items, total, err := a.Memory.ListMemoriesForProject(ctx, project.ProjectID, "", "", 20, 0)
	if err != nil || total == 0 || len(items) == 0 {
		t.Fatalf("memories=%#v total=%d err=%v", items, total, err)
	}
	base := items[0]
	objectID := memory.StructuredMemoryObjectID(project.ProjectID, base.MemoryID)
	object, err := a.Harness.Object(ctx, objectID)
	if err != nil || object.CurrentRevision != 1 {
		t.Fatalf("base authority=%#v err=%v", object, err)
	}
	baseHash := object.Revision.ContentHash
	var legacySummary, legacyBody string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT summary,body FROM memory_records WHERE memory_id=?`, base.MemoryID).Scan(&legacySummary, &legacyBody); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	candidate := base
	candidate.Summary = "人工确认后的权威检索记忆"
	candidate.Body = "MemoryOS 的权威长期记忆必须只以当前受审核 Revision 进入统一检索。"
	candidate.Confidence = .97
	candidate.Importance = .91
	var proposed struct {
		Review harness.RevisionReview `json:"review"`
	}
	postJSON(t, ts.URL+"/v1/memories/"+base.MemoryID+"/revision-proposals", memoryProposalBody(t, project.ProjectID, 1, "Owner 对照 Evidence 后纠正长期记忆表述", "memory-authority-r2", candidate), http.StatusCreated, &proposed)
	if proposed.Review.Status != "pending" || proposed.Review.BaseRevision != 1 || proposed.Review.Revision != 2 {
		t.Fatalf("proposal=%#v", proposed.Review)
	}
	stillCurrent, err := a.Harness.Object(ctx, objectID)
	if err != nil || stillCurrent.CurrentRevision != 1 || stillCurrent.Revision.ContentHash != baseHash {
		t.Fatalf("pending proposal changed current=%#v err=%v", stillCurrent, err)
	}

	postJSON(t, ts.URL+"/v1/harness/revision-reviews/"+proposed.Review.ReviewID+"/decision", `{"decision":"approve","note":"evidence checked"}`, http.StatusOK, &map[string]any{})
	approved, err := a.Harness.Object(ctx, objectID)
	if err != nil || approved.CurrentRevision != 2 {
		t.Fatalf("approved authority=%#v err=%v", approved, err)
	}
	current, err := a.Memory.MemoryForProject(ctx, project.ProjectID, base.MemoryID)
	if err != nil || current.Summary != candidate.Summary || current.Body != candidate.Body {
		t.Fatalf("authority overlay=%#v err=%v", current, err)
	}
	var legacySummaryAfter, legacyBodyAfter string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT summary,body FROM memory_records WHERE memory_id=?`, base.MemoryID).Scan(&legacySummaryAfter, &legacyBodyAfter); err != nil {
		t.Fatal(err)
	}
	if legacySummaryAfter != legacySummary || legacyBodyAfter != legacyBody {
		t.Fatalf("legacy compatibility row was dual-written: before=%q/%q after=%q/%q", legacySummary, legacyBody, legacySummaryAfter, legacyBodyAfter)
	}

	var docs int
	var indexedTitle, indexedBody string
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*),max(title),max(body) FROM documents WHERE doc_key=?`, "memory:"+base.MemoryID).Scan(&docs, &indexedTitle, &indexedBody); err != nil {
		t.Fatal(err)
	}
	if docs != 1 || indexedTitle != candidate.Summary || indexedBody != candidate.Body {
		t.Fatalf("authority search doc count=%d title=%q body=%q", docs, indexedTitle, indexedBody)
	}
	var genericObjectDocs int
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE doc_key=?`, "object:"+objectID).Scan(&genericObjectDocs); err != nil {
		t.Fatal(err)
	}
	if genericObjectDocs != 0 {
		t.Fatalf("authority memory was duplicated as generic object: %d", genericObjectDocs)
	}

	var detail struct {
		Memory               memory.MemoryRecord `json:"memory"`
		ObjectID             string              `json:"object_id"`
		LegacyProjectionOnly bool                `json:"legacy_projection_only"`
		Governance           harness.Object      `json:"governance"`
	}
	getJSON(t, ts.URL+"/v1/memories/"+base.MemoryID+"/governance?project_id="+project.ProjectID, &detail)
	if detail.LegacyProjectionOnly || detail.ObjectID != objectID || detail.Memory.Body != candidate.Body || detail.Governance.CurrentRevision != 2 {
		t.Fatalf("governance detail=%#v", detail)
	}

	tampered := current
	tampered.EvidenceIDs = []string{"ev-other"}
	postJSON(t, ts.URL+"/v1/memories/"+base.MemoryID+"/revision-proposals", memoryProposalBody(t, project.ProjectID, 2, "attempt source relink", "memory-authority-tamper", tampered), http.StatusBadRequest, &map[string]any{})
	postJSON(t, ts.URL+"/v1/memories/"+base.MemoryID+"/revision-proposals", memoryProposalBody(t, project.ProjectID, 1, "stale edit", "memory-authority-stale", current), http.StatusConflict, &map[string]any{})
}
