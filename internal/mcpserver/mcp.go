package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/search"
	"github.com/luoyif/memory-harness/internal/teammemory"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchInput struct {
	Query             string   `json:"query" jsonschema:"plain-language or keyword query"`
	ProjectID         string   `json:"project_id" jsonschema:"project scope required for Agent access"`
	Limit             int      `json:"limit,omitempty" jsonschema:"maximum hits, default 5"`
	SourceSystem      string   `json:"source_system,omitempty" jsonschema:"optional source-system filter"`
	SessionID         string   `json:"session_id,omitempty" jsonschema:"optional exact session filter"`
	Scope             string   `json:"scope,omitempty" jsonschema:"optional exact scope_hints value filter"`
	DateFrom          string   `json:"date_from,omitempty" jsonschema:"optional YYYY-MM-DD or RFC3339 lower time bound"`
	DateTo            string   `json:"date_to,omitempty" jsonschema:"optional YYYY-MM-DD or RFC3339 upper time bound"`
	NeighborTurns     int      `json:"neighbor_turns,omitempty" jsonschema:"number of neighboring turns to include, 0-10"`
	ExcludeSessionIDs []string `json:"exclude_session_ids,omitempty" jsonschema:"sessions already inspected in an iterative search"`
}

type CaptureInput struct {
	ProjectID      string `json:"project_id" jsonschema:"target project granted to this Agent"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"stable retry key; reuse it only for equivalent content"`
	SourceSystem   string `json:"source_system" jsonschema:"origin such as codex claude-code or deepseek-harness"`
	SessionID      string `json:"session_id" jsonschema:"stable source conversation or work-session id"`
	Role           string `json:"role,omitempty" jsonschema:"user assistant system or tool; default assistant"`
	Text           string `json:"text" jsonschema:"visible evidence text to preserve and distill"`
	ObservedAt     string `json:"observed_at,omitempty" jsonschema:"optional RFC3339 source time"`
}

type CaptureOutput struct {
	EvidenceID string           `json:"evidence_id"`
	SessionID  string           `json:"session_id"`
	ProjectID  string           `json:"project_id"`
	Duplicate  bool             `json:"duplicate"`
	LedgerPath string           `json:"ledger_path"`
	Pipeline   memory.RunResult `json:"pipeline"`
}

type ProjectRecordInput struct {
	Kind              string   `json:"kind" jsonschema:"goal decision or risk"`
	ProjectID         string   `json:"project_id" jsonschema:"target project granted to this Agent"`
	Title             string   `json:"title" jsonschema:"short record title"`
	Description       string   `json:"description,omitempty" jsonschema:"goal or risk description"`
	Priority          int      `json:"priority,omitempty" jsonschema:"goal priority from 0 to 9"`
	TargetAt          string   `json:"target_at,omitempty" jsonschema:"optional RFC3339 goal target time"`
	Decision          string   `json:"decision,omitempty" jsonschema:"decision body when kind is decision"`
	Rationale         string   `json:"rationale,omitempty" jsonschema:"decision rationale"`
	DecidedAt         string   `json:"decided_at,omitempty" jsonschema:"RFC3339 decision time"`
	Probability       int      `json:"probability,omitempty" jsonschema:"risk probability from 1 to 5"`
	Impact            int      `json:"impact,omitempty" jsonschema:"risk impact from 1 to 5"`
	Mitigation        string   `json:"mitigation,omitempty" jsonschema:"risk mitigation"`
	Owner             string   `json:"owner,omitempty" jsonschema:"risk owner"`
	SourceEvidenceID  string   `json:"source_evidence_id,omitempty" jsonschema:"optional Evidence ID from the same project"`
	SourceEvidenceIDs []string `json:"source_evidence_ids,omitempty" jsonschema:"optional Evidence IDs from the same project"`
}

type ProjectRecordOutput struct {
	Kind   string `json:"kind"`
	Record any    `json:"record"`
}

type FinanceWriteInput struct {
	ProjectID        string `json:"project_id" jsonschema:"target project granted to this Agent"`
	AccountID        string `json:"account_id,omitempty" jsonschema:"optional account in the same project"`
	EntryType        string `json:"entry_type" jsonschema:"income expense or adjustment"`
	Category         string `json:"category" jsonschema:"finance category"`
	Description      string `json:"description" jsonschema:"human-readable reason"`
	AmountMinor      int64  `json:"amount_minor" jsonschema:"signed integer minor units; never a decimal"`
	Currency         string `json:"currency" jsonschema:"ISO currency code"`
	OccurredAt       string `json:"occurred_at" jsonschema:"RFC3339 occurrence time"`
	SourceEvidenceID string `json:"source_evidence_id,omitempty" jsonschema:"optional Evidence ID from the same project"`
	IdempotencyKey   string `json:"idempotency_key" jsonschema:"stable retry key"`
}

type FinanceWriteOutput struct {
	Entry     portfolio.FinanceEntry `json:"entry"`
	Duplicate bool                   `json:"duplicate"`
}

type ReadInput struct {
	EvidenceID string `json:"evidence_id" jsonschema:"stable evidence identifier"`
}
type ReadOutput struct {
	Evidence map[string]any `json:"evidence"`
}

type RecallInput struct {
	Query          string   `json:"query" jsonschema:"plain-language or keyword query"`
	ProjectID      string   `json:"project_id,omitempty" jsonschema:"project scope; required unless all_projects is true"`
	AllProjects    bool     `json:"all_projects,omitempty" jsonschema:"explicitly allow recall across project boundaries"`
	Kinds          []string `json:"kinds,omitempty" jsonschema:"optional evidence episode memory asset project fact goal decision risk finance filters"`
	AsOf           string   `json:"as_of,omitempty" jsonschema:"optional RFC3339 historical point in time"`
	IncludeHistory bool     `json:"include_history,omitempty" jsonschema:"include superseded expired deprecated void and archived records"`
	Limit          int      `json:"limit,omitempty" jsonschema:"maximum hits, default 10"`
}

type ProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"stable MemoryOS project identifier"`
	AsOf      string `json:"as_of,omitempty" jsonschema:"optional RFC3339 historical fact time"`
}

type TimelineInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project scope granted to this Agent"`
	AnchorAt  string   `json:"anchor_at,omitempty" jsonschema:"RFC3339 temporal anchor; defaults to now"`
	From      string   `json:"from,omitempty" jsonschema:"optional RFC3339 lower event-time bound"`
	Until     string   `json:"until,omitempty" jsonschema:"optional RFC3339 exclusive upper event-time bound"`
	Kinds     []string `json:"kinds,omitempty" jsonschema:"fact goal milestone decision episode memory run evidence finance filters"`
	Limit     int      `json:"limit,omitempty" jsonschema:"maximum temporal events, default 200"`
}

type ActiveAssetInput struct {
	ProjectID string `json:"project_id" jsonschema:"project scope granted to this Agent"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum active governed assets, default 100"`
}

type AssetRevisionProposalInput struct {
	ObjectID         string         `json:"object_id" jsonschema:"template-governed Agent Asset V4 or compatible governed V3 object identifier"`
	ExpectedRevision int            `json:"expected_revision" jsonschema:"current revision observed by the Agent; stale proposals fail closed"`
	EditReason       string         `json:"edit_reason" jsonschema:"why this revision is being proposed; persisted for Owner review"`
	Payload          map[string]any `json:"payload" jsonschema:"complete V4 template payload or compatible governed V3 payload; server revalidates the matching contract"`
	IdempotencyKey   string         `json:"idempotency_key" jsonschema:"stable retry key for this proposal"`
}

type ProjectContext struct {
	Summary       portfolio.ProjectSummary `json:"summary"`
	ContextBlocks []portfolio.ContextBlock `json:"context_blocks"`
	Facts         []portfolio.TemporalFact `json:"facts"`
	Goals         []portfolio.Goal         `json:"goals"`
	Milestones    []portfolio.Milestone    `json:"milestones"`
	Decisions     []portfolio.Decision     `json:"decisions"`
	Risks         []portfolio.Risk         `json:"risks"`
	Finance       []portfolio.FinanceEntry `json:"finance_entries"`
	Timeline      portfolio.Timeline       `json:"timeline"`
}

type ObjectListInput struct {
	ProjectID string `json:"project_id" jsonschema:"project scope granted to this Agent"`
	TypeID    string `json:"type_id,omitempty" jsonschema:"optional namespaced memory type filter"`
	Status    string `json:"status,omitempty" jsonschema:"optional lifecycle status filter"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum objects, default 100"`
}

type ObjectReadInput struct {
	ObjectID string `json:"object_id" jsonschema:"stable generic memory object identifier"`
}

type RunListInput struct {
	ProjectID string `json:"project_id" jsonschema:"project scope granted to this Agent"`
	Status    string `json:"status,omitempty" jsonschema:"optional run status filter"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum runs, default 100"`
}

type RunReadInput struct {
	RunID string `json:"run_id" jsonschema:"stable Memory Harness run identifier"`
}

type TeamTaskInput struct {
	TaskID string `json:"task_id" jsonschema:"team task assigned to the authenticated Agent"`
}

type TeamPrivateWriteInput struct {
	TaskID            string   `json:"task_id" jsonschema:"team task assigned to the authenticated Agent"`
	Content           string   `json:"content" jsonschema:"private working note visible only to this Agent"`
	RunID             string   `json:"run_id,omitempty" jsonschema:"optional same-project run produced by this Agent"`
	SourceEvidenceIDs []string `json:"source_evidence_ids,omitempty" jsonschema:"optional granted Evidence supporting the note"`
	Confidence        float64  `json:"confidence" jsonschema:"confidence between zero and one"`
	EpistemicStatus   string   `json:"epistemic_status" jsonschema:"observed inferred hypothesis or disputed"`
	TTLSeconds        int64    `json:"ttl_seconds" jsonschema:"note lifetime from 60 to 604800 seconds bounded by the task"`
	IdempotencyKey    string   `json:"idempotency_key" jsonschema:"stable retry key"`
}

type TeamSharedWriteInput struct {
	TeamPrivateWriteInput
	Topic               string   `json:"topic" jsonschema:"human-readable subject"`
	ClaimKey            string   `json:"claim_key" jsonschema:"stable key used to detect conflicting values"`
	ClaimValue          string   `json:"claim_value" jsonschema:"normalized value for conflict detection"`
	DirectShareAgentIDs []string `json:"direct_share_agent_ids,omitempty" jsonschema:"explicit direct recipients; recipients cannot forward"`
}

type TeamShareInput struct {
	EntryID             string   `json:"entry_id" jsonschema:"shared entry originally authored by this Agent"`
	DirectShareAgentIDs []string `json:"direct_share_agent_ids" jsonschema:"replacement list of direct recipients"`
	IdempotencyKey      string   `json:"idempotency_key" jsonschema:"stable retry key"`
}

type TeamLeaveInput struct {
	TaskID           string `json:"task_id" jsonschema:"active team task to leave"`
	ExpectedRevision int    `json:"expected_revision" jsonschema:"current task revision observed by the Agent"`
	EditReason       string `json:"edit_reason" jsonschema:"why the Agent is leaving"`
	IdempotencyKey   string `json:"idempotency_key" jsonschema:"stable retry key"`
}

type GenericItemsOutput struct {
	Items []map[string]any `json:"items"`
}

func genericMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}

func genericItems[T any](values []T) (GenericItemsOutput, error) {
	items := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, err := genericMap(value)
		if err != nil {
			return GenericItemsOutput{}, err
		}
		items = append(items, item)
	}
	return GenericItemsOutput{Items: items}, nil
}

type Backend interface {
	Search(context.Context, SearchInput) (search.Result, error)
	ReadEvidence(context.Context, string) ([]byte, error)
	Recall(context.Context, unifiedsearch.Query) (unifiedsearch.Result, error)
	ListProjects(context.Context) ([]portfolio.ProjectSummary, error)
	ProjectContext(context.Context, string, string) (ProjectContext, error)
	Timeline(context.Context, TimelineInput) (portfolio.Timeline, error)
	Capture(context.Context, CaptureInput) (CaptureOutput, error)
	WriteProjectRecord(context.Context, ProjectRecordInput) (ProjectRecordOutput, error)
	WriteFinanceEntry(context.Context, FinanceWriteInput) (FinanceWriteOutput, error)
	ListMemoryTypes(context.Context) ([]harness.MemoryType, error)
	ListObjects(context.Context, ObjectListInput) ([]harness.Object, error)
	ReadObject(context.Context, string) (harness.Object, error)
	ProposeAssetRevision(context.Context, AssetRevisionProposalInput) (harness.RevisionReview, error)
	ListRuns(context.Context, RunListInput) ([]harness.Run, error)
	ReadRun(context.Context, string) (harness.RunDetail, error)
	ProjectBlueprint(context.Context, string) (blueprint.Current, error)
}

