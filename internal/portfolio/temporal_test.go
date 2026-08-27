package portfolio_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestTimelineUnifiesFactsGoalsAndTemporalRelevance(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	projectID := portfolio.PersonalProjectID
	if _, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{
		ProjectID: projectID, Subject: "Memory Harness", Predicate: "phase", Object: "M1",
		ObservedAt: "2026-08-22T01:00:00Z", ValidFrom: "2026-08-22T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{
		ProjectID: projectID, Title: "完成时间治理", Status: "active", Priority: 5, TargetAt: "2026-08-24T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	line, err := a.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: projectID, AnchorAt: "2026-08-23T00:00:00Z", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if line.Counts["fact"] != 1 || line.Counts["goal"] != 1 {
		t.Fatalf("unexpected counts %#v", line.Counts)
	}
	var factRelation, goalRelation string
	for _, event := range line.Events {
		if event.Kind == "fact" {
			factRelation = event.TemporalRelation
			if event.TemporalRelevance != 1 {
				t.Fatalf("active fact relevance=%v", event.TemporalRelevance)
			}
		}
		if event.Kind == "goal" {
			goalRelation = event.TemporalRelation
		}
	}
	if factRelation != "active" || goalRelation != "upcoming" {
		t.Fatalf("fact=%q goal=%q", factRelation, goalRelation)
	}
	if len(line.Relations) == 0 {
		t.Fatal("expected chronological relations")
	}
}

func TestTimelineMarksOpenPastDueTaskOverdue(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	if _, err := a.Portfolio.CreateMilestone(ctx, portfolio.MilestoneInput{ProjectID: portfolio.PersonalProjectID, Title: "过期里程碑", Status: "planned", DueAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	line, err := a.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: portfolio.PersonalProjectID, AnchorAt: "2026-08-23T00:00:00Z", Kinds: []string{"milestone"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(line.Events) != 1 || line.Events[0].TemporalRelation != "overdue" {
		t.Fatalf("unexpected timeline %#v", line.Events)
	}
}

func TestTimelinePreservesFactSupersessionAsTemporalRelation(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	oldFact, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{ProjectID: portfolio.PersonalProjectID, Subject: "Memory Harness", Predicate: "phase", Object: "M1", ObservedAt: "2026-08-20T00:00:00Z", ValidFrom: "2026-08-20T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	newFact, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{ProjectID: portfolio.PersonalProjectID, Subject: "Memory Harness", Predicate: "phase", Object: "M2", ObservedAt: "2026-08-22T00:00:00Z", ValidFrom: "2026-08-22T00:00:00Z", SupersedesFactID: oldFact.FactID})
	if err != nil {
		t.Fatal(err)
	}
	line, err := a.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: portfolio.PersonalProjectID, AnchorAt: "2026-08-23T00:00:00Z", Kinds: []string{"fact"}, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	wantFrom, wantTo := "fact:"+newFact.FactID, "fact:"+oldFact.FactID
	for _, relation := range line.Relations {
		if relation.Kind == "supersedes" && relation.FromID == wantFrom && relation.ToID == wantTo {
			return
		}
	}
	t.Fatalf("missing supersedes relation %s -> %s in %#v", wantFrom, wantTo, line.Relations)
}

func TestTimelineBuildsExplicitTemporalCorrelations(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	first, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{
		ProjectID: portfolio.PersonalProjectID, Subject: "Memory Harness", Predicate: "开发窗口", Object: "M1",
		ObservedAt: "2026-08-20T00:00:00Z", ValidFrom: "2026-08-20T00:00:00Z", ValidUntil: "2026-08-30T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{
		ProjectID: portfolio.PersonalProjectID, Subject: "Temporal UI", Predicate: "开发窗口", Object: "M5",
		ObservedAt: "2026-08-25T00:00:00Z", ValidFrom: "2026-08-25T00:00:00Z", ValidUntil: "2026-09-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	line, err := a.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: portfolio.PersonalProjectID, AnchorAt: "2026-08-26T00:00:00Z", Kinds: []string{"fact"}, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	left, right := "fact:"+first.FactID, "fact:"+second.FactID
	for _, correlation := range line.Correlations {
		matches := (correlation.LeftID == left && correlation.RightID == right) || (correlation.LeftID == right && correlation.RightID == left)
		if matches {
			if correlation.Kind != "overlaps" || correlation.Strength < .8 {
				t.Fatalf("weak/wrong overlap correlation %#v", correlation)
			}
			return
		}
	}
	t.Fatalf("missing temporal correlation between %s and %s: %#v", left, right, line.Correlations)
}

func TestTimelineCorrelatesNearbyTaskDeadlines(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: portfolio.PersonalProjectID, Title: "完成 M1", Status: "active", TargetAt: "2026-08-24T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateMilestone(ctx, portfolio.MilestoneInput{ProjectID: portfolio.PersonalProjectID, Title: "M1 验收", Status: "planned", DueAt: "2026-08-25T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	line, err := a.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: portfolio.PersonalProjectID, AnchorAt: "2026-08-23T00:00:00Z", Kinds: []string{"goal", "milestone"}, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, correlation := range line.Correlations {
		if correlation.Kind == "near_in_time" && correlation.Strength >= .5 {
			foundReason := false
			for _, reason := range correlation.Reasons {
				foundReason = foundReason || reason == "task_time_context"
			}
			if !foundReason {
				t.Fatalf("task correlation missing reason %#v", correlation)
			}
			return
		}
	}
	t.Fatalf("nearby task deadlines were not correlated: %#v", line.Correlations)
}
