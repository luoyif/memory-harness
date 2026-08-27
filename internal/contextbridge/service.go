package contextbridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/profile"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

const (
	ContextRunPipelineID      = "builtin.context-bridge.exchange"
	ContextRunPipelineVersion = "1.0.0"
	OutcomeRunPipelineID      = "builtin.context-bridge.outcome"
	OutcomeRunPipelineVersion = "1.0.0"
)

type Service struct {
	harness    *harness.Service
	recall     *unifiedsearch.Engine
	ledger     *ledger.Ledger
	blueprints *blueprint.Service
	profiles   *profile.Service
}

type NegotiationResult struct {
	CapabilitySet     ContextCapabilitySet `json:"capability_set"`
	CapabilitySetHash string               `json:"capability_set_hash"`
	Level             string               `json:"level"`
}

type ExchangeDetail struct {
	Run            harness.Run       `json:"run"`
	Plan           *ContextPlan      `json:"plan,omitempty"`
	Receipt        *ContextReceipt   `json:"receipt,omitempty"`
	DeliveryStatus map[string]string `json:"delivery_status"`
}

func New(h *harness.Service, recall *unifiedsearch.Engine, ledger *ledger.Ledger, blueprints *blueprint.Service, profiles *profile.Service) *Service {
	return &Service{harness: h, recall: recall, ledger: ledger, blueprints: blueprints, profiles: profiles}
}

