package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func ownerRequest(t *testing.T, client *http.Client, method, url, origin string, credential ownerauth.Credential, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if credential.Token != "" {
		req.Header.Set(ownerauth.TokenHeader, credential.Token)
	}
	if credential.CSRFToken != "" {
		req.Header.Set(ownerauth.CSRFHeader, credential.CSRFToken)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

func TestOwnerBoundaryRejectsBrowserAgentAndForgedMutation(t *testing.T) {
	a, _ := testutil.Open(t)
	s := server.New(a)
	credential, err := s.IssueOwnerSession("wails-test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, _ := ownerRequest(t, ts.Client(), http.MethodGet, ts.URL+"/health", "", ownerauth.Credential{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public health status=%d", resp.StatusCode)
	}
	resp, raw := ownerRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/agents", "", ownerauth.Credential{}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ordinary browser status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = ownerRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/owner/session", "", credential, nil)
	if resp.StatusCode != http.StatusOK || bytes.Contains(raw, []byte(credential.Token)) || bytes.Contains(raw, []byte(credential.CSRFToken)) {
		t.Fatalf("owner session status=%d body=%s", resp.StatusCode, raw)
	}

	createBody := []byte(`{"name":"Scoped Agent","kind":"codex","permissions":["memory.read"],"project_ids":["project-inbox"]}`)
	missingCSRF := credential
	missingCSRF.CSRFToken = ""
	resp, raw = ownerRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", ts.URL, missingCSRF, createBody)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = ownerRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", "https://attacker.example", credential, createBody)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forged origin status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = ownerRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", ts.URL, credential, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("owner create status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Token == "" {
		t.Fatalf("agent credential=%#v err=%v", created, err)
	}

	agentCredential := ownerauth.Credential{Token: created.Token, CSRFToken: credential.CSRFToken}
	resp, raw = ownerRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/agents", ts.URL, agentCredential, createBody)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("agent confused deputy status=%d body=%s", resp.StatusCode, raw)
	}

	var denied int
	if err := a.Control.DB.QueryRow(`SELECT count(*) FROM owner_audit_log WHERE status='denied'`).Scan(&denied); err != nil || denied < 4 {
		t.Fatalf("denied audit=%d err=%v", denied, err)
	}
}

func TestDesktopCORSPermitsLoopbackDevelopmentOriginOnly(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a).Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/v1/projects", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("loopback preflight status=%d allow=%q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	req, _ = http.NewRequest(http.MethodOptions, ts.URL+"/v1/projects", nil)
	req.Header.Set("Origin", "wails://wails.localhost:34115")
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "wails://wails.localhost:34115" {
		t.Fatalf("Wails development preflight status=%d allow=%q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}

	req, _ = http.NewRequest(http.MethodOptions, ts.URL+"/v1/projects", nil)
	req.Header.Set("Origin", "https://attacker.example")
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("remote origin was allowed: %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
