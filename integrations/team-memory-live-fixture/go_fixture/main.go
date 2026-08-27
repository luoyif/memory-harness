package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/teammemory"
)

type seedOutput struct {
	ProjectID string               `json:"project_id"`
	TaskID    string               `json:"task_id"`
	AgentA    agentauth.Credential `json:"agent_a"`
	AgentB    agentauth.Credential `json:"agent_b"`
	AgentC    agentauth.Credential `json:"agent_c"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func open(home string) *app.App {
	cfg, err := config.Resolve(home, "")
	must(err)
	a, err := app.Open(cfg)
	must(err)
	return a
}

func makeAgent(ctx context.Context, a *app.App, name, projectID string) agentauth.Credential {
	credential, err := a.Agents.Create(ctx, agentauth.CreateInput{
		Name: name, Kind: "ft6-live", ProjectIDs: []string{projectID},
		Permissions: []string{
			agentauth.PermissionMemoryRead,
			agentauth.PermissionTeamPrivate,
			agentauth.PermissionTeamBlackboardRead,
			agentauth.PermissionTeamBlackboardWrite,
			agentauth.PermissionTeamBlackboardShare,
		},
	})
	must(err)
	return credential
}

func seed(home string) {
	a := open(home)
	defer a.Close()
	ctx := context.Background()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "ft6-live", Name: "FT6 Live", DefaultCurrency: "CNY"})
	must(err)
	a1, b, c := makeAgent(ctx, a, "FT6 Agent A", project.ProjectID), makeAgent(ctx, a, "FT6 Agent B", project.ProjectID), makeAgent(ctx, a, "FT6 Agent C", project.ProjectID)
	task, err := a.TeamMemory.CreateTask(ctx, teammemory.CreateTaskInput{
		ProjectID: project.ProjectID, Title: "FT6 live shared task",
		MemberAgentIDs: []string{a1.Agent.AgentID, b.Agent.AgentID, c.Agent.AgentID},
		TTLSeconds:     3600, IdempotencyKey: "ft6-live-task",
	})
	must(err)
	must(json.NewEncoder(os.Stdout).Encode(seedOutput{ProjectID: project.ProjectID, TaskID: task.ObjectID, AgentA: a1, AgentB: b, AgentC: c}))
}

func approve(home, reviewID, note string) {
	a := open(home)
	defer a.Close()
	item, err := a.Harness.DecideRevisionReview(context.Background(), strings.TrimSpace(reviewID), "approve", "ft6-live-owner", note)
	must(err)
	must(json.NewEncoder(os.Stdout).Encode(item))
}

func finish(home, taskID, conflictReviewID, selectedEntryID string) {
	a := open(home)
	defer a.Close()
	ctx := context.Background()
	if conflictReviewID != "" {
		_, err := a.Harness.DecideRevisionReview(ctx, conflictReviewID, "approve", "ft6-live-owner", "conflict reviewed; Owner will choose one branch")
		must(err)
	}
	task, _, err := a.TeamMemory.Task(ctx, taskID)
	must(err)
	closeReview, err := a.TeamMemory.ProposeTaskClosure(ctx, taskID, teammemory.ActivationInput{ExpectedRevision: task.CurrentRevision, EditReason: "FT6 live task complete", IdempotencyKey: "ft6-live-close"})
	must(err)
	_, err = a.Harness.DecideRevisionReview(ctx, closeReview.ReviewID, "approve", "ft6-live-owner", "close before durable promotion")
	must(err)
	durable, err := a.TeamMemory.CreateDurable(ctx, teammemory.DurableInput{
		TaskID: taskID, EntryIDs: []string{selectedEntryID}, Title: "FT6 Live Durable",
		Summary: "Owner-selected multi-agent result", Body: "FT6-LIVE-DURABLE-MARKER approved shared result",
		IdempotencyKey: "ft6-live-durable",
	})
	must(err)
	activate, err := a.TeamMemory.ProposeDurableActivation(ctx, durable.ObjectID, teammemory.ActivationInput{ExpectedRevision: durable.CurrentRevision, EditReason: "Owner selects durable result", IdempotencyKey: "ft6-live-durable-activate"})
	must(err)
	_, err = a.Harness.DecideRevisionReview(ctx, activate.ReviewID, "approve", "ft6-live-owner", "activate durable result")
	must(err)
	current, err := a.Harness.Object(ctx, durable.ObjectID)
	must(err)
	_, err = a.Reprojection.RefreshApprovedObject(ctx, current)
	must(err)
	must(json.NewEncoder(os.Stdout).Encode(map[string]any{"durable_id": current.ObjectID, "status": current.Status, "task_id": taskID}))
}

func main() {
	home := flag.String("home", "", "isolated Memory Harness home")
	action := flag.String("action", "seed", "seed|approve|finish")
	review := flag.String("review", "", "review id")
	task := flag.String("task", "", "task id")
	entry := flag.String("entry", "", "selected blackboard entry")
	flag.Parse()
	if strings.TrimSpace(*home) == "" {
		panic("-home is required")
	}
	switch *action {
	case "seed":
		seed(*home)
	case "approve":
		approve(*home, *review, "approve governed FT6 live proposal")
	case "finish":
		finish(*home, *task, *review, *entry)
	default:
		panic(fmt.Sprintf("unknown action %s", *action))
	}
}
