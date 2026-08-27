package plugins

import (
	"encoding/json"

	"github.com/luoyif/memory-harness/internal/harness"
)

const APIVersion = "memory-harness.plugin/v1alpha1"

type Manifest struct {
	APIVersion    string        `yaml:"apiVersion" json:"api_version"`
	Kind          string        `yaml:"kind" json:"kind"`
	Metadata      Metadata      `yaml:"metadata" json:"metadata"`
	Compatibility Compatibility `yaml:"compatibility" json:"compatibility"`
	Trust         Trust         `yaml:"trust" json:"trust"`
	Contributes   Contributions `yaml:"contributes" json:"contributes"`
	Permissions   Permissions   `yaml:"permissions" json:"permissions"`
	Configuration Configuration `yaml:"configuration" json:"configuration"`
}

type Metadata struct {
	ID        string `yaml:"id" json:"id"`
	Name      string `yaml:"name" json:"name"`
	Version   string `yaml:"version" json:"version"`
	Publisher string `yaml:"publisher" json:"publisher"`
	License   string `yaml:"license" json:"license"`
}

type Compatibility struct {
	MemoryHarness string `yaml:"memoryHarness" json:"memory_harness"`
}

type Trust struct {
	Class string `yaml:"class" json:"class"`
}

type Contributions struct {
	MemoryTypes        []MemoryTypeContribution        `yaml:"memoryTypes" json:"memory_types"`
	Stages             []StageContribution             `yaml:"stages" json:"stages"`
	Pipelines          []PipelineContribution          `yaml:"pipelines" json:"pipelines"`
	StrategyComponents []StrategyComponentContribution `yaml:"strategyComponents" json:"strategy_components"`
	Blueprints         []BlueprintContribution         `yaml:"blueprints" json:"blueprints"`
	Connectors         []NamedContribution             `yaml:"connectors" json:"connectors"`
	Projections        []NamedContribution             `yaml:"projections" json:"projections"`
	Views              []NamedContribution             `yaml:"views" json:"views"`
}

type MemoryTypeContribution struct {
	TypeID          string            `yaml:"typeId" json:"type_id"`
	DisplayName     string            `yaml:"displayName" json:"display_name"`
	SchemaVersion   string            `yaml:"schemaVersion" json:"schema_version"`
	SchemaPath      string            `yaml:"schema" json:"schema"`
	Lifecycle       harness.Lifecycle `yaml:"lifecycle" json:"lifecycle"`
	ProtectionClass string            `yaml:"protectionClass" json:"protection_class"`
	RendererPath    string            `yaml:"renderer" json:"renderer,omitempty"`
}

type StageContribution struct {
	StageType    string   `yaml:"stageType" json:"stage_type"`
	Version      string   `yaml:"version" json:"version"`
	Class        string   `yaml:"class" json:"class"`
	InputSchema  string   `yaml:"inputSchema" json:"input_schema"`
	OutputSchema string   `yaml:"outputSchema" json:"output_schema"`
	Capabilities []string `yaml:"capabilities" json:"capabilities"`
}

type PipelineContribution struct {
	PipelineID string `yaml:"pipelineId" json:"pipeline_id"`
	Version    string `yaml:"version" json:"version"`
	Definition string `yaml:"definition" json:"definition"`
}

// StrategyComponentContribution makes a plugin capability discoverable in the
// Blueprint workbench. Role is the replacement slot (for example recall.deep),
// while kind identifies the stable kernel primitive used to execute it.
type StrategyComponentContribution struct {
	ComponentID   string   `yaml:"componentId" json:"component_id"`
	Version       string   `yaml:"version" json:"version"`
	DisplayName   string   `yaml:"displayName" json:"display_name"`
	Description   string   `yaml:"description" json:"description"`
	Role          string   `yaml:"role" json:"role"`
	Kind          string   `yaml:"kind" json:"kind"`
	StageType     string   `yaml:"stageType" json:"stage_type,omitempty"`
	Configuration string   `yaml:"configuration" json:"configuration,omitempty"`
	Capabilities  []string `yaml:"capabilities" json:"capabilities"`
}

type BlueprintContribution struct {
	BlueprintID string `yaml:"blueprintId" json:"blueprint_id"`
	Version     string `yaml:"version" json:"version"`
	Definition  string `yaml:"definition" json:"definition"`
}

type NamedContribution struct {
	ID string `yaml:"id" json:"id"`
}

type Permissions struct {
	Required []string `yaml:"required" json:"required"`
	Optional []string `yaml:"optional" json:"optional"`
}

type Configuration struct {
	Schema string `yaml:"schema" json:"schema,omitempty"`
}

type Signature struct {
	SignerID  string `json:"signer_id"`
	Algorithm string `json:"algorithm"`
	Signature string `json:"signature"`
}

type InstallOptions struct {
	DeveloperMode bool
	EnableProject string
	Capabilities  []string
}

type PluginVersion struct {
	PluginID         string          `json:"plugin_id"`
	Version          string          `json:"version"`
	Name             string          `json:"name"`
	Publisher        string          `json:"publisher"`
	TrustClass       string          `json:"trust_class"`
	SignerID         string          `json:"signer_id,omitempty"`
	SignatureStatus  string          `json:"signature_status"`
	ContentHash      string          `json:"content_hash"`
	Permissions      []string        `json:"permissions"`
	Contributions    Contributions   `json:"contributions"`
	Status           string          `json:"status"`
	InstalledAt      string          `json:"installed_at"`
	UpdatedAt        string          `json:"updated_at"`
	ProjectStates    []ProjectState  `json:"project_states"`
	Manifest         json.RawMessage `json:"manifest"`
	PackageSizeBytes int64           `json:"package_size_bytes"`
}

type ProjectState struct {
	ProjectID           string          `json:"project_id"`
	Status              string          `json:"status"`
	GrantedCapabilities []string        `json:"granted_capabilities"`
	Config              json.RawMessage `json:"config"`
	UpdatedAt           string          `json:"updated_at"`
}

type TrustSignerInput struct {
	SignerID        string   `json:"signer_id"`
	Publisher       string   `json:"publisher"`
	PublicKeyBase64 string   `json:"public_key_base64"`
	Scope           []string `json:"scope"`
}

type TrustedSigner struct {
	SignerID    string   `json:"signer_id"`
	Publisher   string   `json:"publisher"`
	Fingerprint string   `json:"fingerprint"`
	Scope       []string `json:"scope"`
	Status      string   `json:"status"`
	ApprovedAt  string   `json:"approved_at"`
	RevokedAt   string   `json:"revoked_at,omitempty"`
}

type BuiltinSpec struct {
	PluginID      string
	Version       string
	Name          string
	Description   string
	Contributions Contributions
	Permissions   []string
	Status        string
}

type PluginImpact struct {
	PluginID              string   `json:"plugin_id"`
	Version               string   `json:"version"`
	ProjectID             string   `json:"project_id,omitempty"`
	CurrentObjects        int      `json:"current_objects"`
	HistoricalRevisions   int      `json:"historical_revisions"`
	HistoricalRuns        int      `json:"historical_runs"`
	PipelineVersions      int      `json:"pipeline_versions"`
	BlueprintVersions     int      `json:"blueprint_versions"`
	EnabledProjects       int      `json:"enabled_projects"`
	ActiveBlueprintRefs   int      `json:"active_blueprint_refs"`
	PackageBytesReclaimed int64    `json:"package_bytes_reclaimed"`
	CanRetire             bool     `json:"can_retire"`
	HistoryPreserved      bool     `json:"history_preserved"`
	Blockers              []string `json:"blockers"`
}