func containsCapability(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func capabilityLevel(values []string) string {
	level := 0
	if containsCapability(values, "recall") {
		level = 1
	}
	if containsCapability(values, "capture") {
		level = 2
	}
	if containsCapability(values, "pre_turn_injection") {
		level = 3
	}
	if containsCapability(values, "thread_lifecycle") || containsCapability(values, "item_lifecycle") || containsCapability(values, "compaction_hook") {
		level = 4
	}
	if containsCapability(values, "outcome_feedback") {
		level = 5
	}
	return fmt.Sprintf("L%d", level)
}

func (s *Service) Negotiate(value ContextCapabilitySet) (NegotiationResult, error) {
	if err := ValidateCapabilitySet(value); err != nil {
		return NegotiationResult{}, err
	}
	value.Capabilities = append([]string(nil), value.Capabilities...)
	sort.Strings(value.Capabilities)
	hash, err := HashCapabilitySet(value)
	if err != nil {
		return NegotiationResult{}, err
	}
	return NegotiationResult{CapabilitySet: value, CapabilitySetHash: hash, Level: capabilityLevel(value.Capabilities)}, nil
}

func normalizedBudget(value ContextBudget) ContextBudget {
	if value.MaxTokens == 0 {
		value.MaxTokens = 4096
	}
	if value.MaxChars == 0 {
		value.MaxChars = 16000
	}
	if value.MaxLatencyMS == 0 {
		value.MaxLatencyMS = 3000
	}
	return value
}

func contextPipelineHash(id, version string) string {
	return contracts.HashBytes([]byte(id + "\x00" + version))
}

func stableContextID(prefix string, parts ...string) string {
	return prefix + contracts.HashBytes([]byte(strings.Join(parts, "\x00")))[:24]
}

func allowedPlanKinds(values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{"object", "memory", "evidence"}, nil
	}
	allowed := map[string]bool{"object": true, "memory": true, "evidence": true}
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !allowed[value] {
			return nil, fmt.Errorf("context plan v1 only supports object, memory and evidence; got %q", value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out, nil
}

type compiledContextPolicy struct {
	Enabled              bool     `json:"enabled"`
	ProfileEnabled       bool     `json:"profile_enabled"`
	ProfileViews         []string `json:"profile_views,omitempty"`
	MaxProfiles          int      `json:"max_profiles,omitempty"`
	ProfileMaxChars      int      `json:"profile_max_chars,omitempty"`
	ProfileMaxTokens     int      `json:"profile_max_tokens,omitempty"`
	RetrievalKinds       []string `json:"retrieval_kinds,omitempty"`
	MaxCandidates        int      `json:"max_candidates,omitempty"`
	CandidateMultiplier  int      `json:"candidate_multiplier,omitempty"`
	ProfilePresentation  string   `json:"profile_presentation,omitempty"`
	ObjectPresentation   string   `json:"object_presentation,omitempty"`
	EvidencePresentation string   `json:"evidence_presentation,omitempty"`
}

func defaultCompiledContextPolicy() compiledContextPolicy {
	return compiledContextPolicy{
		MaxProfiles: 2, ProfileMaxChars: 7000, ProfileMaxTokens: 1600,
		RetrievalKinds: []string{"object", "memory", "evidence"}, MaxCandidates: 40, CandidateMultiplier: 4,
		ProfilePresentation: "profile", ObjectPresentation: "summary", EvidencePresentation: "verbatim",
	}
}

func decodeContextNodeConfig(node blueprint.NodeBinding, target any) error {
	if len(node.Config) == 0 {
		return nil
	}
	if err := json.Unmarshal(node.Config, target); err != nil {
		return fmt.Errorf("context component %s config: %w", node.ComponentID, err)
	}
	return nil
}

func allowedPresentation(value string) bool {
	switch value {
	case "summary", "verbatim", "profile", "skill_index", "object":
		return true
	default:
		return false
	}
}

func compileContextPolicy(definition blueprint.Definition) (compiledContextPolicy, error) {
	policy := defaultCompiledContextPolicy()
	allowedViews := map[string]bool{
		profile.ViewAgentIdentity: true, profile.ViewStablePreference: true, profile.ViewDynamicProject: true,
		profile.ViewRelationship: true, profile.ViewSessionResume: true,
	}
	for _, track := range definition.Tracks {
		if track.Role != "context" {
			continue
		}
		policy.Enabled = true
		for _, node := range track.Nodes {
			if !node.Enabled {
				continue
			}
			switch node.Role {
			case "context.profile-compiler":
				var cfg struct {
					Views       []string `json:"views"`
					MaxProfiles int      `json:"max_profiles"`
					MaxChars    int      `json:"max_chars"`
				}
				if err := decodeContextNodeConfig(node, &cfg); err != nil {
					return policy, err
				}
				policy.ProfileEnabled = true
				if len(cfg.Views) > 0 {
					policy.ProfileViews = append([]string(nil), cfg.Views...)
				} else {
					policy.ProfileViews = []string{profile.ViewDynamicProject, profile.ViewSessionResume}
				}
				if cfg.MaxProfiles > 0 {
					policy.MaxProfiles = cfg.MaxProfiles
				}
				if cfg.MaxChars > 0 {
					policy.ProfileMaxChars = cfg.MaxChars
				}
			case "context.retrieval-policy":
				var cfg struct {
					Kinds         []string `json:"kinds"`
					MaxCandidates int      `json:"max_candidates"`
				}
				if err := decodeContextNodeConfig(node, &cfg); err != nil {
					return policy, err
				}
				if len(cfg.Kinds) > 0 {
					policy.RetrievalKinds = append([]string(nil), cfg.Kinds...)
				}
				if cfg.MaxCandidates > 0 {
					policy.MaxCandidates = cfg.MaxCandidates
				}
			case "context.presentation-policy":
				var cfg struct{ Profile, Object, Evidence string }
				if err := decodeContextNodeConfig(node, &cfg); err != nil {
					return policy, err
				}
				if cfg.Profile != "" {
					policy.ProfilePresentation = cfg.Profile
				}
				if cfg.Object != "" {
					policy.ObjectPresentation = cfg.Object
				}
				if cfg.Evidence != "" {
					policy.EvidencePresentation = cfg.Evidence
				}
			case "context.budget-policy":
				var cfg struct {
					ProfileMaxTokens    int `json:"profile_max_tokens"`
					ProfileMaxChars     int `json:"profile_max_chars"`
					CandidateMultiplier int `json:"candidate_multiplier"`
				}
				if err := decodeContextNodeConfig(node, &cfg); err != nil {
					return policy, err
				}
				if cfg.ProfileMaxTokens > 0 {
					policy.ProfileMaxTokens = cfg.ProfileMaxTokens
				}
				if cfg.ProfileMaxChars > 0 {
					policy.ProfileMaxChars = cfg.ProfileMaxChars
				}
				if cfg.CandidateMultiplier > 0 {
					policy.CandidateMultiplier = cfg.CandidateMultiplier
				}
			}
		}
	}
	if !policy.Enabled {
		return policy, nil
	}
	if policy.MaxProfiles < 1 || policy.MaxProfiles > 8 || policy.ProfileMaxChars < 256 || policy.ProfileMaxChars > 100000 || policy.ProfileMaxTokens < 64 || policy.ProfileMaxTokens > 50000 {
		return policy, errors.New("context profile policy exceeds safe limits")
	}
	for _, view := range policy.ProfileViews {
		if !allowedViews[view] {
			return policy, fmt.Errorf("context profile view %q is not Agent-safe", view)
		}
	}
	kinds, err := allowedPlanKinds(policy.RetrievalKinds)
	if err != nil {
		return policy, err
	}
	policy.RetrievalKinds = kinds
	if policy.MaxCandidates < 1 || policy.MaxCandidates > 100 || policy.CandidateMultiplier < 1 || policy.CandidateMultiplier > 20 {
		return policy, errors.New("context retrieval policy exceeds safe limits")
	}
	if !allowedPresentation(policy.ProfilePresentation) || !allowedPresentation(policy.ObjectPresentation) || !allowedPresentation(policy.EvidencePresentation) {
		return policy, errors.New("context presentation policy is invalid")
	}
	return policy, nil
}

func estimateTokens(hit unifiedsearch.Hit) int {
	runes := len([]rune(strings.TrimSpace(hit.Title + "\n" + hit.Snippet)))
	if runes <= 0 {
		return 1
	}
	return max(1, (runes+3)/4)
}

func reasonCodes(hit unifiedsearch.Hit) []string {
	out := []string{"unified_recall", "kind:" + hit.Kind}
	if hit.LexicalRank > 0 {
		out = append(out, "lexical_match")
	}
	if hit.TemporalRelation != "" {
		out = append(out, "temporal:"+hit.TemporalRelation)
	}
	return out
}

func priority(hit unifiedsearch.Hit) int {
	score := math.Max(0, math.Min(1, hit.Score))
	return int(math.Round(score * 100))
}

func (s *Service) planItemFromHit(ctx context.Context, planID, projectID string, hit unifiedsearch.Hit) (ContextPlanItem, bool, error) {
	if hit.ProjectID != projectID {
		return ContextPlanItem{}, false, errors.New("recall returned a cross-project hit")
	}
	base := ContextPlanItem{
		ProjectID: projectID, ReasonCodes: reasonCodes(hit), Priority: priority(hit),
		TokenEstimate: estimateTokens(hit), ValidFrom: hit.ValidFrom, ValidUntil: hit.ValidUntil,
	}
	switch hit.Kind {
	case "object":
		object, err := s.harness.Object(ctx, hit.SourceID)
		if err != nil {
			return ContextPlanItem{}, false, err
		}
		if object.ProjectID != projectID || object.Status != "active" {
			return ContextPlanItem{}, false, nil
		}
		base.SourceKind, base.SourceID = "object", object.ObjectID
		base.Revision, base.ContentHash = object.CurrentRevision, object.Revision.ContentHash
		base.Presentation, base.SourceRefs = "summary", append([]string(nil), object.Revision.SourceEvidenceIDs...)
		base.ValidFrom, base.ValidUntil = object.Revision.ValidFrom, object.Revision.ValidUntil
	case "memory":
		authority, _ := hit.Metadata["authority_object_id"].(string)
		if strings.TrimSpace(authority) == "" {
			// A legacy Memory without a governed current revision is still
			// searchable, but FT1 will not pretend it is a verifiable context item.
			return ContextPlanItem{}, false, nil
		}
		object, err := s.harness.Object(ctx, authority)
		if err != nil {
			return ContextPlanItem{}, false, err
		}
		if object.ProjectID != projectID || object.Status != "active" {
			return ContextPlanItem{}, false, nil
		}
		base.SourceKind, base.SourceID = "object", object.ObjectID
		base.Revision, base.ContentHash = object.CurrentRevision, object.Revision.ContentHash
		base.Presentation, base.SourceRefs = "summary", append([]string(nil), object.Revision.SourceEvidenceIDs...)
		base.ValidFrom, base.ValidUntil = object.Revision.ValidFrom, object.Revision.ValidUntil
		base.ReasonCodes = append(base.ReasonCodes, "memory_authority")
	case "evidence":
		raw, err := s.ledger.ReadEvidence(ctx, hit.SourceID)
		if err != nil {
			return ContextPlanItem{}, false, err
		}
		base.SourceKind, base.SourceID = "evidence", hit.SourceID
		base.ContentHash, base.Presentation = contracts.HashBytes(raw), "verbatim"
		base.SourceRefs = []string{hit.SourceID}
	default:
		return ContextPlanItem{}, false, nil
	}
	base.ItemID = stableContextID("ctxitem-", planID, base.SourceKind, base.SourceID, fmt.Sprint(base.Revision), base.ContentHash)
	return base, true, nil
}

func (s *Service) CompilePlan(ctx context.Context, input PlanRequest) (PlanResult, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Query = strings.TrimSpace(input.Query)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID == "" || input.AgentID == "" || input.Query == "" || input.IdempotencyKey == "" {
		return PlanResult{}, errors.New("project_id, agent_id, query and idempotency_key are required")
	}
	negotiated, err := s.Negotiate(input.CapabilitySet)
	if err != nil {
		return PlanResult{}, err
	}
	if !containsCapability(negotiated.CapabilitySet.Capabilities, "recall") {
		return PlanResult{}, errors.New("adapter must declare recall capability before requesting a context plan")
	}
	current, err := s.blueprints.Current(ctx, input.ProjectID)
	if err != nil {
		return PlanResult{}, err
	}
	contextPolicy, err := compileContextPolicy(current.Blueprint.Definition)
	if err != nil {
		return PlanResult{}, err
	}
	requestedKinds := input.Kinds
	if len(requestedKinds) == 0 && contextPolicy.Enabled {
		requestedKinds = contextPolicy.RetrievalKinds
	}
	kinds, err := allowedPlanKinds(requestedKinds)
	if err != nil {
		return PlanResult{}, err
	}
	rawBudget := input.Budget
	input.Budget = normalizedBudget(input.Budget)
	if contextPolicy.Enabled && rawBudget.MaxChars == 0 && current.Blueprint.Definition.Policy.DefaultContextBudget > 0 {
		input.Budget.MaxChars = current.Blueprint.Definition.Policy.DefaultContextBudget
	}
	if input.Budget.MaxTokens < 0 || input.Budget.MaxChars < 0 || input.Budget.MaxLatencyMS < 0 || input.Budget.MaxCostMinor < 0 {
		return PlanResult{}, errors.New("context budget values must be non-negative")
	}
	fingerprint := contracts.HashBytes([]byte(input.Query))
	planID := stableContextID("plan-", input.ProjectID, input.AgentID, input.IdempotencyKey)
	now := time.Now().UTC()
	snapshot, _ := json.Marshal(map[string]any{
		"schema_version": PlanSchemaVersion, "request_fingerprint": fingerprint,
		"capability_set": negotiated.CapabilitySet, "capability_set_hash": negotiated.CapabilitySetHash,
		"capability_level": negotiated.Level, "budget": input.Budget, "correlation": input.Correlation,
		"context_policy": contextPolicy,
		"blueprint_id":   current.Assignment.BlueprintID, "blueprint_version": current.Assignment.BlueprintVersion, "blueprint_hash": current.Assignment.BlueprintHash,
	})
	run, err := s.harness.StartRun(ctx, harness.StartRunInput{
		ProjectID: input.ProjectID, CallerType: "agent", CallerID: input.AgentID, Channel: "context-bridge",
		PipelineID: ContextRunPipelineID, PipelineVersion: ContextRunPipelineVersion,
		PipelineHash:   contextPipelineHash(ContextRunPipelineID, ContextRunPipelineVersion),
		IdempotencyKey: "context-plan:" + input.IdempotencyKey, Snapshot: snapshot,
	})
	if err != nil {
		return PlanResult{}, err
	}
	if existing, getErr := s.harness.StageOutput(ctx, run.RunID, "context.plan"); getErr == nil {
		var plan ContextPlan
		if err := json.Unmarshal(existing.Payload, &plan); err != nil {
			return PlanResult{}, err
		}
		if plan.RequestFingerprint != fingerprint || plan.ProjectID != input.ProjectID || plan.AgentID != input.AgentID {
			return PlanResult{}, errors.New("context plan idempotency key was reused for a different request")
		}
		return PlanResult{RunID: run.RunID, Plan: plan, DeliveryStatus: EffectiveDeliveryStatus(plan, nil), Duplicate: true}, nil
	} else if getErr != sql.ErrNoRows {
		return PlanResult{}, getErr
	}
	if run.Status == "queued" {
		if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.started", harness.CorePluginID, map[string]any{"purpose": "context_exchange"}); err != nil {
			return PlanResult{}, err
		}
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "context.capabilities.negotiated", harness.CorePluginID, map[string]any{
		"capability_set_id": negotiated.CapabilitySet.CapabilitySetID, "capability_set_hash": negotiated.CapabilitySetHash,
		"adapter_id": negotiated.CapabilitySet.AdapterID, "runtime": negotiated.CapabilitySet.Runtime, "level": negotiated.Level,
	}); err != nil {
		return PlanResult{}, err
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "context.policy.compiled", harness.CorePluginID, map[string]any{
		"context_track": contextPolicy.Enabled, "profile_views": contextPolicy.ProfileViews,
		"retrieval_kinds": kinds, "max_candidates": contextPolicy.MaxCandidates,
	}); err != nil {
		return PlanResult{}, err
	}
	limit := negotiated.CapabilitySet.MaxContextItems * contextPolicy.CandidateMultiplier
	if limit < 20 {
		limit = 20
	}
	if contextPolicy.Enabled && limit > contextPolicy.MaxCandidates {
		limit = contextPolicy.MaxCandidates
	}
	if limit > 100 {
		limit = 100
	}
	recalled, err := s.recall.Search(ctx, unifiedsearch.Query{Text: input.Query, ProjectID: input.ProjectID, Kinds: kinds, Limit: limit})
	if err != nil {
		return PlanResult{}, err
	}
	profilePolicy := contextPolicy
	if profilePolicy.MaxProfiles > negotiated.CapabilitySet.MaxContextItems {
		profilePolicy.MaxProfiles = negotiated.CapabilitySet.MaxContextItems
	}
	if input.Budget.MaxTokens > 0 && profilePolicy.ProfileMaxTokens > input.Budget.MaxTokens {
		profilePolicy.ProfileMaxTokens = input.Budget.MaxTokens
	}
	if input.Budget.MaxChars > 0 && profilePolicy.ProfileMaxChars > input.Budget.MaxChars {
		profilePolicy.ProfileMaxChars = input.Budget.MaxChars
	}
	profileItems, profileTokens, profileChars, err := s.profilePlanItems(ctx, planID, input.ProjectID, profilePolicy)
	if err != nil {
		return PlanResult{}, err
	}
	items := []ContextPlanItem{}
	seen := map[string]bool{}
	for _, item := range profileItems {
		if len(items) >= negotiated.CapabilitySet.MaxContextItems || len(items) >= MaxPlanItems {
			break
		}
		seen[item.ItemID] = true
		items = append(items, item)
	}
	usedTokens, usedChars := profileTokens, profileChars
	for _, hit := range recalled.Hits {
		item, ok, err := s.planItemFromHit(ctx, planID, input.ProjectID, hit)
		if err != nil {
			return PlanResult{}, err
		}
		if !ok || seen[item.ItemID] {
			continue
		}
		if contextPolicy.Enabled {
			if hit.Kind == "evidence" {
				item.Presentation = contextPolicy.EvidencePresentation
			} else {
				item.Presentation = contextPolicy.ObjectPresentation
			}
		}
		chars := len([]rune(hit.Title + hit.Snippet))
		if input.Budget.MaxTokens > 0 && usedTokens+item.TokenEstimate > input.Budget.MaxTokens {
			continue
		}
		if input.Budget.MaxChars > 0 && usedChars+chars > input.Budget.MaxChars {
			continue
		}
		seen[item.ItemID] = true
		items = append(items, item)
		usedTokens += item.TokenEstimate
		usedChars += chars
		if len(items) >= negotiated.CapabilitySet.MaxContextItems || len(items) >= MaxPlanItems {
			break
		}
	}
	plan := ContextPlan{
		SchemaVersion: PlanSchemaVersion, PlanID: planID, ProjectID: input.ProjectID, AgentID: input.AgentID,
		RequestFingerprint: fingerprint, BlueprintID: current.Assignment.BlueprintID, BlueprintVersion: current.Assignment.BlueprintVersion,
		BlueprintHash: current.Assignment.BlueprintHash, Budget: input.Budget, Items: items, Correlation: input.Correlation,
		IdempotencyKey: input.IdempotencyKey, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano),
	}
	if err := ValidatePlan(plan); err != nil {
		return PlanResult{}, err
	}
	plan.PlanHash, err = HashPlan(plan)
	if err != nil {
		return PlanResult{}, err
	}
	raw, _ := json.Marshal(plan)
	if _, err := s.harness.RecordStageOutput(ctx, run.RunID, "context.plan", raw); err != nil {
		return PlanResult{}, err
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "context.plan.created", harness.CorePluginID, map[string]any{
		"plan_id": plan.PlanID, "plan_hash": plan.PlanHash, "items": len(plan.Items), "candidate_count": recalled.CandidateCount,
		"profile_items": len(profileItems), "budget_tokens": plan.Budget.MaxTokens, "estimated_tokens": usedTokens,
	}); err != nil {
		return PlanResult{}, err
	}
	return PlanResult{RunID: run.RunID, Plan: plan, DeliveryStatus: EffectiveDeliveryStatus(plan, nil)}, nil
}

