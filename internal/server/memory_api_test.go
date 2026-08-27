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

func TestM2MemoryAPIImportTraceReviewAndGraph(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	request := `{
  "source_system":"markdown",
  "session_id":"api-growth",
  "scope_hints":["project:memoryos"],
  "documents":[{"title":"个人工作方法","text":"我偏好证据可追溯的系统。每次操作时必须先运行完整测试。我决定使用 MemoryOS。"}]
}`
	resp, err := http.Post(ts.URL+"/v1/import/text", "application/json", bytes.NewBufferString(request))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", resp.StatusCode, body)
	}
	var imported struct {
		Pipeline struct {
			EpisodeID      string `json:"episode_id"`
			KnowledgeUnits int    `json:"knowledge_units"`
		} `json:"pipeline"`
	}
	if err := json.Unmarshal(body, &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Pipeline.EpisodeID == "" || imported.Pipeline.KnowledgeUnits < 3 {
		t.Fatalf("bad pipeline %#v", imported)
	}
	var beforeReprocess struct {
		Episode struct {
			Revision    int      `json:"revision"`
			EvidenceIDs []string `json:"evidence_ids"`
			Units       int      `json:"units"`
		} `json:"episode"`
	}
	getJSON(t, ts.URL+"/v1/episodes/"+imported.Pipeline.EpisodeID, &beforeReprocess)
	var reprocessed struct {
		Results []struct {
			EpisodeID      string `json:"episode_id"`
			KnowledgeUnits int    `json:"knowledge_units"`
		} `json:"results"`
	}
	postJSON(t, ts.URL+"/v1/process", `{"session_id":"api-growth","force":true}`, http.StatusOK, &reprocessed)
	var afterReprocess struct {
		Episode struct {
			Revision    int      `json:"revision"`
			EvidenceIDs []string `json:"evidence_ids"`
			Units       int      `json:"units"`
		} `json:"episode"`
	}
	getJSON(t, ts.URL+"/v1/episodes/"+imported.Pipeline.EpisodeID, &afterReprocess)
	if len(reprocessed.Results) != 1 || reprocessed.Results[0].EpisodeID != imported.Pipeline.EpisodeID || afterReprocess.Episode.Revision <= beforeReprocess.Episode.Revision || len(afterReprocess.Episode.EvidenceIDs) != len(beforeReprocess.Episode.EvidenceIDs) || afterReprocess.Episode.Units != beforeReprocess.Episode.Units {
		t.Fatalf("safe reprocess did not preserve canonical evidence and replace derivatives: before=%#v after=%#v result=%#v", beforeReprocess, afterReprocess, reprocessed)
	}

	var layers struct {
		Layers []struct {
			ID    string `json:"id"`
			Count int    `json:"count"`
		} `json:"layers"`
		NeedsReview int `json:"needs_review"`
	}
	getJSON(t, ts.URL+"/v1/layers", &layers)
	if len(layers.Layers) != 6 || layers.Layers[0].Count != 1 || layers.Layers[1].Count < 3 || layers.NeedsReview < 2 {
		t.Fatalf("layers=%#v", layers)
	}

	for _, endpoint := range []string{
		"/v1/episodes", "/v1/episodes/" + imported.Pipeline.EpisodeID,
		"/v1/knowledge-units", "/v1/memories", "/v1/living", "/v1/assets", "/v1/graph",
	} {
		var payload map[string]any
		getJSON(t, ts.URL+endpoint, &payload)
		if len(payload) == 0 {
			t.Fatalf("empty response for %s", endpoint)
		}
	}

	var memories struct {
		Items []struct {
			MemoryID string `json:"memory_id"`
			Tier     string `json:"tier"`
		} `json:"memories"`
	}
	getJSON(t, ts.URL+"/v1/memories", &memories)
	if len(memories.Items) < 4 {
		t.Fatalf("memories=%#v", memories.Items)
	}
	var trace map[string]any
	getJSON(t, ts.URL+"/v1/memories/"+memories.Items[0].MemoryID+"/trace", &trace)
	if trace["memory"] == nil || trace["operations"] == nil || trace["episodes"] == nil || trace["evidence"] == nil {
		t.Fatalf("incomplete trace %#v", trace)
	}

	var reviews struct {
		Operations []struct {
			OperationID string `json:"operation_id"`
			Summary     string `json:"summary"`
		} `json:"operations"`
	}
	getJSON(t, ts.URL+"/v1/operations?status=review_required", &reviews)
	if len(reviews.Operations) < 2 || reviews.Operations[0].Summary == "" {
		t.Fatalf("reviews=%#v", reviews)
	}
	var reviewDetail struct {
		Operation struct {
			OperationID string `json:"operation_id"`
			Type        string `json:"type"`
		} `json:"operation"`
		KnowledgeUnit *struct {
			Statement string `json:"statement"`
		} `json:"knowledge_unit"`
		Evidence []struct {
			EvidenceID string `json:"evidence_id"`
			Preview    string `json:"preview"`
		} `json:"evidence"`
	}
	getJSON(t, ts.URL+"/v1/operations/"+reviews.Operations[0].OperationID, &reviewDetail)
	if reviewDetail.Operation.OperationID != reviews.Operations[0].OperationID || reviewDetail.Operation.Type == "" {
		t.Fatalf("review detail operation=%#v", reviewDetail.Operation)
	}
	if reviewDetail.KnowledgeUnit == nil || reviewDetail.KnowledgeUnit.Statement == "" || len(reviewDetail.Evidence) == 0 || reviewDetail.Evidence[0].EvidenceID == "" || reviewDetail.Evidence[0].Preview == "" {
		t.Fatalf("review detail lacks decision context=%#v", reviewDetail)
	}
	decision := bytes.NewBufferString(`{"decision":"approve","reviewer":"api-test"}`)
	resp, err = http.Post(ts.URL+"/v1/operations/"+reviews.Operations[0].OperationID+"/review", "application/json", decision)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("review status=%d body=%s", resp.StatusCode, body)
	}
}

