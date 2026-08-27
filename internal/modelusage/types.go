package modelusage

import "context"

type ContextInfo struct {
	RunID     string `json:"run_id,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	StageType string `json:"stage_type,omitempty"`
}

type contextKey struct{}

func WithContext(ctx context.Context, info ContextInfo) context.Context {
	return context.WithValue(ctx, contextKey{}, info)
}

func FromContext(ctx context.Context) ContextInfo {
	if value, ok := ctx.Value(contextKey{}).(ContextInfo); ok {
		return value
	}
	return ContextInfo{}
}

type Observation struct {
	CallID string `json:"call_id"`
	ContextInfo
	ProviderID              string `json:"provider_id"`
	Provider                string `json:"provider"`
	Model                   string `json:"model"`
	Status                  string `json:"status"`
	UsageSource             string `json:"usage_source"`
	PromptTokens            int    `json:"prompt_tokens"`
	CompletionTokens        int    `json:"completion_tokens"`
	TotalTokens             int    `json:"total_tokens"`
	ReasoningTokens         int    `json:"reasoning_tokens,omitempty"`
	CachedPromptTokens      int    `json:"cached_prompt_tokens,omitempty"`
	LatencyMS               int64  `json:"latency_ms"`
	Currency                string `json:"currency,omitempty"`
	EstimatedCostMicrominor int64  `json:"estimated_cost_microminor,omitempty"`
	PricingSource           string `json:"pricing_source"`
	ErrorCode               string `json:"error_code,omitempty"`
	CreatedAt               string `json:"created_at"`
}

type Summary struct {
	Calls                   int    `json:"calls"`
	SuccessfulCalls         int    `json:"successful_calls"`
	FailedCalls             int    `json:"failed_calls"`
	ProviderReportedCalls   int    `json:"provider_reported_calls"`
	PricedCalls             int    `json:"priced_calls"`
	PromptTokens            int    `json:"prompt_tokens"`
	CompletionTokens        int    `json:"completion_tokens"`
	TotalTokens             int    `json:"total_tokens"`
	ReasoningTokens         int    `json:"reasoning_tokens"`
	CachedPromptTokens      int    `json:"cached_prompt_tokens"`
	TotalLatencyMS          int64  `json:"total_latency_ms"`
	MaxLatencyMS            int64  `json:"max_latency_ms"`
	EstimatedCostMicrominor int64  `json:"estimated_cost_microminor"`
	Currency                string `json:"currency,omitempty"`
	CostStatus              string `json:"cost_status"`
}

func Aggregate(items []Observation) Summary {
	out := Summary{CostStatus: "unavailable"}
	currencies := map[string]bool{}
	for _, item := range items {
		out.Calls++
		if item.Status == "success" {
			out.SuccessfulCalls++
		} else {
			out.FailedCalls++
		}
		if item.UsageSource == "provider_reported" {
			out.ProviderReportedCalls++
		}
		out.PromptTokens += item.PromptTokens
		out.CompletionTokens += item.CompletionTokens
		out.TotalTokens += item.TotalTokens
		out.ReasoningTokens += item.ReasoningTokens
		out.CachedPromptTokens += item.CachedPromptTokens
		out.TotalLatencyMS += item.LatencyMS
		if item.LatencyMS > out.MaxLatencyMS {
			out.MaxLatencyMS = item.LatencyMS
		}
		if item.PricingSource == "provider_config" {
			out.PricedCalls++
			currencies[item.Currency] = true
			out.EstimatedCostMicrominor += item.EstimatedCostMicrominor
		}
	}
	if len(currencies) > 1 {
		out.CostStatus, out.Currency, out.EstimatedCostMicrominor = "mixed_currency", "", 0
	} else if out.PricedCalls > 0 {
		for currency := range currencies {
			out.Currency = currency
		}
		if out.PricedCalls == out.Calls {
			out.CostStatus = "estimated"
		} else {
			out.CostStatus = "partial_estimate"
		}
	}
	return out
}

type ProviderSummary struct {
	ProviderID string  `json:"provider_id"`
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	LastCallAt string  `json:"last_call_at,omitempty"`
	Health     Summary `json:"health"`
}

type Dashboard struct {
	WindowHours int               `json:"window_hours"`
	GeneratedAt string            `json:"generated_at"`
	Health      Summary           `json:"health"`
	Providers   []ProviderSummary `json:"providers"`
}

func Merge(summaries []Summary) Summary {
	out := Summary{CostStatus: "unavailable"}
	currencies := map[string]bool{}
	for _, item := range summaries {
		out.Calls += item.Calls
		out.SuccessfulCalls += item.SuccessfulCalls
		out.FailedCalls += item.FailedCalls
		out.ProviderReportedCalls += item.ProviderReportedCalls
		out.PricedCalls += item.PricedCalls
		out.PromptTokens += item.PromptTokens
		out.CompletionTokens += item.CompletionTokens
		out.TotalTokens += item.TotalTokens
		out.ReasoningTokens += item.ReasoningTokens
		out.CachedPromptTokens += item.CachedPromptTokens
		out.TotalLatencyMS += item.TotalLatencyMS
		if item.MaxLatencyMS > out.MaxLatencyMS {
			out.MaxLatencyMS = item.MaxLatencyMS
		}
		if item.CostStatus == "mixed_currency" {
			currencies["__mixed__"] = true
		} else if item.PricedCalls > 0 {
			currencies[item.Currency] = true
			out.EstimatedCostMicrominor += item.EstimatedCostMicrominor
		}
	}
	if len(currencies) > 1 || currencies["__mixed__"] {
		out.CostStatus, out.Currency, out.EstimatedCostMicrominor = "mixed_currency", "", 0
	} else if out.PricedCalls > 0 {
		for currency := range currencies {
			out.Currency = currency
		}
		if out.PricedCalls == out.Calls {
			out.CostStatus = "estimated"
		} else {
			out.CostStatus = "partial_estimate"
		}
	}
	return out
}
