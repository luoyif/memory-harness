package harness

import (
	"encoding/json"

	"github.com/luoyif/memory-harness/internal/modelusage"
)

const (
	GovernedAgentAssetTypeV2 = "builtin.agent-assets.governed-asset.v2"
	GovernedAgentAssetTypeV3 = "builtin.agent-assets.governed-asset.v3"
	GovernedAgentAssetTypeV4 = "builtin.agent-assets.governed-asset.v4"
	KnowledgeProductTypeV1   = "builtin.living-asset-vault.knowledge-product.v1"
	ProfileProjectionTypeV1  = "builtin.living-asset-vault.profile-projection.v1"
	TraceSchemaVersion       = "memory-harness.trace/v1alpha1"
	CorePluginID             = "builtin.memory-harness-core"
)

type Lifecycle struct {
	Initial     string              `json:"initial"`
	States      []string            `json:"states"`
	Transitions map[string][]string `json:"transitions"`
}

type MemoryType struct {
	TypeID          string          `json:"type_id"`
	PluginID        string          `json:"plugin_id"`
	DisplayName     string          `json:"display_name"`
	SchemaVersion   string          `json:"schema_version"`
	Schema          json.RawMessage `json:"schema"`
	Lifecycle       Lifecycle       `json:"lifecycle"`
	ProtectionClass string          `json:"protection_class"`
	Renderer        json.RawMessage `json:"renderer"`
	Status          string          `json:"status"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type RegisterTypeInput struct {
	TypeID          string          `json:"type_id"`
	PluginID        string          `json:"plugin_id"`
	DisplayName     string          `json:"display_name"`
	SchemaVersion   string          `json:"schema_version"`
	Schema          json.RawMessage `json:"schema"`
	Lifecycle       Lifecycle       `json:"lifecycle"`
	ProtectionClass string          `json:"protection_class"`
	Renderer        json.RawMessage `json:"renderer"`
}

type MaterializeInput struct {
	ObjectID          string          `json:"object_id"`
	TypeID            string          `json:"type_id"`
	ProjectID         string          `json:"project_id"`
	Status            string          `json:"status"`
	Payload           json.RawMessage `json:"payload"`
	Confidence        float64         `json:"confidence"`
	Importance        float64         `json:"importance"`
	ValidFrom         string          `json:"valid_from"`
	ValidUntil        string          `json:"valid_until"`
	SourceEvidenceIDs []string        `json:"source_evidence_ids"`
	SourceObjectIDs   []string        `json:"source_object_ids"`
	RunID             string          `json:"run_id"`
	StageID           string          `json:"stage_id"`
	PluginID          string          `json:"plugin_id"`
	PluginVersion     string          `json:"plugin_version"`
	IdempotencyKey    string          `json:"idempotency_key"`
	LivingAssetPath   string          `json:"living_asset_path"`
}

type ObjectRevision struct {
	ObjectID          string          `json:"object_id"`
	Revision          int             `json:"revision"`
	Status            string          `json:"status"`
	SchemaVersion     string          `json:"schema_version"`
	Payload           json.RawMessage `json:"payload"`
	ContentHash       string          `json:"content_hash"`
	Confidence        float64         `json:"confidence"`
	Importance        float64         `json:"importance"`
	ValidFrom         string          `json:"valid_from"`
	ValidUntil        string          `json:"valid_until,omitempty"`
	SourceEvidenceIDs []string        `json:"source_evidence_ids"`
	SourceObjectIDs   []string        `json:"source_object_ids"`
	RunID             string          `json:"run_id,omitempty"`
	StageID           string          `json:"stage_id,omitempty"`
	PluginID          string          `json:"plugin_id"`
	PluginVersion     string          `json:"plugin_version"`
	IdempotencyKey    string          `json:"idempotency_key"`
	LivingAssetPath   string          `json:"living_asset_path,omitempty"`
	CreatedAt         string          `json:"created_at"`
}

type Object struct {
	ObjectID        string         `json:"object_id"`
	TypeID          string         `json:"type_id"`
	ProjectID       string         `json:"project_id"`
	Status          string         `json:"status"`
	ProtectionClass string         `json:"protection_class"`
	CurrentRevision int            `json:"current_revision"`
	Revision        ObjectRevision `json:"revision"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	Duplicate       bool           `json:"duplicate,omitempty"`
}

type ProposeRevisionInput struct {
	Payload           json.RawMessage `json:"payload"`
	ExpectedRevision  int             `json:"expected_revision"`
	EditReason        string          `json:"edit_reason"`
	TargetStatus      string          `json:"target_status"`
	Confidence        float64         `json:"confidence"`
	Importance        float64         `json:"importance"`
	ValidFrom         string          `json:"valid_from"`
	ValidUntil        string          `json:"valid_until"`
	SourceEvidenceIDs []string        `json:"source_evidence_ids"`
	SourceObjectIDs   []string        `json:"source_object_ids"`
	PluginID          string          `json:"plugin_id"`
	PluginVersion     string          `json:"plugin_version"`
	IdempotencyKey    string          `json:"idempotency_key"`
	RequestedBy       string          `json:"requested_by"`
	Validation        json.RawMessage `json:"validation"`
	RollbackFrom      int             `json:"rollback_from_revision,omitempty"`
}

