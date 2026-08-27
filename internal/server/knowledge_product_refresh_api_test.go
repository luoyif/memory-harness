package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestProjectBriefBackfillUsesCurrentProjectionsWithoutReprocessingEvidence(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "brief-backfill", Name: "Brief Backfill", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "完成安全发布", Description: "九月交付", Priority: 5, TargetAt: "2026-09-10T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: project.ProjectID, Title: "采用受治理 Revision", Decision: "人工修正必须先审核", DecidedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateRisk(ctx, portfolio.RiskInput{ProjectID: project.ProjectID, Title: "历史数据未回填", Description: "旧项目页面可能为空", Probability: 2, Impact: 3}); err != nil {
		t.Fatal(err)
	}

	var evidenceBefore int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM evidence_receipts`).Scan(&evidenceBefore); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var response struct {
		Status string         `json:"status"`
		Object harness.Object `json:"object"`
	}
	postJSON(t, ts.URL+"/v1/projects/"+project.ProjectID+"/knowledge-products/project-brief/refresh", `{}`, http.StatusOK, &response)
	if response.Status != "refreshed" || response.Object.TypeID != harness.KnowledgeProductTypeV1 || response.Object.CurrentRevision != 1 {
		t.Fatalf("refresh response=%#v", response)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Object.Revision.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	body, _ := payload["body"].(string)
	if !strings.Contains(body, "## 时间状态") || !strings.Contains(body, "## 最近变化") || !strings.Contains(body, "完成安全发布") {
		t.Fatalf("project brief lacks temporal/evolution sections:\n%s", body)
	}
	var evidenceAfter int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM evidence_receipts`).Scan(&evidenceAfter); err != nil {
		t.Fatal(err)
	}
	if evidenceAfter != evidenceBefore {
		t.Fatalf("backfill changed Evidence count: before=%d after=%d", evidenceBefore, evidenceAfter)
	}

	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "补齐时间确认", Description: "确认相对时间", Priority: 4}); err != nil {
		t.Fatal(err)
	}
	var second struct {
		Object harness.Object `json:"object"`
	}
	postJSON(t, ts.URL+"/v1/projects/"+project.ProjectID+"/knowledge-products/project-brief/refresh", `{}`, http.StatusOK, &second)
	if second.Object.CurrentRevision <= response.Object.CurrentRevision {
		t.Fatalf("refresh did not append changed Project Brief revision: first=%d second=%d", response.Object.CurrentRevision, second.Object.CurrentRevision)
	}
}
