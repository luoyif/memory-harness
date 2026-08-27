package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestHTTPM0Flow(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	raw := testutil.Evidence(t, "ev-http", "dsh", "sess-http", "user", "2026-08-20T04:00:00Z", "MemoryOS HTTP capture and search")
	resp, err := http.Post(ts.URL+"/v1/evidence", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("capture status=%d %s", resp.StatusCode, b)
	}
	resp, err = http.Post(ts.URL+"/v1/search", "application/json", bytes.NewBufferString(`{"query":"HTTP capture","limit":5,"neighbor_turns":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("search status=%d %s", resp.StatusCode, b)
	}
	var got struct {
		Hits []struct {
			Turn struct {
				EvidenceID string `json:"evidence_id"`
			} `json:"turn"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(got.Hits) == 0 || got.Hits[0].Turn.EvidenceID != "ev-http" {
		t.Fatalf("bad search %#v", got)
	}
	resp, err = http.Get(ts.URL + "/v1/evidence/ev-http")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("read status=%d", resp.StatusCode)
	}
	resp, err = http.Get(ts.URL + "/v1/doctor")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("doctor status=%d %s", resp.StatusCode, b)
	}
}
