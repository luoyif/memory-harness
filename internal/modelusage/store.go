package modelusage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

func NewCallID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err == nil {
		return "model-call-" + hex.EncodeToString(raw)
	}
	return "model-call-" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}

func Insert(ctx context.Context, db *sql.DB, item Observation) error {
	_, err := db.ExecContext(ctx, `INSERT INTO model_call_observations(
call_id,run_id,node_id,project_id,stage_type,provider_id,provider_kind,model,status,usage_source,
prompt_tokens,completion_tokens,total_tokens,reasoning_tokens,cached_prompt_tokens,latency_ms,currency,
estimated_cost_microminor,pricing_source,error_code,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.CallID, item.RunID, item.NodeID, item.ProjectID, item.StageType, item.ProviderID, item.Provider, item.Model,
		item.Status, item.UsageSource, item.PromptTokens, item.CompletionTokens, item.TotalTokens, item.ReasoningTokens,
		item.CachedPromptTokens, item.LatencyMS, item.Currency, item.EstimatedCostMicrominor, item.PricingSource, item.ErrorCode, item.CreatedAt)
	return err
}
func scanObservation(scanner interface{ Scan(...any) error }) (Observation, error) {
	var item Observation
	err := scanner.Scan(
		&item.CallID, &item.RunID, &item.NodeID, &item.ProjectID, &item.StageType,
		&item.ProviderID, &item.Provider, &item.Model, &item.Status, &item.UsageSource,
		&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.ReasoningTokens,
		&item.CachedPromptTokens, &item.LatencyMS, &item.Currency, &item.EstimatedCostMicrominor,
		&item.PricingSource, &item.ErrorCode, &item.CreatedAt,
	)
	return item, err
}

const observationColumns = `call_id,run_id,node_id,project_id,stage_type,provider_id,provider_kind,model,status,usage_source,
prompt_tokens,completion_tokens,total_tokens,reasoning_tokens,cached_prompt_tokens,latency_ms,currency,
estimated_cost_microminor,pricing_source,error_code,created_at`

func ListByRun(ctx context.Context, db *sql.DB, runID string) ([]Observation, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+observationColumns+` FROM model_call_observations WHERE run_id=? ORDER BY created_at,call_id`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Observation{}
	for rows.Next() {
		item, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func DashboardForWindow(ctx context.Context, db *sql.DB, hours int) (Dashboard, error) {
	if hours < 1 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339Nano)
	rows, err := db.QueryContext(ctx, `SELECT provider_id,provider_kind,model,
COUNT(*),SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),SUM(CASE WHEN status<>'success' THEN 1 ELSE 0 END),
SUM(CASE WHEN usage_source='provider_reported' THEN 1 ELSE 0 END),SUM(prompt_tokens),SUM(completion_tokens),SUM(total_tokens),
SUM(reasoning_tokens),SUM(cached_prompt_tokens),SUM(latency_ms),MAX(latency_ms),MAX(created_at),
COUNT(DISTINCT CASE WHEN pricing_source='provider_config' THEN currency END),
MIN(CASE WHEN pricing_source='provider_config' THEN currency END),
SUM(CASE WHEN pricing_source='provider_config' THEN estimated_cost_microminor ELSE 0 END),
SUM(CASE WHEN pricing_source='provider_config' THEN 1 ELSE 0 END)
FROM model_call_observations WHERE created_at>=? GROUP BY provider_id,provider_kind,model ORDER BY MAX(created_at) DESC`, since)
	if err != nil {
		return Dashboard{}, err
	}
	defer rows.Close()
	out := Dashboard{WindowHours: hours, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Providers: []ProviderSummary{}}
	for rows.Next() {
		var item ProviderSummary
		var currency sql.NullString
		var currencyCount, pricedCalls int
		if err := rows.Scan(&item.ProviderID, &item.Provider, &item.Model,
			&item.Health.Calls, &item.Health.SuccessfulCalls, &item.Health.FailedCalls, &item.Health.ProviderReportedCalls,
			&item.Health.PromptTokens, &item.Health.CompletionTokens, &item.Health.TotalTokens, &item.Health.ReasoningTokens,
			&item.Health.CachedPromptTokens, &item.Health.TotalLatencyMS, &item.Health.MaxLatencyMS, &item.LastCallAt,
			&currencyCount, &currency, &item.Health.EstimatedCostMicrominor, &pricedCalls); err != nil {
			return Dashboard{}, err
		}
		item.Health.PricedCalls = pricedCalls
		if pricedCalls == 0 {
			item.Health.CostStatus = "unavailable"
			item.Health.EstimatedCostMicrominor = 0
		} else if currencyCount == 1 {
			item.Health.Currency = currency.String
			if pricedCalls == item.Health.Calls {
				item.Health.CostStatus = "estimated"
			} else {
				item.Health.CostStatus = "partial_estimate"
			}
		} else {
			item.Health.CostStatus, item.Health.EstimatedCostMicrominor = "mixed_currency", 0
		}
		out.Providers = append(out.Providers, item)
	}
	if err := rows.Err(); err != nil {
		return Dashboard{}, err
	}
	summaries := make([]Summary, 0, len(out.Providers))
	for _, item := range out.Providers {
		summaries = append(summaries, item.Health)
	}
	out.Health = Merge(summaries)
	return out, nil
}
