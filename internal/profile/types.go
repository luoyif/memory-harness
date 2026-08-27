package profile

const (
	ViewOwnerIdentity    = "owner_identity"
	ViewAgentIdentity    = "agent_identity"
	ViewStablePreference = "stable_preference"
	ViewDynamicProject   = "dynamic_project"
	ViewRelationship     = "relationship"
	ViewSessionResume    = "session_resume"
)

type Block struct {
	BlockID             string   `json:"block_id"`
	Label               string   `json:"label"`
	Content             string   `json:"content"`
	SourceRefs          []string `json:"source_refs"`
	SourceObjectIDs     []string `json:"source_object_ids"`
	SourceHash          string   `json:"source_hash"`
	CandidateSourceHash string   `json:"candidate_source_hash,omitempty"`
	ValidFrom           string   `json:"valid_from,omitempty"`
	ValidUntil          string   `json:"valid_until,omitempty"`
	LastVerifiedAt      string   `json:"last_verified_at"`
	Confidence          float64  `json:"confidence"`
	Locked              bool     `json:"locked"`
	ReviewStatus        string   `json:"review_status"`
}

type Projection struct {
	ProfileID             string   `json:"profile_id"`
	ViewKind              string   `json:"view_kind"`
	ProfileClass          string   `json:"profile_class"`
	Title                 string   `json:"title"`
	Summary               string   `json:"summary"`
	Blocks                []Block  `json:"blocks"`
	LockedBlockIDs        []string `json:"locked_block_ids"`
	GenerationStatus      string   `json:"generation_status"`
	GeneratedFromRevision int      `json:"generated_from_revision"`
	GeneratedAt           string   `json:"generated_at"`
}

type AgentView struct {
	AgentID             string   `json:"agent_id"`
	ProjectID           string   `json:"project_id"`
	GeneratedAt         string   `json:"generated_at"`
	Blocks              []Block  `json:"blocks"`
	SourceProjectionIDs []string `json:"source_projection_ids"`
	DeliveryStatus      string   `json:"delivery_status"`
}
