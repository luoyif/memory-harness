package mcpserver_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/mcpserver"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPMemorySearch(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	raw := testutil.Evidence(t, "ev-mcp", "codex", "sess-mcp", "assistant", "2026-08-20T05:00:00Z", "MCP memory search must return normalized evidence")
	if _, err := a.Ledger.Append(ctx, raw); err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(mcpserver.LocalBackend{App: a}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "memoryos-test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_search", Arguments: map[string]any{"query": "normalized evidence", "project_id": "project-inbox", "limit": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error %#v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty MCP content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content %T", res.Content[0])
	}
	if !strings.Contains(text.Text, "ev-mcp") {
		t.Fatalf("MCP result missing evidence: %s", text.Text)
	}
	read, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_read_evidence", Arguments: map[string]any{"evidence_id": "ev-mcp"}})
	if err != nil || read.IsError || len(read.Content) == 0 {
		t.Fatalf("read Evidence through declared MCP schema: result=%#v err=%v", read, err)
	}
	readText, ok := read.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(readText.Text, "MCP memory search must return normalized evidence") {
		t.Fatalf("MCP exact Evidence read mismatch: %#v", read.Content)
	}
}

func TestMCPProjectScopedRecallAndContext(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "mcp-project", Name: "MCP Project", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: project.ProjectID, Title: "Recall boundary", Decision: "Project recall stays isolated", DecidedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.UpsertContextBlock(ctx, portfolio.ContextBlockInput{ProjectID: project.ProjectID, Label: "active", Content: "Ship the complete application", BudgetChars: 100}); err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(mcpserver.LocalBackend{App: a}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "memoryos-test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	projects, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_list_projects", Arguments: map[string]any{}})
	if err != nil || projects.IsError || !strings.Contains(projects.Content[0].(*mcp.TextContent).Text, "MCP Project") {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	recall, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_recall", Arguments: map[string]any{"query": "Project recall stays isolated", "project_id": project.ProjectID}})
	if err != nil || recall.IsError || !strings.Contains(recall.Content[0].(*mcp.TextContent).Text, "Recall boundary") {
		t.Fatalf("recall=%#v err=%v", recall, err)
	}
	projectContext, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_project_context", Arguments: map[string]any{"project_id": project.ProjectID}})
	if err != nil || projectContext.IsError || !strings.Contains(projectContext.Content[0].(*mcp.TextContent).Text, "Ship the complete application") {
		t.Fatalf("context=%#v err=%v", projectContext, err)
	}
	projectBlueprint, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "memory_project_blueprint", Arguments: map[string]any{"project_id": project.ProjectID}})
	if err != nil || projectBlueprint.IsError || !strings.Contains(projectBlueprint.Content[0].(*mcp.TextContent).Text, "builtin.memory-harness-core.default") {
		t.Fatalf("blueprint=%#v err=%v", projectBlueprint, err)
	}
}

func TestHTTPBackendRequiresLoopback(t *testing.T) {
	if _, err := mcpserver.NewHTTPBackend("http://0.0.0.0:19777"); err == nil {
		t.Fatal("expected non-loopback endpoint rejection")
	}
	if _, err := mcpserver.NewHTTPBackend("https://127.0.0.1:19777"); err == nil {
		t.Fatal("M0 bridge intentionally requires local http")
	}
	backend, err := mcpserver.NewHTTPBackend("http://127.0.0.1:19777")
	if err != nil {
		t.Fatal(err)
	}
	if backend.Client.Timeout < 2*time.Minute {
		t.Fatalf("model-backed MCP calls still have a short transport timeout: %s", backend.Client.Timeout)
	}
	if _, err := mcpserver.NewHTTPBackend("http://localhost:19777"); err != nil {
		t.Fatal(err)
	}
}
