package modelconfig_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func usageProvider(t *testing.T, content string) (*modelconfig.Service, string, *sql.DB, *httptest.Server) {
	t.Helper()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
			"usage": map[string]any{"prompt_tokens": 120, "completion_tokens": 30, "total_tokens": 150,
				"prompt_tokens_details":     map[string]int{"cached_tokens": 20},
				"completion_tokens_details": map[string]int{"reasoning_tokens": 7}},
		})
	}))
	a, _ := testutil.Open(t)
	service := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := service.SaveProvider(t.Context(), modelconfig.ProviderInput{
		Name: "Usage Model", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "usage-model", Enabled: true,
		Pricing: &modelconfig.PricingInput{Currency: "USD", InputPerMillionMinor: 300, OutputPerMillionMinor: 1200},
	})
	if err != nil {
		mock.Close()
		t.Fatal(err)
	}
	if _, err := service.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		mock.Close()
		t.Fatal(err)
	}
	return service, provider.ProviderID, a.Control.DB, mock
}

func TestGenerateJSONRecordsProviderUsagePricingAndRunContext(t *testing.T) {
	service, providerID, db, mock := usageProvider(t, `{"answer":"ok"}`)
	defer mock.Close()
	ctx := modelusage.WithContext(t.Context(), modelusage.ContextInfo{RunID: "run-health", NodeID: "node-model", ProjectID: "project-health", StageType: "llm.extract"})
	_, err := service.GenerateJSON(ctx, modelconfig.JSONGenerationRequest{
		SystemPrompt: "Return the answer.", Input: []byte(`{"question":"x"}`),
		OutputSchema: []byte(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`), MaxTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := modelusage.ListByRun(t.Context(), db, "run-health")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("observations=%#v", items)
	}
	item := items[0]
	if item.ProviderID != providerID || item.Status != "success" || item.UsageSource != "provider_reported" {
		t.Fatalf("observation=%#v", item)
	}
	if item.RunID != "run-health" || item.NodeID != "node-model" || item.ProjectID != "project-health" || item.StageType != "llm.extract" {
		t.Fatalf("run context=%#v", item.ContextInfo)
	}
	if item.PromptTokens != 120 || item.CompletionTokens != 30 || item.TotalTokens != 150 || item.CachedPromptTokens != 20 || item.ReasoningTokens != 7 {
		t.Fatalf("usage=%#v", item)
	}
	if item.PricingSource != "provider_config" || item.Currency != "USD" || item.EstimatedCostMicrominor != 72000 {
		t.Fatalf("pricing=%#v", item)
	}
	summary := modelusage.Aggregate(items)
	if summary.Calls != 1 || summary.TotalTokens != 150 || summary.CostStatus != "estimated" || summary.EstimatedCostMicrominor != 72000 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestGenerateJSONRecordsFailedProviderOutputWithoutPretendingSuccess(t *testing.T) {
	service, _, db, mock := usageProvider(t, `not-json`)
	defer mock.Close()
	ctx := modelusage.WithContext(t.Context(), modelusage.ContextInfo{RunID: "run-failed", NodeID: "node-failed", ProjectID: "project-health", StageType: "llm.extract"})
	_, err := service.GenerateJSON(ctx, modelconfig.JSONGenerationRequest{
		SystemPrompt: "Return JSON.", Input: []byte(`{"x":1}`), OutputSchema: []byte(`{"type":"object"}`), MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected output contract failure")
	}
	items, listErr := modelusage.ListByRun(t.Context(), db, "run-failed")
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(items) != 1 || items[0].Status != "failed" || items[0].ErrorCode != "output_contract_error" || items[0].TotalTokens != 150 {
		t.Fatalf("failed observation=%#v", items)
	}
}
