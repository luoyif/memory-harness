package adaptation

import (
	"encoding/json"
	"github.com/luoyif/memory-harness/internal/harness"
)

const (
	PluginID              = "builtin.adaptation-lab"
	PluginVersion         = "1.0.0"
	ChangeProposalTypeV1  = PluginID + ".change-proposal.v1"
	CaseOverlayTypeV1     = PluginID + ".case-overlay.v1"
	CanaryPipelineID      = PluginID + ".canary"
	CanaryPipelineVersion = "1.0.0"
)

type StopConditions struct {
	MaxRegressionRate   float64 `json:"max_regression_rate"`
	StopOnSafetyFailure bool    `json:"stop_on_safety_failure"`
}

type Patch struct {
	Role   string          `json:"role"`
	Config json.RawMessage `json:"config"`
}
type ProposalInput struct {
	ProjectID            string         `json:"project_id"`
	SourceCaseIDs        []string       `json:"source_case_ids"`
	SourcePatternIDs     []string       `json:"source_pattern_ids,omitempty"`
	Patch                Patch          `json:"patch"`
	PredictedFix         string         `json:"predicted_fix"`
	PredictedRegressions []string       `json:"predicted_regressions"`
	EvaluationSuite      []string       `json:"evaluation_suite"`
	MinimumSample        int            `json:"minimum_sample"`
	StopConditions       StopConditions `json:"stop_conditions"`
	PrivacyImpact        string         `json:"privacy_impact"`
	CostImpact           string         `json:"cost_impact"`
	ProposerID           string         `json:"proposer_id"`
	CanaryScope          string         `json:"canary_scope"`
	OverlayTTLSeconds    int64          `json:"overlay_ttl_seconds"`
	IdempotencyKey       string         `json:"idempotency_key"`
}

type ChangeProposal struct {
	ProposalID             string         `json:"proposal_id"`
	ProjectID              string         `json:"project_id"`
	SourceCaseIDs          []string       `json:"source_case_ids"`
	SourcePatternIDs       []string       `json:"source_pattern_ids"`
	SourceRunIDs           []string       `json:"source_run_ids"`
	SourceOutcomeRunIDs    []string       `json:"source_outcome_run_ids"`
	BaseBlueprintID        string         `json:"base_blueprint_id"`
	BaseBlueprintVersion   string         `json:"base_blueprint_version"`
	BaseBlueprintHash      string         `json:"base_blueprint_hash"`
	Patch                  Patch          `json:"patch"`
	EffectiveBlueprintHash string         `json:"effective_blueprint_hash"`
	PredictedFix           string         `json:"predicted_fix"`
	PredictedRegressions   []string       `json:"predicted_regressions"`
	EvaluationSuite        []string       `json:"evaluation_suite"`
	MinimumSample          int            `json:"minimum_sample"`
	StopConditions         StopConditions `json:"stop_conditions"`
	PermissionImpact       []string       `json:"permission_impact"`
	PrivacyImpact          string         `json:"privacy_impact"`
	CostImpact             string         `json:"cost_impact"`
	ProposerID             string         `json:"proposer_id"`
	VerifierID             string         `json:"verifier_id,omitempty"`
	EvaluationObjectIDs    []string       `json:"evaluation_object_ids"`
	CanaryScope            string         `json:"canary_scope"`
	OverlayTTLSeconds      int64          `json:"overlay_ttl_seconds"`
	RollbackBlueprintHash  string         `json:"rollback_blueprint_hash"`
	CreatedAt              string         `json:"created_at"`
}

type DryRunResult struct {
	ProjectID              string          `json:"project_id"`
	BaseBlueprintHash      string          `json:"base_blueprint_hash"`
	EffectiveBlueprintHash string          `json:"effective_blueprint_hash"`
	TargetRole             string          `json:"target_role"`
	BaseConfig             json.RawMessage `json:"base_config"`
	EffectiveConfig        json.RawMessage `json:"effective_config"`
	PermissionDelta        []string        `json:"permission_delta"`
	NoWritesPerformed      bool            `json:"no_writes_performed"`
}

type ApprovalInput struct {
	ExpectedRevision int    `json:"expected_revision"`
	VerifierID       string `json:"verifier_id"`
	EditReason       string `json:"edit_reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type OverlayInput struct {
	ProposalID     string `json:"proposal_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type CaseOverlay struct {
	OverlayID              string          `json:"overlay_id"`
	ProjectID              string          `json:"project_id"`
	ProposalID             string          `json:"proposal_id"`
	BaseBlueprintID        string          `json:"base_blueprint_id"`
	BaseBlueprintVersion   string          `json:"base_blueprint_version"`
	BaseBlueprintHash      string          `json:"base_blueprint_hash"`
	EffectiveBlueprintHash string          `json:"effective_blueprint_hash"`
	Patch                  Patch           `json:"patch"`
	EffectiveBlueprint     json.RawMessage `json:"effective_blueprint"`
	PermissionDelta        []string        `json:"permission_delta"`
	TTLSeconds             int64           `json:"ttl_seconds"`
	CreatedAt              string          `json:"created_at"`
	ExpiresAt              string          `json:"expires_at"`
}

type CanaryInput struct {
	OverlayID           string   `json:"overlay_id"`
	VerifierID          string   `json:"verifier_id"`
	EvaluationObjectIDs []string `json:"evaluation_object_ids"`
	IdempotencyKey      string   `json:"idempotency_key"`
}
type CanaryResult struct {
	RunID                    string   `json:"run_id"`
	OverlayID                string   `json:"overlay_id"`
	ProposalID               string   `json:"proposal_id"`
	Status                   string   `json:"status"`
	Samples                  int      `json:"samples"`
	ImprovedSamples          int      `json:"improved_samples"`
	RegressedSamples         int      `json:"regressed_samples"`
	RegressionRate           float64  `json:"regression_rate"`
	SafetyFailure            bool     `json:"safety_failure"`
	BaseBlueprintHash        string   `json:"base_blueprint_hash"`
	EffectiveBlueprintHash   string   `json:"effective_blueprint_hash"`
	FallbackBlueprintHash    string   `json:"fallback_blueprint_hash"`
	GlobalBlueprintUnchanged bool     `json:"global_blueprint_unchanged"`
	EvaluationObjectIDs      []string `json:"evaluation_object_ids"`
}

type ProposalDetail struct {
	Object   harness.Object `json:"object"`
	Proposal ChangeProposal `json:"proposal"`
}
