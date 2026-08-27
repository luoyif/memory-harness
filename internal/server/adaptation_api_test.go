package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestAdaptationAPIRemainsOwnerOnly(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "adaptation-owner", Name: "Adaptation Owner", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := a.Agents.Create(t.Context(), agentauth.CreateInput{
		Name: "adaptation-agent", Kind: "test", Permissions: []string{agentauth.PermissionMemoryRead, agentauth.PermissionProjectRead}, ProjectIDs: []string{project.ProjectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewServer(server.New(a).Handler())
	defer secure.Close()
	resp, raw := agentRequest(t, secure.Client(), http.MethodGet, secure.URL+"/v1/adaptation/proposals?project_id="+project.ProjectID, credential.Token, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner Proposal API status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, secure.Client(), http.MethodGet, secure.URL+"/v1/adaptation/overlays?project_id="+project.ProjectID, credential.Token, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner Overlay API status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, secure.Client(), http.MethodPost, secure.URL+"/v1/adaptation/proposals/dry-run", credential.Token, map[string]any{})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Agent reached Owner Dry Run API status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, secure.Client(), http.MethodPost, secure.URL+"/v1/agent/adaptation/proposals", credential.Token, map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected Agent adaptation endpoint status=%d body=%s", resp.StatusCode, raw)
	}
}
