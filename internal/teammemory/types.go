package teammemory

const (
	PluginID              = "builtin.team-memory"
	PluginVersion         = "1.0.0"
	TaskTypeV1            = PluginID + ".task.v1"
	PrivateScratchTypeV1  = PluginID + ".private-scratchpad.v1"
	BlackboardEntryTypeV1 = PluginID + ".blackboard-entry.v1"
	ConflictTypeV1        = PluginID + ".conflict.v1"
	ProjectDurableTypeV1  = PluginID + ".project-durable.v1"
)

type Task struct {
	TaskID         string   `json:"task_id"`
	ProjectID      string   `json:"project_id"`
	Title          string   `json:"title"`
	MemberAgentIDs []string `json:"member_agent_ids"`
	Status         string   `json:"status"`
	CreatedAt      string   `json:"created_at"`
	ExpiresAt      string   `json:"expires_at"`
	ClosedAt       string   `json:"closed_at,omitempty"`
}

type ContributionMeta struct {
	AgentID           string   `json:"agent_id"`
	RunID             string   `json:"run_id,omitempty"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	Confidence        float64  `json:"confidence"`
	EpistemicStatus   string   `json:"epistemic_status"`
	CreatedAt         string   `json:"created_at"`
	ExpiresAt         string   `json:"expires_at"`
}

type PrivateScratch struct {
	EntryID   string           `json:"entry_id"`
	TaskID    string           `json:"task_id"`
	ProjectID string           `json:"project_id"`
	Content   string           `json:"content"`
	Meta      ContributionMeta `json:"meta"`
}

type BlackboardEntry struct {
	EntryID             string           `json:"entry_id"`
	TaskID              string           `json:"task_id"`
	ProjectID           string           `json:"project_id"`
	Topic               string           `json:"topic"`
	ClaimKey            string           `json:"claim_key"`
	ClaimValue          string           `json:"claim_value"`
	Content             string           `json:"content"`
	DirectShareAgentIDs []string         `json:"direct_share_agent_ids"`
	Meta                ContributionMeta `json:"meta"`
}

type Conflict struct {
	ConflictID string   `json:"conflict_id"`
	TaskID     string   `json:"task_id"`
	ProjectID  string   `json:"project_id"`
	Topic      string   `json:"topic"`
	ClaimKey   string   `json:"claim_key"`
	EntryIDs   []string `json:"entry_ids"`
	AgentIDs   []string `json:"agent_ids"`
	Status     string   `json:"status"`
	CreatedAt  string   `json:"created_at"`
}

type ProjectDurable struct {
	DurableID         string   `json:"durable_id"`
	ProjectID         string   `json:"project_id"`
	TaskID            string   `json:"task_id"`
	EntryIDs          []string `json:"entry_ids"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	Body              string   `json:"body"`
	SourceAgentIDs    []string `json:"source_agent_ids"`
	SourceRunIDs      []string `json:"source_run_ids"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	EpistemicStatus   string   `json:"epistemic_status"`
	GenerationStatus  string   `json:"generation_status"`
	CreatedAt         string   `json:"created_at"`
}

type CreateTaskInput struct {
	ProjectID      string   `json:"project_id"`
	Title          string   `json:"title"`
	MemberAgentIDs []string `json:"member_agent_ids"`
	TTLSeconds     int64    `json:"ttl_seconds"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type ContributionInput struct {
	Content           string   `json:"content"`
	RunID             string   `json:"run_id,omitempty"`
	SourceEvidenceIDs []string `json:"source_evidence_ids,omitempty"`
	Confidence        float64  `json:"confidence"`
	EpistemicStatus   string   `json:"epistemic_status"`
	TTLSeconds        int64    `json:"ttl_seconds"`
	IdempotencyKey    string   `json:"idempotency_key"`
}

type BlackboardInput struct {
	ContributionInput
	Topic               string   `json:"topic"`
	ClaimKey            string   `json:"claim_key"`
	ClaimValue          string   `json:"claim_value"`
	DirectShareAgentIDs []string `json:"direct_share_agent_ids,omitempty"`
}

type ShareInput struct {
	DirectShareAgentIDs []string `json:"direct_share_agent_ids"`
	IdempotencyKey      string   `json:"idempotency_key"`
}

type DurableInput struct {
	TaskID         string   `json:"task_id"`
	EntryIDs       []string `json:"entry_ids"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Body           string   `json:"body"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type ActivationInput struct {
	ExpectedRevision int    `json:"expected_revision"`
	EditReason       string `json:"edit_reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type ConflictResult struct {
	ObjectID string `json:"object_id"`
	ReviewID string `json:"review_id"`
}

type BlackboardResult struct {
	Entry     BlackboardEntry  `json:"entry"`
	Conflicts []ConflictResult `json:"conflicts"`
}
