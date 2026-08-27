package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func mustOwnerJSON(t *testing.T, client *http.Client, method, endpoint, origin string, credential ownerauth.Credential, body string, want int, target any) {
	t.Helper()
	resp, raw := ownerRequest(t, client, method, endpoint, origin, credential, []byte(body))
	if resp.StatusCode != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, endpoint, resp.StatusCode, want, raw)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHarnessOwnerAPIMaterializationAndTrace(t *testing.T) {
	a, _ := testutil.Open(t)
	s := server.New(a)
	credential, err := s.IssueOwnerSession("desktop-api-test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, raw := ownerRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/harness/types", "", ownerauth.Credential{}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("untrusted harness access status=%d body=%s", resp.StatusCode, raw)
	}

	var project struct {
		ProjectID string `json:"project_id"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/projects", ts.URL, credential, `{"slug":"harness-api","name":"Harness API","default_currency":"CNY"}`, http.StatusCreated, &project)
	typeBody := `{
		"type_id":"memory.preference","plugin_id":"builtin.preference","display_name":"Preference","schema_version":"1.0.0",
		"schema":{"type":"object","required":["topic","choice"],"properties":{"topic":{"type":"string"},"choice":{"type":"string"}},"additionalProperties":false},
		"lifecycle":{"initial":"candidate","states":["candidate","active","superseded"],"transitions":{"candidate":["active"],"active":["superseded"]}},
		"protection_class":"private","renderer":{"title_field":"topic"}
	}`
	var typeResult struct {
		TypeID string `json:"type_id"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/types", ts.URL, credential, typeBody, http.StatusCreated, &typeResult)
	if typeResult.TypeID != "memory.preference" {
		t.Fatalf("type=%#v", typeResult)
	}
	var listedTypes struct {
		Types []map[string]any `json:"types"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodGet, ts.URL+"/v1/harness/types", "", credential, "", http.StatusOK, &listedTypes)
	foundPreference := false
	for _, item := range listedTypes.Types {
		foundPreference = foundPreference || item["type_id"] == "memory.preference"
	}
	if len(listedTypes.Types) < 10 || !foundPreference {
		t.Fatalf("types=%#v", listedTypes)
	}

	objectBody := `{"type_id":"memory.preference","project_id":"` + project.ProjectID + `","payload":{"topic":"interface","choice":"visible traces"},"confidence":0.9,"importance":0.8,"plugin_id":"builtin.preference","plugin_version":"1.0.0","idempotency_key":"object-api-1"}`
	var object struct {
		ObjectID string `json:"object_id"`
		Status   string `json:"status"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/objects", ts.URL, credential, objectBody, http.StatusCreated, &object)
	if object.ObjectID == "" || object.Status != "candidate" {
		t.Fatalf("object=%#v", object)
	}
	var objects struct {
		Objects []map[string]any `json:"objects"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodGet, ts.URL+"/v1/harness/objects?project_id="+url.QueryEscape(project.ProjectID), "", credential, "", http.StatusOK, &objects)
	if len(objects.Objects) != 1 {
		t.Fatalf("objects=%#v", objects)
	}

	runBody := `{"project_id":"` + project.ProjectID + `","caller_type":"owner","caller_id":"desktop-test","channel":"desktop","pipeline_id":"pipeline.preference","pipeline_version":"1.0.0","pipeline_hash":"sha256:pipeline","idempotency_key":"run-api-1","snapshot":{"mode":"rules"}}`
	var run struct {
		RunID string `json:"run_id"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/runs", ts.URL, credential, runBody, http.StatusCreated, &run)
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/runs/"+run.RunID+"/events", ts.URL, credential, `{"event_type":"run.started","producer":"builtin.memory-harness-core","data":{"mode":"rules"}}`, http.StatusCreated, nil)
	var span struct {
		SpanID string `json:"span_id"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/runs/"+run.RunID+"/spans", ts.URL, credential, `{"node_id":"extract","stage_type":"extract.candidates","stage_version":"1.0.0","plugin_id":"builtin.preference","input_hash":"sha256:input","detail":{"count":1}}`, http.StatusCreated, &span)
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/spans/"+span.SpanID+"/finish", ts.URL, credential, `{"status":"completed","output_hash":"sha256:output","detail":{"count":1}}`, http.StatusOK, nil)
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/harness/runs/"+run.RunID+"/events", ts.URL, credential, `{"event_type":"run.completed","producer":"builtin.memory-harness-core","data":{"objects":1}}`, http.StatusCreated, nil)

	var detail struct {
		Run struct {
			Status string `json:"status"`
		} `json:"run"`
		Spans  []map[string]any `json:"spans"`
		Events []map[string]any `json:"events"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodGet, ts.URL+"/v1/harness/runs/"+run.RunID, "", credential, "", http.StatusOK, &detail)
	if detail.Run.Status != "completed" || len(detail.Spans) != 1 || len(detail.Events) != 5 {
		t.Fatalf("detail=%#v", detail)
	}
}
