package pipeline

import (
	"context"
	"encoding/json"
)

const APIVersion = "memory-harness.pipeline/v1alpha1"

type Definition struct {
	APIVersion           string          `yaml:"apiVersion" json:"api_version"`
	PipelineID           string          `yaml:"pipelineId" json:"pipeline_id"`
	Version              string          `yaml:"version" json:"version"`
	Name                 string          `yaml:"name" json:"name"`
	Intent               string          `yaml:"intent" json:"intent"`
	RequiredCapabilities []string        `yaml:"requiredCapabilities" json:"required_capabilities"`
	Nodes                []Node          `yaml:"nodes" json:"nodes"`
	Outputs              []Output        `yaml:"outputs" json:"outputs"`
	Policy               ExecutionPolicy `yaml:"policy" json:"policy"`
	Editor               EditorMetadata  `yaml:"editor,omitempty" json:"editor,omitempty"`
}

type EditorMetadata struct {
	Positions map[string]NodePosition `yaml:"positions,omitempty" json:"positions,omitempty"`
}

type NodePosition struct {
	X float64 `yaml:"x" json:"x"`
	Y float64 `yaml:"y" json:"y"`
}

type Node struct {
	ID           string          `yaml:"id" json:"id"`
	StageType    string          `yaml:"stageType" json:"stage_type"`
	StageVersion string          `yaml:"stageVersion" json:"stage_version"`
	PluginID     string          `yaml:"pluginId" json:"plugin_id"`
	DependsOn    []string        `yaml:"dependsOn" json:"depends_on"`
	Config       json.RawMessage `yaml:"-" json:"config"`
	ConfigYAML   any             `yaml:"config" json:"-"`
}

type Output struct {
	Name   string `yaml:"name" json:"name"`
	NodeID string `yaml:"nodeId" json:"node_id"`
}

type ExecutionPolicy struct {
	MaxStages      int `yaml:"maxStages" json:"max_stages"`
	TimeoutSeconds int `yaml:"timeoutSeconds" json:"timeout_seconds"`
	MaxModelCalls  int `yaml:"maxModelCalls" json:"max_model_calls"`
}

type Version struct {
	PipelineID  string     `json:"pipeline_id"`
	Version     string     `json:"version"`
	PluginID    string     `json:"plugin_id"`
	Name        string     `json:"name"`
	Definition  Definition `json:"definition"`
	ContentHash string     `json:"content_hash"`
	Status      string     `json:"status"`
	CreatedAt   string     `json:"created_at"`
}

type Draft struct {
	DraftID     string     `json:"draft_id"`
	PipelineID  string     `json:"pipeline_id"`
	PluginID    string     `json:"plugin_id"`
	BaseVersion string     `json:"base_version,omitempty"`
	Definition  Definition `json:"definition"`
	Revision    int        `json:"revision"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type SaveDraftInput struct {
	PluginID         string     `json:"plugin_id"`
	BaseVersion      string     `json:"base_version,omitempty"`
	ExpectedRevision int        `json:"expected_revision,omitempty"`
	Definition       Definition `json:"definition"`
}

type ValidationResult struct {
	Valid                bool     `json:"valid"`
	ExecutionOrder       []string `json:"execution_order"`
	RequiredCapabilities []string `json:"required_capabilities"`
	ModelCalls           int      `json:"model_calls"`
	NodeCount            int      `json:"node_count"`
}

type ExecuteInput struct {
	ProjectID             string          `json:"project_id"`
	CallerType            string          `json:"caller_type"`
	CallerID              string          `json:"caller_id"`
	Channel               string          `json:"channel"`
	PipelineID            string          `json:"pipeline_id"`
	PipelineVersion       string          `json:"pipeline_version"`
	IdempotencyKey        string          `json:"idempotency_key"`
	Input                 json.RawMessage `json:"input"`
	EffectiveCapabilities []string        `json:"effective_capabilities"`
	BlueprintSnapshot     json.RawMessage `json:"-"`
	RetryOfRunID          string          `json:"-"`
	ForkedFromRunID       string          `json:"-"`
}

type ExecutionResult struct {
	RunID     string                     `json:"run_id"`
	Status    string                     `json:"status"`
	Outputs   map[string]json.RawMessage `json:"outputs"`
	Duplicate bool                       `json:"duplicate,omitempty"`
}

type StageDescriptor struct {
	StageType    string   `json:"stage_type"`
	Version      string   `json:"version"`
	Class        string   `json:"class"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description"`
}

// StageInvocation is the stable host contract exposed to built-in and future
// plugin-backed stage adapters. The pipeline engine owns orchestration and
// tracing; the registered handler owns one bounded business transformation.
type StageInvocation struct {
	Pipeline Version         `json:"pipeline"`
	RunID    string          `json:"run_id"`
	SpanID   string          `json:"span_id"`
	Node     Node            `json:"node"`
	Input    json.RawMessage `json:"input"`
	Execute  ExecuteInput    `json:"execute"`
}

type StageHandler func(context.Context, StageInvocation) (json.RawMessage, error)

type Review struct {
	ReviewID     string          `json:"review_id"`
	RunID        string          `json:"run_id"`
	NodeID       string          `json:"node_id"`
	ProjectID    string          `json:"project_id"`
	PipelineID   string          `json:"pipeline_id"`
	Reason       string          `json:"reason"`
	Status       string          `json:"status"`
	RequestedBy  string          `json:"requested_by"`
	Request      json.RawMessage `json:"request"`
	DecisionBy   string          `json:"decision_by,omitempty"`
	DecisionNote string          `json:"decision_note,omitempty"`
	CreatedAt    string          `json:"created_at"`
	DecidedAt    string          `json:"decided_at,omitempty"`
}

type ReviewDecisionInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
	OwnerID  string `json:"-"`
}

type DryRunNode struct {
	NodeID           string          `json:"node_id"`
	StageType        string          `json:"stage_type"`
	Class            string          `json:"class"`
	Status           string          `json:"status"`
	InputHash        string          `json:"input_hash,omitempty"`
	OutputHash       string          `json:"output_hash,omitempty"`
	Preview          json.RawMessage `json:"preview,omitempty"`
	WouldWrite       bool            `json:"would_write"`
	WouldInvokeModel bool            `json:"would_invoke_model"`
	ReviewGate       bool            `json:"review_gate"`
	Detail           string          `json:"detail"`
}

type DryRunResult struct {
	ProjectID         string       `json:"project_id"`
	PipelineID        string       `json:"pipeline_id"`
	PipelineVersion   string       `json:"pipeline_version"`
	PipelineHash      string       `json:"pipeline_hash"`
	Nodes             []DryRunNode `json:"nodes"`
	PurePreviewed     int          `json:"pure_previewed"`
	PlannedWrites     int          `json:"planned_writes"`
	PlannedModelCalls int          `json:"planned_model_calls"`
	ReviewGates       int          `json:"review_gates"`
	Warnings          []string     `json:"warnings"`
	NoWritesPerformed bool         `json:"no_writes_performed"`
}
