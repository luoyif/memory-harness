package dashboard_test

import (
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/dashboard"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestProjectActivityCalendarCombinesMemoryOutputAndDueTasks(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "activity-calendar", Name: "Activity Calendar", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-activity", "note", "session-activity", "user", now.UTC().Format(time.RFC3339), "今天决定完成首页活动日历，并在发布前检查全部测试。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateProjectTask(ctx, portfolio.ProjectTaskInput{ProjectID: project.ProjectID, Title: "检查活动日历", Priority: 2, DueAt: now.UTC().Format(time.RFC3339), SourceKind: "manual"}); err != nil {
		t.Fatal(err)
	}
	calendar, err := dashboard.ReadProjectActivity(ctx, a, project.ProjectID, now, 28)
	if err != nil {
		t.Fatal(err)
	}
	if len(calendar.Days) != 28 || calendar.Definition == "" {
		t.Fatalf("calendar=%#v", calendar)
	}
	today := calendar.Days[len(calendar.Days)-1]
	if today.Evidence < 1 || today.Output < 1 || today.TasksDue != 1 || len(today.Tasks) != 1 {
		t.Fatalf("today=%#v", today)
	}
}
