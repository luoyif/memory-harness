package experience

import "encoding/json"

const (
	PluginID      = "builtin.experience-bank"
	PluginVersion = "1.0.0"

	EvaluationTypeV1 = PluginID + ".evaluation.v1"
	EvaluationTypeV2 = PluginID + ".evaluation.v2"
	CaseTypeV1       = PluginID + ".case.v1"
	PatternTypeV1    = PluginID + ".pattern.v1"
)

type EvaluationDimension struct {
	Name       string  `json:"name"`
	Verdict    string  `json:"verdict"`
	Score      float64 `json:"score,omitempty"`
	Confidence float64 `json:"confidence"`
	Note       string  `json:"note,omitempty"`
}

type Evaluation struct {
	EvaluationID        string                `json:"evaluation_id"`
	ProjectID           string                `json:"project_id"`
	TargetKind          string                `json:"target_kind"`
	TargetID            string                `json:"target_id"`
	Protocol            string                `json:"protocol"`
	EvaluatorID         string                `json:"evaluator_id"`
	EvaluatorVersion    string                `json:"evaluator_version"`
	Verdict             string                `json:"verdict"`
	Dimensions          []EvaluationDimension `json:"dimensions"`
	Expected            string                `json:"expected,omitempty"`
	Observed            string                `json:"observed,omitempty"`
	Confidence          float64               `json:"confidence"`
	SampleSize          int                   `json:"sample_size"`
	BaselineRef         string                `json:"baseline_ref,omitempty"`
	ChallengerRef       string                `json:"challenger_ref,omitempty"`
	SourceRunIDs        []string              `json:"source_run_ids"`
	SourceOutcomeRunIDs []string              `json:"source_outcome_run_ids"`
	Notes               string                `json:"notes,omitempty"`
	EvaluatedAt         string                `json:"evaluated_at"`
}

type DeliverySummary struct {
	Total              int    `json:"total"`
	Delivered          int    `json:"delivered"`
	Trimmed            int    `json:"trimmed"`
	Denied             int    `json:"denied"`
	Failed             int    `json:"failed"`
	DeliveryUnverified int    `json:"delivery_unverified"`
	EvidenceLevel      string `json:"evidence_level,omitempty"`
	Completeness       string `json:"completeness,omitempty"`
}

type CostSummary struct {
	Tokens       int   `json:"tokens,omitempty"`
	LatencyMS    int   `json:"latency_ms,omitempty"`
	MoneyMinor   int64 `json:"money_minor,omitempty"`
	SafetyEvents int   `json:"safety_events,omitempty"`
}

type OutcomeObservation struct {
	Name       string          `json:"name"`
	Value      json.RawMessage `json:"value"`
	Confidence float64         `json:"confidence"`
}

type Case struct {
	CaseID                     string               `json:"case_id"`
	ProjectID                  string               `json:"project_id"`
	SourceRunID                string               `json:"source_run_id"`
	PlanID                     string               `json:"plan_id,omitempty"`
	ReceiptID                  string               `json:"receipt_id,omitempty"`
	RequestFingerprint         string               `json:"request_fingerprint,omitempty"`
	BlueprintID                string               `json:"blueprint_id,omitempty"`
	BlueprintVersion           string               `json:"blueprint_version,omitempty"`
	BlueprintHash              string               `json:"blueprint_hash,omitempty"`
	AdapterID                  string               `json:"adapter_id,omitempty"`
	Runtime                    string               `json:"runtime,omitempty"`
	ProtocolVersion            string               `json:"protocol_version,omitempty"`
	TaskFeatures               map[string]string    `json:"task_features"`
	Delivery                   DeliverySummary      `json:"delivery"`
	OutcomeRunIDs              []string             `json:"outcome_run_ids"`
	OutcomeMetrics             []OutcomeObservation `json:"outcome_metrics"`
	Cost                       CostSummary          `json:"cost"`
	EvaluationObjectIDs        []string             `json:"evaluation_object_ids"`
	Result                     string               `json:"result"`
	PrimaryFailureDimension    string               `json:"primary_failure_dimension,omitempty"`
	SecondaryFailureDimensions []string             `json:"secondary_failure_dimensions"`
	Diagnosis                  string               `json:"diagnosis,omitempty"`
	CounterfactualHypothesis   string               `json:"counterfactual_hypothesis,omitempty"`
	TransferScope              []string             `json:"transfer_scope"`
	ExpiresAt                  string               `json:"expires_at,omitempty"`
	Sensitivity                string               `json:"sensitivity"`
	SourceArtifactRefs         []string             `json:"source_artifact_refs"`
	SourceHash                 string               `json:"source_hash"`
	GeneratedAt                string               `json:"generated_at"`
}

type Pattern struct {
	PatternID             string   `json:"pattern_id"`
	ProjectID             string   `json:"project_id"`
	NormalizedPattern     string   `json:"normalized_pattern"`
	SupportingCaseIDs     []string `json:"supporting_case_ids"`
	CounterexampleCaseIDs []string `json:"counterexample_case_ids"`
	TargetComponents      []string `json:"target_components"`
	Conditions            []string `json:"conditions"`
	ExpectedEffect        string   `json:"expected_effect"`
	Confidence            float64  `json:"confidence"`
	SampleSize            int      `json:"sample_size"`
	EvaluationObjectIDs   []string `json:"evaluation_object_ids"`
	KnownRegressions      []string `json:"known_regressions"`
	NegativeDomains       []string `json:"negative_domains"`
	LastValidated         string   `json:"last_validated,omitempty"`
	GeneratedAt           string   `json:"generated_at"`
}

type EvaluateInput struct {
	TargetKind       string                `json:"target_kind"`
	TargetID         string                `json:"target_id"`
	Protocol         string                `json:"protocol"`
	EvaluatorID      string                `json:"evaluator_id"`
	EvaluatorVersion string                `json:"evaluator_version"`
	Verdict          string                `json:"verdict"`
	Dimensions       []EvaluationDimension `json:"dimensions"`
	Expected         string                `json:"expected,omitempty"`
	Observed         string                `json:"observed,omitempty"`
	Confidence       float64               `json:"confidence"`
	SampleSize       int                   `json:"sample_size"`
	BaselineRef      string                `json:"baseline_ref,omitempty"`
	ChallengerRef    string                `json:"challenger_ref,omitempty"`
	Notes            string                `json:"notes,omitempty"`
	IdempotencyKey   string                `json:"idempotency_key"`
}

type ActivationInput struct {
	ExpectedRevision int    `json:"expected_revision"`
	EditReason       string `json:"edit_reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}
type PatternInput struct {
	ProjectID             string   `json:"project_id"`
	NormalizedPattern     string   `json:"normalized_pattern"`
	SupportingCaseIDs     []string `json:"supporting_case_ids"`
	CounterexampleCaseIDs []string `json:"counterexample_case_ids"`
	TargetComponents      []string `json:"target_components"`
	Conditions            []string `json:"conditions"`
	ExpectedEffect        string   `json:"expected_effect"`
	Confidence            float64  `json:"confidence"`
	KnownRegressions      []string `json:"known_regressions"`
	NegativeDomains       []string `json:"negative_domains"`
	IdempotencyKey        string   `json:"idempotency_key"`
}

type CaseDetail struct {
	Object      any          `json:"object"`
	Case        Case         `json:"case"`
	Evaluations []Evaluation `json:"evaluations"`
}
