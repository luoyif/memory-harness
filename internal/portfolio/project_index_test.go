package portfolio_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestRebuildProjectIndexDoesNotTouchOtherProjects(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	alpha, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "index-alpha", Name: "Index Alpha", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "index-beta", Name: "Index Beta", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: alpha.ProjectID, Title: "Alpha Decision", Decision: "Use project-local rebuild"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: beta.ProjectID, Title: "Beta Decision", Decision: "Must remain untouched"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	var betaBefore int
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE project_id=?`, beta.ProjectID).Scan(&betaBefore); err != nil {
		t.Fatal(err)
	}
	if betaBefore == 0 {
		t.Fatal("beta index unexpectedly empty")
	}

	goal, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: alpha.ProjectID, Title: "Incremental refresh target", Description: "Only Alpha should change", Priority: 4})
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := a.Portfolio.RebuildProjectIndex(ctx, alpha.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var betaAfter, alphaAfter, goalDocs, duplicateKeys int
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE project_id=?`, beta.ProjectID).Scan(&betaAfter); err != nil {
		t.Fatal(err)
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE project_id=?`, alpha.ProjectID).Scan(&alphaAfter); err != nil {
		t.Fatal(err)
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE doc_key=?`, "goal:"+goal.GoalID).Scan(&goalDocs); err != nil {
		t.Fatal(err)
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT doc_key,count(*) n FROM documents GROUP BY doc_key HAVING n>1)`).Scan(&duplicateKeys); err != nil {
		t.Fatal(err)
	}
	if betaAfter != betaBefore {
		t.Fatalf("beta index changed: before=%d after=%d", betaBefore, betaAfter)
	}
	if indexed != alphaAfter || goalDocs != 1 || duplicateKeys != 0 {
		t.Fatalf("indexed=%d alpha=%d goal=%d duplicates=%d", indexed, alphaAfter, goalDocs, duplicateKeys)
	}
}
