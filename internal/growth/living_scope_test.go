package growth_test

import (
	"path/filepath"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestLivingKnowledgeIsGeneratedInsideProjectBoundary(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	alpha, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "living-alpha", Name: "Living Alpha", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "living-beta", Name: "Living Beta", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	alphaEvidence, err := a.Ledger.Append(ctx, testutil.Evidence(t, "living-alpha-e", "notes", "living-alpha-s", "user", "2026-08-22T09:00:00Z", "Alpha 项目目标是完成项目级 Living Knowledge 隔离。"))
	if err != nil {
		t.Fatal(err)
	}
	betaEvidence, err := a.Ledger.Append(ctx, testutil.Evidence(t, "living-beta-e", "notes", "living-beta-s", "user", "2026-08-22T10:00:00Z", "Beta 项目风险是错误共享其他项目的长期记忆。"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		projectID  string
		sessionID  string
		evidenceID string
	}{{alpha.ProjectID, alphaEvidence.SessionID, alphaEvidence.EvidenceID}, {beta.ProjectID, betaEvidence.SessionID, betaEvidence.EvidenceID}} {
		if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: item.projectID, SessionID: item.sessionID, EvidenceIDs: []string{item.evidenceID}, Primary: true}); err != nil {
			t.Fatal(err)
		}
	}
	alphaMemories, _, err := a.Memory.ListMemoriesForProject(ctx, alpha.ProjectID, "", "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	betaMemories, _, err := a.Memory.ListMemoriesForProject(ctx, beta.ProjectID, "", "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	alphaIDs, betaIDs := map[string]bool{}, map[string]bool{}
	for _, item := range alphaMemories {
		alphaIDs[item.MemoryID] = true
	}
	for _, item := range betaMemories {
		betaIDs[item.MemoryID] = true
	}
	alphaViews, err := a.Memory.ListLivingViewsForProject(ctx, alpha.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	betaViews, err := a.Memory.ListLivingViewsForProject(ctx, beta.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alphaViews) != 3 || len(betaViews) != 3 {
		t.Fatalf("alpha=%d beta=%d", len(alphaViews), len(betaViews))
	}
	for _, view := range alphaViews {
		if view.ProjectID != alpha.ProjectID {
			t.Fatalf("alpha view escaped scope: %#v", view)
		}
		for _, memoryID := range view.SourceMemoryIDs {
			if !alphaIDs[memoryID] || betaIDs[memoryID] {
				t.Fatalf("alpha view contains foreign memory %s", memoryID)
			}
		}
	}
	for _, view := range betaViews {
		if view.ProjectID != beta.ProjectID {
			t.Fatalf("beta view escaped scope: %#v", view)
		}
		for _, memoryID := range view.SourceMemoryIDs {
			if !betaIDs[memoryID] || alphaIDs[memoryID] {
				t.Fatalf("beta view contains foreign memory %s", memoryID)
			}
		}
	}
	for _, alphaView := range alphaViews {
		for _, betaView := range betaViews {
			if filepath.Base(alphaView.CanonicalPath) == filepath.Base(betaView.CanonicalPath) {
				t.Fatalf("projects share living markdown path %s", alphaView.CanonicalPath)
			}
		}
	}
}
