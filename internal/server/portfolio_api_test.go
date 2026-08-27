package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func postJSON(t *testing.T, url, body string, want int, target any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("POST %s status=%d want=%d body=%s", url, resp.StatusCode, want, raw)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
}

func patchJSON(t *testing.T, url, body string, want int, target any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("PATCH %s status=%d want=%d body=%s", url, resp.StatusCode, want, raw)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectTaskAndAutomationHTTP(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var project struct {
		ProjectID string `json:"project_id"`
		Slug      string `json:"slug"`
	}
	postJSON(t, ts.URL+"/v1/projects", `{"name":"新手空间"}`, http.StatusCreated, &project)
	if project.ProjectID == "" || project.Slug == "" {
		t.Fatalf("project defaults=%#v", project)
	}
	var automation struct {
		ImportMode string `json:"import_mode"`
	}
	getJSON(t, ts.URL+"/v1/projects/"+project.ProjectID+"/automation", &automation)
	if automation.ImportMode != "auto_new" {
		t.Fatalf("default automation=%#v", automation)
	}
	patchJSON(t, ts.URL+"/v1/projects/"+project.ProjectID+"/automation", `{"import_mode":"manual"}`, http.StatusOK, &automation)
	if automation.ImportMode != "manual" {
		t.Fatalf("updated automation=%#v", automation)
	}
	var task struct {
		TaskID     string `json:"task_id"`
		Status     string `json:"status"`
		SourceKind string `json:"source_kind"`
	}
	postJSON(t, ts.URL+"/v1/project-tasks", `{"project_id":"`+project.ProjectID+`","title":"准备提交","source_kind":"ai_suggestion","source_record_id":"forged","priority":1}`, http.StatusCreated, &task)
	if task.TaskID == "" || task.Status != "todo" || task.SourceKind != "manual" {
		t.Fatalf("owner task endpoint accepted forged AI status=%#v", task)
	}
	patchJSON(t, ts.URL+"/v1/project-tasks/"+task.TaskID, `{"status":"in_progress"}`, http.StatusOK, &task)
	patchJSON(t, ts.URL+"/v1/project-tasks/"+task.TaskID, `{"status":"done"}`, http.StatusOK, &task)
	patchJSON(t, ts.URL+"/v1/project-tasks/"+task.TaskID, `{"status":"todo"}`, http.StatusOK, &task)
	if task.Status != "todo" {
		t.Fatalf("reopened task=%#v", task)
	}
	var listed struct {
		Tasks []map[string]any `json:"tasks"`
	}
	getJSON(t, ts.URL+"/v1/project-tasks?project_id="+project.ProjectID, &listed)
	if len(listed.Tasks) != 1 {
		t.Fatalf("tasks=%#v", listed.Tasks)
	}
}

func TestConversationExportDryRunImportArchiveAndRetry(t *testing.T) {
	a, cfg := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()
	var project struct {
		ProjectID string `json:"project_id"`
	}
	postJSON(t, ts.URL+"/v1/projects", `{"slug":"archive-test","name":"历史会话","default_currency":"CNY"}`, http.StatusCreated, &project)
	payload := `[{"id":"conversation-1","title":"历史决定","create_time":1787260000,"update_time":1787260100,"current_node":"a1","mapping":{"root":{"id":"root","parent":"","message":null},"u1":{"id":"u1","parent":"root","message":{"id":"u1","author":{"role":"user"},"create_time":1787260001,"content":{"content_type":"text","parts":["我决定构建完整的记忆应用"]}}},"a1":{"id":"a1","parent":"u1","message":{"id":"a1","author":{"role":"assistant"},"create_time":1787260002,"content":{"content_type":"text","parts":["决定已记录并等待验证"]}}}}}]`
	body := `{"format":"chatgpt","project_id":"` + project.ProjectID + `","idempotency_key":"chat-export-1","dry_run":true,"payload":` + payload + `}`
	var dryRun struct {
		DryRun  bool `json:"dry_run"`
		Preview struct {
			Conversations int `json:"conversations"`
			Messages      int `json:"messages"`
		} `json:"preview"`
	}
	postJSON(t, ts.URL+"/v1/import/conversations", body, http.StatusOK, &dryRun)
	if !dryRun.DryRun || dryRun.Preview.Conversations != 1 || dryRun.Preview.Messages != 2 {
		t.Fatalf("dry run=%#v", dryRun)
	}
	body = `{"format":"chatgpt","project_id":"` + project.ProjectID + `","idempotency_key":"chat-export-1","payload":` + payload + `}`
	var imported struct {
		SourceArchive struct {
			RelPath string `json:"relative_path"`
		} `json:"source_archive"`
		Batch struct {
			Status    string `json:"status"`
			ItemCount int    `json:"item_count"`
		} `json:"batch"`
	}
	postJSON(t, ts.URL+"/v1/import/conversations", body, http.StatusCreated, &imported)
	if imported.Batch.Status != "completed" || imported.Batch.ItemCount != 2 || imported.SourceArchive.RelPath == "" {
		t.Fatalf("import=%#v", imported)
	}
	if _, err := os.Stat(filepath.Join(cfg.Home, filepath.FromSlash(imported.SourceArchive.RelPath))); err != nil {
		t.Fatal(err)
	}
	var duplicate map[string]any
	postJSON(t, ts.URL+"/v1/import/conversations", body, http.StatusOK, &duplicate)
	if duplicate["duplicate"] != true {
		t.Fatalf("retry=%#v", duplicate)
	}
	var summary struct {
		Summary struct {
			Metrics struct {
				Evidence int `json:"evidence"`
			} `json:"metrics"`
		} `json:"summary"`
	}
	getJSON(t, ts.URL+"/v1/projects/"+project.ProjectID, &summary)
	if summary.Summary.Metrics.Evidence != 2 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestPortfolioHTTPProjectImportSearchTimelineAndFinance(t *testing.T) {
	a, _ := testutil.Open(t)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	var project struct {
		ProjectID string `json:"project_id"`
	}
	postJSON(t, ts.URL+"/v1/projects", `{"slug":"memoryos","name":"MemoryOS","description":"完整个人记忆应用","default_currency":"CNY","budget_minor":500000,"aliases":["记忆项目"]}`, http.StatusCreated, &project)
	if project.ProjectID == "" {
		t.Fatal("missing project id")
	}

	var imported struct {
		ProjectID string `json:"project_id"`
		Pipeline  struct {
			EpisodeID string `json:"episode_id"`
		} `json:"pipeline"`
	}
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"chatgpt","session_id":"project-session","project_id":"`+project.ProjectID+`","idempotency_key":"import-001","documents":[{"title":"检索决定","text":"我决定采用可追溯混合检索。当前项目正在完成时间记忆。"}]}`, http.StatusCreated, &imported)
	if imported.ProjectID != project.ProjectID || imported.Pipeline.EpisodeID == "" {
		t.Fatalf("import=%#v", imported)
	}
	for _, endpoint := range []string{"/v1/episodes", "/v1/knowledge-units", "/v1/memories"} {
		var scoped struct {
			Total  int `json:"total"`
			Offset int `json:"offset"`
		}
		getJSON(t, ts.URL+endpoint+"?project_id="+project.ProjectID+"&limit=1&offset=0", &scoped)
		if scoped.Total == 0 {
			t.Fatalf("project-scoped endpoint %s returned no imported memory", endpoint)
		}
		if scoped.Offset != 0 {
			t.Fatalf("project-scoped endpoint %s ignored pagination offset", endpoint)
		}
		var inbox struct {
			Total int `json:"total"`
		}
		getJSON(t, ts.URL+endpoint+"?project_id=project-inbox&limit=100", &inbox)
		if inbox.Total != 0 {
			t.Fatalf("project-scoped endpoint %s leaked %d records into inbox", endpoint, inbox.Total)
		}
	}
	var layers struct {
		Layers []struct {
			ID    string `json:"id"`
			Count int    `json:"count"`
		} `json:"layers"`
		NeedsReview int `json:"needs_review"`
	}
	getJSON(t, ts.URL+"/v1/layers?project_id="+project.ProjectID, &layers)
	if len(layers.Layers) != 6 || layers.Layers[0].Count != 1 || layers.Layers[1].Count == 0 || layers.Layers[2].Count != 1 || layers.Layers[3].Count == 0 {
		t.Fatalf("project layers=%#v", layers)
	}
	var living struct {
		Views []struct {
			ViewID string `json:"view_id"`
		} `json:"views"`
	}
	getJSON(t, ts.URL+"/v1/living?project_id="+project.ProjectID, &living)
	if len(living.Views) != 3 {
		t.Fatalf("project living knowledge=%#v", living)
	}
	var livingDetail struct {
		Content  string           `json:"content"`
		Memories []map[string]any `json:"memories"`
	}
	getJSON(t, ts.URL+"/v1/living/"+living.Views[0].ViewID, &livingDetail)
	if livingDetail.Content == "" || len(livingDetail.Memories) == 0 {
		t.Fatalf("living detail is not inspectable=%#v", livingDetail)
	}
	var graph struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
	}
	getJSON(t, ts.URL+"/v1/graph?project_id="+project.ProjectID, &graph)
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("project graph is empty=%#v", graph)
	}
	var runs struct {
		Runs []struct {
			RunID      string `json:"run_id"`
			PipelineID string `json:"pipeline_id"`
			Status     string `json:"status"`
		} `json:"runs"`
	}
	getJSON(t, ts.URL+"/v1/harness/runs?project_id="+project.ProjectID, &runs)
	if len(runs.Runs) != 1 || runs.Runs[0].PipelineID != "builtin.core-memory-growth.default-import" || runs.Runs[0].Status != "completed" {
		t.Fatalf("default import trace=%#v", runs)
	}
	var runDetail struct {
		Spans []struct {
			NodeID string `json:"node_id"`
			Status string `json:"status"`
		} `json:"spans"`
	}
	getJSON(t, ts.URL+"/v1/harness/runs/"+runs.Runs[0].RunID, &runDetail)
	if len(runDetail.Spans) != 6 {
		t.Fatalf("default import trace lost stages=%#v", runDetail)
	}
	for index, nodeID := range []string{"evidence", "compile", "project", "semantic", "materialize", "index"} {
		if runDetail.Spans[index].NodeID != nodeID || runDetail.Spans[index].Status != "completed" {
			t.Fatalf("default import stage %d=%#v", index, runDetail.Spans[index])
		}
	}
	var automaticProject struct {
		Decisions     []map[string]any `json:"decisions"`
		ContextBlocks []map[string]any `json:"context_blocks"`
	}
	getJSON(t, ts.URL+"/v1/projects/"+project.ProjectID, &automaticProject)
	if len(automaticProject.Decisions) == 0 || len(automaticProject.ContextBlocks) == 0 {
		t.Fatalf("import was not projected into the project workspace=%#v", automaticProject)
	}
	var duplicate map[string]any
	postJSON(t, ts.URL+"/v1/import/text", `{"source_system":"chatgpt","session_id":"project-session","project_id":"`+project.ProjectID+`","idempotency_key":"import-001","documents":[{"title":"检索决定","text":"我决定采用可追溯混合检索。当前项目正在完成时间记忆。"}]}`, http.StatusOK, &duplicate)
	if duplicate["duplicate"] != true {
		t.Fatalf("duplicate import=%#v", duplicate)
	}

	var fact struct {
		FactID string `json:"fact_id"`
	}
	postJSON(t, ts.URL+"/v1/facts", `{"project_id":"`+project.ProjectID+`","subject":"MemoryOS","predicate":"阶段","object":"完整应用开发","valid_from":"2026-08-20T00:00:00Z","confidence":0.95}`, http.StatusCreated, &fact)
	if fact.FactID == "" {
		t.Fatal("missing fact id")
	}
	postJSON(t, ts.URL+"/v1/goals", `{"project_id":"`+project.ProjectID+`","title":"完整交付","description":"完成检索、时间和经营管理","priority":5}`, http.StatusCreated, nil)
	postJSON(t, ts.URL+"/v1/decisions", `{"project_id":"`+project.ProjectID+`","title":"本地优先","decision":"保持单二进制和本地数据主权","decided_at":"2026-08-20T00:00:00Z"}`, http.StatusCreated, nil)
	postJSON(t, ts.URL+"/v1/risks", `{"project_id":"`+project.ProjectID+`","title":"跨项目泄漏","probability":2,"impact":5,"mitigation":"默认隔离"}`, http.StatusCreated, nil)

	var account struct {
		AccountID string `json:"account_id"`
	}
	postJSON(t, ts.URL+"/v1/finance/accounts", `{"project_id":"`+project.ProjectID+`","name":"项目账户","account_type":"cash","currency":"CNY"}`, http.StatusCreated, &account)
	postJSON(t, ts.URL+"/v1/finance/entries", `{"project_id":"`+project.ProjectID+`","account_id":"`+account.AccountID+`","entry_type":"expense","category":"研发","description":"本地测试资源","amount_minor":-12000,"currency":"CNY","occurred_at":"2026-08-20T02:00:00Z","idempotency_key":"cost-001"}`, http.StatusCreated, nil)

	var searchResult struct {
		ContextID string `json:"context_id"`
		Hits      []struct {
			ProjectID string `json:"project_id"`
			Kind      string `json:"kind"`
		} `json:"hits"`
	}
	postJSON(t, ts.URL+"/v1/search/unified", `{"query":"可追溯混合检索","project_id":"`+project.ProjectID+`","limit":10}`, http.StatusOK, &searchResult)
	if searchResult.ContextID == "" || len(searchResult.Hits) == 0 || searchResult.Hits[0].ProjectID != project.ProjectID {
		t.Fatalf("search=%#v", searchResult)
	}
	postJSON(t, ts.URL+"/v1/recall/feedback", `{"project_id":"`+project.ProjectID+`","context_id":"`+searchResult.ContextID+`","result_id":"result-1","rating":"helpful"}`, http.StatusCreated, nil)

	var detail struct {
		Summary struct {
			Metrics struct {
				Evidence          int `json:"evidence"`
				KnowledgeUnits    int `json:"knowledge_units"`
				Memories          int `json:"memories"`
				AvailableMemories int `json:"available_memories"`
				Facts             int `json:"facts"`
				OpenGoals         int `json:"open_goals"`
				OpenRisks         int `json:"open_risks"`
				PendingReview     int `json:"pending_review"`
			} `json:"metrics"`
			Finance struct {
				Currencies []struct {
					ExpenseMinor int64 `json:"expense_minor"`
				} `json:"currencies"`
			} `json:"finance"`
		} `json:"summary"`
	}
	getJSON(t, ts.URL+"/v1/projects/"+project.ProjectID, &detail)
	if detail.Summary.Metrics.Evidence != 1 || detail.Summary.Metrics.KnowledgeUnits == 0 || detail.Summary.Metrics.Memories == 0 || detail.Summary.Metrics.AvailableMemories == 0 || detail.Summary.Metrics.AvailableMemories > detail.Summary.Metrics.Memories || detail.Summary.Metrics.PendingReview != layers.NeedsReview || detail.Summary.Metrics.Facts != 1 || detail.Summary.Metrics.OpenGoals != 1 || detail.Summary.Metrics.OpenRisks != 1 || detail.Summary.Finance.Currencies[0].ExpenseMinor != 12000 {
		t.Fatalf("detail=%#v", detail)
	}

	var facts struct {
		Facts []map[string]any `json:"facts"`
	}
	getJSON(t, ts.URL+"/v1/facts?project_id="+project.ProjectID+"&as_of=2026-08-21T00:00:00Z", &facts)
	if len(facts.Facts) != 1 {
		t.Fatalf("facts=%#v", facts)
	}
}