func (s *Service) loadPlan(ctx context.Context, runID string) (harness.Run, ContextPlan, error) {
	run, err := s.harness.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return harness.Run{}, ContextPlan{}, err
	}
	output, err := s.harness.StageOutput(ctx, run.RunID, "context.plan")
	if err != nil {
		return run, ContextPlan{}, err
	}
	var plan ContextPlan
	if err := json.Unmarshal(output.Payload, &plan); err != nil {
		return run, ContextPlan{}, err
	}
	if err := ValidatePlan(plan); err != nil {
		return run, ContextPlan{}, err
	}
	return run, plan, nil
}

func (s *Service) RecordReceipt(ctx context.Context, input ReceiptRequest) (ReceiptResult, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	if input.AgentID == "" || strings.TrimSpace(input.RunID) == "" {
		return ReceiptResult{}, errors.New("agent_id and run_id are required")
	}
	run, plan, err := s.loadPlan(ctx, input.RunID)
	if err != nil {
		return ReceiptResult{}, err
	}
	if run.ProjectID != plan.ProjectID || run.CallerID != input.AgentID || plan.AgentID != input.AgentID {
		return ReceiptResult{}, errors.New("context receipt Agent/project does not match the plan run")
	}
	receipt := input.Receipt
	receipt.ProjectID, receipt.AgentID = plan.ProjectID, input.AgentID
	if receipt.ReceivedAt == "" {
		receipt.ReceivedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := ValidateReceipt(plan, receipt); err != nil {
		return ReceiptResult{}, err
	}
	hash, err := HashReceipt(receipt)
	if err != nil {
		return ReceiptResult{}, err
	}
	if receipt.ReceiptHash != "" && receipt.ReceiptHash != hash {
		return ReceiptResult{}, errors.New("receipt_hash does not match canonical receipt")
	}
	receipt.ReceiptHash = hash
	if existing, getErr := s.harness.StageOutput(ctx, run.RunID, "context.receipt"); getErr == nil {
		var prior ContextReceipt
		if err := json.Unmarshal(existing.Payload, &prior); err != nil {
			return ReceiptResult{}, err
		}
		if prior.ReceiptHash != receipt.ReceiptHash {
			return ReceiptResult{}, errors.New("context receipt is immutable within the exchange run")
		}
		return ReceiptResult{RunID: run.RunID, Receipt: prior, DeliveryStatus: EffectiveDeliveryStatus(plan, &prior), Duplicate: true}, nil
	} else if getErr != sql.ErrNoRows {
		return ReceiptResult{}, getErr
	}
	raw, _ := json.Marshal(receipt)
	if _, err := s.harness.RecordStageOutput(ctx, run.RunID, "context.receipt", raw); err != nil {
		return ReceiptResult{}, err
	}
	status := EffectiveDeliveryStatus(plan, &receipt)
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "context.receipt.recorded", harness.CorePluginID, map[string]any{
		"receipt_id": receipt.ReceiptID, "receipt_hash": receipt.ReceiptHash, "evidence_level": receipt.EvidenceLevel,
		"completeness": receipt.Completeness, "delivery_status": status,
	}); err != nil {
		return ReceiptResult{}, err
	}
	terminal := "run.completed"
	if receipt.Completeness != "complete" {
		terminal = "run.completed_with_warnings"
	} else {
		for _, value := range status {
			if value != "delivered" {
				terminal = "run.completed_with_warnings"
				break
			}
		}
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, terminal, harness.CorePluginID, map[string]any{"context_exchange": "receipt_recorded"}); err != nil {
		return ReceiptResult{}, err
	}
	return ReceiptResult{RunID: run.RunID, Receipt: receipt, DeliveryStatus: status}, nil
}

