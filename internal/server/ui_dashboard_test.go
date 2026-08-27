package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestEmbeddedUIRoutesAndHeaders(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Location") != "/ui/" {
		t.Fatalf("/ status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp, err = client.Get(ts.URL + "/ui")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Location") != "/ui/" {
		t.Fatalf("/ui status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp, err = http.Get(ts.URL + "/ui/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Memory Harness") || !strings.Contains(string(body), `id="root"`) {
		t.Fatalf("index status=%d body=%q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("index content-type=%q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("index cache-control=%q", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("missing CSP: %q", got)
	}

	match := regexp.MustCompile(`src="\./([^"]+\.js)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing bundled script in index: %s", body)
	}
	resp, err = http.Get(ts.URL + "/ui/" + string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "/v1/memory-pins") || !strings.Contains(string(body), "/v1/harness/types") || !strings.Contains(string(body), "/v1/pipelines") {
		t.Fatalf("bundle status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("app.js content-type=%q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("app.js cache-control=%q", got)
	}

	resp, err = http.Get(ts.URL + "/ui/does-not-exist.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset status=%d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/v1/integrations/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "memory_capture") || !strings.Contains(string(body), "memory_project_write") || !strings.Contains(string(body), "memory_finance_write") || !strings.Contains(string(body), "memory_timeline") || !strings.Contains(string(body), "memory_list_active_assets") || !strings.Contains(string(body), "memory_propose_asset_revision") {
		t.Fatalf("capabilities status=%d body=%s", resp.StatusCode, body)
	}
	var capabilities struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &capabilities); err != nil {
		t.Fatal(err)
	}
	if len(capabilities.Tools) != 24 {
		t.Fatalf("capability tool count=%d want=24", len(capabilities.Tools))
	}
	wantTeamTools := map[string]bool{
		"memory_team_list_tasks":    false,
		"memory_team_read_private":  false,
		"memory_team_write_private": false,
		"memory_team_read_shared":   false,
		"memory_team_write_shared":  false,
		"memory_team_update_share":  false,
		"memory_team_propose_leave": false,
	}
	for _, tool := range capabilities.Tools {
		if _, ok := wantTeamTools[tool.Name]; ok {
			wantTeamTools[tool.Name] = true
		}
	}
	for name, found := range wantTeamTools {
		if !found {
			t.Fatalf("capability list missing team tool %q", name)
		}
	}
}

func TestDashboardUsesPersistedDataAndReportsActiveMemoryBoundaries(t *testing.T) {
	a, _ := testutil.Open(t)
	localNow := time.Now().In(time.Local)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 1, 0, 0, 0, localNow.Location()).UTC()
	yesterday := today.Add(-24 * time.Hour)

	items := [][]byte{
		testutil.Evidence(t, "dash-1", "codex", "session-a", "user", today.Format(time.RFC3339), "first dashboard event"),
		testutil.Evidence(t, "dash-2", "codex", "session-a", "assistant", today.Add(time.Minute).Format(time.RFC3339), "second dashboard event"),
		testutil.Evidence(t, "dash-3", "dsh", "session-b", "user", today.Add(2*time.Minute).Format(time.RFC3339), "third dashboard event"),
		testutil.Evidence(t, "dash-old", "chatgpt", "session-old", "user", yesterday.Format(time.RFC3339), "older dashboard event"),
	}
	for _, raw := range items {
		if _, err := a.Ledger.Append(t.Context(), raw); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.Control.DB.Exec(`INSERT INTO jobs(job_id,kind,status,payload_json,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "job-pending", "import", "pending", `{}`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Control.DB.Exec(`INSERT INTO jobs(job_id,kind,status,payload_json,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "job-failed", "import", "failed", `{}`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Control.DB.Exec(`INSERT INTO operation_receipts(operation_id,kind,status,created_at) VALUES(?,?,?,?)`, "op-review", "memory_update", "review_required", now); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	var dashboard struct {
		Today struct {
			Evidence int `json:"evidence"`
			Sessions int `json:"sessions"`
			Sources  int `json:"sources"`
			Recent   []struct {
				EvidenceID string `json:"evidence_id"`
			} `json:"recent"`
		} `json:"today"`
		Totals struct {
			Evidence int `json:"evidence"`
			Sessions int `json:"sessions"`
			Sources  int `json:"sources"`
		} `json:"totals"`
		Search struct {
			Status     string `json:"status"`
			Consistent bool   `json:"consistent"`
		} `json:"search"`
		Jobs struct {
			Pending        int `json:"pending"`
			Failed         int `json:"failed"`
			NeedsAttention int `json:"needs_attention"`
		} `json:"jobs"`
		Memory struct {
			Enabled bool   `json:"enabled"`
			Stage   string `json:"stage"`
		} `json:"memory"`
		Assets struct {
			Enabled bool   `json:"enabled"`
			Stage   string `json:"stage"`
		} `json:"assets"`
		Sources []struct {
			Name string `json:"name"`
		} `json:"sources"`
	}
	getJSON(t, ts.URL+"/v1/dashboard", &dashboard)
	if dashboard.Today.Evidence != 3 || dashboard.Today.Sessions != 2 || dashboard.Today.Sources != 2 || len(dashboard.Today.Recent) != 3 {
		t.Fatalf("bad today snapshot %#v", dashboard.Today)
	}
	if dashboard.Totals.Evidence != 4 || dashboard.Totals.Sessions != 3 || dashboard.Totals.Sources != 3 {
		t.Fatalf("bad totals %#v", dashboard.Totals)
	}
	if dashboard.Search.Status != "healthy" || !dashboard.Search.Consistent {
		t.Fatalf("bad search %#v", dashboard.Search)
	}
	if dashboard.Jobs.Pending != 1 || dashboard.Jobs.Failed != 1 || dashboard.Jobs.NeedsAttention != 2 {
		t.Fatalf("bad jobs %#v", dashboard.Jobs)
	}
	if !dashboard.Memory.Enabled || dashboard.Memory.Stage != "memory_growth_active" {
		t.Fatalf("memory capability missing %#v", dashboard.Memory)
	}
	if !dashboard.Assets.Enabled || dashboard.Assets.Stage != "protected_registry_active" {
		t.Fatalf("protected asset registry missing %#v", dashboard.Assets)
	}
	if len(dashboard.Sources) != 3 {
		t.Fatalf("sources=%#v", dashboard.Sources)
	}

	for _, endpoint := range []string{"/v1/today", "/v1/memory", "/v1/assets", "/v1/sources", "/v1/jobs", "/v1/health/detail"} {
		var payload map[string]any
		getJSON(t, ts.URL+endpoint, &payload)
		if len(payload) == 0 {
			t.Fatalf("empty payload for %s", endpoint)
		}
	}
}

func getJSON(t *testing.T, url string, target any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("GET %s content-type=%q", url, got)
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(target); err != nil {
		t.Fatalf("GET %s decode: %v body=%s", url, err, body)
	}
}
