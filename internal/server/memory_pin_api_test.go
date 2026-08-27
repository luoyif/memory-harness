package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func putJSONStatus(t *testing.T, url, body string, want int, target any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("PUT %s status=%d want=%d body=%s", url, resp.StatusCode, want, raw)
	}
	if target != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOwnerMemoryPinsAreProjectScopedPresentationState(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "pin-home", Name: "Pin Home", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "pin-other", Name: "Pin Other", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(ctx, testutil.Evidence(t, "ev-pin-home", "meeting", "session-pin-home", "user", "2026-08-26T02:00:00Z", "Owner 确认首页只展示亲自固定的长期记忆。"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true}); err != nil {
		t.Fatal(err)
	}
	memories, _, err := a.Memory.ListMemoriesForProject(ctx, project.ProjectID, "", "", 20, 0)
	if err != nil || len(memories) == 0 {
		t.Fatalf("memories=%#v err=%v", memories, err)
	}
	before := memories[0]

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	body, _ := json.Marshal(map[string]any{"project_id": project.ProjectID, "pinned": true})
	putJSONStatus(t, ts.URL+"/v1/memories/"+before.MemoryID+"/pin", string(body), http.StatusOK, &map[string]any{})

	var pinned struct {
		Memories []struct {
			MemoryID   string   `json:"memory_id"`
			Summary    string   `json:"summary"`
			EvidenceID []string `json:"source_evidence_ids"`
			PinnedAt   string   `json:"pinned_at"`
		} `json:"memories"`
		Total int `json:"total"`
	}
	getJSON(t, ts.URL+"/v1/memory-pins?project_id="+project.ProjectID, &pinned)
	if pinned.Total != 1 || len(pinned.Memories) != 1 || pinned.Memories[0].MemoryID != before.MemoryID || pinned.Memories[0].Summary != before.Summary || pinned.Memories[0].PinnedAt == "" {
		t.Fatalf("pinned=%#v", pinned)
	}
	if len(pinned.Memories[0].EvidenceID) != len(before.EvidenceIDs) {
		t.Fatalf("pin changed Evidence references: before=%#v after=%#v", before.EvidenceIDs, pinned.Memories[0].EvidenceID)
	}

	wrongBody, _ := json.Marshal(map[string]any{"project_id": other.ProjectID, "pinned": true})
	putJSONStatus(t, ts.URL+"/v1/memories/"+before.MemoryID+"/pin", string(wrongBody), http.StatusNotFound, nil)

	unpinBody, _ := json.Marshal(map[string]any{"project_id": project.ProjectID, "pinned": false})
	putJSONStatus(t, ts.URL+"/v1/memories/"+before.MemoryID+"/pin", string(unpinBody), http.StatusOK, &map[string]any{})
	getJSON(t, ts.URL+"/v1/memory-pins?project_id="+project.ProjectID, &pinned)
	if pinned.Total != 0 || len(pinned.Memories) != 0 {
		t.Fatalf("unpin left presentation state=%#v", pinned)
	}
	after, err := a.Memory.MemoryForProject(ctx, project.ProjectID, before.MemoryID)
	if err != nil || after.Summary != before.Summary || after.Body != before.Body || len(after.EvidenceIDs) != len(before.EvidenceIDs) {
		t.Fatalf("pinning mutated memory: before=%#v after=%#v err=%v", before, after, err)
	}
}
