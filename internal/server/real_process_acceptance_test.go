package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/server"
)

// This acceptance test is opt-in because it deliberately rebuilds derived
// memory in an existing local vault. It never mutates immutable Evidence.
func TestRealProjectGrowthCompletesBeyondDefaultHTTPDeadline(t *testing.T) {
	home := os.Getenv("MEMORYOS_REAL_ACCEPTANCE_HOME")
	if home == "" {
		t.Skip("set MEMORYOS_REAL_ACCEPTANCE_HOME to test a real local vault")
	}
	projectID := os.Getenv("MEMORYOS_REAL_ACCEPTANCE_PROJECT")
	if projectID == "" {
		projectID = "project-inbox"
	}
	cfg, err := config.Resolve(home, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := server.New(a, server.WithOwnerAuthBypassForTests())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	body, _ := json.Marshal(map[string]any{"project_id": projectID, "force": true})
	request, _ := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/v1/process", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("process status=%d body=%s", response.StatusCode, raw)
	}
	var result struct {
		Total   int `json:"total"`
		Results []struct {
			EpisodeID      string `json:"episode_id"`
			KnowledgeUnits int    `json:"knowledge_units"`
			Compiler       string `json:"compiler"`
			QualityStatus  string `json:"quality_status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Total < 1 || len(result.Results) != result.Total || result.Results[0].EpisodeID == "" || result.Results[0].KnowledgeUnits < 1 {
		t.Fatalf("incomplete process result: %s", raw)
	}
	if result.Results[0].QualityStatus != "high_quality" || result.Results[0].Compiler == "" {
		t.Fatalf("model-backed quality gate did not pass: %s", raw)
	}
	runs, err := a.Harness.ListRuns(t.Context(), projectID, "completed", 10)
	if err != nil || len(runs) == 0 || runs[0].PipelineVersion != "3.0.0" {
		t.Fatalf("completed runs=%#v err=%v", runs, err)
	}
	detail, err := a.Harness.RunDetail(t.Context(), runs[0].RunID)
	if err != nil || len(detail.Spans) != 6 {
		t.Fatalf("run detail=%#v err=%v", detail, err)
	}
	for _, span := range detail.Spans {
		if span.Status != "completed" {
			t.Fatalf("span %s status=%s", span.NodeID, span.Status)
		}
	}
	objects, err := a.Harness.ListObjects(t.Context(), projectID, "", "", 100)
	if err != nil || len(objects) == 0 {
		t.Fatalf("objects=%#v err=%v", objects, err)
	}
	t.Logf("real project completed in %s: sessions=%d knowledge_units=%d objects=%d run=%s", time.Since(started).Round(time.Millisecond), result.Total, result.Results[0].KnowledgeUnits, len(objects), runs[0].RunID)
}
