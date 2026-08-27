package contextbridge

import "encoding/json"

const (
	CapabilitySchemaVersion = "memory-harness.context-capability-set/v1alpha1"
	PlanSchemaVersion       = "memory-harness.context-plan/v1alpha1"
	ReceiptSchemaVersion    = "memory-harness.context-receipt/v1alpha1"
	OutcomeSchemaVersion    = "memory-harness.outcome-feedback/v1alpha1"

	PermissionContextPlan    = "context.plan"
	PermissionContextReceipt = "context.receipt"
	PermissionOutcomeReport  = "outcome.report"
)

type ExternalCorrelation struct {
	Runtime         string `json:"external_runtime,omitempty"`
	ProtocolVersion string `json:"external_protocol_version,omitempty"`
	ThreadID        string `json:"external_thread_id,omitempty"`
	TurnID          string `json:"external_turn_id,omitempty"`
	ItemID          string `json:"external_item_id,omitempty"`
}

type RetentionPolicy struct {
	Mode       string `json:"mode"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
	Redaction  string `json:"redaction"`
}

type ContextCapabilitySet struct {
	SchemaVersion       string              `json:"schema_version"`
	CapabilitySetID     string              `json:"capability_set_id"`
	AdapterID           string              `json:"adapter_id"`
	Runtime             string              `json:"runtime"`
	ProtocolVersion     string              `json:"protocol_version"`
	Transport           string              `json:"transport"`
	Capabilities        []string            `json:"capabilities"`
	MaxContextItems     int                 `json:"max_context_items"`
	MaxItemBytes        int                 `json:"max_item_bytes"`
	MaxTotalBytes       int                 `json:"max_total_bytes"`
	MaxConcurrent       int                 `json:"max_concurrent"`
	SupportsStreaming   bool                `json:"supports_streaming"`
	SupportsIdempotency bool                `json:"supports_idempotency"`
	Retention           RetentionPolicy     `json:"retention"`
	Correlation         ExternalCorrelation `json:"correlation,omitempty"`
}

type ContextBudget struct {
	MaxTokens    int   `json:"max_tokens,omitempty"`
	MaxChars     int   `json:"max_chars,omitempty"`
	MaxLatencyMS int   `json:"max_latency_ms,omitempty"`
	MaxCostMinor int64 `json:"max_cost_minor,omitempty"`
}

type ContextPlanItem struct {
	ItemID        string   `json:"item_id"`
	SourceKind    string   `json:"source_kind"`
	SourceID      string   `json:"source_id"`
	Revision      int      `json:"revision,omitempty"`
	ContentHash   string   `json:"content_hash"`
	ProjectID     string   `json:"project_id"`
	ReasonCodes   []string `json:"reason_codes"`
	Priority      int      `json:"priority"`
	TokenEstimate int      `json:"token_estimate,omitempty"`
	Presentation  string   `json:"presentation"`
	ValidFrom     string   `json:"valid_from,omitempty"`
	ValidUntil    string   `json:"valid_until,omitempty"`
	TTLSeconds    int64    `json:"ttl_seconds,omitempty"`
	PolicyLabels  []string `json:"policy_labels,omitempty"`
	SourceRefs    []string `json:"source_refs,omitempty"`
}

type ContextPlan struct {
	SchemaVersion      string              `json:"schema_version"`
	PlanID             string              `json:"plan_id"`
	ProjectID          string              `json:"project_id"`
	AgentID            string              `json:"agent_id"`
	RequestFingerprint string              `json:"request_fingerprint"`
	BlueprintID        string              `json:"blueprint_id,omitempty"`
	BlueprintVersion   string              `json:"blueprint_version,omitempty"`
	BlueprintHash      string              `json:"blueprint_hash,omitempty"`
	Budget             ContextBudget       `json:"budget"`
	Items              []ContextPlanItem   `json:"items"`
	Correlation        ExternalCorrelation `json:"correlation,omitempty"`
	IdempotencyKey     string              `json:"idempotency_key"`
	CreatedAt          string              `json:"created_at"`
	ExpiresAt          string              `json:"expires_at,omitempty"`
	PlanHash           string              `json:"plan_hash,omitempty"`
}

type ReceiptItem struct {
	ItemID       string `json:"item_id"`
	Status       string `json:"status"`
	Revision     int    `json:"revision,omitempty"`
	ContentHash  string `json:"content_hash,omitempty"`
	Presentation string `json:"presentation,omitempty"`
	ActualTokens int    `json:"actual_tokens,omitempty"`
	ActualChars  int    `json:"actual_chars,omitempty"`
	Compaction   string `json:"compaction,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

type ContextReceipt struct {
	SchemaVersion  string              `json:"schema_version"`
	ReceiptID      string              `json:"receipt_id"`
	PlanID         string              `json:"plan_id"`
	ProjectID      string              `json:"project_id"`
	AgentID        string              `json:"agent_id"`
	EvidenceLevel  string              `json:"evidence_level"`
	Completeness   string              `json:"completeness"`
	Items          []ReceiptItem       `json:"items"`
	Correlation    ExternalCorrelation `json:"correlation,omitempty"`
	LatencyMS      int                 `json:"latency_ms,omitempty"`
	Retention      RetentionPolicy     `json:"retention"`
	IdempotencyKey string              `json:"idempotency_key"`
	ReceivedAt     string              `json:"received_at"`
	ReceiptHash    string              `json:"receipt_hash,omitempty"`
}

type PlanRequest struct {
	ProjectID      string               `json:"project_id"`
	AgentID        string               `json:"-"`
	Query          string               `json:"query"`
	Kinds          []string             `json:"kinds,omitempty"`
	CapabilitySet  ContextCapabilitySet `json:"capability_set"`
	Budget         ContextBudget        `json:"budget"`
	Correlation    ExternalCorrelation  `json:"correlation,omitempty"`
	IdempotencyKey string               `json:"idempotency_key"`
}

type PlanResult struct {
	RunID          string            `json:"run_id"`
	Plan           ContextPlan       `json:"plan"`
	DeliveryStatus map[string]string `json:"delivery_status"`
	Duplicate      bool              `json:"duplicate"`
}

type ReceiptRequest struct {
	RunID   string         `json:"run_id"`
	AgentID string         `json:"-"`
	Receipt ContextReceipt `json:"receipt"`
}

type ReceiptResult struct {
	RunID          string            `json:"run_id"`
	Receipt        ContextReceipt    `json:"receipt"`
	DeliveryStatus map[string]string `json:"delivery_status"`
	Duplicate      bool              `json:"duplicate"`
}

type OutcomeResult struct {
	RunID     string          `json:"run_id"`
	Outcome   OutcomeFeedback `json:"outcome"`
	Duplicate bool            `json:"duplicate"`
}

type OutcomeMetric struct {
	Name       string          `json:"name"`
	Value      json.RawMessage `json:"value"`
	Confidence float64         `json:"confidence"`
}

type OutcomeCost struct {
	Tokens       int   `json:"tokens,omitempty"`
	LatencyMS    int   `json:"latency_ms,omitempty"`
	MoneyMinor   int64 `json:"money_minor,omitempty"`
	SafetyEvents int   `json:"safety_events,omitempty"`
}

type OutcomeFeedback struct {
	SchemaVersion  string              `json:"schema_version"`
	OutcomeID      string              `json:"outcome_id"`
	ProjectID      string              `json:"project_id"`
	AgentID        string              `json:"agent_id"`
	RunID          string              `json:"run_id"`
	PlanID         string              `json:"plan_id,omitempty"`
	ReceiptID      string              `json:"receipt_id,omitempty"`
	Source         string              `json:"source"`
	EvaluatorID    string              `json:"evaluator_id,omitempty"`
	EvaluatorVer   string              `json:"evaluator_version,omitempty"`
	Metrics        []OutcomeMetric     `json:"metrics"`
	Cost           OutcomeCost         `json:"cost"`
	ArtifactRefs   []string            `json:"artifact_refs,omitempty"`
	Correlation    ExternalCorrelation `json:"correlation,omitempty"`
	IdempotencyKey string              `json:"idempotency_key"`
	ObservedAt     string              `json:"observed_at"`
	OutcomeHash    string              `json:"outcome_hash,omitempty"`
}
