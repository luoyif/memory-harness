package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/doctor"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/search"
)

var Version = buildinfo.Version

type Server struct {
	app             *app.App
	http            *http.Server
	startedAt       time.Time
	ownerAuthBypass bool
}

type Option func(*Server)

// WithOwnerAuthBypassForTests keeps legacy feature tests focused on their own
// contract. Production callers must never use this option; owner-boundary tests
// exercise the secure default.
func WithOwnerAuthBypassForTests() Option {
	return func(s *Server) { s.ownerAuthBypass = true }
}

func New(a *app.App, options ...Option) *Server {
	s := &Server{app: a, startedAt: time.Now().UTC()}
	for _, option := range options {
		option(s)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", redirectUI)
	mux.HandleFunc("GET /ui", redirectUI)
	mux.Handle("GET /ui/", UIHandler())
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/version", s.version)
	mux.HandleFunc("POST /v1/evidence", s.capture)
	mux.HandleFunc("POST /v1/evidence/batch", s.captureBatch)
	mux.HandleFunc("GET /v1/evidence", s.readEvidence)
	mux.HandleFunc("GET /v1/evidence/{id}", s.readEvidence)
	mux.HandleFunc("POST /v1/search", s.search)
	mux.HandleFunc("POST /v1/search/unified", s.unifiedSearch)
	mux.HandleFunc("POST /v1/recall/feedback", s.recallFeedback)
	mux.HandleFunc("GET /v1/doctor", s.doctor)
	mux.HandleFunc("POST /v1/rebuild/search", s.rebuild)
	mux.HandleFunc("GET /v1/dashboard", s.dashboard)
	mux.HandleFunc("GET /v1/today", s.today)
	mux.HandleFunc("GET /v1/memory", s.memory)
	mux.HandleFunc("GET /v1/assets", s.assets)
	mux.HandleFunc("GET /v1/asset-templates", s.assetTemplates)
	mux.HandleFunc("GET /v1/assets/{id}", s.assetDetail)
	mux.HandleFunc("POST /v1/assets/{id}/classification", s.resolveAssetClassification)
	mux.HandleFunc("GET /v1/layers", s.layers)
	mux.HandleFunc("GET /v1/episodes", s.episodes)
	mux.HandleFunc("GET /v1/episodes/{id}", s.episode)
	mux.HandleFunc("GET /v1/sessions/{id}/participants", s.sessionParticipants)
	mux.HandleFunc("PUT /v1/sessions/{id}/participants", s.replaceSessionParticipants)
	mux.HandleFunc("GET /v1/knowledge-units", s.knowledgeUnits)
	mux.HandleFunc("GET /v1/knowledge-units/{id}", s.knowledgeUnit)
	mux.HandleFunc("POST /v1/knowledge-units/{id}/revision-proposals", s.proposeKnowledgeUnitRevision)
	mux.HandleFunc("GET /v1/memories", s.memories)
	mux.HandleFunc("GET /v1/memory-pins", s.memoryPins)
	mux.HandleFunc("GET /v1/memories/{id}", s.memoryRecord)
	mux.HandleFunc("PUT /v1/memories/{id}/pin", s.setMemoryPin)
	mux.HandleFunc("GET /v1/memories/{id}/governance", s.memoryGovernance)
	mux.HandleFunc("POST /v1/memories/{id}/revision-proposals", s.proposeMemoryRevision)
	mux.HandleFunc("GET /v1/memories/{id}/trace", s.memoryTrace)
	mux.HandleFunc("GET /v1/operations", s.operations)
	mux.HandleFunc("GET /v1/operations/{id}", s.operationDetail)
	mux.HandleFunc("POST /v1/operations/{id}/review", s.reviewOperation)
	mux.HandleFunc("GET /v1/living", s.livingViews)
	mux.HandleFunc("GET /v1/living/{id}", s.livingDetail)
	mux.HandleFunc("GET /v1/graph", s.graph)
	mux.HandleFunc("GET /v1/graph/semantic", s.semanticGraph)
	mux.HandleFunc("POST /v1/process", s.processMemory)
	mux.HandleFunc("GET /v1/process/sources", s.processSources)
	mux.HandleFunc("POST /v1/import/text", s.importText)
	mux.HandleFunc("POST /v1/import/conversations", s.importConversations)
	mux.HandleFunc("GET /v1/sources", s.sources)
	mux.HandleFunc("GET /v1/jobs", s.jobs)
	mux.HandleFunc("GET /v1/health/detail", s.healthDetail)
	mux.HandleFunc("GET /v1/projects", s.projects)
	mux.HandleFunc("POST /v1/projects", s.createProject)
	mux.HandleFunc("GET /v1/projects/{id}", s.project)
	mux.HandleFunc("GET /v1/projects/{id}/activity-calendar", s.projectActivityCalendar)
	mux.HandleFunc("POST /v1/projects/{id}/assets/refine", s.refineProjectAssets)
	mux.HandleFunc("GET /v1/projects/{id}/automation", s.projectAutomation)
	mux.HandleFunc("PATCH /v1/projects/{id}/automation", s.updateProjectAutomation)
	mux.HandleFunc("GET /v1/profiles", s.profiles)
	mux.HandleFunc("POST /v1/projects/{id}/profiles/refresh", s.refreshProfiles)
	mux.HandleFunc("PUT /v1/projects/{id}/profiles/{view}/locks", s.setProfileLocks)
	mux.HandleFunc("GET /v1/experience/cases", s.experienceCases)
	mux.HandleFunc("POST /v1/projects/{id}/experience/rebuild", s.rebuildExperienceCases)
	mux.HandleFunc("GET /v1/experience/cases/{id}", s.experienceCase)
	mux.HandleFunc("POST /v1/experience/cases/{id}/evaluations", s.evaluateExperienceCase)
	mux.HandleFunc("POST /v1/experience/cases/{id}/activation-proposal", s.proposeExperienceCaseActivation)
	mux.HandleFunc("GET /v1/experience/evaluations", s.experienceEvaluations)
	mux.HandleFunc("GET /v1/experience/patterns", s.experiencePatterns)
	mux.HandleFunc("POST /v1/experience/patterns", s.createExperiencePattern)
	mux.HandleFunc("GET /v1/experience/patterns/{id}", s.experiencePattern)
	mux.HandleFunc("POST /v1/experience/patterns/{id}/evaluations", s.evaluateExperiencePattern)
	mux.HandleFunc("POST /v1/experience/patterns/{id}/activation-proposal", s.proposeExperiencePatternActivation)
	mux.HandleFunc("GET /v1/adaptation/proposals", s.adaptationProposals)
	mux.HandleFunc("POST /v1/adaptation/proposals/dry-run", s.dryRunAdaptationProposal)
	mux.HandleFunc("POST /v1/adaptation/proposals", s.createAdaptationProposal)
	mux.HandleFunc("GET /v1/adaptation/proposals/{id}", s.adaptationProposal)
	mux.HandleFunc("POST /v1/adaptation/proposals/{id}/evaluations", s.evaluateAdaptationProposal)
	mux.HandleFunc("POST /v1/adaptation/proposals/{id}/approval-proposal", s.proposeAdaptationApproval)
	mux.HandleFunc("GET /v1/adaptation/overlays", s.adaptationOverlays)
	mux.HandleFunc("POST /v1/adaptation/overlays", s.createAdaptationOverlay)
	mux.HandleFunc("GET /v1/adaptation/overlays/{id}", s.adaptationOverlay)
	mux.HandleFunc("POST /v1/adaptation/overlays/{id}/activation-proposal", s.proposeAdaptationOverlayActivation)
	mux.HandleFunc("POST /v1/adaptation/overlays/{id}/canary", s.runAdaptationCanary)
	mux.HandleFunc("POST /v1/adaptation/overlays/{id}/rollback", s.rollbackAdaptationOverlay)
	mux.HandleFunc("GET /v1/team/tasks", s.teamTasks)
	mux.HandleFunc("POST /v1/team/tasks", s.createTeamTask)
	mux.HandleFunc("GET /v1/team/tasks/{id}", s.teamTask)
	mux.HandleFunc("POST /v1/team/tasks/{id}/close-proposal", s.proposeTeamTaskClosure)
	mux.HandleFunc("GET /v1/team/conflicts", s.teamConflicts)
	mux.HandleFunc("GET /v1/team/durables", s.teamDurables)
	mux.HandleFunc("POST /v1/team/durables", s.createTeamDurable)
	mux.HandleFunc("POST /v1/team/durables/{id}/activation-proposal", s.proposeTeamDurableActivation)
	mux.HandleFunc("POST /v1/projects/{id}/knowledge-products/project-brief/refresh", s.refreshProjectBrief)
	mux.HandleFunc("GET /v1/knowledge-products/{id}/blocks", s.knowledgeProductBlocks)
	mux.HandleFunc("PUT /v1/knowledge-products/{id}/block-locks", s.setKnowledgeProductBlockLocks)
	mux.HandleFunc("GET /v1/knowledge-products/{id}/merge-preview", s.knowledgeProductMergePreview)
	mux.HandleFunc("POST /v1/project-links", s.linkProjectRecord)
	mux.HandleFunc("GET /v1/facts", s.facts)
	mux.HandleFunc("GET /v1/timeline", s.timeline)
	mux.HandleFunc("GET /v1/corrections/impact", s.correctionImpact)
	mux.HandleFunc("POST /v1/entities/{id}/revision-proposals", s.proposeEntityCorrection)
	mux.HandleFunc("POST /v1/facts", s.createFact)
	mux.HandleFunc("GET /v1/context-blocks", s.contextBlocks)
	mux.HandleFunc("POST /v1/context-blocks", s.upsertContextBlock)
	mux.HandleFunc("GET /v1/goals", s.goals)
	mux.HandleFunc("POST /v1/goals", s.createGoal)
	mux.HandleFunc("PATCH /v1/goals/{id}", s.updateGoal)
	mux.HandleFunc("GET /v1/project-tasks", s.projectTasks)
	mux.HandleFunc("POST /v1/project-tasks", s.createProjectTask)
	mux.HandleFunc("PATCH /v1/project-tasks/{id}", s.updateProjectTask)
	mux.HandleFunc("GET /v1/milestones", s.milestones)
	mux.HandleFunc("POST /v1/milestones", s.createMilestone)
	mux.HandleFunc("PATCH /v1/milestones/{id}", s.updateMilestone)
	mux.HandleFunc("GET /v1/decisions", s.decisions)
	mux.HandleFunc("POST /v1/decisions", s.createDecision)
	mux.HandleFunc("GET /v1/risks", s.risks)
	mux.HandleFunc("POST /v1/risks", s.createRisk)
	mux.HandleFunc("PATCH /v1/risks/{id}", s.updateRisk)
	mux.HandleFunc("GET /v1/finance/accounts", s.financeAccounts)
	mux.HandleFunc("POST /v1/finance/accounts", s.createFinanceAccount)
	mux.HandleFunc("GET /v1/finance/entries", s.financeEntries)
	mux.HandleFunc("POST /v1/finance/entries", s.createFinanceEntry)
	mux.HandleFunc("PATCH /v1/finance/entries/{id}", s.updateFinanceEntry)
	mux.HandleFunc("GET /v1/finance/summary", s.financeSummary)
	mux.HandleFunc("GET /v1/connectors", s.connectors)
	mux.HandleFunc("POST /v1/connectors", s.createConnector)
	mux.HandleFunc("GET /v1/agents", s.agents)
	mux.HandleFunc("POST /v1/agents", s.createAgent)
	mux.HandleFunc("PATCH /v1/agents/{id}", s.updateAgent)
	mux.HandleFunc("POST /v1/agents/{id}/rotate-token", s.rotateAgentToken)
	mux.HandleFunc("GET /v1/agent-audit", s.agentAudit)
	mux.HandleFunc("GET /v1/agent/me", s.agentMe)
	mux.HandleFunc("POST /v1/agent/context/handshake", s.agentContextHandshake)
	mux.HandleFunc("POST /v1/agent/context/plans", s.agentContextPlan)
	mux.HandleFunc("POST /v1/agent/context/receipts", s.agentContextReceipt)
	mux.HandleFunc("POST /v1/agent/outcomes", s.agentOutcomeFeedback)
	mux.HandleFunc("GET /v1/agent/team/tasks", s.agentTeamTasks)
	mux.HandleFunc("POST /v1/agent/team/tasks/{id}/leave-proposal", s.agentTeamLeaveProposal)
	mux.HandleFunc("GET /v1/agent/team/tasks/{id}/private", s.agentTeamPrivate)
	mux.HandleFunc("POST /v1/agent/team/tasks/{id}/private", s.agentWriteTeamPrivate)
	mux.HandleFunc("GET /v1/agent/team/tasks/{id}/blackboard", s.agentTeamBlackboard)
	mux.HandleFunc("POST /v1/agent/team/tasks/{id}/blackboard", s.agentWriteTeamBlackboard)
	mux.HandleFunc("POST /v1/agent/team/blackboard/{id}/share", s.agentShareTeamBlackboard)
	mux.HandleFunc("GET /v1/agent/projects", s.agentProjects)
	mux.HandleFunc("GET /v1/agent/projects/{id}/context", s.agentProjectContext)
	mux.HandleFunc("GET /v1/agent/projects/{id}/profile", s.agentProfileView)
	mux.HandleFunc("GET /v1/agent/timeline", s.agentTimeline)
	mux.HandleFunc("GET /v1/agent/projects/{id}/blueprint", s.agentProjectBlueprint)
	mux.HandleFunc("POST /v1/agent/search", s.agentSearch)
	mux.HandleFunc("POST /v1/agent/recall", s.agentRecall)
	mux.HandleFunc("GET /v1/agent/evidence/{id}", s.agentReadEvidence)
	mux.HandleFunc("POST /v1/agent/capture", s.agentCapture)
	mux.HandleFunc("POST /v1/agent/project-records", s.agentProjectRecord)
	mux.HandleFunc("POST /v1/agent/finance-entries", s.agentFinanceEntry)
	mux.HandleFunc("GET /v1/agent/memory-types", s.agentHarnessTypes)
	mux.HandleFunc("GET /v1/agent/objects", s.agentHarnessObjects)
	mux.HandleFunc("GET /v1/agent/objects/{id}", s.agentHarnessObject)
	mux.HandleFunc("POST /v1/agent/objects/{id}/revision-proposals", s.agentProposeHarnessRevision)
	mux.HandleFunc("GET /v1/agent/runs", s.agentHarnessRuns)
	mux.HandleFunc("GET /v1/agent/runs/{id}", s.agentHarnessRun)
	mux.HandleFunc("GET /v1/model/config", s.modelConfig)
	mux.HandleFunc("POST /v1/model/providers", s.createModelProvider)
	mux.HandleFunc("PATCH /v1/model/providers/{id}", s.updateModelProvider)
	mux.HandleFunc("POST /v1/model/providers/{id}/test", s.testModelProvider)
	mux.HandleFunc("PUT /v1/model/runtime", s.updateModelRuntime)
	mux.HandleFunc("GET /v1/integrations/capabilities", s.integrationCapabilities)
	mux.HandleFunc("GET /v1/harness/types", s.harnessTypes)
	mux.HandleFunc("POST /v1/harness/types", s.registerHarnessType)
	mux.HandleFunc("GET /v1/harness/objects", s.harnessObjects)
	mux.HandleFunc("POST /v1/harness/objects", s.materializeHarnessObject)
	mux.HandleFunc("GET /v1/harness/objects/{id}", s.harnessObject)
	mux.HandleFunc("GET /v1/harness/objects/{id}/revisions", s.harnessObjectRevisions)
	mux.HandleFunc("POST /v1/harness/objects/{id}/revisions", s.proposeHarnessObjectRevision)
	mux.HandleFunc("POST /v1/harness/objects/{id}/rollback", s.rollbackHarnessObject)
	mux.HandleFunc("GET /v1/harness/revision-reviews", s.harnessRevisionReviews)
	mux.HandleFunc("GET /v1/harness/revision-reviews/{id}", s.harnessRevisionReview)
	mux.HandleFunc("POST /v1/harness/revision-reviews/{id}/decision", s.decideHarnessRevisionReview)
	mux.HandleFunc("GET /v1/harness/runs", s.harnessRuns)
	mux.HandleFunc("POST /v1/harness/runs", s.startHarnessRun)
	mux.HandleFunc("GET /v1/harness/runs/{id}", s.harnessRun)
	mux.HandleFunc("GET /v1/context/exchanges/{id}", s.contextExchange)
	mux.HandleFunc("POST /v1/harness/runs/{id}/events", s.appendHarnessEvent)
	mux.HandleFunc("POST /v1/harness/runs/{id}/spans", s.startHarnessSpan)
	mux.HandleFunc("POST /v1/harness/spans/{id}/finish", s.finishHarnessSpan)
	mux.HandleFunc("POST /v1/harness/runs/{id}/effects", s.harnessEffect)
	mux.HandleFunc("POST /v1/harness/runs/{id}/cancel", s.cancelHarnessRun)
	mux.HandleFunc("POST /v1/harness/runs/{id}/retry", s.retryHarnessRun)
	mux.HandleFunc("POST /v1/harness/runs/{id}/fork", s.forkHarnessRun)
	mux.HandleFunc("GET /v1/pipelines", s.pipelines)
	mux.HandleFunc("POST /v1/pipelines", s.publishPipeline)
	mux.HandleFunc("GET /v1/pipelines/stages", s.pipelineStages)
	mux.HandleFunc("POST /v1/pipelines/validate", s.validatePipeline)
	mux.HandleFunc("GET /v1/pipelines/drafts", s.pipelineDrafts)
	mux.HandleFunc("PUT /v1/pipelines/drafts/{id}", s.savePipelineDraft)
	mux.HandleFunc("DELETE /v1/pipelines/drafts/{id}", s.deletePipelineDraft)
	mux.HandleFunc("POST /v1/pipelines/execute", s.executePipeline)
	mux.HandleFunc("POST /v1/pipelines/dry-run", s.dryRunPipeline)
	mux.HandleFunc("GET /v1/pipelines/reviews", s.pipelineReviews)
	mux.HandleFunc("POST /v1/pipelines/reviews/{id}/decision", s.decidePipelineReview)
	mux.HandleFunc("GET /v1/blueprints", s.blueprints)
	mux.HandleFunc("POST /v1/blueprints", s.publishBlueprint)
	mux.HandleFunc("POST /v1/blueprints/validate", s.validateBlueprint)
	mux.HandleFunc("GET /v1/projects/{id}/blueprint", s.currentBlueprint)
	mux.HandleFunc("PUT /v1/projects/{id}/blueprint", s.activateBlueprint)
	mux.HandleFunc("GET /v1/plugins", s.plugins)
	mux.HandleFunc("POST /v1/plugins/install", s.installPlugin)
	mux.HandleFunc("GET /v1/plugins/{id}/{version}/impact", s.pluginImpact)
	mux.HandleFunc("GET /v1/plugins/{id}/{version}/conformance", s.pluginConformance)
	mux.HandleFunc("POST /v1/plugins/{id}/{version}/retire", s.retirePlugin)
	mux.HandleFunc("PUT /v1/plugins/{id}/{version}/projects/{project}", s.setPluginProjectState)
	mux.HandleFunc("POST /v1/plugins/trust", s.approvePluginSigner)
	mux.HandleFunc("POST /v1/plugins/trust/{id}/revoke", s.revokePluginSigner)
	mux.HandleFunc("GET /v1/owner/session", s.ownerSession)
	s.http = &http.Server{Addr: a.Config.Addr, Handler: secureHeaders(s.desktopCORS(s.ownerBoundary(mux))), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	return s
}

func (s *Server) IssueOwnerSession(label string) (ownerauth.Credential, error) {
	return s.app.Owner.Issue(label)
}

func (s *Server) RevokeOwnerSession(ctx context.Context, token string) {
	s.app.Owner.Revoke(ctx, token)
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) desktopCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.ToLower(strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"))
		allowed := ownerauth.IsTrustedDesktopOrigin(origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Memory-Harness-Owner, X-Memory-Harness-CSRF")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "version": Version, "home": s.app.Config.Home})
}
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"version": Version})
}
func (s *Server) capture(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeErr(w, 400, "bad_body", err.Error())
		return
	}
	projectID, err := s.projectForEvidence(r.Context(), raw, "")
	if err != nil {
		writeErr(w, 400, "capture_failed", err.Error())
		return
	}
	res, err := s.app.Ledger.Append(r.Context(), raw)
	if err != nil {
		if errors.Is(err, ledger.ErrEvidenceConflict) {
			writeErr(w, 409, "evidence_conflict", err.Error())
			return
		}
		writeErr(w, 400, "capture_failed", err.Error())
		return
	}
	status := 201
	if res.Duplicate {
		status = 200
	}
	pipeline, processErr := s.growSession(r.Context(), projectID, res.SessionID, []string{res.EvidenceID}, true, false)
	response := map[string]any{
		"evidence_id": res.EvidenceID,
		"session_id":  res.SessionID,
		"ledger_path": res.LedgerPath,
		"line_hash":   res.LineHash,
		"ordinal":     res.Ordinal,
		"duplicate":   res.Duplicate,
		"project_id":  projectID,
	}
	if processErr == nil {
		response["pipeline"] = pipeline
	} else {
		// Canonical capture has succeeded. Surface the derived-processing failure
		// without pretending the Evidence append rolled back.
		response["pipeline_status"] = "failed"
		response["pipeline_error"] = processErr.Error()
	}
	writeJSON(w, status, response)
}
func (s *Server) readEvidence(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if id == "" {
		writeErr(w, 400, "missing_id", "evidence id required")
		return
	}
	raw, err := s.app.Ledger.ReadEvidence(r.Context(), id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeErr(w, 404, "not_found", "evidence not found")
			return
		}
		writeErr(w, 500, "read_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(append(raw, '\n'))
}
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	var q search.Query
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q); err != nil {
		writeErr(w, 400, "bad_query", err.Error())
		return
	}
	// API default enables session fusion unless explicitly disabled with ?fusion=0.
	if r.URL.Query().Get("fusion") != "0" {
		q.SessionFusion = true
	}
	res, err := s.app.Search.Search(r.Context(), q)
	if err != nil {
		writeErr(w, 400, "search_failed", err.Error())
		return
	}
	writeJSON(w, 200, res)
}
func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	rep, err := doctor.Run(r.Context(), s.app)
	if err != nil {
		writeErr(w, 500, "doctor_failed", err.Error())
		return
	}
	status := 200
	if rep.Status != "pass" {
		status = 503
	}
	writeJSON(w, status, rep)
}
func (s *Server) rebuild(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.Ledger.RebuildSearch(r.Context())
	if err != nil {
		writeErr(w, 500, "rebuild_failed", err.Error())
		return
	}
	documents, err := s.app.Portfolio.RebuildIndex(r.Context())
	if err != nil {
		writeErr(w, 500, "rebuild_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "evidence_indexed": n, "documents_indexed": documents})
}
func (s *Server) Handler() http.Handler              { return s.http.Handler }
func (s *Server) ListenAndServe() error              { return s.http.ListenAndServe() }
func (s *Server) Serve(listener net.Listener) error  { return s.http.Serve(listener) }
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