func (s *Service) RecordOutcome(ctx context.Context, agentID string, value OutcomeFeedback) (OutcomeResult, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return OutcomeResult{}, errors.New("agent_id is required")
	}
	value.AgentID = agentID
	if value.ObservedAt == "" {
		value.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := ValidateOutcome(value); err != nil {
		return OutcomeResult{}, err
	}
	targetRun, err := s.harness.Run(ctx, value.RunID)
	if err != nil {
		return OutcomeResult{}, err
	}
	if targetRun.ProjectID != value.ProjectID || targetRun.CallerID != agentID {
		return OutcomeResult{}, errors.New("outcome Agent/project does not match target run")
	}
	_, plan, err := s.loadPlan(ctx, value.RunID)
	if err != nil {
		return OutcomeResult{}, errors.New("outcome target must be a context exchange run")
	}
	if value.PlanID != "" && value.PlanID != plan.PlanID {
		return OutcomeResult{}, errors.New("outcome plan_id does not match target run")
	}
	if value.ReceiptID != "" {
		receiptOutput, err := s.harness.StageOutput(ctx, value.RunID, "context.receipt")
		if err != nil {
			return OutcomeResult{}, errors.New("outcome references a receipt that is not recorded")
		}
		var receipt ContextReceipt
		if err := json.Unmarshal(receiptOutput.Payload, &receipt); err != nil || receipt.ReceiptID != value.ReceiptID {
			return OutcomeResult{}, errors.New("outcome receipt_id does not match target run")
		}
	}
	hash, err := HashOutcome(value)
	if err != nil {
		return OutcomeResult{}, err
	}
	if value.OutcomeHash != "" && value.OutcomeHash != hash {
		return OutcomeResult{}, errors.New("outcome_hash does not match canonical outcome")
	}
	value.OutcomeHash = hash
	snapshot, _ := json.Marshal(map[string]any{"target_context_run_id": value.RunID, "plan_id": plan.PlanID, "receipt_id": value.ReceiptID, "outcome_hash": hash})
	run, err := s.harness.StartRun(ctx, harness.StartRunInput{
		ProjectID: value.ProjectID, CallerType: "agent", CallerID: agentID, Channel: "context-outcome",
		PipelineID: OutcomeRunPipelineID, PipelineVersion: OutcomeRunPipelineVersion,
		PipelineHash:   contextPipelineHash(OutcomeRunPipelineID, OutcomeRunPipelineVersion),
		IdempotencyKey: "context-outcome:" + value.IdempotencyKey, Snapshot: snapshot,
	})
	if err != nil {
		return OutcomeResult{}, err
	}
	if existing, getErr := s.harness.StageOutput(ctx, run.RunID, "context.outcome"); getErr == nil {
		var prior OutcomeFeedback
		if err := json.Unmarshal(existing.Payload, &prior); err != nil {
			return OutcomeResult{}, err
		}
		if prior.OutcomeHash != value.OutcomeHash {
			return OutcomeResult{}, errors.New("outcome idempotency key was reused for different feedback")
		}
		return OutcomeResult{RunID: run.RunID, Outcome: prior, Duplicate: true}, nil
	} else if getErr != sql.ErrNoRows {
		return OutcomeResult{}, getErr
	}
	if run.Status == "queued" {
		if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.started", harness.CorePluginID, map[string]any{"purpose": "outcome_observation"}); err != nil {
			return OutcomeResult{}, err
		}
	}
	raw, _ := json.Marshal(value)
	if _, err := s.harness.RecordStageOutput(ctx, run.RunID, "context.outcome", raw); err != nil {
		return OutcomeResult{}, err
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "context.outcome.recorded", harness.CorePluginID, map[string]any{
		"outcome_id": value.OutcomeID, "outcome_hash": value.OutcomeHash, "target_context_run_id": value.RunID,
		"metrics": len(value.Metrics), "tokens": value.Cost.Tokens, "latency_ms": value.Cost.LatencyMS, "money_minor": value.Cost.MoneyMinor,
	}); err != nil {
		return OutcomeResult{}, err
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.completed", harness.CorePluginID, map[string]any{"outcome_observation": "recorded_only"}); err != nil {
		return OutcomeResult{}, err
	}
	return OutcomeResult{RunID: run.RunID, Outcome: value}, nil
}

func (s *Service) Exchange(ctx context.Context, runID string) (ExchangeDetail, error) {
	run, err := s.harness.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return ExchangeDetail{}, err
	}
	detail := ExchangeDetail{Run: run, DeliveryStatus: map[string]string{}}
	planOutput, err := s.harness.StageOutput(ctx, run.RunID, "context.plan")
	if err != nil {
		return detail, err
	}
	var plan ContextPlan
	if err := json.Unmarshal(planOutput.Payload, &plan); err != nil {
		return detail, err
	}
	detail.Plan = &plan
	receiptOutput, err := s.harness.StageOutput(ctx, run.RunID, "context.receipt")
	if err == nil {
		var receipt ContextReceipt
		if err := json.Unmarshal(receiptOutput.Payload, &receipt); err != nil {
			return detail, err
		}
		detail.Receipt = &receipt
		detail.DeliveryStatus = EffectiveDeliveryStatus(plan, &receipt)
	} else if err == sql.ErrNoRows {
		detail.DeliveryStatus = EffectiveDeliveryStatus(plan, nil)
	} else {
		return detail, err
	}
	return detail, nil
}