func TestProjectScopedGrowthCreatesObjectsSemanticEndpointAndDetailedRun(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	var first, second struct {
		ProjectID string `json:"project_id"`
	}
	postJSON(t, ts.URL+"/v1/projects", `{"slug":"growth-a","name":"Growth A","default_currency":"CNY"}`, http.StatusCreated, &first)
	postJSON(t, ts.URL+"/v1/projects", `{"slug":"growth-b","name":"Growth B","default_currency":"CNY"}`, http.StatusCreated, &second)
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"meeting","session_id":"growth-a-session","project_id":"`+first.ProjectID+`","documents":[{"title":"产品决定","text":"李明决定下周在上海发布记忆产品。"}]}`, http.StatusCreated, &map[string]any{})
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"meeting","session_id":"growth-b-session","project_id":"`+second.ProjectID+`","documents":[{"title":"其他项目","text":"王芳计划下个月在北京测试搜索产品。"}]}`, http.StatusCreated, &map[string]any{})

	var processed struct {
		Total   int `json:"total"`
		Results []struct {
			SessionID string `json:"session_id"`
		} `json:"results"`
	}
	postJSON(t, ts.URL+"/v1/process", `{"project_id":"`+first.ProjectID+`","force":true}`, http.StatusOK, &processed)
	if processed.Total != 1 || len(processed.Results) != 1 || processed.Results[0].SessionID != "growth-a-session" {
		t.Fatalf("project growth crossed memory boundary: %#v", processed)
	}

	var objects struct {
		Objects []struct {
			ObjectID string `json:"object_id"`
			Revision struct {
				RunID             string   `json:"run_id"`
				SourceEvidenceIDs []string `json:"source_evidence_ids"`
				SourceObjectIDs   []string `json:"source_object_ids"`
			} `json:"revision"`
		} `json:"objects"`
	}
	getJSON(t, ts.URL+"/v1/harness/objects?project_id="+first.ProjectID+"&limit=100", &objects)
	if len(objects.Objects) == 0 {
		t.Fatalf("growth objects lack provenance: %#v", objects)
	}
	directEvidence := false
	for _, object := range objects.Objects {
		if object.Revision.RunID == "" || (len(object.Revision.SourceEvidenceIDs) == 0 && len(object.Revision.SourceObjectIDs) == 0) {
			t.Fatalf("object %s has no run or source chain: %#v", object.ObjectID, object.Revision)
		}
		directEvidence = directEvidence || len(object.Revision.SourceEvidenceIDs) > 0
	}
	if !directEvidence {
		t.Fatalf("no materialized object points directly to Evidence: %#v", objects)
	}

	var graph struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
	}
	getJSON(t, ts.URL+"/v1/graph/semantic?project_id="+first.ProjectID+"&limit=100", &graph)
	if graph.Nodes == nil || graph.Edges == nil {
		t.Fatalf("semantic endpoint omitted stable graph arrays: %#v", graph)
	}

	var runs struct {
		Runs []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
	}
	getJSON(t, ts.URL+"/v1/harness/runs?project_id="+first.ProjectID+"&limit=20", &runs)
	if len(runs.Runs) == 0 {
		t.Fatal("growth produced no inspectable run")
	}
	var detail struct {
		Spans []struct {
			StageType string         `json:"stage_type"`
			Detail    map[string]any `json:"detail"`
		} `json:"spans"`
	}
	getJSON(t, ts.URL+"/v1/harness/runs/"+runs.Runs[0].RunID, &detail)
	if len(detail.Spans) != 6 {
		t.Fatalf("growth trace has %d spans, want six: %#v", len(detail.Spans), detail)
	}
	for _, span := range detail.Spans {
		if span.StageType != "trigger.manual" && span.Detail["result"] == nil {
			t.Fatalf("stage %s has no inspectable result: %#v", span.StageType, span.Detail)
		}
	}
}

