package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/server"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestModelConfigurationAPIAndMemoryAgentCompilation(t *testing.T) {
	const secret = "model-api-secret-must-never-leak"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "memory-acceptance-model"}}})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"content": `{"candidates":[{"evidence_id":"ev-model-api","statement":"Agents require explicit project grants.","unit_type":"decision","tier_hint":"semantic","risk_tier":"B","confidence":0.96}]}`}}},
				"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 25, "total_tokens": 125},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	a.Models = modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	a.Memory.SetCandidateExtractor(a.Models)
	ts := httptest.NewServer(server.New(a, server.WithOwnerAuthBypassForTests()).Handler())
	defer ts.Close()

	resp, raw := agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/model/providers", "", map[string]any{"name": "Acceptance Provider", "kind": "openai_compatible", "base_url": mock.URL + "/v1", "model": "memory-acceptance-model", "api_key": secret, "enabled": true, "pricing": map[string]any{"currency": "USD", "input_per_million_minor": 200, "output_per_million_minor": 800}})
	if resp.StatusCode != http.StatusCreated || strings.Contains(string(raw), secret) {
		t.Fatalf("provider status=%d body=%s", resp.StatusCode, raw)
	}
	var provider modelconfig.Provider
	if err := json.Unmarshal(raw, &provider); err != nil || !provider.HasSecret {
		t.Fatalf("provider=%#v err=%v", provider, err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodPost, ts.URL+"/v1/model/providers/"+provider.ProviderID+"/test", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"selected_model_found":true`) {
		t.Fatalf("probe status=%d body=%s", resp.StatusCode, raw)
	}
	resp, raw = agentRequest(t, ts.Client(), http.MethodPut, ts.URL+"/v1/model/runtime", "", map[string]any{"mode": "agent", "active_provider_id": provider.ProviderID, "fallback_to_rules": false})
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"fallback_to_rules":true`) {
		t.Fatalf("runtime status=%d body=%s", resp.StatusCode, raw)
	}

	appendResult, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-model-api", "codex", "session-model-api", "user", "2026-08-21T12:00:00Z", "Agents require explicit project grants."))
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Memory.EnqueueAndProcess(t.Context(), appendResult.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := a.Memory.Episode(t.Context(), run.EpisodeID)
	if err != nil || episode.Compiler != "agent/openai_compatible/memory-acceptance-model" {
		t.Fatalf("episode=%#v err=%v", episode, err)
	}
	units, err := a.Memory.ListKnowledgeUnits(t.Context(), run.EpisodeID, "", 20)
	if err != nil || len(units) != 1 || units[0].Statement != "Agents require explicit project grants." {
		t.Fatalf("units=%#v err=%v", units, err)
	}

	resp, raw = agentRequest(t, ts.Client(), http.MethodGet, ts.URL+"/v1/model/config", "", nil)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "volatile process memory") || !strings.Contains(string(raw), `"secret_persistent":false`) {
		t.Fatalf("config status=%d body=%s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), `"total_tokens":125`) || !strings.Contains(string(raw), `"cost_status":"estimated"`) || !strings.Contains(string(raw), `"currency":"USD"`) {
		t.Fatalf("model health/pricing missing from config: %s", raw)
	}
	if !strings.Contains(string(raw), `"input_per_million_minor":200`) || !strings.Contains(string(raw), `"output_per_million_minor":800`) {
		t.Fatalf("owner pricing missing from provider config: %s", raw)
	}
	if !strings.Contains(string(raw), `"protocol":"openai_chat"`) || !strings.Contains(string(raw), `"preset_id":"openai-responses"`) || !strings.Contains(string(raw), `"provider_kind":"opencode_go"`) || !strings.Contains(string(raw), `"model_catalog_updated_at":"2026-08-25"`) {
		t.Fatalf("provider protocol knowledge missing from config: %s", raw)
	}
}
