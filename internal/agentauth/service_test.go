package agentauth_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestAgentTokenPermissionsProjectsAndAudit(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "agent-scope", Name: "Agent Scope", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	service := agentauth.New(a.Control)
	credential, err := service.Create(ctx, agentauth.CreateInput{
		Name:        "Codex Project Agent",
		Kind:        "codex",
		Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionMemoryCapture},
		ProjectIDs:  []string{project.ProjectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(credential.Token, "mos_") || credential.Agent.AllProjects || !agentauth.CanAccessProject(credential.Agent, project.ProjectID) || agentauth.CanAccessProject(credential.Agent, "project-personal") {
		t.Fatalf("credential=%#v", credential)
	}
	var storedHash string
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT token_hash FROM agent_principals WHERE agent_id=?`, credential.Agent.AgentID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == credential.Token || strings.Contains(storedHash, "mos_") {
		t.Fatal("agent token was stored in plaintext")
	}
	authenticated, err := service.Authenticate(ctx, credential.Token)
	if err != nil || authenticated.AgentID != credential.Agent.AgentID || !agentauth.HasPermission(authenticated, agentauth.PermissionMemoryCapture) {
		t.Fatalf("authenticated=%#v err=%v", authenticated, err)
	}
	if err := service.Audit(ctx, authenticated, "memory.capture", "evidence", "ev-1", project.ProjectID, "allowed", map[string]any{"test": true}); err != nil {
		t.Fatal(err)
	}
	events, err := service.ListAudit(ctx, authenticated.AgentID, 10)
	if err != nil || len(events) != 1 || events[0].ProjectID != project.ProjectID {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	rotated, err := service.RotateToken(ctx, authenticated.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, credential.Token); err == nil {
		t.Fatal("old token remained valid after rotation")
	}
	if _, err := service.Authenticate(ctx, rotated.Token); err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(ctx, authenticated.AgentID, agentauth.UpdateInput{Status: "disabled", Permissions: authenticated.Permissions, ProjectIDs: authenticated.ProjectIDs})
	if err != nil || updated.Status != "disabled" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if _, err := service.Authenticate(ctx, rotated.Token); err == nil {
		t.Fatal("disabled agent authenticated")
	}
	if _, err := service.Get(ctx, "missing"); err != sql.ErrNoRows {
		t.Fatalf("missing err=%v", err)
	}
}

func TestAgentRequiresProjectGrantAndRejectsUnknownPermission(t *testing.T) {
	a, _ := testutil.Open(t)
	service := agentauth.New(a.Control)
	if _, err := service.Create(t.Context(), agentauth.CreateInput{Name: "No scope", Kind: "codex"}); err == nil {
		t.Fatal("expected project grant validation")
	}
	if _, err := service.Create(t.Context(), agentauth.CreateInput{Name: "Bad permission", Kind: "codex", AllProjects: true, Permissions: []string{"admin.everything"}}); err == nil {
		t.Fatal("expected permission validation")
	}
}
