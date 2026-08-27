package memory

import "context"

const (
	KnowledgeUnitSchemaV2         = "memory-harness.knowledge-unit/v2"
	StructuredKnowledgeUnitTypeV2 = "builtin.core-memory-growth.knowledge-unit.v2"
	StructuredMemoryRecordTypeV1  = "builtin.core-memory-growth.memory-record.v1"
)

type ParticipantIdentity struct {
	ParticipantID string   `json:"participant_id"`
	DisplayName   string   `json:"display_name"`
	Role          string   `json:"role"`
	Aliases       []string `json:"aliases,omitempty"`
}

type ExtractionTurn struct {
	EvidenceID   string   `json:"evidence_id"`
	SourceSystem string   `json:"source_system"`
	Role         string   `json:"role"`
	Text         string   `json:"text"`
	Scopes       []string `json:"scopes"`
	ObservedAt   string   `json:"observed_at"`
}

type ExtractionRequest struct {
	SessionID    string                `json:"session_id"`
	Participants []ParticipantIdentity `json:"participants,omitempty"`
	Turns        []ExtractionTurn      `json:"turns"`
}

type Attribution struct {
	SourceSpeakerRef string   `json:"source_speaker_ref,omitempty"`
	AssertedByRef    string   `json:"asserted_by_ref,omitempty"`
	SubjectRef       string   `json:"subject_ref,omitempty"`
	SubjectSurface   string   `json:"subject_surface,omitempty"`
	Resolution       string   `json:"resolution"`
	CandidateRefs    []string `json:"candidate_refs,omitempty"`
	ReasonCodes      []string `json:"reason_codes,omitempty"`
	OwnerMapping     string   `json:"owner_mapping"`
}

