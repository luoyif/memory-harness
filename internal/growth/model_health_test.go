package growth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestGrowthModelUsageIsBoundIntoRunDetail(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		content := `{"candidates":[{"evidence_id":"ev-growth-health","statement":"项目决定采用可观测模型调用。","unit_type":"decision","tier_hint":"semantic","risk_tier":"B","confidence":0.95}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 90, "completion_tokens": 10, "total_tokens": 100},
		})
	}))
	defer mock.Close()

	a, _ := testutil.Open(t)
	models := modelconfig.New(a.Control, modelconfig.NewMemorySecretStore(), mock.Client())
	provider, err := models.SaveProvider(t.Context(), modelconfig.ProviderInput{
		Name: "Growth Health", Kind: "openai_compatible", BaseURL: mock.URL + "/v1", Model: "growth-health-model", Enabled: true,
		Pricing: &modelconfig.PricingInput{Currency: "USD", InputPerMillionMinor: 200, OutputPerMillionMinor: 800},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.SetRuntime(t.Context(), modelconfig.RuntimeInput{Mode: "agent", ActiveProviderID: provider.ProviderID, FallbackToRules: true}); err != nil {
		t.Fatal(err)
	}
	a.Memory.SetCandidateExtractor(models)

	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "growth-health", Name: "Growth Health", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-growth-health", "recording", "session-growth-health", "user", "2026-08-24T06:20:00Z", "项目决定采用可观测模型调用。"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Growth.Process(t.Context(), growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Execution.RunID == "" || result.Execution.Status != "completed" {
		t.Fatalf("execution=%#v", result.Execution)
	}
	detail, err := a.Harness.RunDetail(t.Context(), result.Execution.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ModelCalls) != 1 {
		t.Fatalf("model calls=%#v", detail.ModelCalls)
	}
	call := detail.ModelCalls[0]
	if call.RunID != result.Execution.RunID || call.ProjectID != project.ProjectID || call.StageType != "memory.compile" || call.NodeID == "" {
		t.Fatalf("model call binding=%#v", call)
	}
	if call.ProviderID != provider.ProviderID || call.Model != "growth-health-model" || call.TotalTokens != 100 || call.Status != "success" {
		t.Fatalf("model call=%#v", call)
	}
	if detail.ModelHealth.Calls != 1 || detail.ModelHealth.TotalTokens != 100 || detail.ModelHealth.CostStatus != "estimated" || detail.ModelHealth.PricedCalls != 1 {
		t.Fatalf("model health=%#v", detail.ModelHealth)
	}
}
