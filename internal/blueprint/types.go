package blueprint

import "encoding/json"

const (
	APIVersion               = "memory-harness.blueprint/v1alpha1"
	DefaultBlueprintID       = "builtin.memory-harness-core.default"
	DefaultBlueprintVersion  = "1.0.0"
	ContextBlueprintVersion  = "1.1.0"
	LegacyDefaultBlueprintID = "builtin.memory-harness.default"
)

// Definition is a complete, immutable memory strategy. Tracks are deliberately
// generic graphs of typed bindings: the kernel understands the safety envelope,
// while plugins own the concrete memory, organization and recall techniques.
type Definition struct {
	APIVersion  string  `json:"api_version" yaml:"apiVersion"`
	BlueprintID string  `json:"blueprint_id" yaml:"blueprintId"`
	Version     string  `json:"version" yaml:"version"`
	Name        string  `json:"name" yaml:"name"`
	Description string  `json:"description" yaml:"description"`
	Intent      string  `json:"intent" yaml:"intent"`
	Tracks      []Track `json:"tracks" yaml:"tracks"`
	Policy      Policy  `json:"policy" yaml:"policy"`
}

type Policy struct {
	EvidenceMode         string `json:"evidence_mode" yaml:"evidenceMode"`
	ModelBoundary        string `json:"model_boundary" yaml:"modelBoundary"`
	DefaultContextBudget int    `json:"default_context_budget" yaml:"defaultContextBudget"`
	CrossProjectRecall   bool   `json:"cross_project_recall" yaml:"crossProjectRecall"`
}

type Track struct {
	TrackID     string        `json:"track_id" yaml:"trackId"`
	Role        string        `json:"role" yaml:"role"`
	DisplayName string        `json:"display_name" yaml:"displayName"`
	Description string        `json:"description" yaml:"description"`
	Nodes       []NodeBinding `json:"nodes" yaml:"nodes"`
}

type NodeBinding struct {
	NodeID               string          `json:"node_id" yaml:"nodeId"`
	Role                 string          `json:"role" yaml:"role"`
	DisplayName          string          `json:"display_name" yaml:"displayName"`
	BindingKind          string          `json:"binding_kind" yaml:"bindingKind"`
	PluginID             string          `json:"plugin_id" yaml:"pluginId"`
	PluginVersion        string          `json:"plugin_version" yaml:"pluginVersion"`
	ComponentID          string          `json:"component_id" yaml:"componentId"`
	ComponentVersion     string          `json:"component_version" yaml:"componentVersion"`
	Enabled              bool            `json:"enabled" yaml:"enabled"`
	RequiredCapabilities []string        `json:"required_capabilities" yaml:"requiredCapabilities"`
	Config               json.RawMessage `json:"config" yaml:"-"`
	ConfigYAML           any             `json:"-" yaml:"config"`
}

type Version struct {
	BlueprintID string     `json:"blueprint_id"`
	Version     string     `json:"version"`
	PluginID    string     `json:"plugin_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
	ContentHash string     `json:"content_hash"`
	Status      string     `json:"status"`
	CreatedAt   string     `json:"created_at"`
}

type Assignment struct {
	ProjectID        string `json:"project_id"`
	BlueprintID      string `json:"blueprint_id"`
	BlueprintVersion string `json:"blueprint_version"`
	BlueprintHash    string `json:"blueprint_hash"`
	Status           string `json:"status"`
	ActivatedBy      string `json:"activated_by"`
	ActivatedAt      string `json:"activated_at"`
	UpdatedAt        string `json:"updated_at"`
}

type Current struct {
	Assignment Assignment       `json:"assignment"`
	Blueprint  Version          `json:"blueprint"`
	Inherited  bool             `json:"inherited"`
	Validation ValidationResult `json:"validation"`
}

type ActivateInput struct {
	BlueprintID string `json:"blueprint_id"`
	Version     string `json:"version"`
}

type ValidationResult struct {
	Valid                 bool     `json:"valid"`
	Errors                []string `json:"errors"`
	Warnings              []string `json:"warnings"`
	TrackCount            int      `json:"track_count"`
	EnabledComponentCount int      `json:"enabled_component_count"`
	RequiredCapabilities  []string `json:"required_capabilities"`
}