type EntityRef struct {
	EntityID      string   `json:"entity_id,omitempty"`
	EntityType    string   `json:"entity_type,omitempty"`
	Surface       string   `json:"surface,omitempty"`
	CanonicalName string   `json:"canonical_name,omitempty"`
	Aliases       []string `json:"aliases,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
}

type SemanticObject struct {
	Kind   string    `json:"kind,omitempty"`
	Entity EntityRef `json:"entity,omitempty"`
	Value  string    `json:"value,omitempty"`
}

type ParticipantRole struct {
	Role   string    `json:"role"`
	Entity EntityRef `json:"entity"`
}

type LocationRole struct {
	Role   string    `json:"role"`
	Entity EntityRef `json:"entity"`
}

type SemanticFrame struct {
	Subject      EntityRef         `json:"subject"`
	Predicate    string            `json:"predicate,omitempty"`
	InverseLabel string            `json:"inverse_label,omitempty"`
	Object       SemanticObject    `json:"object"`
	Action       string            `json:"action,omitempty"`
	Participants []ParticipantRole `json:"participants,omitempty"`
	Locations    []LocationRole    `json:"locations,omitempty"`
	Context      string            `json:"context,omitempty"`
}

type TemporalContext struct {
	ObservedAt         string `json:"observed_at"`
	RecordedAt         string `json:"recorded_at,omitempty"`
	EventTimeText      string `json:"event_time_text,omitempty"`
	ValidFrom          string `json:"valid_from,omitempty"`
	ValidUntil         string `json:"valid_until,omitempty"`
	OccurredFrom       string `json:"occurred_from,omitempty"`
	OccurredUntil      string `json:"occurred_until,omitempty"`
	Precision          string `json:"precision"`
	Resolution         string `json:"resolution"`
	AnchorEvidenceTime string `json:"anchor_evidence_time,omitempty"`
}

type EpistemicContext struct {
	Polarity      string   `json:"polarity"`
	Modality      string   `json:"modality"`
	Confidence    float64  `json:"confidence"`
	Importance    float64  `json:"importance,omitempty"`
	Novelty       float64  `json:"novelty,omitempty"`
	QualityFlags  []string `json:"quality_flags,omitempty"`
	ReviewReasons []string `json:"review_reasons,omitempty"`
}

type EvidenceSpan struct {
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Quote     string `json:"quote"`
	QuoteHash string `json:"quote_hash,omitempty"`
}

type UnitProvenance struct {
	EvidenceID       string       `json:"evidence_id"`
	EpisodeID        string       `json:"episode_id,omitempty"`
	RunID            string       `json:"run_id,omitempty"`
	SpanID           string       `json:"span_id,omitempty"`
	ExtractorPlugin  string       `json:"extractor_plugin,omitempty"`
	ExtractorVersion string       `json:"extractor_version,omitempty"`
	ModelProfile     string       `json:"model_profile,omitempty"`
	PromptHash       string       `json:"prompt_hash,omitempty"`
	EvidenceSpan     EvidenceSpan `json:"evidence_span"`
}

type KnowledgeStructure struct {
	Attribution Attribution      `json:"attribution"`
	Frame       SemanticFrame    `json:"frame"`
	Temporal    TemporalContext  `json:"temporal"`
	Epistemic   EpistemicContext `json:"epistemic"`
	Provenance  UnitProvenance   `json:"provenance"`
}

type ExtractionCandidate struct {
	EvidenceID string             `json:"evidence_id"`
	Statement  string             `json:"statement"`
	UnitType   string             `json:"unit_type"`
	TierHint   string             `json:"tier_hint"`
	RiskTier   string             `json:"risk_tier"`
	Confidence float64            `json:"confidence"`
	Structure  KnowledgeStructure `json:"structure"`
}

type ExtractionResult struct {
	Compiler        string                `json:"compiler"`
	Candidates      []ExtractionCandidate `json:"candidates"`
	TotalChunks     int                   `json:"total_chunks,omitempty"`
	SucceededChunks int                   `json:"succeeded_chunks,omitempty"`
	FailedChunks    int                   `json:"failed_chunks,omitempty"`
	Degraded        bool                  `json:"degraded,omitempty"`
	FailureReason   string                `json:"failure_reason,omitempty"`
}

// CandidateExtractor is optional. A configured implementation may extract
// semantic candidates with an LLM; the engine validates every returned field
// and falls back to the deterministic compiler when it is unavailable.
type CandidateExtractor interface {
	Extract(context.Context, ExtractionRequest) (ExtractionResult, error)
	Compiler(context.Context) string
}

type KnowledgeUnit struct {
	UnitID        string             `json:"unit_id"`
	EpisodeID     string             `json:"episode_id"`
	EvidenceID    string             `json:"evidence_id"`
	UnitType      string             `json:"unit_type"`
	TierHint      string             `json:"tier_hint"`
	Statement     string             `json:"statement"`
	NormalizedKey string             `json:"normalized_key"`
	Confidence    float64            `json:"confidence"`
	RiskTier      string             `json:"risk_tier"`
	Status        string             `json:"status"`
	Scopes        []string           `json:"scopes"`
	ObservedAt    string             `json:"observed_at"`
	CreatedAt     string             `json:"created_at"`
	ProcessedAt   string             `json:"processed_at,omitempty"`
	SchemaVersion string             `json:"schema_version"`
	Structure     KnowledgeStructure `json:"structure"`
}

type Episode struct {
	EpisodeID    string   `json:"episode_id"`
	SessionID    string   `json:"session_id"`
	SourceSystem string   `json:"source_system"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Status       string   `json:"status"`
	EvidenceIDs  []string `json:"evidence_ids"`
	StartedAt    string   `json:"started_at"`
	EndedAt      string   `json:"ended_at"`
	Compiler     string   `json:"compiler"`
	Revision     int      `json:"revision"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	Units        int      `json:"units"`
}

type MemoryRecord struct {
	MemoryID         string   `json:"memory_id"`
	Tier             string   `json:"tier"`
	AssetForm        string   `json:"asset_form"`
	Domain           string   `json:"domain"`
	Status           string   `json:"status"`
	Summary          string   `json:"summary"`
	Body             string   `json:"body"`
	CanonicalKey     string   `json:"canonical_key"`
	Confidence       float64  `json:"confidence"`
	Importance       float64  `json:"importance"`
	Strength         float64  `json:"strength"`
	EvidenceIDs      []string `json:"source_evidence_ids"`
	EpisodeIDs       []string `json:"source_episode_ids"`
	Scopes           []string `json:"scopes"`
	Visibility       string   `json:"visibility"`
	ObservedAt       string   `json:"observed_at"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	LastReinforcedAt string   `json:"last_reinforced_at,omitempty"`
}

