package profile_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/profile"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func seedIdentityMemory(t *testing.T, a *app.App, projectID string) string {
	t.Helper()
	const memoryID = "mem-profile-identity"
	const ts = "2026-08-22T10:00:00Z"
	_, err := a.Control.DB.ExecContext(t.Context(), `INSERT INTO memory_records(memory_id,tier,asset_form,domain,status,summary,body,canonical_key,confidence,importance,strength,evidence_ids_json,episode_ids_json,scopes_json,visibility,observed_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, memoryID, "identity_core", "fact", "identity", "active", "长期工作原则", "重要项目优先保持可追溯、可回滚。", "profile-identity-key", .95, .9, 1.0, `[]`, `[]`, `[]`, "private", ts, ts, ts)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Portfolio.LinkRecord(t.Context(), "memory", memoryID, projectID, true); err != nil {
		t.Fatal(err)
	}
	return memoryID
}

func TestProfileReconcileIsIdempotentAndAgentViewRespectsPermissions(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-view", Name: "Profile View", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	seedIdentityMemory(t, a, project.ProjectID)
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "完成 Profile Compiler", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	first, err := a.Profiles.ReconcileProject(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 {
		t.Fatalf("profiles=%d want=3 %#v", len(first), first)
	}
	revisions := map[string]int{}
	for _, object := range first {
		revisions[object.ObjectID] = object.CurrentRevision
	}
	second, err := a.Profiles.ReconcileProject(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range second {
		if object.CurrentRevision != revisions[object.ObjectID] {
			t.Fatalf("non-idempotent profile %s: %d -> %d", object.ObjectID, revisions[object.ObjectID], object.CurrentRevision)
		}
	}
	dynamicProjection, dynamicObject, err := a.Profiles.Projection(ctx, project.ProjectID, profile.ViewDynamicProject)
	if err != nil || len(dynamicProjection.Blocks) == 0 {
		t.Fatalf("dynamic=%#v err=%v", dynamicProjection, err)
	}
	_, ownerObject, err := a.Profiles.Projection(ctx, project.ProjectID, profile.ViewOwnerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	_, sessionObject, err := a.Profiles.Projection(ctx, project.ProjectID, profile.ViewSessionResume)
	if err != nil {
		t.Fatal(err)
	}
	projectOnly, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "project-only", Kind: "test", Permissions: []string{agentauth.PermissionProjectRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	projectView, err := a.Profiles.AgentView(ctx, projectOnly.Agent, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projectView.SourceProjectionIDs) != 1 || projectView.SourceProjectionIDs[0] != dynamicObject.ObjectID {
		t.Fatalf("project-only view leaked profile: %#v", projectView.SourceProjectionIDs)
	}
	full, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "memory-reader", Kind: "test", Permissions: []string{agentauth.PermissionProjectRead, agentauth.PermissionMemoryRead}, ProjectIDs: []string{project.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	fullView, err := a.Profiles.AgentView(ctx, full.Agent, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range fullView.SourceProjectionIDs {
		seen[id] = true
	}
	if !seen[dynamicObject.ObjectID] || !seen[sessionObject.ObjectID] || seen[ownerObject.ObjectID] {
		t.Fatalf("full agent view=%#v", fullView.SourceProjectionIDs)
	}
	if fullView.DeliveryStatus != "not_delivered" {
		t.Fatalf("delivery=%q", fullView.DeliveryStatus)
	}
}

func TestLockedProfileBlockSurvivesRebuildAndBecomesStale(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-lock", Name: "Profile Lock", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "第一目标", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(ctx, project.ProjectID); err != nil {
		t.Fatal(err)
	}
	before, _, err := a.Profiles.Projection(ctx, project.ProjectID, profile.ViewDynamicProject)
	if err != nil {
		t.Fatal(err)
	}
	var old profile.Block
	for _, block := range before.Blocks {
		if block.BlockID == "project:goals" {
			old = block
		}
	}
	if old.BlockID == "" {
		t.Fatalf("goals block missing: %#v", before.Blocks)
	}
	if _, err := a.Profiles.SetLockedBlocks(ctx, project.ProjectID, profile.ViewDynamicProject, []string{"project:goals"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: project.ProjectID, Title: "第二目标", Priority: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.ReconcileProject(ctx, project.ProjectID); err != nil {
		t.Fatal(err)
	}
	after, _, err := a.Profiles.Projection(ctx, project.ProjectID, profile.ViewDynamicProject)
	if err != nil {
		t.Fatal(err)
	}
	var current profile.Block
	for _, block := range after.Blocks {
		if block.BlockID == "project:goals" {
			current = block
		}
	}
	if current.BlockID == "" {
		t.Fatalf("goals block missing after rebuild: %#v", after.Blocks)
	}
	if current.Content != old.Content {
		t.Fatalf("locked content overwritten: before=%q after=%q", old.Content, current.Content)
	}
	if !current.Locked || current.ReviewStatus != "stale_locked" {
		t.Fatalf("locked block state=%#v", current)
	}
	if current.CandidateSourceHash == "" || current.CandidateSourceHash == current.SourceHash {
		t.Fatalf("candidate hash not recorded: %#v", current)
	}
	if after.GenerationStatus != "human_mixed" {
		t.Fatalf("generation_status=%q", after.GenerationStatus)
	}
	if len(after.LockedBlockIDs) != 1 || after.LockedBlockIDs[0] != "project:goals" {
		t.Fatalf("locked ids=%#v", after.LockedBlockIDs)
	}
}

func TestAgentViewRejectsCrossProjectAccess(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	alpha, _ := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-alpha", Name: "Alpha", DefaultCurrency: "CNY"})
	beta, _ := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "profile-beta", Name: "Beta", DefaultCurrency: "CNY"})
	if _, err := a.Profiles.ReconcileProject(ctx, beta.ProjectID); err != nil {
		t.Fatal(err)
	}
	cred, err := a.Agents.Create(ctx, agentauth.CreateInput{Name: "alpha-only", Kind: "test", Permissions: []string{agentauth.PermissionProjectRead, agentauth.PermissionMemoryRead}, ProjectIDs: []string{alpha.ProjectID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Profiles.AgentView(ctx, cred.Agent, beta.ProjectID); err == nil {
		t.Fatal("cross-project Agent View should fail closed")
	}
}
