package harness_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/modelusage"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestRunDetailAggregatesBoundModelCalls(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "model-health", Name: "Model Health", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Harness.StartRun(t.Context(), harness.StartRunInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "test",
		PipelineID: "pipeline.health", PipelineVersion: "1.0.0", PipelineHash: "hash-health",
		IdempotencyKey: "health-run-1", Snapshot: []byte(`{"purpose":"health"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	first := modelusage.Observation{CallID: "call-health-1", ContextInfo: modelusage.ContextInfo{RunID: run.RunID, NodeID: "node-model", ProjectID: project.ProjectID, StageType: "llm.extract"}, ProviderID: "provider-a", Provider: "openai_compatible", Model: "model-a", Status: "success", UsageSource: "provider_reported", PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140, LatencyMS: 600, Currency: "USD", EstimatedCostMicrominor: 50000, PricingSource: "provider_config", CreatedAt: "2026-08-24T06:00:00Z"}
	if err := modelusage.Insert(t.Context(), a.Control.DB, first); err != nil {
		t.Fatal(err)
	}
	second := modelusage.Observation{CallID: "call-health-2", ContextInfo: modelusage.ContextInfo{RunID: run.RunID, NodeID: "node-model", ProjectID: project.ProjectID, StageType: "llm.extract"}, ProviderID: "provider-a", Provider: "openai_compatible", Model: "model-a", Status: "failed", UsageSource: "provider_reported", PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60, ReasoningTokens: 4, CachedPromptTokens: 12, LatencyMS: 900, Currency: "USD", EstimatedCostMicrominor: 10000, PricingSource: "provider_config", ErrorCode: "output_contract_error", CreatedAt: "2026-08-24T06:00:01Z"}
	if err := modelusage.Insert(t.Context(), a.Control.DB, second); err != nil {
		t.Fatal(err)
	}
	other := modelusage.Observation{CallID: "call-other-run", ContextInfo: modelusage.ContextInfo{RunID: "other-run", ProjectID: project.ProjectID}, ProviderID: "provider-a", Provider: "openai_compatible", Model: "model-a", Status: "success", UsageSource: "provider_reported", TotalTokens: 999, LatencyMS: 1, PricingSource: "unavailable", CreatedAt: "2026-08-24T06:00:02Z"}
	if err := modelusage.Insert(t.Context(), a.Control.DB, other); err != nil {
		t.Fatal(err)
	}

	detail, err := a.Harness.RunDetail(t.Context(), run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ModelCalls) != 2 {
		t.Fatalf("model calls=%#v", detail.ModelCalls)
	}
	if detail.ModelHealth.Calls != 2 || detail.ModelHealth.SuccessfulCalls != 1 || detail.ModelHealth.FailedCalls != 1 {
		t.Fatalf("call health=%#v", detail.ModelHealth)
	}
	if detail.ModelHealth.TotalTokens != 200 || detail.ModelHealth.ReasoningTokens != 4 || detail.ModelHealth.CachedPromptTokens != 12 || detail.ModelHealth.MaxLatencyMS != 900 {
		t.Fatalf("usage health=%#v", detail.ModelHealth)
	}
	if detail.ModelHealth.CostStatus != "estimated" || detail.ModelHealth.Currency != "USD" || detail.ModelHealth.EstimatedCostMicrominor != 60000 {
		t.Fatalf("cost health=%#v", detail.ModelHealth)
	}
}