type MemoryOperation struct {
	OperationID    string   `json:"operation_id"`
	Type           string   `json:"type"`
	Status         string   `json:"status"`
	TargetMemoryID string   `json:"target_memory_id,omitempty"`
	UnitID         string   `json:"unit_id,omitempty"`
	EpisodeID      string   `json:"episode_id,omitempty"`
	EvidenceIDs    []string `json:"evidence_ids"`
	ReasonCodes    []string `json:"reason_codes"`
	RiskTier       string   `json:"risk_tier"`
	Confidence     float64  `json:"confidence"`
	PatchJSON      string   `json:"patch_json"`
	CreatedAt      string   `json:"created_at"`
	DecidedAt      string   `json:"decided_at,omitempty"`
	AppliedAt      string   `json:"applied_at,omitempty"`
	ReviewedBy     string   `json:"reviewed_by,omitempty"`
	Summary        string   `json:"summary,omitempty"`
}

type LivingView struct {
	ViewID          string   `json:"view_id"`
	ProjectID       string   `json:"project_id,omitempty"`
	ViewType        string   `json:"view_type"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	Status          string   `json:"status"`
	SourceMemoryIDs []string `json:"source_memory_ids"`
	CanonicalPath   string   `json:"canonical_path"`
	UpdatedAt       string   `json:"updated_at"`
}

type LivingDetail struct {
	View     LivingView     `json:"view"`
	Content  string         `json:"content"`
	Memories []MemoryRecord `json:"memories"`
}

type AgentAsset struct {
	AssetID               string         `json:"asset_id"`
	AssetType             string         `json:"asset_type"`
	Title                 string         `json:"title"`
	Summary               string         `json:"summary"`
	Status                string         `json:"status"`
	Version               string         `json:"version"`
	RiskLevel             string         `json:"risk_level"`
	SourceMemoryIDs       []string       `json:"source_memory_ids"`
	ValidationStatus      string         `json:"validation_status"`
	ClassificationStatus  string         `json:"classification_status,omitempty"`
	ClassificationScores  map[string]int `json:"classification_scores,omitempty"`
	ClassificationReasons []string       `json:"classification_reasons,omitempty"`
	CreatedAt             string         `json:"created_at"`
	UpdatedAt             string         `json:"updated_at"`
}

type AssetDetail struct {
	Asset    AgentAsset     `json:"asset"`
	Memories []MemoryRecord `json:"memories"`
}

type LayerSummary struct {
	Ordinal     int    `json:"ordinal"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	ChineseName string `json:"chinese_name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
	Status      string `json:"status"`
}

type Overview struct {
	GeneratedAt   string         `json:"generated_at"`
	Compiler      string         `json:"compiler"`
	Policy        string         `json:"policy"`
	Layers        []LayerSummary `json:"layers"`
	NeedsReview   int            `json:"needs_review"`
	CompletedJobs int            `json:"completed_jobs"`
}

type Trace struct {
	Memory     MemoryRecord      `json:"memory"`
	Operations []MemoryOperation `json:"operations"`
	Units      []KnowledgeUnit   `json:"knowledge_units"`
	Episodes   []Episode         `json:"episodes"`
	Evidence   []EvidenceRef     `json:"evidence"`
}

// ReviewDetail gives the owner enough context to make a protected-memory
// decision without exposing storage implementation details in the UI.
type ReviewDetail struct {
	Operation      MemoryOperation `json:"operation"`
	ProposedMemory *MemoryRecord   `json:"proposed_memory,omitempty"`
	KnowledgeUnit  *KnowledgeUnit  `json:"knowledge_unit,omitempty"`
	Episode        *Episode        `json:"episode,omitempty"`
	Evidence       []EvidenceRef   `json:"evidence"`
}

type EvidenceRef struct {
	EvidenceID   string `json:"evidence_id"`
	SourceSystem string `json:"source_system"`
	SessionID    string `json:"session_id"`
	Role         string `json:"role,omitempty"`
	ObservedAt   string `json:"observed_at"`
	Preview      string `json:"preview"`
}

type GraphNode struct {
	ID         string  `json:"id"`
	Layer      string  `json:"layer"`
	Label      string  `json:"label"`
	Status     string  `json:"status"`
	EntityType string  `json:"entity_type,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type GraphEdge struct {
	ID           string  `json:"id,omitempty"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label,omitempty"`
	InverseLabel string  `json:"inverse_label,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	EvidenceID   string  `json:"evidence_id,omitempty"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type RunResult struct {
	JobID          string `json:"job_id"`
	SessionID      string `json:"session_id"`
	EpisodeID      string `json:"episode_id"`
	Compiler       string `json:"compiler"`
	QualityStatus  string `json:"quality_status"`
	Evidence       int    `json:"evidence"`
	KnowledgeUnits int    `json:"knowledge_units"`
	Operations     int    `json:"operations"`
	Status         string `json:"status"`
}
