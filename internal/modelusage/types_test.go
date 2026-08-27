package modelusage

import "testing"

func TestAggregateMarksPartialEstimateWhenOnlySomeCallsArePriceable(t *testing.T) {
	items := []Observation{
		{Status: "success", UsageSource: "provider_reported", TotalTokens: 100, Currency: "USD", EstimatedCostMicrominor: 50000, PricingSource: "provider_config"},
		{Status: "failed", UsageSource: "unavailable", TotalTokens: 0, PricingSource: "unavailable"},
	}
	got := Aggregate(items)
	if got.Calls != 2 || got.PricedCalls != 1 || got.CostStatus != "partial_estimate" || got.Currency != "USD" || got.EstimatedCostMicrominor != 50000 {
		t.Fatalf("partial aggregate=%#v", got)
	}
}

func TestAggregateNeverAddsDifferentCurrencies(t *testing.T) {
	items := []Observation{
		{Status: "success", UsageSource: "provider_reported", Currency: "USD", EstimatedCostMicrominor: 50000, PricingSource: "provider_config"},
		{Status: "success", UsageSource: "provider_reported", Currency: "CNY", EstimatedCostMicrominor: 70000, PricingSource: "provider_config"},
	}
	got := Aggregate(items)
	if got.CostStatus != "mixed_currency" || got.Currency != "" || got.EstimatedCostMicrominor != 0 || got.PricedCalls != 2 {
		t.Fatalf("mixed aggregate=%#v", got)
	}
}
