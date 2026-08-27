package portfolio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func TestProjectTemporalOperationsFinanceAndConnectorLifecycle(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	projects, err := a.Portfolio.ListProjects(ctx, false)
	if err != nil || len(projects) != 2 {
		t.Fatalf("built-in projects=%#v err=%v", projects, err)
	}
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "memoryos", Name: "MemoryOS 产品", Description: "个人记忆与项目经营系统", DefaultCurrency: "CNY", BudgetMinor: 1_000_000, Aliases: []string{"记忆系统"}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, exact, err := a.Portfolio.Resolve(ctx, "记忆系统")
	if err != nil || !exact || resolved.ProjectID != project.ProjectID {
		t.Fatalf("resolve=%#v exact=%v err=%v", resolved, exact, err)
	}
	fallback, exact, err := a.Portfolio.Resolve(ctx, "not-a-project")
	if err != nil || exact || fallback.ProjectID != portfolio.InboxProjectID {
		t.Fatalf("fallback=%#v exact=%v err=%v", fallback, exact, err)
	}

	first, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{ProjectID: project.ProjectID, Subject: "MemoryOS", Predicate: "阶段", Object: "设计", ValidFrom: "2026-08-01T00:00:00Z", ObservedAt: "2026-08-01T01:00:00Z", Confidence: .9})
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{ProjectID: project.ProjectID, Subject: "MemoryOS", Predicate: "阶段", Object: "开发", ValidFrom: "2026-08-20T00:00:00Z", SupersedesFactID: first.FactID, Confidence: .95})
	if err != nil {
		t.Fatal(err)
	}
	current, err := a.Portfolio.ListFacts(ctx, project.ProjectID, "2026-08-21T00:00:00Z", false, 20)
	if err != nil || len(current) != 1 || current[0].FactID != second.FactID {
		t.Fatalf("current=%#v err=%v", current, err)
	}
	prior, err := a.Portfolio.ListFacts(ctx, project.ProjectID, "2026-08-10T00:00:00Z", false, 20)
	if err != nil || len(prior) != 1 || prior[0].FactID != first.FactID {
		t.Fatalf("prior=%#v err=%v", prior, err)
	}
	history, err := a.Portfolio.ListFacts(ctx, project.ProjectID, "", true, 20)
	if err != nil || len(history) != 2 {
		t.Fatalf("history=%#v err=%v", history, err)
	}

	block, err := a.Portfolio.UpsertContextBlock(ctx, portfolio.ContextBlockInput{ProjectID: project.ProjectID, Label: "active-project", Content: "完成统一检索与时间记忆", BudgetChars: 200, SourceRefs: []string{second.FactID}})
	if err != nil || block.Label != "active-project" {
		t.Fatalf("block=%#v err=%v", block, err)
	}
	if _, err := a.Portfolio.UpsertContextBlock(ctx, portfolio.ContextBlockInput{ProjectID: project.ProjectID, Label: "too-small", Content: "12345", BudgetChars: 4}); err == nil {
		t.Fatal("context budget overflow was accepted")
	}

	goal, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "交付完整应用", Description: "完成项目、检索和经营闭环", Priority: 5, TargetAt: "2026-09-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	milestone, err := a.Portfolio.CreateMilestone(ctx, portfolio.MilestoneInput{ProjectID: project.ProjectID, GoalID: goal.GoalID, Title: "M3 验收", DueAt: "2026-08-25T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if goal, err = a.Portfolio.UpdateGoalStatus(ctx, goal.GoalID, "completed"); err != nil || goal.Status != "completed" {
		t.Fatalf("goal status=%#v err=%v", goal, err)
	}
	if milestone, err = a.Portfolio.UpdateMilestoneStatus(ctx, milestone.MilestoneID, "completed"); err != nil || milestone.Status != "completed" || milestone.CompletedAt == "" {
		t.Fatalf("milestone status=%#v err=%v", milestone, err)
	}
	decision, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: project.ProjectID, Title: "采用双时间模型", Decision: "事实同时记录系统时间与有效时间", Rationale: "支持历史回看", DecidedAt: "2026-08-21T00:00:00Z"})
	if err != nil || decision.Status != "active" {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	risk, err := a.Portfolio.CreateRisk(ctx, portfolio.RiskInput{ProjectID: project.ProjectID, Title: "错误跨项目召回", Probability: 2, Impact: 5, Mitigation: "默认项目隔离"})
	if err != nil || risk.Score != 10 {
		t.Fatalf("risk=%#v err=%v", risk, err)
	}
	if risk, err = a.Portfolio.UpdateRiskStatus(ctx, risk.RiskID, "mitigated"); err != nil || risk.Status != "mitigated" {
		t.Fatalf("risk status=%#v err=%v", risk, err)
	}

	account, err := a.Portfolio.CreateFinanceAccount(ctx, portfolio.FinanceAccount{ProjectID: project.ProjectID, Name: "项目现金", AccountType: "cash", Currency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	expenseInput := portfolio.FinanceEntryInput{ProjectID: project.ProjectID, AccountID: account.AccountID, EntryType: "expense", Category: "软件", Description: "开发工具", AmountMinor: -25_000, Currency: "CNY", OccurredAt: "2026-08-21T02:00:00Z", IdempotencyKey: "invoice-001"}
	entry, duplicate, err := a.Portfolio.CreateFinanceEntry(ctx, expenseInput)
	if err != nil || duplicate || entry.AmountMinor != -25_000 {
		t.Fatalf("entry=%#v duplicate=%v err=%v", entry, duplicate, err)
	}
	retry, duplicate, err := a.Portfolio.CreateFinanceEntry(ctx, expenseInput)
	if err != nil || !duplicate || retry.EntryID != entry.EntryID {
		t.Fatalf("retry=%#v duplicate=%v err=%v", retry, duplicate, err)
	}
	if _, _, err := a.Portfolio.CreateFinanceEntry(ctx, portfolio.FinanceEntryInput{ProjectID: portfolio.PersonalProjectID, AccountID: account.AccountID, EntryType: "expense", Description: "wrong scope", AmountMinor: -1, Currency: "CNY"}); err == nil {
		t.Fatal("cross-project account was accepted")
	}
	if entry, err = a.Portfolio.VoidFinanceEntry(ctx, entry.EntryID); err != nil || entry.Status != "void" {
		t.Fatalf("void entry=%#v err=%v", entry, err)
	}
	summary, err := a.Portfolio.FinanceSummary(ctx, project.ProjectID)
	if err != nil || len(summary.Currencies) != 1 || summary.Currencies[0].ExpenseMinor != 0 || summary.Currencies[0].RemainingMinor != 1_000_000 {
		t.Fatalf("finance=%#v err=%v", summary, err)
	}

	connector, err := a.Portfolio.CreateConnector(ctx, portfolio.ConnectorInput{Kind: "chatgpt", Name: "ChatGPT 导出", ProjectID: project.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	batch, duplicate, err := a.Portfolio.BeginImportBatch(ctx, connector.ConnectorID, project.ProjectID, "chatgpt-export-001")
	if err != nil || duplicate || batch.Status != "running" {
		t.Fatalf("batch=%#v duplicate=%v err=%v", batch, duplicate, err)
	}
	batch, err = a.Portfolio.CompleteImportBatch(ctx, batch.BatchID, []string{"ev-1", "ev-2"}, "")
	if err != nil || batch.Status != "completed" || batch.ItemCount != 2 {
		t.Fatalf("completed batch=%#v err=%v", batch, err)
	}
	retryBatch, duplicate, err := a.Portfolio.BeginImportBatch(ctx, connector.ConnectorID, project.ProjectID, "chatgpt-export-001")
	if err != nil || !duplicate || retryBatch.BatchID != batch.BatchID {
		t.Fatalf("retry batch=%#v duplicate=%v err=%v", retryBatch, duplicate, err)
	}

	summaryView, err := a.Portfolio.ProjectSummary(ctx, project.ProjectID)
	if err != nil || summaryView.Metrics.Facts != 1 || summaryView.Metrics.OpenGoals != 0 || summaryView.Metrics.OpenRisks != 0 {
		t.Fatalf("project summary=%#v err=%v", summaryView, err)
	}
}

func TestUnifiedSearchIsProjectScopedAndProvenanceBearing(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	alpha, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "alpha", Name: "Alpha", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "beta", Name: "Beta", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: alpha.ProjectID, Title: "检索策略", Decision: "使用可追溯混合检索", DecidedAt: "2026-08-20T00:00:00Z", SourceEvidenceIDs: []string{"alpha-evidence"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: beta.ProjectID, Title: "检索策略", Decision: "使用外部服务检索", DecidedAt: "2026-08-20T00:00:00Z", SourceEvidenceIDs: []string{"beta-evidence"}}); err != nil {
		t.Fatal(err)
	}
	object, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		TypeID: "builtin.living-asset-vault.document", ProjectID: alpha.ProjectID, Status: "candidate",
		Payload:  json.RawMessage(`{"title":"可视化运行轨迹","summary":"展示每个插件阶段、审核点与外部效果回执","format":"markdown"}`),
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: "search-object-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := a.Unified.Search(ctx, unifiedQuery("可追溯混合检索", alpha.ProjectID, false))
	if err != nil || len(result.Hits) != 1 || result.Hits[0].ProjectID != alpha.ProjectID || result.Hits[0].Kind != "decision" || result.Hits[0].Metadata["source_evidence_ids"] == nil {
		t.Fatalf("scoped result=%#v err=%v", result, err)
	}
	portfolioResult, err := a.Unified.Search(ctx, unifiedQuery("检索", "", true))
	if err != nil || len(portfolioResult.Hits) < 2 {
		t.Fatalf("portfolio result=%#v err=%v", portfolioResult, err)
	}
	objectResult, err := a.Unified.Search(ctx, unifiedsearch.Query{Text: "外部效果回执", ProjectID: alpha.ProjectID, Kinds: []string{"object"}, Limit: 20})
	if err != nil || len(objectResult.Hits) != 1 || objectResult.Hits[0].SourceID != object.ObjectID || objectResult.Hits[0].Metadata["type_id"] != "builtin.living-asset-vault.document" {
		t.Fatalf("object result=%#v err=%v", objectResult, err)
	}
}

func TestProjectTasksAutomationAndGeneratedProjectDefaults(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Name: "新手空间", Description: "只填写名称也能创建"})
	if err != nil {
		t.Fatal(err)
	}
	if project.Slug == "" || project.DefaultCurrency != "CNY" {
		t.Fatalf("generated project defaults=%#v", project)
	}
	automation, err := a.Portfolio.ProjectAutomation(ctx, project.ProjectID)
	if err != nil || automation.ImportMode != "auto_new" {
		t.Fatalf("default automation=%#v err=%v", automation, err)
	}
	automation, err = a.Portfolio.SetProjectAutomation(ctx, project.ProjectID, "manual")
	if err != nil || automation.ImportMode != "manual" {
		t.Fatalf("manual automation=%#v err=%v", automation, err)
	}

	manual, err := a.Portfolio.CreateProjectTask(ctx, portfolio.ProjectTaskInput{ProjectID: project.ProjectID, Title: "今天提交", Priority: 1, DueAt: "2026-08-24T23:00:00+08:00"})
	if err != nil || manual.Status != "todo" || manual.SourceKind != "manual" || manual.DueAt == "" {
		t.Fatalf("manual task=%#v err=%v", manual, err)
	}
	manual, err = a.Portfolio.UpdateProjectTaskStatus(ctx, manual.TaskID, "in_progress")
	if err != nil || manual.Status != "in_progress" {
		t.Fatalf("start task=%#v err=%v", manual, err)
	}
	manual, err = a.Portfolio.UpdateProjectTaskStatus(ctx, manual.TaskID, "done")
	if err != nil || manual.Status != "done" || manual.CompletedAt == "" {
		t.Fatalf("complete task=%#v err=%v", manual, err)
	}
	manual, err = a.Portfolio.UpdateProjectTaskStatus(ctx, manual.TaskID, "todo")
	if err != nil || manual.Status != "todo" || manual.CompletedAt != "" {
		t.Fatalf("reopen task=%#v err=%v", manual, err)
	}

	suggestion, err := a.Portfolio.CreateProjectTask(ctx, portfolio.ProjectTaskInput{ProjectID: project.ProjectID, Title: "AI 识别的行动项", Priority: 2, SourceKind: "ai_suggestion", SourceRecordID: "goal-source-1"})
	if err != nil || suggestion.Status != "suggested" || suggestion.SourceRecordID != "goal-source-1" {
		t.Fatalf("suggestion=%#v err=%v", suggestion, err)
	}
	if _, err := a.Portfolio.UpdateProjectTaskStatus(ctx, suggestion.TaskID, "done"); err == nil {
		t.Fatal("AI suggestion bypassed explicit acceptance")
	}
	suggestion, err = a.Portfolio.UpdateProjectTaskStatus(ctx, suggestion.TaskID, "todo")
	if err != nil || suggestion.Status != "todo" || suggestion.SourceRecordID != "goal-source-1" {
		t.Fatalf("accepted suggestion lost provenance=%#v err=%v", suggestion, err)
	}
	items, err := a.Portfolio.ListProjectTasks(ctx, project.ProjectID, "")
	if err != nil || len(items) != 2 {
		t.Fatalf("tasks=%#v err=%v", items, err)
	}
	summary, err := a.Portfolio.ProjectSummary(ctx, project.ProjectID)
	if err != nil || summary.Metrics.OpenTasks != 2 || summary.Metrics.SuggestedTasks != 0 {
		t.Fatalf("task metrics=%#v err=%v", summary.Metrics, err)
	}
}

func unifiedQuery(text, projectID string, all bool) unifiedsearch.Query {
	return unifiedsearch.Query{Text: text, ProjectID: projectID, AllProjects: all, Limit: 20}
}

func TestDerivedProjectionJoinsOnlyMatchingStructuredKnowledgeObject(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "projection-join", Name: "Projection Join", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-projection-join", "meeting", "session-projection-join", "user", "2026-08-22T03:00:00Z", "项目目标是九月完成安全上线。当前风险是旧接口可能阻塞发布。"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil {
		t.Fatal(err)
	}
	units, total, err := a.Memory.ListKnowledgeUnitsForProject(ctx, project.ProjectID, "", 20, 0)
	if err != nil || total != 2 || len(units) != 2 {
		t.Fatalf("units=%#v total=%d err=%v", units, total, err)
	}
	// The second sync happens after both structured KU objects exist. A broad
	// LEFT JOIN would multiply each legacy row by every active object.
	projection, err := a.Portfolio.SyncDerivedFromEpisode(ctx, project.ProjectID, result.Compilation.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Goals != 1 || projection.Risks != 1 {
		t.Fatalf("structured KU join multiplied or mixed records: %#v", projection)
	}
	var goals, risks int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_goals WHERE project_id=? AND goal_id LIKE 'auto-goal-%'`, project.ProjectID).Scan(&goals); err != nil {
		t.Fatal(err)
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_risks WHERE project_id=? AND risk_id LIKE 'auto-risk-%'`, project.ProjectID).Scan(&risks); err != nil {
		t.Fatal(err)
	}
	if goals != 1 || risks != 1 {
		t.Fatalf("stored projections goals=%d risks=%d", goals, risks)
	}
}
