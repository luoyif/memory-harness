package server

import (
	"net/http"

	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/integrationcatalog"
)

func (s *Server) integrationCapabilities(w http.ResponseWriter, r *http.Request) {
	tools := integrationcatalog.Tools()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":          buildinfo.Version,
		"transport":        "stdio",
		"command":          "memoryosd mcp --endpoint http://127.0.0.1:19777",
		"token_env":        "MEMORYOS_AGENT_TOKEN",
		"authentication":   "Bearer token; plaintext is shown once and only its SHA-256 hash is stored",
		"project_boundary": "Every operation is checked against the Agent project grants before data access.",
		"review_boundary":  "Agents cannot approve identity, procedure, correction or protected asset changes.",
		"tools":            tools,
		"context_bridge": map[string]any{
			"status":                 "mvp",
			"protocol":               "memory-harness.adapter/v1alpha1",
			"mcp_baseline_unchanged": true,
			"contracts": map[string]string{
				"capability_set": contextbridge.CapabilitySchemaVersion,
				"plan":           contextbridge.PlanSchemaVersion,
				"receipt":        contextbridge.ReceiptSchemaVersion,
				"outcome":        contextbridge.OutcomeSchemaVersion,
			},
			"endpoints": []map[string]string{
				{"method": "POST", "path": "/v1/agent/context/handshake", "permission": agentauth.PermissionContextPlan},
				{"method": "POST", "path": "/v1/agent/context/plans", "permission": agentauth.PermissionContextPlan},
				{"method": "POST", "path": "/v1/agent/context/receipts", "permission": agentauth.PermissionContextReceipt},
				{"method": "POST", "path": "/v1/agent/outcomes", "permission": agentauth.PermissionOutcomeReport},
			},
			"evidence_states": []string{"retrieved", "planned", "delivery_unverified", "delivered", "used_unknown", "outcome_observed"},
			"invariants":      []string{"Plan is intent, not delivery", "Receipt must match item revision/hash", "Outcome is observation only and cannot mutate current Memory or Blueprint"},
		},
		"documentation": map[string]string{
			"api":         "docs/API.md",
			"mcp":         "docs/MCP.md",
			"permissions": "docs/PERMISSIONS.md",
			"model_agent": "docs/MODEL_AGENT.md",
			"skill":       "skills/memoryos-agent/SKILL.md",
		},
	})
}