// TeamBackend is intentionally optional. Production MCP uses the authenticated
// loopback HTTP backend; an unauthenticated in-process backend cannot safely
// impersonate a Team Memory principal.
type TeamBackend interface {
	ListTeamTasks(context.Context) ([]harness.Object, error)
	ReadTeamPrivate(context.Context, string) ([]harness.Object, error)
	WriteTeamPrivate(context.Context, TeamPrivateWriteInput) (harness.Object, error)
	ReadTeamShared(context.Context, string) ([]harness.Object, error)
	WriteTeamShared(context.Context, TeamSharedWriteInput) (teammemory.BlackboardResult, error)
	UpdateTeamShare(context.Context, TeamShareInput) (harness.Object, error)
	ProposeTeamLeave(context.Context, TeamLeaveInput) (harness.RevisionReview, error)
}

type LocalBackend struct{ App *app.App }

func (b LocalBackend) Search(ctx context.Context, in SearchInput) (search.Result, error) {
	q := search.Query{Text: in.Query, Limit: in.Limit, SourceSystem: in.SourceSystem, SessionID: in.SessionID, Scope: in.Scope, DateFrom: in.DateFrom, DateTo: in.DateTo, NeighborTurns: in.NeighborTurns, SessionFusion: true, ExcludeSessionIDs: in.ExcludeSessionIDs}
	return b.App.Search.Search(ctx, q)
}
func (b LocalBackend) ReadEvidence(ctx context.Context, id string) ([]byte, error) {
	return b.App.Ledger.ReadEvidence(ctx, id)
}
func (b LocalBackend) Recall(ctx context.Context, q unifiedsearch.Query) (unifiedsearch.Result, error) {
	return b.App.Unified.Search(ctx, q)
}
func (b LocalBackend) ListProjects(ctx context.Context) ([]portfolio.ProjectSummary, error) {
	projects, err := b.App.Portfolio.ListProjects(ctx, false)
	if err != nil {
		return nil, err
	}
	items := make([]portfolio.ProjectSummary, 0, len(projects))
	for _, project := range projects {
		summary, err := b.App.Portfolio.ProjectSummary(ctx, project.ProjectID)
		if err != nil {
			return nil, err
		}
		items = append(items, summary)
	}
	return items, nil
}
func (b LocalBackend) ProjectContext(ctx context.Context, projectID, asOf string) (ProjectContext, error) {
	summary, err := b.App.Portfolio.ProjectSummary(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	blocks, err := b.App.Portfolio.ListContextBlocks(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	facts, err := b.App.Portfolio.ListFacts(ctx, projectID, asOf, false, 500)
	if err != nil {
		return ProjectContext{}, err
	}
	goals, err := b.App.Portfolio.ListGoals(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	milestones, err := b.App.Portfolio.ListMilestones(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	decisions, err := b.App.Portfolio.ListDecisions(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	risks, err := b.App.Portfolio.ListRisks(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	finance, err := b.App.Portfolio.ListFinanceEntries(ctx, projectID, 100)
	if err != nil {
		return ProjectContext{}, err
	}
	timeline, err := b.App.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: projectID, AnchorAt: asOf, Limit: 100})
	if err != nil {
		return ProjectContext{}, err
	}
	return ProjectContext{Summary: summary, ContextBlocks: blocks, Facts: facts, Goals: goals, Milestones: milestones, Decisions: decisions, Risks: risks, Finance: finance, Timeline: timeline}, nil
}

func (b LocalBackend) Timeline(ctx context.Context, in TimelineInput) (portfolio.Timeline, error) {
	return b.App.Portfolio.Timeline(ctx, portfolio.TimelineQuery{ProjectID: in.ProjectID, AnchorAt: in.AnchorAt, From: in.From, Until: in.Until, Kinds: in.Kinds, Limit: in.Limit})
}

func (b LocalBackend) Capture(ctx context.Context, in CaptureInput) (CaptureOutput, error) {
	project, exact, err := b.App.Portfolio.Resolve(ctx, in.ProjectID)
	if err != nil || !exact {
		return CaptureOutput{}, errors.New("project_id must identify a registered project")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" || strings.TrimSpace(in.SourceSystem) == "" || strings.TrimSpace(in.SessionID) == "" || strings.TrimSpace(in.Text) == "" {
		return CaptureOutput{}, errors.New("project_id, idempotency_key, source_system, session_id and text are required")
	}
	observed := time.Now().UTC()
	if strings.TrimSpace(in.ObservedAt) != "" {
		observed, err = time.Parse(time.RFC3339, in.ObservedAt)
		if err != nil {
			return CaptureOutput{}, errors.New("observed_at must be RFC3339")
		}
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role == "" {
		role = "assistant"
	}
	evidenceID := "mcp_" + contracts.HashBytes([]byte(in.ProjectID + "\x00" + in.IdempotencyKey))[:24]
	if priorRaw, readErr := b.App.Ledger.ReadEvidence(ctx, evidenceID); readErr == nil {
		prior, _, parseErr := contracts.ParseEvidence(priorRaw)
		roleMatches := prior.Role != nil && *prior.Role == role
		contentMatches := len(prior.Content) == 1 && prior.Content[0].Type == "text" && prior.Content[0].Text == strings.TrimSpace(in.Text)
		observedMatches := strings.TrimSpace(in.ObservedAt) == "" || prior.EffectiveObservedAt().Equal(observed.UTC())
		if parseErr != nil || prior.SourceSystem != strings.TrimSpace(in.SourceSystem) || prior.LogicalSessionID() != strings.TrimSpace(in.SessionID) || !roleMatches || !contentMatches || !observedMatches || prior.Provenance.CaptureMethod != "mcp_local" || !slices.Contains(prior.ScopeHints, "project:"+project.Slug) {
			return CaptureOutput{}, fmt.Errorf("%w: %s", ledger.ErrEvidenceConflict, evidenceID)
		}
		receipt, ok, receiptErr := b.App.Control.Receipt(ctx, evidenceID)
		if receiptErr != nil || !ok {
			return CaptureOutput{}, errors.New("existing Evidence receipt is missing")
		}
		if err := b.App.Portfolio.LinkRecord(ctx, "evidence", evidenceID, project.ProjectID, true); err != nil {
			return CaptureOutput{}, err
		}
		return CaptureOutput{EvidenceID: evidenceID, SessionID: receipt.SessionID, ProjectID: project.ProjectID, Duplicate: true, LedgerPath: receipt.LedgerRelPath}, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return CaptureOutput{}, readErr
	}
	envelope := contracts.EvidenceEnvelope{SchemaVersion: "0.1", EvidenceID: evidenceID, SourceSystem: strings.TrimSpace(in.SourceSystem), ExternalConversationID: &in.SessionID, Role: &role, ObservedAt: &observed, CapturedAt: time.Now().UTC(), Content: []contracts.ContentBlock{{Type: "text", Text: strings.TrimSpace(in.Text)}}, Provenance: contracts.Provenance{CaptureMethod: "mcp_local"}, ScopeHints: []string{"project:" + project.Slug}, Visibility: "private"}
	raw, _ := json.Marshal(envelope)
	result, err := b.App.Ledger.Append(ctx, raw)
	if err != nil {
		return CaptureOutput{}, err
	}
	if err := b.App.Portfolio.LinkRecord(ctx, "evidence", result.EvidenceID, project.ProjectID, true); err != nil {
		return CaptureOutput{}, err
	}
	grown, err := b.App.Growth.Process(ctx, growth.ProcessInput{
		ProjectID: project.ProjectID, SessionID: result.SessionID, EvidenceIDs: []string{result.EvidenceID}, Primary: true,
		CallerType: "system", CallerID: "mcp-local", Channel: "mcp-capture", IdempotencyKey: "mcp-growth:" + in.IdempotencyKey,
	})
	if err != nil {
		return CaptureOutput{}, err
	}
	return CaptureOutput{EvidenceID: result.EvidenceID, SessionID: result.SessionID, ProjectID: project.ProjectID, Duplicate: result.Duplicate, LedgerPath: result.LedgerPath, Pipeline: grown.Compilation}, nil
}

func (b LocalBackend) WriteProjectRecord(ctx context.Context, in ProjectRecordInput) (ProjectRecordOutput, error) {
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "goal":
		item, err := b.App.Portfolio.CreateGoal(ctx, portfolio.GoalInput{ProjectID: in.ProjectID, Title: in.Title, Description: in.Description, Priority: in.Priority, TargetAt: in.TargetAt, SourceEvidenceID: in.SourceEvidenceID})
		return ProjectRecordOutput{Kind: "goal", Record: item}, err
	case "decision":
		item, err := b.App.Portfolio.CreateDecision(ctx, portfolio.DecisionInput{ProjectID: in.ProjectID, Title: in.Title, Decision: in.Decision, Rationale: in.Rationale, DecidedAt: in.DecidedAt, SourceEvidenceIDs: in.SourceEvidenceIDs})
		return ProjectRecordOutput{Kind: "decision", Record: item}, err
	case "risk":
		item, err := b.App.Portfolio.CreateRisk(ctx, portfolio.RiskInput{ProjectID: in.ProjectID, Title: in.Title, Description: in.Description, Probability: in.Probability, Impact: in.Impact, Mitigation: in.Mitigation, Owner: in.Owner, SourceEvidenceID: in.SourceEvidenceID})
		return ProjectRecordOutput{Kind: "risk", Record: item}, err
	default:
		return ProjectRecordOutput{}, errors.New("kind must be goal, decision or risk")
	}
}

func (b LocalBackend) WriteFinanceEntry(ctx context.Context, in FinanceWriteInput) (FinanceWriteOutput, error) {
	item, duplicate, err := b.App.Portfolio.CreateFinanceEntry(ctx, portfolio.FinanceEntryInput{ProjectID: in.ProjectID, AccountID: in.AccountID, EntryType: in.EntryType, Category: in.Category, Description: in.Description, AmountMinor: in.AmountMinor, Currency: in.Currency, OccurredAt: in.OccurredAt, SourceEvidenceID: in.SourceEvidenceID, IdempotencyKey: in.IdempotencyKey})
	return FinanceWriteOutput{Entry: item, Duplicate: duplicate}, err
}

func (b LocalBackend) ListMemoryTypes(ctx context.Context) ([]harness.MemoryType, error) {
	return b.App.Harness.ListTypes(ctx)
}
func (b LocalBackend) ListObjects(ctx context.Context, in ObjectListInput) ([]harness.Object, error) {
	return b.App.Harness.ListObjects(ctx, in.ProjectID, in.TypeID, in.Status, in.Limit)
}
func (b LocalBackend) ReadObject(ctx context.Context, id string) (harness.Object, error) {
	return b.App.Harness.Object(ctx, id)
}
func (b LocalBackend) ProposeAssetRevision(ctx context.Context, in AssetRevisionProposalInput) (harness.RevisionReview, error) {
	object, err := b.App.Harness.Object(ctx, strings.TrimSpace(in.ObjectID))
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.TypeID != harness.GovernedAgentAssetTypeV3 && object.TypeID != harness.GovernedAgentAssetTypeV4 {
		return harness.RevisionReview{}, errors.New("revision proposals are limited to governed Agent Asset v4 or v3")
	}
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	return b.App.Harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{
		Payload: payload, ExpectedRevision: in.ExpectedRevision, EditReason: in.EditReason, TargetStatus: "active",
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs,
		PluginID: object.Revision.PluginID, PluginVersion: object.Revision.PluginVersion, IdempotencyKey: in.IdempotencyKey,
		RequestedBy: "mcp-proposal", Validation: json.RawMessage(`{"status":"not_run"}`),
	})
}
func (b LocalBackend) ListRuns(ctx context.Context, in RunListInput) ([]harness.Run, error) {
	return b.App.Harness.ListRuns(ctx, in.ProjectID, in.Status, in.Limit)
}
func (b LocalBackend) ReadRun(ctx context.Context, id string) (harness.RunDetail, error) {
	return b.App.Harness.RunDetail(ctx, id)
}
func (b LocalBackend) ProjectBlueprint(ctx context.Context, projectID string) (blueprint.Current, error) {
	return b.App.Blueprints.Current(ctx, projectID)
}

type HTTPBackend struct {
	BaseURL string
	Client  *http.Client
	Token   string
}

func NewHTTPBackend(endpoint string) (HTTPBackend, error) {
	return newHTTPBackend(endpoint, "")
}

func NewAgentHTTPBackend(endpoint, token string) (HTTPBackend, error) {
	if strings.TrimSpace(token) == "" {
		return HTTPBackend{}, errors.New("MEMORYOS_AGENT_TOKEN is required for MCP")
	}
	return newHTTPBackend(endpoint, token)
}

func newHTTPBackend(endpoint, token string) (HTTPBackend, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	u, err := url.Parse(endpoint)
	if err != nil {
		return HTTPBackend{}, err
	}
	if u.Scheme != "http" {
		return HTTPBackend{}, errors.New("M0 MCP endpoint must use http on loopback")
	}
	host := u.Hostname()
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return HTTPBackend{}, errors.New("M0 MCP endpoint must be loopback")
		}
	}
	// A model-backed capture can legitimately exceed the previous 30-second
	// transport limit. Keep a finite outer bound while allowing the server's
	// governed pipeline to return its durable receipt. The MCP call context can
	// still cancel earlier when the client no longer needs the result.
	return HTTPBackend{BaseURL: endpoint, Client: &http.Client{Timeout: 5 * time.Minute}, Token: strings.TrimSpace(token)}, nil
}

func (b HTTPBackend) agentPath(localPath, agentPath string) string {
	if b.Token != "" {
		return agentPath
	}
	return localPath
}

func (b HTTPBackend) authorize(req *http.Request) {
	if b.Token != "" {
		req.Header.Set("Authorization", "Bearer "+b.Token)
	}
}

func (b HTTPBackend) ValidateAgent(ctx context.Context) error {
	if b.Token == "" {
		return errors.New("Agent token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.BaseURL+"/v1/agent/me", nil)
	if err != nil {
		return err
	}
	b.authorize(req)
	resp, err := b.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MemoryOS Agent authentication failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (b HTTPBackend) Search(ctx context.Context, in SearchInput) (search.Result, error) {
	var payload any = search.Query{Text: in.Query, Limit: in.Limit, SourceSystem: in.SourceSystem, SessionID: in.SessionID, Scope: in.Scope, DateFrom: in.DateFrom, DateTo: in.DateTo, NeighborTurns: in.NeighborTurns, SessionFusion: true, ExcludeSessionIDs: in.ExcludeSessionIDs}
	path := "/v1/search"
	if b.Token != "" {
		payload = in
		path = "/v1/agent/search"
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return search.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	b.authorize(req)
	resp, err := b.Client.Do(req)
	if err != nil {
		return search.Result{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return search.Result{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return search.Result{}, fmt.Errorf("MemoryOS HTTP search: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out search.Result
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}
func (b HTTPBackend) ReadEvidence(ctx context.Context, id string) ([]byte, error) {
	path := "/v1/evidence?id=" + url.QueryEscape(id)
	if b.Token != "" {
		path = "/v1/agent/evidence/" + url.PathEscape(id)
	}
	u := b.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	b.authorize(req)
	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MemoryOS HTTP read: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return bytes.TrimSpace(body), nil
}

func (b HTTPBackend) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.BaseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	b.authorize(req)
	resp, err := b.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MemoryOS HTTP: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, output)
}

func (b HTTPBackend) Recall(ctx context.Context, q unifiedsearch.Query) (unifiedsearch.Result, error) {
	var out unifiedsearch.Result
	err := b.doJSON(ctx, http.MethodPost, b.agentPath("/v1/search/unified", "/v1/agent/recall"), q, &out)
	return out, err
}
func (b HTTPBackend) ListProjects(ctx context.Context) ([]portfolio.ProjectSummary, error) {
	var out struct {
		Projects []portfolio.ProjectSummary `json:"projects"`
	}
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/projects", "/v1/agent/projects"), nil, &out)
	return out.Projects, err
}
func (b HTTPBackend) ProjectContext(ctx context.Context, projectID, asOf string) (ProjectContext, error) {
	if b.Token != "" {
		path := "/v1/agent/projects/" + url.PathEscape(projectID) + "/context"
		if asOf != "" {
			path += "?as_of=" + url.QueryEscape(asOf)
		}
		var out ProjectContext
		err := b.doJSON(ctx, http.MethodGet, path, nil, &out)
		return out, err
	}
	var base struct {
		Summary       portfolio.ProjectSummary `json:"summary"`
		ContextBlocks []portfolio.ContextBlock `json:"context_blocks"`
		Goals         []portfolio.Goal         `json:"goals"`
		Milestones    []portfolio.Milestone    `json:"milestones"`
		Decisions     []portfolio.Decision     `json:"decisions"`
		Risks         []portfolio.Risk         `json:"risks"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(projectID), nil, &base); err != nil {
		return ProjectContext{}, err
	}
	query := url.Values{"project_id": []string{projectID}, "limit": []string{"500"}}
	if asOf != "" {
		query.Set("as_of", asOf)
	}
	var facts struct {
		Facts []portfolio.TemporalFact `json:"facts"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v1/facts?"+query.Encode(), nil, &facts); err != nil {
		return ProjectContext{}, err
	}
	var finance struct {
		Entries []portfolio.FinanceEntry `json:"entries"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v1/finance/entries?project_id="+url.QueryEscape(projectID)+"&limit=100", nil, &finance); err != nil {
		return ProjectContext{}, err
	}
	timeline, err := b.Timeline(ctx, TimelineInput{ProjectID: projectID, AnchorAt: asOf, Limit: 100})
	if err != nil {
		return ProjectContext{}, err
	}
	return ProjectContext{Summary: base.Summary, ContextBlocks: base.ContextBlocks, Facts: facts.Facts, Goals: base.Goals, Milestones: base.Milestones, Decisions: base.Decisions, Risks: base.Risks, Finance: finance.Entries, Timeline: timeline}, nil
}

func (b HTTPBackend) Timeline(ctx context.Context, input TimelineInput) (portfolio.Timeline, error) {
	query := url.Values{"project_id": []string{input.ProjectID}}
	if input.AnchorAt != "" {
		query.Set("anchor_at", input.AnchorAt)
	}
	if input.From != "" {
		query.Set("from", input.From)
	}
	if input.Until != "" {
		query.Set("until", input.Until)
	}
	if len(input.Kinds) > 0 {
		query.Set("kinds", strings.Join(input.Kinds, ","))
	}
	if input.Limit > 0 {
		query.Set("limit", fmt.Sprint(input.Limit))
	}
	var out portfolio.Timeline
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/timeline", "/v1/agent/timeline")+"?"+query.Encode(), nil, &out)
	return out, err
}

func (b HTTPBackend) Capture(ctx context.Context, input CaptureInput) (CaptureOutput, error) {
	if b.Token == "" {
		return CaptureOutput{}, errors.New("memory_capture requires an authenticated Agent backend")
	}
	var out CaptureOutput
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/capture", input, &out)
	return out, err
}

func (b HTTPBackend) WriteProjectRecord(ctx context.Context, input ProjectRecordInput) (ProjectRecordOutput, error) {
	if b.Token == "" {
		return ProjectRecordOutput{}, errors.New("memory_project_write requires an authenticated Agent backend")
	}
	var out ProjectRecordOutput
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/project-records", input, &out)
	return out, err
}

func (b HTTPBackend) WriteFinanceEntry(ctx context.Context, input FinanceWriteInput) (FinanceWriteOutput, error) {
	if b.Token == "" {
		return FinanceWriteOutput{}, errors.New("memory_finance_write requires an authenticated Agent backend")
	}
	var out FinanceWriteOutput
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/finance-entries", input, &out)
	return out, err
}

func (b HTTPBackend) ListMemoryTypes(ctx context.Context) ([]harness.MemoryType, error) {
	var out struct {
		Types []harness.MemoryType `json:"types"`
	}
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/harness/types", "/v1/agent/memory-types"), nil, &out)
	return out.Types, err
}

func (b HTTPBackend) ListObjects(ctx context.Context, input ObjectListInput) ([]harness.Object, error) {
	query := url.Values{"project_id": []string{input.ProjectID}, "type_id": []string{input.TypeID}, "status": []string{input.Status}}
	if input.Limit > 0 {
		query.Set("limit", fmt.Sprint(input.Limit))
	}
	var out struct {
		Objects []harness.Object `json:"objects"`
	}
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/harness/objects", "/v1/agent/objects")+"?"+query.Encode(), nil, &out)
	return out.Objects, err
}

func (b HTTPBackend) ReadObject(ctx context.Context, id string) (harness.Object, error) {
	var out harness.Object
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/harness/objects/", "/v1/agent/objects/")+url.PathEscape(id), nil, &out)
	return out, err
}

func (b HTTPBackend) ProposeAssetRevision(ctx context.Context, in AssetRevisionProposalInput) (harness.RevisionReview, error) {
	if b.Token == "" {
		return harness.RevisionReview{}, errors.New("memory_propose_asset_revision requires an authenticated Agent backend")
	}
	var out harness.RevisionReview
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/objects/"+url.PathEscape(in.ObjectID)+"/revision-proposals", map[string]any{
		"expected_revision": in.ExpectedRevision, "edit_reason": in.EditReason, "payload": in.Payload, "idempotency_key": in.IdempotencyKey,
	}, &out)
	return out, err
}

func (b HTTPBackend) ListRuns(ctx context.Context, input RunListInput) ([]harness.Run, error) {
	query := url.Values{"project_id": []string{input.ProjectID}, "status": []string{input.Status}}
	if input.Limit > 0 {
		query.Set("limit", fmt.Sprint(input.Limit))
	}
	var out struct {
		Runs []harness.Run `json:"runs"`
	}
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/harness/runs", "/v1/agent/runs")+"?"+query.Encode(), nil, &out)
	return out.Runs, err
}

func (b HTTPBackend) ReadRun(ctx context.Context, id string) (harness.RunDetail, error) {
	var out harness.RunDetail
	err := b.doJSON(ctx, http.MethodGet, b.agentPath("/v1/harness/runs/", "/v1/agent/runs/")+url.PathEscape(id), nil, &out)
	return out, err
}

func (b HTTPBackend) ProjectBlueprint(ctx context.Context, projectID string) (blueprint.Current, error) {
	var out blueprint.Current
	path := b.agentPath("/v1/projects/", "/v1/agent/projects/") + url.PathEscape(projectID) + "/blueprint"
	err := b.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (b HTTPBackend) requireAgentTeam(tool string) error {
	if b.Token == "" {
		return fmt.Errorf("%s requires an authenticated Agent backend", tool)
	}
	return nil
}

func (b HTTPBackend) ListTeamTasks(ctx context.Context) ([]harness.Object, error) {
	if err := b.requireAgentTeam("memory_team_list_tasks"); err != nil {
		return nil, err
	}
	var out struct {
		Tasks []harness.Object `json:"tasks"`
	}
	err := b.doJSON(ctx, http.MethodGet, "/v1/agent/team/tasks", nil, &out)
	return out.Tasks, err
}

func (b HTTPBackend) ReadTeamPrivate(ctx context.Context, taskID string) ([]harness.Object, error) {
	if err := b.requireAgentTeam("memory_team_read_private"); err != nil {
		return nil, err
	}
	var out struct {
		Private []harness.Object `json:"private"`
	}
	err := b.doJSON(ctx, http.MethodGet, "/v1/agent/team/tasks/"+url.PathEscape(taskID)+"/private", nil, &out)
	return out.Private, err
}

func (b HTTPBackend) WriteTeamPrivate(ctx context.Context, input TeamPrivateWriteInput) (harness.Object, error) {
	if err := b.requireAgentTeam("memory_team_write_private"); err != nil {
		return harness.Object{}, err
	}
	var out harness.Object
	body := teammemory.ContributionInput{Content: input.Content, RunID: input.RunID, SourceEvidenceIDs: input.SourceEvidenceIDs, Confidence: input.Confidence, EpistemicStatus: input.EpistemicStatus, TTLSeconds: input.TTLSeconds, IdempotencyKey: input.IdempotencyKey}
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/team/tasks/"+url.PathEscape(input.TaskID)+"/private", body, &out)
	return out, err
}

func (b HTTPBackend) ReadTeamShared(ctx context.Context, taskID string) ([]harness.Object, error) {
	if err := b.requireAgentTeam("memory_team_read_shared"); err != nil {
		return nil, err
	}
	var out struct {
		Blackboard []harness.Object `json:"blackboard"`
	}
	err := b.doJSON(ctx, http.MethodGet, "/v1/agent/team/tasks/"+url.PathEscape(taskID)+"/blackboard", nil, &out)
	return out.Blackboard, err
}

func (b HTTPBackend) WriteTeamShared(ctx context.Context, input TeamSharedWriteInput) (teammemory.BlackboardResult, error) {
	if err := b.requireAgentTeam("memory_team_write_shared"); err != nil {
		return teammemory.BlackboardResult{}, err
	}
	body := teammemory.BlackboardInput{
		ContributionInput: teammemory.ContributionInput{Content: input.Content, RunID: input.RunID, SourceEvidenceIDs: input.SourceEvidenceIDs, Confidence: input.Confidence, EpistemicStatus: input.EpistemicStatus, TTLSeconds: input.TTLSeconds, IdempotencyKey: input.IdempotencyKey},
		Topic:             input.Topic, ClaimKey: input.ClaimKey, ClaimValue: input.ClaimValue, DirectShareAgentIDs: input.DirectShareAgentIDs,
	}
	var out teammemory.BlackboardResult
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/team/tasks/"+url.PathEscape(input.TaskID)+"/blackboard", body, &out)
	return out, err
}

func (b HTTPBackend) UpdateTeamShare(ctx context.Context, input TeamShareInput) (harness.Object, error) {
	if err := b.requireAgentTeam("memory_team_update_share"); err != nil {
		return harness.Object{}, err
	}
	var out harness.Object
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/team/blackboard/"+url.PathEscape(input.EntryID)+"/share", teammemory.ShareInput{DirectShareAgentIDs: input.DirectShareAgentIDs, IdempotencyKey: input.IdempotencyKey}, &out)
	return out, err
}

func (b HTTPBackend) ProposeTeamLeave(ctx context.Context, input TeamLeaveInput) (harness.RevisionReview, error) {
	if err := b.requireAgentTeam("memory_team_propose_leave"); err != nil {
		return harness.RevisionReview{}, err
	}
	var out harness.RevisionReview
	err := b.doJSON(ctx, http.MethodPost, "/v1/agent/team/tasks/"+url.PathEscape(input.TaskID)+"/leave-proposal", teammemory.ActivationInput{ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, IdempotencyKey: input.IdempotencyKey}, &out)
	return out, err
}

func New(backend Backend) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "memoryos", Version: buildinfo.Version}, nil)
	teamBackend, hasTeamBackend := backend.(TeamBackend)
	mcp.AddTool(s, &mcp.Tool{Name: "memory_search", Description: "Search raw immutable Evidence inside one project granted to the authenticated Agent. Project scope is mandatory. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, search.Result, error) {
			out, err := backend.Search(ctx, in)
			if err != nil {
				return nil, search.Result{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_read_evidence", Description: "Read one exact immutable Evidence envelope by ID. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
			raw, err := backend.ReadEvidence(ctx, in.EvidenceID)
			if err != nil {
				return nil, ReadOutput{}, err
			}
			var evidence map[string]any
			if err := json.Unmarshal(raw, &evidence); err != nil {
				return nil, ReadOutput{}, fmt.Errorf("decode Evidence envelope: %w", err)
			}
			out := ReadOutput{Evidence: evidence}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_recall", Description: "Recall across MemoryOS evidence, episodes, memories, temporal facts, project decisions, goals, risks and finance. Project scope is mandatory unless all_projects is explicitly true. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RecallInput) (*mcp.CallToolResult, unifiedsearch.Result, error) {
			out, err := backend.Recall(ctx, unifiedsearch.Query{Text: in.Query, ProjectID: in.ProjectID, AllProjects: in.AllProjects, Kinds: in.Kinds, AsOf: in.AsOf, IncludeHistory: in.IncludeHistory, Limit: in.Limit})
			if err != nil {
				return nil, unifiedsearch.Result{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list_projects", Description: "List MemoryOS project spaces with isolated memory and finance metrics. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []portfolio.ProjectSummary, error) {
			out, err := backend.ListProjects(ctx)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_project_context", Description: "Read one project's budgeted context blocks, current or historical facts, goals, milestones, decisions, risks and recent finance entries. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectInput) (*mcp.CallToolResult, ProjectContext, error) {
			out, err := backend.ProjectContext(ctx, in.ProjectID, in.AsOf)
			if err != nil {
				return nil, ProjectContext{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_timeline", Description: "Read one project's temporal context around an explicit or current time anchor. Returns event time, valid time, recorded time, temporal relation and relevance across facts, goals, milestones, decisions, memories, runs, Evidence and permitted finance. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TimelineInput) (*mcp.CallToolResult, portfolio.Timeline, error) {
			out, err := backend.Timeline(ctx, in)
			if err != nil {
				return nil, portfolio.Timeline{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list_active_assets", Description: "List only Owner-governed Agent assets whose current immutable revision is active. Candidate, pending-review, rejected and superseded assets are intentionally excluded. Requires memory.read and project access. This is the safe consumption surface for Agent behavior."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ActiveAssetInput) (*mcp.CallToolResult, GenericItemsOutput, error) {
			limit := in.Limit
			if limit <= 0 {
				limit = 100
			}
			out, err := backend.ListObjects(ctx, ObjectListInput{ProjectID: in.ProjectID, TypeID: harness.GovernedAgentAssetTypeV4, Status: "active", Limit: limit})
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			if remaining := limit - len(out); remaining > 0 {
				legacy, legacyErr := backend.ListObjects(ctx, ObjectListInput{ProjectID: in.ProjectID, TypeID: harness.GovernedAgentAssetTypeV3, Status: "active", Limit: remaining})
				if legacyErr != nil {
					return nil, GenericItemsOutput{}, legacyErr
				}
				out = append(out, legacy...)
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_propose_asset_revision", Description: "Propose a new immutable revision for an existing governed Agent Asset v4 or v3. Requires memory.propose and project access. The server revalidates the complete payload, stale expected_revision values fail closed, current content does not move, and the Agent cannot approve or activate its own proposal."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in AssetRevisionProposalInput) (*mcp.CallToolResult, map[string]any, error) {
			out, err := backend.ProposeAssetRevision(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_project_blueprint", Description: "Read the exact active Memory Blueprint for one granted project, including semantic growth, spatial organization, progressive recall, plugin versions, policy and validation. Requires project.read and is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectInput) (*mcp.CallToolResult, map[string]any, error) {
			out, err := backend.ProjectBlueprint(ctx, in.ProjectID)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_capture", Description: "Append one provenance-bearing Evidence item to an Agent-granted project, then run governed memory distillation. Requires memory.capture and a stable idempotency_key. This cannot approve protected memory, activate assets or delete canonical Evidence."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CaptureInput) (*mcp.CallToolResult, CaptureOutput, error) {
			out, err := backend.Capture(ctx, in)
			if err != nil {
				return nil, CaptureOutput{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_project_write", Description: "Create a project goal, decision or risk in an Agent-granted project. Requires project.write. Source Evidence, when supplied, must belong to that same project. This cannot approve protected memory."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectRecordInput) (*mcp.CallToolResult, ProjectRecordOutput, error) {
			out, err := backend.WriteProjectRecord(ctx, in)
			if err != nil {
				return nil, ProjectRecordOutput{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_finance_write", Description: "Append an idempotent project finance entry using signed integer minor units. Requires finance.write and an Agent project grant. Currency totals remain separate."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in FinanceWriteInput) (*mcp.CallToolResult, FinanceWriteOutput, error) {
			out, err := backend.WriteFinanceEntry(ctx, in)
			if err != nil {
				return nil, FinanceWriteOutput{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list_types", Description: "List the versioned generic memory types contributed by built-in and installed plugins. Requires memory.read. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, GenericItemsOutput, error) {
			out, err := backend.ListMemoryTypes(ctx)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list_objects", Description: "List versioned generic memory objects inside one granted project, optionally filtered by type and lifecycle state. Requires memory.read. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ObjectListInput) (*mcp.CallToolResult, GenericItemsOutput, error) {
			out, err := backend.ListObjects(ctx, in)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_read_object", Description: "Read one generic memory object with its current revision, provenance, producing run and content hash. Requires memory.read and project access. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ObjectReadInput) (*mcp.CallToolResult, map[string]any, error) {
			out, err := backend.ReadObject(ctx, in.ObjectID)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_list_runs", Description: "List visible Memory Harness runs in one granted project. Requires trace.read. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RunListInput) (*mcp.CallToolResult, GenericItemsOutput, error) {
			out, err := backend.ListRuns(ctx, in)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_read_run", Description: "Read a complete run trace including stages, events and redacted external-effect receipts. Requires trace.read and project access. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RunReadInput) (*mcp.CallToolResult, map[string]any, error) {
			out, err := backend.ReadRun(ctx, in.RunID)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_list_tasks", Description: "List active collaboration tasks that directly include the authenticated Agent. Memory Harness does not launch Agents; it only enforces membership, project scope and expiry. This is read-only."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, GenericItemsOutput, error) {
			if !hasTeamBackend {
				return nil, GenericItemsOutput{}, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.ListTeamTasks(ctx)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_read_private", Description: "Read only this Agent's unexpired private notes inside one assigned collaboration task. Other Agents and the normal Owner page cannot read the private body. Requires team.private."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamTaskInput) (*mcp.CallToolResult, GenericItemsOutput, error) {
			if !hasTeamBackend {
				return nil, GenericItemsOutput{}, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.ReadTeamPrivate(ctx, in.TaskID)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_write_private", Description: "Write an expiring private working note visible only to this Agent inside an assigned task. Requires team.private and a stable idempotency key."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamPrivateWriteInput) (*mcp.CallToolResult, map[string]any, error) {
			if !hasTeamBackend {
				return nil, nil, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.WriteTeamPrivate(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_read_shared", Description: "Read unexpired collaboration entries authored by this Agent or directly shared to it. Sharing is direct and cannot be forwarded. Requires team.blackboard.read."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamTaskInput) (*mcp.CallToolResult, GenericItemsOutput, error) {
			if !hasTeamBackend {
				return nil, GenericItemsOutput{}, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.ReadTeamShared(ctx, in.TaskID)
			if err != nil {
				return nil, GenericItemsOutput{}, err
			}
			generic, err := genericItems(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_write_shared", Description: "Submit an expiring task contribution with explicit direct recipients. Conflicting claim values create Owner review instead of overwriting each other. Requires team.blackboard.write; non-empty recipients additionally require team.blackboard.share."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamSharedWriteInput) (*mcp.CallToolResult, teammemory.BlackboardResult, error) {
			if !hasTeamBackend {
				return nil, teammemory.BlackboardResult{}, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.WriteTeamShared(ctx, in)
			if err != nil {
				return nil, teammemory.BlackboardResult{}, err
			}
			return nil, out, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_update_share", Description: "Replace the direct-recipient list of a shared entry originally authored by this Agent. Recipients cannot forward another Agent's entry. Requires team.blackboard.share."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamShareInput) (*mcp.CallToolResult, map[string]any, error) {
			if !hasTeamBackend {
				return nil, nil, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.UpdateTeamShare(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_team_propose_leave", Description: "Ask the Owner to remove this Agent from an active collaboration task. The request creates a review and does not change membership immediately."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in TeamLeaveInput) (*mcp.CallToolResult, map[string]any, error) {
			if !hasTeamBackend {
				return nil, nil, errors.New("team tools require an authenticated Agent backend")
			}
			out, err := teamBackend.ProposeTeamLeave(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			generic, err := genericMap(out)
			return nil, generic, err
		})
	return s
}
func Run(ctx context.Context, b Backend) error { return New(b).Run(ctx, &mcp.StdioTransport{}) }
