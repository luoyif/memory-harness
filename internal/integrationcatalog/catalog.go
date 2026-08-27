package integrationcatalog

import "github.com/luoyif/memory-harness/internal/agentauth"

type Tool struct {
	Name       string   `json:"name"`
	Mode       string   `json:"mode"`
	Permission string   `json:"permission"`
	Scope      string   `json:"scope"`
	Summary    string   `json:"summary"`
	Limits     []string `json:"limits,omitempty"`
}

var tools = []Tool{
	{"memory_search", "read", agentauth.PermissionMemoryRead, "one granted project", "Search immutable Evidence with filters and neighboring turns.", nil},
	{"memory_read_evidence", "read", agentauth.PermissionMemoryRead, "record project", "Read an exact immutable Evidence envelope by ID.", nil},
	{"memory_recall", "read", agentauth.PermissionMemoryRead, "one granted project or explicit all-project grant", "Recall Evidence, Episodes, governed Memory and project records.", []string{"finance results additionally require finance.read"}},
	{"memory_list_projects", "read", agentauth.PermissionProjectRead, "granted projects only", "List visible project spaces and summary metrics.", nil},
	{"memory_project_context", "read", agentauth.PermissionProjectRead, "one granted project", "Read bounded context, facts, goals, decisions and risks.", []string{"finance entries additionally require finance.read"}},
	{"memory_project_blueprint", "read", agentauth.PermissionProjectRead, "one granted project", "Read the exact active Memory Blueprint, plugin bindings, policy and validation.", []string{"cannot publish or activate Blueprints"}},
	{"memory_timeline", "read", agentauth.PermissionProjectRead, "one granted project", "Read event/valid/recorded time, temporal relevance and pairwise time correlations around an anchor.", []string{"temporal proximity is not causal evidence"}},
	{"memory_list_active_assets", "read", agentauth.PermissionMemoryRead, "one granted project", "List template-governed V4 assets first, then compatible active V3 assets.", []string{"candidate and pending revisions are excluded"}},
	{"memory_propose_asset_revision", "write", agentauth.PermissionMemoryPropose, "one granted project", "Propose a new immutable revision for a governed Agent Asset v4 or v3.", []string{"requires expected_revision and edit_reason", "server revalidates", "cannot approve or activate"}},
	{"memory_list_types", "read", agentauth.PermissionMemoryRead, "global contracts only", "List versioned generic memory type contracts contributed by plugins.", nil},
	{"memory_list_objects", "read", agentauth.PermissionMemoryRead, "one granted project", "List generic memory objects with lifecycle, revision and provenance.", nil},
	{"memory_read_object", "read", agentauth.PermissionMemoryRead, "record project", "Read one generic memory object and its current immutable revision.", nil},
	{"memory_list_runs", "read", agentauth.PermissionTraceRead, "one granted project", "List visible pipeline runs and terminal status.", nil},
	{"memory_read_run", "read", agentauth.PermissionTraceRead, "run project", "Read stage, event and redacted effect-receipt trace.", nil},
	{"memory_capture", "write", agentauth.PermissionMemoryCapture, "one granted project", "Append idempotent provenance-bearing Evidence and run governed distillation.", []string{"cannot delete Evidence", "cannot approve protected memory"}},
	{"memory_project_write", "write", agentauth.PermissionProjectWrite, "one granted project", "Create a goal, decision or risk with optional same-project Evidence.", []string{"cannot approve protected memory"}},
	{"memory_finance_write", "write", agentauth.PermissionFinanceWrite, "one granted project", "Append an idempotent signed-minor-unit finance entry.", []string{"currencies are never silently combined"}},
	{"memory_team_list_tasks", "read", "direct task membership", "assigned team tasks only", "List active collaboration tasks that directly include this Agent.", []string{"Memory Harness does not launch or schedule external Agents"}},
	{"memory_team_read_private", "read", agentauth.PermissionTeamPrivate, "one assigned task", "Read only this Agent's unexpired private working notes.", []string{"private bodies never enter normal Owner browsing"}},
	{"memory_team_write_private", "write", agentauth.PermissionTeamPrivate, "one assigned task", "Write an expiring private working note for this Agent only.", []string{"bounded by task expiry"}},
	{"memory_team_read_shared", "read", agentauth.PermissionTeamBlackboardRead, "one assigned task", "Read entries authored by or directly shared to this Agent.", []string{"no transitive forwarding"}},
	{"memory_team_write_shared", "write", agentauth.PermissionTeamBlackboardWrite, "one assigned task", "Submit a shared contribution with conflict preservation.", []string{"direct recipients additionally require team.blackboard.share", "conflicts require Owner review"}},
	{"memory_team_update_share", "write", agentauth.PermissionTeamBlackboardShare, "one authored shared entry", "Replace direct recipients for this Agent's own shared entry.", []string{"recipients cannot forward another Agent's entry"}},
	{"memory_team_propose_leave", "write", "direct task membership", "one assigned task", "Create an Owner review request to leave a task.", []string{"membership does not change until Owner approval"}},
}

func Tools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	return out
}

func ByName(name string) (Tool, bool) {
	for _, item := range tools {
		if item.Name == name {
			return item, true
		}
	}
	return Tool{}, false
}
