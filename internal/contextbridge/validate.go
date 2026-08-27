package contextbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxContractBytes = 2 << 20
	MaxPlanItems     = 256
)

var capabilityVocabulary = map[string]bool{
	"recall": true, "capture": true, "pre_turn_injection": true,
	"thread_lifecycle": true, "item_lifecycle": true, "compaction_hook": true,
	"approval_callback": true, "context_receipt": true, "outcome_feedback": true,
}

func canonicalHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(raw) > MaxContractBytes {
		return "", errors.New("contract exceeds 2 MiB")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func requireRFC3339(name, value string, optional bool) error {
	value = strings.TrimSpace(value)
	if value == "" && optional {
		return nil
	}
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		if _, nanoErr := time.Parse(time.RFC3339Nano, value); nanoErr != nil {
			return fmt.Errorf("%s must be RFC3339", name)
		}
	}
	return nil
}

func validateRetention(value RetentionPolicy) error {
	switch value.Mode {
	case "none", "session", "ttl", "provider_policy":
	default:
		return errors.New("invalid retention.mode")
	}
	switch value.Redaction {
	case "none", "supported", "required", "unknown":
	default:
		return errors.New("invalid retention.redaction")
	}
	if value.Mode == "ttl" && value.TTLSeconds <= 0 {
		return errors.New("retention ttl_seconds must be positive")
	}
	return nil
}

func ValidateCapabilitySet(value ContextCapabilitySet) error {
	if value.SchemaVersion != CapabilitySchemaVersion {
		return errors.New("unsupported capability schema_version")
	}
	if strings.TrimSpace(value.CapabilitySetID) == "" || strings.TrimSpace(value.AdapterID) == "" || strings.TrimSpace(value.Runtime) == "" || strings.TrimSpace(value.ProtocolVersion) == "" || strings.TrimSpace(value.Transport) == "" {
		return errors.New("capability identity fields are required")
	}
	if value.MaxContextItems <= 0 || value.MaxContextItems > MaxPlanItems || value.MaxItemBytes <= 0 || value.MaxTotalBytes <= 0 || value.MaxItemBytes > value.MaxTotalBytes {
		return errors.New("invalid capability size limits")
	}
	if value.MaxConcurrent <= 0 || value.MaxConcurrent > 1024 {
		return errors.New("invalid max_concurrent")
	}
	if len(value.Capabilities) == 0 {
		return errors.New("at least one capability is required")
	}
	seen := map[string]bool{}
	for _, item := range value.Capabilities {
		if !capabilityVocabulary[item] {
			return fmt.Errorf("unsupported capability %q", item)
		}
		if seen[item] {
			return errors.New("duplicate capability")
		}
		seen[item] = true
	}
	return validateRetention(value.Retention)
}

func ValidatePlan(value ContextPlan) error {
	if value.SchemaVersion != PlanSchemaVersion {
		return errors.New("unsupported plan schema_version")
	}
	if strings.TrimSpace(value.PlanID) == "" || strings.TrimSpace(value.ProjectID) == "" || strings.TrimSpace(value.AgentID) == "" || strings.TrimSpace(value.RequestFingerprint) == "" || strings.TrimSpace(value.IdempotencyKey) == "" {
		return errors.New("plan identity, scope, fingerprint and idempotency are required")
	}
	if err := requireRFC3339("created_at", value.CreatedAt, false); err != nil {
		return err
	}
	if err := requireRFC3339("expires_at", value.ExpiresAt, true); err != nil {
		return err
	}
	if value.Budget.MaxTokens < 0 || value.Budget.MaxChars < 0 || value.Budget.MaxLatencyMS < 0 || value.Budget.MaxCostMinor < 0 {
		return errors.New("context budget values must be non-negative")
	}
	if len(value.Items) > MaxPlanItems {
		return errors.New("too many context plan items")
	}
	seen := map[string]bool{}
	for i, item := range value.Items {
		if item.ItemID == "" || item.SourceID == "" || item.ContentHash == "" || item.ProjectID != value.ProjectID || len(item.ReasonCodes) == 0 {
			return fmt.Errorf("items[%d] lacks stable identity/hash/project/reason", i)
		}
		if seen[item.ItemID] {
			return fmt.Errorf("duplicate item_id %q", item.ItemID)
		}
		seen[item.ItemID] = true
		if item.SourceKind != "object" && item.SourceKind != "evidence" {
			return fmt.Errorf("items[%d].source_kind must be object or evidence", i)
		}
		if item.SourceKind == "object" && item.Revision <= 0 {
			return fmt.Errorf("items[%d] object revision is required", i)
		}
		switch item.Presentation {
		case "summary", "verbatim", "profile", "skill_index", "object":
		default:
			return fmt.Errorf("items[%d] invalid presentation", i)
		}
		if item.Priority < 0 || item.Priority > 100 {
			return fmt.Errorf("items[%d] invalid priority", i)
		}
		if err := requireRFC3339(fmt.Sprintf("items[%d].valid_from", i), item.ValidFrom, true); err != nil {
			return err
		}
		if err := requireRFC3339(fmt.Sprintf("items[%d].valid_until", i), item.ValidUntil, true); err != nil {
			return err
		}
	}
	return nil
}