type RevisionReview struct {
	ReviewID             string          `json:"review_id"`
	ObjectID             string          `json:"object_id"`
	Revision             int             `json:"revision"`
	BaseRevision         int             `json:"base_revision"`
	EditReason           string          `json:"edit_reason"`
	Status               string          `json:"status"`
	TargetStatus         string          `json:"target_status"`
	RequestedBy          string          `json:"requested_by"`
	DecisionBy           string          `json:"decision_by,omitempty"`
	DecisionNote         string          `json:"decision_note,omitempty"`
	Diff                 json.RawMessage `json:"diff"`
	Validation           json.RawMessage `json:"validation"`
	RollbackFromRevision int             `json:"rollback_from_revision,omitempty"`
	CreatedAt            string          `json:"created_at"`
	DecidedAt            string          `json:"decided_at,omitempty"`
	ActivatedAt          string          `json:"activated_at,omitempty"`
	ProposedRevision     ObjectRevision  `json:"proposed_revision"`
}

type StartRunInput struct {
	ProjectID       string          `json:"project_id"`
	CallerType      string          `json:"caller_type"`
	CallerID        string          `json:"caller_id"`
	Channel         string          `json:"channel"`
	PipelineID      string          `json:"pipeline_id"`
	PipelineVersion string          `json:"pipeline_version"`
	PipelineHash    string          `json:"pipeline_hash"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Snapshot        json.RawMessage `json:"snapshot"`
	RetryOfRunID    string          `json:"retry_of_run_id"`
	ForkedFromRunID string          `json:"forked_from_run_id"`
}

type Run struct {
	RunID           string          `json:"run_id"`
	ProjectID       string          `json:"project_id"`
	CallerType      string          `json:"caller_type"`
	CallerID        string          `json:"caller_id"`
	Channel         string          `json:"channel"`
	PipelineID      string          `json:"pipeline_id"`
	PipelineVersion string          `json:"pipeline_version"`
	PipelineHash    string          `json:"pipeline_hash"`
	Status          string          `json:"status"`
	Snapshot        json.RawMessage `json:"snapshot"`
	IdempotencyKey  string          `json:"idempotency_key"`
	RetryOfRunID    string          `json:"retry_of_run_id,omitempty"`
	ForkedFromRunID string          `json:"forked_from_run_id,omitempty"`
	CreatedAt       string          `json:"created_at"`
	StartedAt       string          `json:"started_at,omitempty"`
	EndedAt         string          `json:"ended_at,omitempty"`
	Duplicate       bool            `json:"duplicate,omitempty"`
}

type Span struct {
	SpanID       string          `json:"span_id"`
	RunID        string          `json:"run_id"`
	ParentSpanID string          `json:"parent_span_id,omitempty"`
	NodeID       string          `json:"node_id"`
	StageType    string          `json:"stage_type"`
	StageVersion string          `json:"stage_version"`
	PluginID     string          `json:"plugin_id"`
	Attempt      int             `json:"attempt"`
	Status       string          `json:"status"`
	InputHash    string          `json:"input_hash,omitempty"`
	OutputHash   string          `json:"output_hash,omitempty"`
	Detail       json.RawMessage `json:"detail"`
	StartedAt    string          `json:"started_at"`
	EndedAt      string          `json:"ended_at,omitempty"`
}

type Event struct {
	RunID         string          `json:"run_id"`
	Sequence      int             `json:"sequence"`
	EventType     string          `json:"event_type"`
	Producer      string          `json:"producer"`
	SchemaVersion string          `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
	CreatedAt     string          `json:"created_at"`
}

type Effect struct {
	RunID                  string          `json:"run_id"`
	NodeID                 string          `json:"node_id"`
	EffectKey              string          `json:"effect_key"`
	ProviderIdempotencyKey string          `json:"provider_idempotency_key,omitempty"`
	Status                 string          `json:"status"`
	Outcome                string          `json:"outcome"`
	RequestHash            string          `json:"request_hash"`
	ResultHash             string          `json:"result_hash,omitempty"`
	Receipt                json.RawMessage `json:"receipt"`
	IntentAt               string          `json:"intent_at"`
	DispatchedAt           string          `json:"dispatched_at,omitempty"`
	ReceivedAt             string          `json:"received_at,omitempty"`
	MaterializedAt         string          `json:"materialized_at,omitempty"`
}

type RunDetail struct {
	Run          Run                      `json:"run"`
	Spans        []Span                   `json:"spans"`
	Events       []Event                  `json:"events"`
	Effects      []Effect                 `json:"effects"`
	StageOutputs []StageOutputSnapshot    `json:"stage_outputs"`
	ModelCalls   []modelusage.Observation `json:"model_calls"`
	ModelHealth  modelusage.Summary       `json:"model_health"`
}

type StageOutputSnapshot struct {
	RunID      string          `json:"run_id"`
	NodeID     string          `json:"node_id"`
	OutputHash string          `json:"output_hash"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  string          `json:"created_at"`
}
