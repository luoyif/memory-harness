package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestBlueprintOwnerAPICloneValidatePublishAndActivate(t *testing.T) {
	a, _ := testutil.Open(t)
	s := server.New(a)
	credential, err := s.IssueOwnerSession("blueprint-api-test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, raw := ownerRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/blueprints", "", ownerauth.Credential{}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("untrusted blueprint access status=%d body=%s", resp.StatusCode, raw)
	}

	var project struct {
		ProjectID string `json:"project_id"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/projects", ts.URL, credential, `{"slug":"blueprint-api","name":"Blueprint API","default_currency":"CNY"}`, http.StatusCreated, &project)

	var listed struct {
		Blueprints []blueprint.Version `json:"blueprints"`
	}
	mustOwnerJSON(t, ts.Client(), http.MethodGet, ts.URL+"/v1/blueprints", "", credential, "", http.StatusOK, &listed)
	if len(listed.Blueprints) == 0 || listed.Blueprints[0].BlueprintID != blueprint.DefaultBlueprintID {
		t.Fatalf("blueprints=%#v", listed.Blueprints)
	}

	var current blueprint.Current
	mustOwnerJSON(t, ts.Client(), http.MethodGet, ts.URL+"/v1/projects/"+project.ProjectID+"/blueprint", "", credential, "", http.StatusOK, &current)
	if !current.Inherited || !current.Validation.Valid || current.Blueprint.BlueprintID != blueprint.DefaultBlueprintID {
		t.Fatalf("inherited current=%#v", current)
	}

	invalid := current.Blueprint.Definition
	invalid.Policy.EvidenceMode = "discard_raw"
	invalidRaw, _ := json.Marshal(invalid)
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/blueprints/validate", ts.URL, credential, string(invalidRaw), http.StatusUnprocessableEntity, nil)

	custom := current.Blueprint.Definition
	custom.BlueprintID = "builtin.user-workflows.blueprint-api"
	custom.Version = "1.0.0"
	custom.Name = "Blueprint API clone"
	publishRaw, _ := json.Marshal(map[string]any{"plugin_id": "builtin.user-workflows", "definition": custom})
	var published blueprint.Version
	mustOwnerJSON(t, ts.Client(), http.MethodPost, ts.URL+"/v1/blueprints", ts.URL, credential, string(publishRaw), http.StatusCreated, &published)
	if published.ContentHash == "" || published.BlueprintID != custom.BlueprintID {
		t.Fatalf("published=%#v", published)
	}

	activateRaw, _ := json.Marshal(blueprint.ActivateInput{BlueprintID: custom.BlueprintID, Version: custom.Version})
	mustOwnerJSON(t, ts.Client(), http.MethodPut, ts.URL+"/v1/projects/"+project.ProjectID+"/blueprint", ts.URL, credential, string(activateRaw), http.StatusOK, &current)
	if current.Inherited || !current.Validation.Valid || current.Assignment.BlueprintHash != published.ContentHash {
		t.Fatalf("active current=%#v", current)
	}
}