func ValidateReceipt(plan ContextPlan, value ContextReceipt) error {
	if value.SchemaVersion != ReceiptSchemaVersion {
		return errors.New("unsupported receipt schema_version")
	}
	if value.ReceiptID == "" || value.PlanID != plan.PlanID || value.ProjectID != plan.ProjectID || value.AgentID != plan.AgentID || value.IdempotencyKey == "" {
		return errors.New("receipt identity/scope does not match plan")
	}
	if err := requireRFC3339("received_at", value.ReceivedAt, false); err != nil {
		return err
	}
	switch value.EvidenceLevel {
	case "provider_reported", "harness_observed", "transport_ack":
	default:
		return errors.New("invalid receipt evidence_level")
	}
	switch value.Completeness {
	case "complete", "partial", "unverified":
	default:
		return errors.New("invalid receipt completeness")
	}
	if err := validateRetention(value.Retention); err != nil {
		return err
	}
	planned := map[string]ContextPlanItem{}
	for _, item := range plan.Items {
		planned[item.ItemID] = item
	}
	seen := map[string]bool{}
	for i, item := range value.Items {
		source, ok := planned[item.ItemID]
		if !ok {
			return fmt.Errorf("receipt items[%d] references unknown plan item", i)
		}
		if seen[item.ItemID] {
			return fmt.Errorf("duplicate receipt item %q", item.ItemID)
		}
		seen[item.ItemID] = true
		switch item.Status {
		case "delivered", "trimmed", "denied", "failed", "delivery_unverified":
		default:
			return fmt.Errorf("receipt items[%d] invalid status", i)
		}
		if item.Revision != 0 && item.Revision != source.Revision {
			return fmt.Errorf("receipt items[%d] revision mismatch", i)
		}
		if item.ContentHash != "" && item.ContentHash != source.ContentHash {
			return fmt.Errorf("receipt items[%d] content_hash mismatch", i)
		}
	}
	if value.Completeness == "complete" && len(seen) != len(plan.Items) {
		return errors.New("complete receipt must report every plan item")
	}
	return nil
}

func ValidateOutcome(value OutcomeFeedback) error {
	if value.SchemaVersion != OutcomeSchemaVersion {
		return errors.New("unsupported outcome schema_version")
	}
	if value.OutcomeID == "" || value.ProjectID == "" || value.AgentID == "" || value.RunID == "" || value.Source == "" || value.IdempotencyKey == "" {
		return errors.New("outcome identity/scope/source/idempotency are required")
	}
	if err := requireRFC3339("observed_at", value.ObservedAt, false); err != nil {
		return err
	}
	if len(value.Metrics) == 0 {
		return errors.New("outcome metrics are required")
	}
	for i, metric := range value.Metrics {
		if metric.Name == "" || len(metric.Value) == 0 || !json.Valid(metric.Value) || metric.Confidence < 0 || metric.Confidence > 1 {
			return fmt.Errorf("invalid outcome metric %d", i)
		}
	}
	if value.Cost.Tokens < 0 || value.Cost.LatencyMS < 0 || value.Cost.MoneyMinor < 0 || value.Cost.SafetyEvents < 0 {
		return errors.New("outcome cost values must be non-negative")
	}
	return nil
}

func HashCapabilitySet(value ContextCapabilitySet) (string, error) { return canonicalHash(value) }
func HashPlan(value ContextPlan) (string, error)                   { value.PlanHash = ""; return canonicalHash(value) }
func HashReceipt(value ContextReceipt) (string, error) {
	value.ReceiptHash = ""
	return canonicalHash(value)
}
func HashOutcome(value OutcomeFeedback) (string, error) {
	value.OutcomeHash = ""
	return canonicalHash(value)
}

func EffectiveDeliveryStatus(plan ContextPlan, receipt *ContextReceipt) map[string]string {
	out := map[string]string{}
	for _, item := range plan.Items {
		out[item.ItemID] = "delivery_unverified"
	}
	if receipt == nil {
		return out
	}
	for _, item := range receipt.Items {
		out[item.ItemID] = item.Status
	}
	return out
}
