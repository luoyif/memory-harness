package modelconfig

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/modelusage"
)

type providerUsageEnvelope struct {
	Usage json.RawMessage `json:"usage"`
}

type providerUsage struct {
	PromptTokens       int `json:"prompt_tokens"`
	CompletionTokens   int `json:"completion_tokens"`
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	PromptCacheHit     int `json:"prompt_cache_hit_tokens"`
	CacheReadInput     int `json:"cache_read_input_tokens"`
	CacheCreationInput int `json:"cache_creation_input_tokens"`
	InputTokenDetails  struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokenDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	PromptTokenDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func parseProviderUsage(raw []byte) (providerUsage, string) {
	var envelope providerUsageEnvelope
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Usage) == 0 || string(envelope.Usage) == "null" {
		return providerUsage{}, "unavailable"
	}
	var usage providerUsage
	if json.Unmarshal(envelope.Usage, &usage) != nil {
		return providerUsage{}, "unavailable"
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	usage.PromptTokenDetails.CachedTokens = max(usage.PromptTokenDetails.CachedTokens, usage.InputTokenDetails.CachedTokens, usage.CacheReadInput)
	usage.CompletionDetails.ReasoningTokens = max(usage.CompletionDetails.ReasoningTokens, usage.OutputTokenDetails.ReasoningTokens)
	return usage, "provider_reported"
}
func normalizePricing(input PricingInput) (PricingInput, error) {
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.InputPerMillionMinor < 0 || input.OutputPerMillionMinor < 0 {
		return input, errors.New("model pricing cannot be negative")
	}
	if input.InputPerMillionMinor == 0 && input.OutputPerMillionMinor == 0 {
		input.Currency = ""
		return input, nil
	}
	if len(input.Currency) != 3 {
		return input, errors.New("pricing currency must be a 3-letter code when rates are configured")
	}
	for _, r := range input.Currency {
		if r < 'A' || r > 'Z' {
			return input, errors.New("pricing currency must contain only A-Z")
		}
	}
	return input, nil
}

func (s *Service) savePricing(ctx context.Context, providerID string, input *PricingInput) error {
	if input == nil {
		return nil
	}
	pricing, err := normalizePricing(*input)
	if err != nil {
		return err
	}
	if pricing.Currency == "" {
		_, err = s.control.DB.ExecContext(ctx, `DELETE FROM model_pricing WHERE provider_id=?`, providerID)
		return err
	}
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO model_pricing(provider_id,currency,input_per_million_minor,output_per_million_minor,updated_at)
VALUES(?,?,?,?,?) ON CONFLICT(provider_id) DO UPDATE SET currency=excluded.currency,input_per_million_minor=excluded.input_per_million_minor,output_per_million_minor=excluded.output_per_million_minor,updated_at=excluded.updated_at`,
		providerID, pricing.Currency, pricing.InputPerMillionMinor, pricing.OutputPerMillionMinor, nowString())
	return err
}
func (s *Service) loadPricing(ctx context.Context, providerID string) Pricing {
	var item Pricing
	err := s.control.DB.QueryRowContext(ctx, `SELECT currency,input_per_million_minor,output_per_million_minor FROM model_pricing WHERE provider_id=?`, providerID).
		Scan(&item.Currency, &item.InputPerMillionMinor, &item.OutputPerMillionMinor)
	if err == nil {
		item.Configured = true
	}
	return item
}

func (s *Service) recordModelCall(ctx context.Context, provider Provider, started time.Time, status string, responseRaw []byte, errorCode string) modelusage.Observation {
	usage, usageSource := parseProviderUsage(responseRaw)
	pricing := provider.Pricing
	item := modelusage.Observation{
		CallID: modelusage.NewCallID(), ContextInfo: modelusage.FromContext(ctx),
		ProviderID: provider.ProviderID, Provider: provider.Kind, Model: provider.Model,
		Status: status, UsageSource: usageSource, PromptTokens: usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		ReasoningTokens:    usage.CompletionDetails.ReasoningTokens,
		CachedPromptTokens: max(usage.PromptCacheHit, usage.PromptTokenDetails.CachedTokens),
		LatencyMS:          time.Since(started).Milliseconds(), PricingSource: "unavailable",
		ErrorCode: errorCode, CreatedAt: nowString(),
	}
	if item.TotalTokens == 0 && (item.PromptTokens > 0 || item.CompletionTokens > 0) {
		item.TotalTokens = item.PromptTokens + item.CompletionTokens
	}
	if pricing.Configured && usageSource == "provider_reported" {
		item.Currency, item.PricingSource = pricing.Currency, "provider_config"
		item.EstimatedCostMicrominor = int64(item.PromptTokens)*pricing.InputPerMillionMinor + int64(item.CompletionTokens)*pricing.OutputPerMillionMinor
	}
	_ = modelusage.Insert(context.WithoutCancel(ctx), s.control.DB, item)
	return item
}
