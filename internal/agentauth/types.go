package agentauth

const (
	PermissionMemoryRead          = "memory.read"
	PermissionMemoryCapture       = "memory.capture"
	PermissionMemoryPropose       = "memory.propose"
	PermissionProjectRead         = "project.read"
	PermissionProjectWrite        = "project.write"
	PermissionFinanceRead         = "finance.read"
	PermissionFinanceWrite        = "finance.write"
	PermissionTraceRead           = "trace.read"
	PermissionContextPlan         = "context.plan"
	PermissionContextReceipt      = "context.receipt"
	PermissionOutcomeReport       = "outcome.report"
	PermissionTeamPrivate         = "team.private"
	PermissionTeamBlackboardRead  = "team.blackboard.read"
	PermissionTeamBlackboardWrite = "team.blackboard.write"
	PermissionTeamBlackboardShare = "team.blackboard.share"
)

var AllowedPermissions = []string{
	PermissionMemoryRead,
	PermissionMemoryCapture,
	PermissionMemoryPropose,
	PermissionProjectRead,
	PermissionProjectWrite,
	PermissionFinanceRead,
	PermissionFinanceWrite,
	PermissionTraceRead,
	PermissionContextPlan,
	PermissionContextReceipt,
	PermissionOutcomeReport,
	PermissionTeamPrivate,
	PermissionTeamBlackboardRead,
	PermissionTeamBlackboardWrite,
	PermissionTeamBlackboardShare,
}

type Principal struct {
	AgentID     string   `json:"agent_id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
	AllProjects bool     `json:"all_projects"`
	ProjectIDs  []string `json:"project_ids"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
}

type CreateInput struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Permissions []string `json:"permissions"`
	AllProjects bool     `json:"all_projects"`
	ProjectIDs  []string `json:"project_ids"`
}

type UpdateInput struct {
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
	AllProjects bool     `json:"all_projects"`
	ProjectIDs  []string `json:"project_ids"`
}

type Credential struct {
	Agent Principal `json:"agent"`
	Token string    `json:"token"`
}

type AuditEvent struct {
	EventID      string `json:"event_id"`
	AgentID      string `json:"agent_id"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Status       string `json:"status"`
	DetailJSON   string `json:"detail_json"`
	CreatedAt    string `json:"created_at"`
}