func TestSelectableIncrementalProcessingStaysScopedAndPreservesEvidence(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var first, second struct {
		ProjectID string `json:"project_id"`
	}
	postJSON(t, ts.URL+"/v1/projects", `{"name":"选择处理 A"}`, http.StatusCreated, &first)
	postJSON(t, ts.URL+"/v1/projects", `{"name":"选择处理 B"}`, http.StatusCreated, &second)
	patchJSON(t, ts.URL+"/v1/projects/"+first.ProjectID+"/automation", `{"import_mode":"manual"}`, http.StatusOK, &map[string]any{})
	patchJSON(t, ts.URL+"/v1/projects/"+second.ProjectID+"/automation", `{"import_mode":"manual"}`, http.StatusOK, &map[string]any{})
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"meeting","session_id":"select-a-1","project_id":"`+first.ProjectID+`","documents":[{"title":"第一份","text":"我决定今天完成原材料选择处理。"}]}`, http.StatusCreated, &map[string]any{})
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"meeting","session_id":"select-a-2","project_id":"`+first.ProjectID+`","documents":[{"title":"第二份","text":"这份材料暂时不要处理。"}]}`, http.StatusCreated, &map[string]any{})
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"meeting","session_id":"select-b-1","project_id":"`+second.ProjectID+`","documents":[{"title":"其他项目","text":"跨项目材料不能被选择。"}]}`, http.StatusCreated, &map[string]any{})

	var evidenceID string
	if err := a.SearchStore.DB.QueryRow(`SELECT evidence_id FROM turns WHERE session_id='select-a-1'`).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	before, err := a.Ledger.ReadEvidence(t.Context(), evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	var sources struct {
		Sources []struct {
			SessionID string `json:"session_id"`
			Status    string `json:"status"`
		} `json:"sources"`
	}
	getJSON(t, ts.URL+"/v1/process/sources?project_id="+first.ProjectID, &sources)
	if len(sources.Sources) != 2 || sources.Sources[0].Status != "pending" || sources.Sources[1].Status != "pending" {
		t.Fatalf("pending sources=%#v", sources.Sources)
	}
	var processed struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Skipped   int `json:"skipped"`
		Failed    int `json:"failed"`
	}
	postJSON(t, ts.URL+"/v1/process", `{"project_id":"`+first.ProjectID+`","session_ids":["select-a-1"],"mode":"incremental"}`, http.StatusOK, &processed)
	if processed.Total != 1 || processed.Succeeded != 1 || processed.Failed != 0 {
		t.Fatalf("selected process=%#v", processed)
	}
	getJSON(t, ts.URL+"/v1/process/sources?project_id="+first.ProjectID, &sources)
	statuses := map[string]string{}
	for _, item := range sources.Sources {
		statuses[item.SessionID] = item.Status
	}
	if statuses["select-a-1"] != "completed" || statuses["select-a-2"] != "pending" {
		t.Fatalf("selection leaked to other source=%#v", statuses)
	}
	postJSON(t, ts.URL+"/v1/process", `{"project_id":"`+first.ProjectID+`","session_ids":["select-a-1"],"mode":"incremental"}`, http.StatusOK, &processed)
	if processed.Skipped != 1 || processed.Succeeded != 0 {
		t.Fatalf("unchanged source was reprocessed=%#v", processed)
	}
	postJSON(t, ts.URL+"/v1/process", `{"project_id":"`+first.ProjectID+`","session_ids":["select-a-1"],"mode":"force"}`, http.StatusOK, &processed)
	if processed.Succeeded != 1 || processed.Total != 1 {
		t.Fatalf("force selected=%#v", processed)
	}
	postJSON(t, ts.URL+"/v1/process", `{"project_id":"`+first.ProjectID+`","session_ids":["select-b-1"],"mode":"incremental"}`, http.StatusConflict, &map[string]any{})
	after, err := a.Ledger.ReadEvidence(t.Context(), evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("processing changed immutable Evidence bytes")
	}
}
