package portablebundle

import "encoding/json"

const (
	SchemaVersion         = "memory-harness.bundle/v1alpha1"
	PluginID              = "builtin.portable-bundle"
	PluginVersion         = "1.0.0"
	ImportCandidateTypeV1 = PluginID + ".import-candidate.v1"
)

type Signature struct {
	Status    string `json:"status"`
	Algorithm string `json:"algorithm"`
	Value     string `json:"value,omitempty"`
}

type Selection struct {
	RootObjectIDs       []string `json:"root_object_ids"`
	IncludeDependencies bool     `json:"include_dependencies"`
}

type Manifest struct {
	SchemaVersion        string    `json:"schema_version"`
	BundleID             string    `json:"bundle_id"`
	CreatedAt            string    `json:"created_at"`
	SourceProjectID      string    `json:"source_project_id"`
	Selection            Selection `json:"selection"`
	ObjectCount          int       `json:"object_count"`
	EvidenceCount        int       `json:"evidence_count"`
	RequiredCapabilities []string  `json:"required_capabilities"`
	BundleHash           string    `json:"bundle_hash"`
	Signature            Signature `json:"signature"`
}
type RevisionRecord struct {
	Revision          int      `json:"revision"`
	Status            string   `json:"status"`
	SchemaVersion     string   `json:"schema_version"`
	ContentHash       string   `json:"content_hash"`
	BlobSHA256        string   `json:"blob_sha256"`
	Confidence        float64  `json:"confidence"`
	Importance        float64  `json:"importance"`
	ValidFrom         string   `json:"valid_from,omitempty"`
	ValidUntil        string   `json:"valid_until,omitempty"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	SourceObjectIDs   []string `json:"source_object_ids"`
	RunID             string   `json:"run_id,omitempty"`
	StageID           string   `json:"stage_id,omitempty"`
	PluginID          string   `json:"plugin_id"`
	PluginVersion     string   `json:"plugin_version"`
	CreatedAt         string   `json:"created_at"`
}

type ObjectRecord struct {
	ObjectID         string           `json:"object_id"`
	TypeID           string           `json:"type_id"`
	ProjectID        string           `json:"project_id"`
	Status           string           `json:"status"`
	ProtectionClass  string           `json:"protection_class"`
	CurrentRevision  int              `json:"current_revision"`
	CreatedAt        string           `json:"created_at"`
	UpdatedAt        string           `json:"updated_at"`
	Capabilities     []string         `json:"capabilities"`
	PresentationHint string           `json:"presentation_hint"`
	Revisions        []RevisionRecord `json:"revisions"`
}
type EvidenceRecord struct {
	EvidenceID   string `json:"evidence_id"`
	BlobSHA256   string `json:"blob_sha256"`
	LineHash     string `json:"line_hash"`
	SourceSystem string `json:"source_system"`
	SessionID    string `json:"session_id"`
	ObservedAt   string `json:"observed_at"`
	CapturedAt   string `json:"captured_at"`
}

type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
}

type CompatibilityReport struct {
	Compatible           bool      `json:"compatible"`
	Blocked              bool      `json:"blocked"`
	TargetID             string    `json:"target_id"`
	MissingCapabilities  []string  `json:"missing_capabilities"`
	UnmappedObjectTypes  []string  `json:"unmapped_object_types"`
	Findings             []Finding `json:"findings"`
	Degradations         []string  `json:"degradations"`
	PermissionDelta      []string  `json:"permission_delta"`
	PresentationFallback bool      `json:"presentation_fallback"`
	ImportMode           string    `json:"import_mode"`
}

type PreflightOptions struct {
	TargetID              string   `json:"target_id"`
	Capabilities          []string `json:"capabilities"`
	KnownObjectTypes      []string `json:"known_object_types"`
	SupportsPresentations bool     `json:"supports_presentations"`
}
type ExportOptions struct {
	ProjectID           string   `json:"project_id"`
	ObjectIDs           []string `json:"object_ids,omitempty"`
	IncludeDependencies bool     `json:"include_dependencies"`
}

type ImportOptions struct {
	TargetProjectID       string   `json:"target_project_id"`
	TargetID              string   `json:"target_id"`
	Capabilities          []string `json:"capabilities"`
	KnownObjectTypes      []string `json:"known_object_types"`
	SupportsPresentations bool     `json:"supports_presentations"`
	IdempotencyKey        string   `json:"idempotency_key"`
}

type ImportCandidate struct {
	BundleID          string                     `json:"bundle_id"`
	OriginalProjectID string                     `json:"original_project_id"`
	OriginalObject    ObjectRecord               `json:"original_object"`
	RevisionPayloads  map[string]json.RawMessage `json:"revision_payloads"`
	Compatibility     CompatibilityReport        `json:"compatibility"`
	ImportedAt        string                     `json:"imported_at"`
}

type ImportResult struct {
	BundleID           string              `json:"bundle_id"`
	TargetProjectID    string              `json:"target_project_id"`
	EvidenceImported   int                 `json:"evidence_imported"`
	EvidenceDuplicates int                 `json:"evidence_duplicates"`
	CandidateObjectIDs []string            `json:"candidate_object_ids"`
	Compatibility      CompatibilityReport `json:"compatibility"`
	NoDirectActivation bool                `json:"no_direct_activation"`
}

const ImportCandidateSchemaV1 = `{"type":"object","required":["bundle_id","original_project_id","original_object","revision_payloads","compatibility","imported_at"],"properties":{"bundle_id":{"type":"string","maxLength":200},"original_project_id":{"type":"string","maxLength":240},"original_object":{"type":"object"},"revision_payloads":{"type":"object"},"compatibility":{"type":"object"},"imported_at":{"type":"string","maxLength":80}},"additionalProperties":false}`
