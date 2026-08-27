package adaptation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control    *store.ControlStore
	harness    *harness.Service
	blueprints *blueprint.Service
	experience *experience.Service
}

func New(control *store.ControlStore, harnessService *harness.Service, blueprintService *blueprint.Service, experienceService *experience.Service) *Service {
	return &Service{control: control, harness: harnessService, blueprints: blueprintService, experience: experienceService}
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(strings.TrimSpace(value)))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func hashDefinition(definition blueprint.Definition) (string, error) {
	raw, err := json.Marshal(definition)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validatePatch(input Patch) (Patch, error) {
	input.Role = strings.TrimSpace(input.Role)
	if len(input.Config) == 0 || !json.Valid(input.Config) {
		return input, errors.New("patch.config must be a JSON object")
	}
	var values map[string]any
	if err := json.Unmarshal(input.Config, &values); err != nil || values == nil {
		return input, errors.New("patch.config must be a JSON object")
	}
	allowedKeys := map[string]bool{}
	switch input.Role {
	case "context.presentation-policy":
		allowedKeys = map[string]bool{"profile": true, "object": true, "evidence": true}
		allowedValue := map[string]bool{"profile": true, "summary": true, "verbatim": true, "index": true}
		for key, raw := range values {
			value, ok := raw.(string)
			if !ok || !allowedValue[strings.TrimSpace(value)] {
				return input, fmt.Errorf("patch.%s has unsupported presentation", key)
			}
		}
	case "context.budget-policy":
		allowedKeys = map[string]bool{"profile_max_tokens": true, "profile_max_chars": true, "candidate_multiplier": true}
		for key, raw := range values {
			value, ok := raw.(float64)
			if !ok || math.Trunc(value) != value {
				return input, fmt.Errorf("patch.%s must be an integer", key)
			}
			iv := int(value)
			switch key {
			case "profile_max_tokens":
				if iv < 64 || iv > 50000 {
					return input, errors.New("profile_max_tokens exceeds safe bounds")
				}
			case "profile_max_chars":
				if iv < 256 || iv > 100000 {
					return input, errors.New("profile_max_chars exceeds safe bounds")
				}
			case "candidate_multiplier":
				if iv < 1 || iv > 16 {
					return input, errors.New("candidate_multiplier exceeds safe bounds")
				}
			}
		}
	case "context.retrieval-policy":
		allowedKeys = map[string]bool{"max_candidates": true}
		value, ok := values["max_candidates"].(float64)
		if !ok || math.Trunc(value) != value || value < 1 || value > 100 {
			return input, errors.New("max_candidates must be an integer between 1 and 100")
		}
	default:
		return input, errors.New("only low-risk context presentation, budget or retrieval settings are overlayable")
	}
	if len(values) == 0 {
		return input, errors.New("patch.config cannot be empty")
	}
	for key := range values {
		if !allowedKeys[key] {
			return input, fmt.Errorf("patch field %q is not overlayable", key)
		}
	}
	canonical, _ := json.Marshal(values)
	input.Config = canonical
	return input, nil
}

func applyPatch(current blueprint.Current, patch Patch) (blueprint.Current, json.RawMessage, json.RawMessage, string, error) {
	raw, _ := json.Marshal(current)
	var effective blueprint.Current
	if err := json.Unmarshal(raw, &effective); err != nil {
		return effective, nil, nil, "", err
	}
	found := false
	var baseConfig, effectiveConfig json.RawMessage
	for ti := range effective.Blueprint.Definition.Tracks {
		track := &effective.Blueprint.Definition.Tracks[ti]
		if track.Role != "context" && track.TrackID != "context" {
			continue
		}
		for ni := range track.Nodes {
			node := &track.Nodes[ni]
			if !node.Enabled || node.Role != patch.Role {
				continue
			}
			found = true
			baseConfig = append(json.RawMessage(nil), node.Config...)
			base := map[string]any{}
			if len(node.Config) > 0 {
				if err := json.Unmarshal(node.Config, &base); err != nil {
					return effective, nil, nil, "", err
				}
			}
			changes := map[string]any{}
			_ = json.Unmarshal(patch.Config, &changes)
			for key, value := range changes {
				base[key] = value
			}
			node.Config, _ = json.Marshal(base)
			effectiveConfig = append(json.RawMessage(nil), node.Config...)
		}
	}
	if !found {
		return effective, nil, nil, "", errors.New("active Blueprint does not expose the requested overlayable context role")
	}
	hash, err := hashDefinition(effective.Blueprint.Definition)
	if err != nil {
		return effective, nil, nil, "", err
	}
	effective.Blueprint.ContentHash = hash
	effective.Assignment.BlueprintHash = hash
	effective.Assignment.Status = "overlay"
	return effective, baseConfig, effectiveConfig, hash, nil
}

func (s *Service) sourceCases(ctx context.Context, projectID string, ids []string) ([]string, []string, []string, error) {
	ids = unique(ids)
	if len(ids) == 0 {
		return nil, nil, nil, errors.New("at least one failed governed Case is required")
	}
	runs, outcomes, objects := []string{}, []string{}, []string{}
	for _, id := range ids {
		object, value, err := s.experience.Case(ctx, id)
		if err != nil {
			return nil, nil, nil, err
		}
		if object.ProjectID != projectID {
			return nil, nil, nil, errors.New("adaptation proposal cannot cross project boundaries")
		}
		if object.Status != "active" || value.Result != "fail" {
			return nil, nil, nil, fmt.Errorf("case %s must be an active failed governed Case", id)
		}
		objects = append(objects, object.ObjectID)
		runs = append(runs, value.SourceRunID)
		outcomes = append(outcomes, value.OutcomeRunIDs...)
	}
	return unique(objects), unique(runs), unique(outcomes), nil
}

func (s *Service) sourcePatterns(ctx context.Context, projectID string, ids []string) ([]string, error) {
	objects := []string{}
	for _, id := range unique(ids) {
		object, _, err := s.experience.Pattern(ctx, id)
		if err != nil {
			return nil, err
		}
		if object.ProjectID != projectID || object.Status != "active" {
			return nil, errors.New("supporting Pattern must be active and project-scoped")
		}
		objects = append(objects, object.ObjectID)
	}
	return unique(objects), nil
}

func (s *Service) compileProposal(ctx context.Context, input ProposalInput) (ChangeProposal, DryRunResult, []string, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ProposerID = strings.TrimSpace(input.ProposerID)
	input.PredictedFix = strings.TrimSpace(input.PredictedFix)
	input.CanaryScope = strings.TrimSpace(input.CanaryScope)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID == "" || input.ProposerID == "" || input.PredictedFix == "" || input.CanaryScope == "" || input.IdempotencyKey == "" {
		return ChangeProposal{}, DryRunResult{}, nil, errors.New("project, proposer, predicted_fix, canary_scope and idempotency_key are required")
	}
	if input.MinimumSample < 1 || input.MinimumSample > 10000 {
		return ChangeProposal{}, DryRunResult{}, nil, errors.New("minimum_sample must be between 1 and 10000")
	}
	if len(unique(input.EvaluationSuite)) == 0 {
		return ChangeProposal{}, DryRunResult{}, nil, errors.New("evaluation_suite is required")
	}
	if input.StopConditions.MaxRegressionRate < 0 || input.StopConditions.MaxRegressionRate > 1 {
		return ChangeProposal{}, DryRunResult{}, nil, errors.New("max_regression_rate must be between 0 and 1")
	}
	if input.OverlayTTLSeconds < 60 || input.OverlayTTLSeconds > 86400 {
		return ChangeProposal{}, DryRunResult{}, nil, errors.New("overlay_ttl_seconds must be between 60 and 86400")
	}
	patch, err := validatePatch(input.Patch)
	if err != nil {
		return ChangeProposal{}, DryRunResult{}, nil, err
	}
	caseIDs, runs, outcomes, err := s.sourceCases(ctx, input.ProjectID, input.SourceCaseIDs)
	if err != nil {
		return ChangeProposal{}, DryRunResult{}, nil, err
	}
	patternIDs, err := s.sourcePatterns(ctx, input.ProjectID, input.SourcePatternIDs)
	if err != nil {
		return ChangeProposal{}, DryRunResult{}, nil, err
	}
	current, err := s.blueprints.Current(ctx, input.ProjectID)
	if err != nil {
		return ChangeProposal{}, DryRunResult{}, nil, err
	}
	_, baseCfg, effectiveCfg, effectiveHash, err := applyPatch(current, patch)
	if err != nil {
		return ChangeProposal{}, DryRunResult{}, nil, err
	}
	proposalID := stableID("change-proposal-", input.ProjectID, current.Assignment.BlueprintHash, patch.Role, string(patch.Config), input.IdempotencyKey)
	proposal := ChangeProposal{
		ProposalID: proposalID, ProjectID: input.ProjectID, SourceCaseIDs: caseIDs, SourcePatternIDs: patternIDs, SourceRunIDs: runs, SourceOutcomeRunIDs: outcomes,
		BaseBlueprintID: current.Blueprint.BlueprintID, BaseBlueprintVersion: current.Blueprint.Version, BaseBlueprintHash: current.Assignment.BlueprintHash,
		Patch: patch, EffectiveBlueprintHash: effectiveHash, PredictedFix: input.PredictedFix, PredictedRegressions: unique(input.PredictedRegressions),
		EvaluationSuite: unique(input.EvaluationSuite), MinimumSample: input.MinimumSample, StopConditions: input.StopConditions, PermissionImpact: []string{},
		PrivacyImpact: strings.TrimSpace(input.PrivacyImpact), CostImpact: strings.TrimSpace(input.CostImpact), ProposerID: input.ProposerID,
		EvaluationObjectIDs: []string{}, CanaryScope: input.CanaryScope, OverlayTTLSeconds: input.OverlayTTLSeconds,
		RollbackBlueprintHash: current.Assignment.BlueprintHash, CreatedAt: nowString(),
	}
	preview := DryRunResult{ProjectID: input.ProjectID, BaseBlueprintHash: current.Assignment.BlueprintHash, EffectiveBlueprintHash: effectiveHash, TargetRole: patch.Role, BaseConfig: baseCfg, EffectiveConfig: effectiveCfg, PermissionDelta: []string{}, NoWritesPerformed: true}
	return proposal, preview, unique(append(caseIDs, patternIDs...)), nil
}

func (s *Service) DryRunProposal(ctx context.Context, input ProposalInput) (DryRunResult, error) {
	_, preview, _, err := s.compileProposal(ctx, input)
	return preview, err
}

func (s *Service) CreateProposal(ctx context.Context, input ProposalInput) (harness.Object, error) {
	proposal, _, sources, err := s.compileProposal(ctx, input)
	if err != nil {
		return harness.Object{}, err
	}
	raw, _ := json.Marshal(proposal)
	return s.harness.Materialize(ctx, harness.MaterializeInput{ObjectID: proposal.ProposalID, TypeID: ChangeProposalTypeV1, ProjectID: proposal.ProjectID, Status: "candidate", Payload: raw,
		Confidence: 1, Importance: .85, SourceObjectIDs: sources, PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "change-proposal:" + input.IdempotencyKey})
}

func decodeProposal(object harness.Object) (ChangeProposal, error) {
	var value ChangeProposal
	if object.TypeID != ChangeProposalTypeV1 {
		return value, errors.New("object is not a Change Proposal")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func decodeOverlay(object harness.Object) (CaseOverlay, error) {
	var value CaseOverlay
	if object.TypeID != CaseOverlayTypeV1 {
		return value, errors.New("object is not a Case Overlay")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func (s *Service) Proposal(ctx context.Context, objectID string) (harness.Object, ChangeProposal, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.Object{}, ChangeProposal{}, err
	}
	value, err := decodeProposal(object)
	return object, value, err
}

func (s *Service) Overlay(ctx context.Context, objectID string) (harness.Object, CaseOverlay, error) {
	object, err := s.harness.Object(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return harness.Object{}, CaseOverlay{}, err
	}
	value, err := decodeOverlay(object)
	return object, value, err
}

func (s *Service) ListProposals(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), ChangeProposalTypeV1, strings.TrimSpace(status), limit)
}

func (s *Service) ListOverlays(ctx context.Context, projectID, status string, limit int) ([]harness.Object, error) {
	return s.harness.ListObjects(ctx, strings.TrimSpace(projectID), CaseOverlayTypeV1, strings.TrimSpace(status), limit)
}

func (s *Service) EvaluateProposal(ctx context.Context, proposalID string, input experience.EvaluateInput) (harness.Object, error) {
	object, proposal, err := s.Proposal(ctx, proposalID)
	if err != nil {
		return harness.Object{}, err
	}
	if object.Status != "candidate" && object.Status != "active" {
		return harness.Object{}, errors.New("only candidate or active Change Proposal can be evaluated")
	}
	input.TargetKind, input.TargetID = "change_proposal", object.ObjectID
	if strings.TrimSpace(input.EvaluatorID) == proposal.ProposerID {
		return harness.Object{}, errors.New("proposal author cannot evaluate the same Change Proposal")
	}
	if input.BaselineRef == "" {
		input.BaselineRef = proposal.BaseBlueprintHash
	}
	if input.ChallengerRef == "" {
		input.ChallengerRef = proposal.EffectiveBlueprintHash
	}
	if input.BaselineRef != proposal.BaseBlueprintHash || input.ChallengerRef != proposal.EffectiveBlueprintHash {
		return harness.Object{}, errors.New("Evaluation baseline/challenger must match the proposal hashes")
	}
	evaluation, err := s.experience.EvaluateGovernedTarget(ctx, input, ChangeProposalTypeV1, proposal.SourceRunIDs, proposal.SourceOutcomeRunIDs)
	if err != nil {
		return harness.Object{}, err
	}
	if object.Status != "candidate" {
		return evaluation, nil
	}
	proposal.EvaluationObjectIDs = unique(append(proposal.EvaluationObjectIDs, evaluation.ObjectID))
	raw, _ := json.Marshal(proposal)
	_, err = s.harness.Materialize(ctx, harness.MaterializeInput{ObjectID: object.ObjectID, TypeID: object.TypeID, ProjectID: object.ProjectID, Status: "candidate", Payload: raw,
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance, SourceObjectIDs: unique(append(object.Revision.SourceObjectIDs, evaluation.ObjectID)),
		PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "proposal-eval:" + object.ObjectID + ":" + contracts.HashBytes(raw)})
	return evaluation, err
}

func (s *Service) ProposeApproval(ctx context.Context, proposalID string, input ApprovalInput) (harness.RevisionReview, error) {
	object, proposal, err := s.Proposal(ctx, proposalID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.Status != "candidate" {
		return harness.RevisionReview{}, errors.New("only candidate Change Proposal can request approval")
	}
	input.VerifierID = strings.TrimSpace(input.VerifierID)
	input.EditReason = strings.TrimSpace(input.EditReason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ExpectedRevision <= 0 || input.VerifierID == "" || input.EditReason == "" || input.IdempotencyKey == "" {
		return harness.RevisionReview{}, errors.New("expected_revision, verifier_id, edit_reason and idempotency_key are required")
	}
	if input.VerifierID == proposal.ProposerID {
		return harness.RevisionReview{}, errors.New("proposal author cannot verify the same Change Proposal")
	}
	evaluationObjects, evaluations, err := s.experience.EvaluationsForTarget(ctx, object.ProjectID, object.ObjectID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if len(evaluations) == 0 {
		return harness.RevisionReview{}, errors.New("Change Proposal approval requires independent Evaluation")
	}
	total := 0
	evalIDs := []string{}
	for index, value := range evaluations {
		if value.EvaluatorID == proposal.ProposerID || value.EvaluatorID == input.VerifierID {
			return harness.RevisionReview{}, errors.New("proposer, evaluator and verifier must be distinct")
		}
		if value.BaselineRef != proposal.BaseBlueprintHash || value.ChallengerRef != proposal.EffectiveBlueprintHash {
			return harness.RevisionReview{}, errors.New("Evaluation hashes do not match proposal")
		}
		if value.Verdict != "pass" {
			return harness.RevisionReview{}, fmt.Errorf("pre-canary Evaluation %d is not pass", index)
		}
		total += value.SampleSize
	}
	if total < proposal.MinimumSample {
		return harness.RevisionReview{}, fmt.Errorf("independent Evaluation sample %d is below minimum %d", total, proposal.MinimumSample)
	}
	for _, item := range evaluationObjects {
		evalIDs = append(evalIDs, item.ObjectID)
	}
	proposal.VerifierID = input.VerifierID
	proposal.EvaluationObjectIDs = unique(evalIDs)
	raw, _ := json.Marshal(proposal)
	validation, _ := json.Marshal(map[string]any{"status": "passed", "kind": "adaptation_pre_canary", "sample_size": total, "base_blueprint_hash": proposal.BaseBlueprintHash, "effective_blueprint_hash": proposal.EffectiveBlueprintHash})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{Payload: raw, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "active",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance, ValidFrom: object.Revision.ValidFrom, SourceEvidenceIDs: object.Revision.SourceEvidenceIDs,
		SourceObjectIDs: unique(append(object.Revision.SourceObjectIDs, evalIDs...)), PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: input.IdempotencyKey,
		RequestedBy: proposal.ProposerID, Validation: validation})
}

func (s *Service) CreateOverlay(ctx context.Context, input OverlayInput) (harness.Object, error) {
	input.ProposalID = strings.TrimSpace(input.ProposalID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProposalID == "" || input.IdempotencyKey == "" {
		return harness.Object{}, errors.New("proposal_id and idempotency_key are required")
	}
	proposalObject, proposal, err := s.Proposal(ctx, input.ProposalID)
	if err != nil {
		return harness.Object{}, err
	}
	if proposalObject.Status != "active" {
		return harness.Object{}, errors.New("Case Overlay requires an approved active Change Proposal")
	}
	current, err := s.blueprints.Current(ctx, proposal.ProjectID)
	if err != nil {
		return harness.Object{}, err
	}
	if current.Assignment.BlueprintHash != proposal.BaseBlueprintHash {
		return harness.Object{}, errors.New("base Blueprint changed; Change Proposal is stale")
	}
	effective, _, _, effectiveHash, err := applyPatch(current, proposal.Patch)
	if err != nil {
		return harness.Object{}, err
	}
	if effectiveHash != proposal.EffectiveBlueprintHash {
		return harness.Object{}, errors.New("recompiled overlay hash does not match approved Change Proposal")
	}
	effectiveRaw, _ := json.Marshal(effective)
	created := time.Now().UTC()
	expires := created.Add(time.Duration(proposal.OverlayTTLSeconds) * time.Second)
	overlayID := stableID("case-overlay-", proposal.ProjectID, proposal.ProposalID, proposal.EffectiveBlueprintHash, input.IdempotencyKey)
	value := CaseOverlay{OverlayID: overlayID, ProjectID: proposal.ProjectID, ProposalID: proposal.ProposalID,
		BaseBlueprintID: proposal.BaseBlueprintID, BaseBlueprintVersion: proposal.BaseBlueprintVersion, BaseBlueprintHash: proposal.BaseBlueprintHash,
		EffectiveBlueprintHash: proposal.EffectiveBlueprintHash, Patch: proposal.Patch, EffectiveBlueprint: effectiveRaw, PermissionDelta: []string{},
		TTLSeconds: proposal.OverlayTTLSeconds, CreatedAt: created.Format(time.RFC3339Nano), ExpiresAt: expires.Format(time.RFC3339Nano)}
	raw, _ := json.Marshal(value)
	sources := unique(append([]string{proposalObject.ObjectID}, append(proposal.SourceCaseIDs, append(proposal.SourcePatternIDs, proposal.EvaluationObjectIDs...)...)...))
	return s.harness.Materialize(ctx, harness.MaterializeInput{ObjectID: overlayID, TypeID: CaseOverlayTypeV1, ProjectID: proposal.ProjectID, Status: "candidate", Payload: raw,
		Confidence: 1, Importance: .8, ValidUntil: value.ExpiresAt, SourceObjectIDs: sources, PluginID: PluginID, PluginVersion: PluginVersion, IdempotencyKey: "case-overlay:" + input.IdempotencyKey})
}

func (s *Service) ProposeOverlayActivation(ctx context.Context, overlayID string, input experience.ActivationInput) (harness.RevisionReview, error) {
	object, overlay, err := s.Overlay(ctx, overlayID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if object.Status != "candidate" {
		return harness.RevisionReview{}, errors.New("only candidate Case Overlay can request activation")
	}
	proposalObject, _, err := s.Proposal(ctx, overlay.ProposalID)
	if err != nil || proposalObject.Status != "active" {
		return harness.RevisionReview{}, errors.New("overlay proposal is not active")
	}
	if expires, err := time.Parse(time.RFC3339Nano, overlay.ExpiresAt); err != nil || !expires.After(time.Now().UTC()) {
		return harness.RevisionReview{}, errors.New("Case Overlay has expired")
	}
	current, err := s.blueprints.Current(ctx, overlay.ProjectID)
	if err != nil {
		return harness.RevisionReview{}, err
	}
	if current.Assignment.BlueprintHash != overlay.BaseBlueprintHash {
		return harness.RevisionReview{}, errors.New("base Blueprint changed; Case Overlay is stale")
	}
	validation, _ := json.Marshal(map[string]any{"status": "passed", "kind": "case_overlay", "base_blueprint_hash": overlay.BaseBlueprintHash, "effective_blueprint_hash": overlay.EffectiveBlueprintHash, "permission_delta": overlay.PermissionDelta})
	return s.harness.ProposeRevision(ctx, object.ObjectID, harness.ProposeRevisionInput{Payload: object.Revision.Payload, ExpectedRevision: input.ExpectedRevision, EditReason: input.EditReason, TargetStatus: "active",
		Confidence: object.Revision.Confidence, Importance: object.Revision.Importance, ValidFrom: object.Revision.ValidFrom, ValidUntil: overlay.ExpiresAt,
		SourceEvidenceIDs: object.Revision.SourceEvidenceIDs, SourceObjectIDs: object.Revision.SourceObjectIDs, PluginID: PluginID, PluginVersion: PluginVersion,
		IdempotencyKey: input.IdempotencyKey, RequestedBy: "adaptation-lab", Validation: validation})
}

func (s *Service) OverlaySnapshot(ctx context.Context, overlayID string) (json.RawMessage, error) {
	object, overlay, err := s.Overlay(ctx, overlayID)
	if err != nil {
		return nil, err
	}
	if object.Status != "active" {
		return nil, errors.New("Case Overlay is not active")
	}
	if len(overlay.PermissionDelta) != 0 {
		return nil, errors.New("Case Overlay permission delta must remain empty")
	}
	expires, err := time.Parse(time.RFC3339Nano, overlay.ExpiresAt)
	if err != nil || !expires.After(time.Now().UTC()) {
		return nil, errors.New("Case Overlay has expired")
	}
	current, err := s.blueprints.Current(ctx, overlay.ProjectID)
	if err != nil {
		return nil, err
	}
	if current.Assignment.BlueprintHash != overlay.BaseBlueprintHash {
		return nil, errors.New("base Blueprint changed; Case Overlay is stale")
	}
	if len(overlay.EffectiveBlueprint) == 0 || !json.Valid(overlay.EffectiveBlueprint) {
		return nil, errors.New("stored Case Overlay effective Blueprint is invalid JSON")
	}
	recompiled, _, _, hash, err := applyPatch(current, overlay.Patch)
	if err != nil {
		return nil, err
	}
	if hash != overlay.EffectiveBlueprintHash {
		return nil, fmt.Errorf("Case Overlay effective Blueprint hash mismatch: expected %s got %s", overlay.EffectiveBlueprintHash, hash)
	}
	raw, err := json.Marshal(recompiled)
	return json.RawMessage(raw), err
}

func evaluationFromObject(object harness.Object) (experience.Evaluation, error) {
	var value experience.Evaluation
	if object.TypeID != experience.EvaluationTypeV1 && object.TypeID != experience.EvaluationTypeV2 {
		return value, errors.New("canary input is not an Evaluation Object")
	}
	return value, json.Unmarshal(object.Revision.Payload, &value)
}

func canarySignals(value experience.Evaluation) (improvement, regression, safety bool, hasRegression bool) {
	for _, dimension := range value.Dimensions {
		switch strings.ToLower(strings.TrimSpace(dimension.Name)) {
		case "improvement":
			improvement = improvement || dimension.Verdict == "pass"
		case "regression":
			hasRegression = true
			regression = regression || dimension.Verdict == "fail"
		case "safety", "safety_regression":
			safety = safety || dimension.Verdict == "fail"
		}
	}
	return
}

func (s *Service) RunCanary(ctx context.Context, input CanaryInput) (CanaryResult, error) {
	input.OverlayID = strings.TrimSpace(input.OverlayID)
	input.VerifierID = strings.TrimSpace(input.VerifierID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.OverlayID == "" || input.VerifierID == "" || input.IdempotencyKey == "" {
		return CanaryResult{}, errors.New("overlay_id, verifier_id and idempotency_key are required")
	}
	overlayObject, overlay, err := s.Overlay(ctx, input.OverlayID)
	if err != nil {
		return CanaryResult{}, err
	}
	if overlayObject.Status != "active" {
		return CanaryResult{}, errors.New("Canary requires an active Case Overlay")
	}
	if _, err := s.OverlaySnapshot(ctx, overlayObject.ObjectID); err != nil {
		return CanaryResult{}, err
	}
	_, proposal, err := s.Proposal(ctx, overlay.ProposalID)
	if err != nil {
		return CanaryResult{}, err
	}
	if input.VerifierID != proposal.VerifierID || input.VerifierID == proposal.ProposerID {
		return CanaryResult{}, errors.New("Canary verifier must be the approved independent verifier")
	}
	evalIDs := unique(input.EvaluationObjectIDs)
	if len(evalIDs) == 0 {
		return CanaryResult{}, errors.New("Canary requires independent Evaluation Objects")
	}
	samples, improved, regressed := 0, 0, 0
	safetyFailure := false
	for _, id := range evalIDs {
		object, err := s.harness.Object(ctx, id)
		if err != nil {
			return CanaryResult{}, err
		}
		value, err := evaluationFromObject(object)
		if err != nil {
			return CanaryResult{}, err
		}
		if value.TargetKind != "change_proposal" || value.TargetID != proposal.ProposalID {
			return CanaryResult{}, errors.New("Canary Evaluation must target the approved Change Proposal")
		}
		if value.EvaluatorID == proposal.ProposerID || value.EvaluatorID == input.VerifierID {
			return CanaryResult{}, errors.New("proposer, evaluator and verifier must remain distinct during Canary")
		}
		if value.BaselineRef != proposal.BaseBlueprintHash || value.ChallengerRef != proposal.EffectiveBlueprintHash {
			return CanaryResult{}, errors.New("Canary Evaluation hashes do not match proposal")
		}
		improvement, regression, safety, hasRegression := canarySignals(value)
		if !hasRegression {
			return CanaryResult{}, errors.New("Canary Evaluation must include an explicit regression dimension")
		}
		weight := max(1, value.SampleSize)
		samples += weight
		if improvement {
			improved += weight
		}
		if regression {
			regressed += weight
		}
		safetyFailure = safetyFailure || safety
	}
	if samples < proposal.MinimumSample {
		return CanaryResult{}, fmt.Errorf("Canary sample %d is below minimum %d", samples, proposal.MinimumSample)
	}
	rate := float64(regressed) / float64(samples)
	status := "canary_passed_overlay_remains_scoped"
	if rate > proposal.StopConditions.MaxRegressionRate || (proposal.StopConditions.StopOnSafetyFailure && safetyFailure) {
		status = "stopped_fallback_base"
	}
	before, err := s.blueprints.Current(ctx, proposal.ProjectID)
	if err != nil {
		return CanaryResult{}, err
	}
	result := CanaryResult{OverlayID: overlay.OverlayID, ProposalID: proposal.ProposalID, Status: status, Samples: samples, ImprovedSamples: improved, RegressedSamples: regressed,
		RegressionRate: rate, SafetyFailure: safetyFailure, BaseBlueprintHash: proposal.BaseBlueprintHash, EffectiveBlueprintHash: proposal.EffectiveBlueprintHash,
		FallbackBlueprintHash: proposal.BaseBlueprintHash, EvaluationObjectIDs: evalIDs}

	snapshot, _ := json.Marshal(map[string]any{"overlay_id": overlay.OverlayID, "proposal_id": proposal.ProposalID, "base_blueprint_hash": proposal.BaseBlueprintHash, "effective_blueprint_hash": proposal.EffectiveBlueprintHash, "evaluation_object_ids": evalIDs, "stop_conditions": proposal.StopConditions, "expires_at": overlay.ExpiresAt})
	run, err := s.harness.StartRun(ctx, harness.StartRunInput{ProjectID: proposal.ProjectID, CallerType: "owner", CallerID: input.VerifierID, Channel: "adaptation-canary",
		PipelineID: CanaryPipelineID, PipelineVersion: CanaryPipelineVersion, PipelineHash: contracts.HashBytes([]byte(CanaryPipelineID + "@" + CanaryPipelineVersion)), IdempotencyKey: "adaptation-canary:" + input.IdempotencyKey, Snapshot: snapshot})
	if err != nil {
		return CanaryResult{}, err
	}
	if existing, getErr := s.harness.StageOutput(ctx, run.RunID, "adaptation.canary"); getErr == nil {
		var prior CanaryResult
		if err := json.Unmarshal(existing.Payload, &prior); err != nil {
			return CanaryResult{}, err
		}
		return prior, nil
	}
	if run.Status == "queued" {
		if _, err := s.harness.AppendEvent(ctx, run.RunID, "run.started", PluginID, map[string]any{"purpose": "governed_canary"}); err != nil {
			return CanaryResult{}, err
		}
	}
	after, err := s.blueprints.Current(ctx, proposal.ProjectID)
	if err != nil {
		return CanaryResult{}, err
	}
	result.RunID = run.RunID
	result.GlobalBlueprintUnchanged = before.Assignment.BlueprintHash == after.Assignment.BlueprintHash && after.Assignment.BlueprintHash == proposal.BaseBlueprintHash
	if !result.GlobalBlueprintUnchanged {
		return CanaryResult{}, errors.New("global Blueprint changed during Canary")
	}
	raw, _ := json.Marshal(result)
	if _, err := s.harness.RecordStageOutput(ctx, run.RunID, "adaptation.canary", raw); err != nil {
		return CanaryResult{}, err
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, "adaptation.canary.evaluated", PluginID, result); err != nil {
		return CanaryResult{}, err
	}
	terminal := "run.completed"
	if status == "stopped_fallback_base" {
		terminal = "run.completed_with_warnings"
		if _, err := s.harness.AppendEvent(ctx, run.RunID, "adaptation.fallback.base", PluginID, map[string]any{"overlay_id": overlay.OverlayID, "fallback_blueprint_hash": proposal.BaseBlueprintHash, "reason": "stop_condition_triggered"}); err != nil {
			return CanaryResult{}, err
		}
	}
	if _, err := s.harness.AppendEvent(ctx, run.RunID, terminal, PluginID, map[string]any{"canary_status": status}); err != nil {
		return CanaryResult{}, err
	}
	return result, nil
}

func (s *Service) RollbackToBase(ctx context.Context, overlayID, actorID, idempotencyKey string) (CanaryResult, error) {
	object, overlay, err := s.Overlay(ctx, overlayID)
	if err != nil {
		return CanaryResult{}, err
	}
	if object.Status != "active" {
		return CanaryResult{}, errors.New("rollback requires an active Case Overlay")
	}
	_, proposal, err := s.Proposal(ctx, overlay.ProposalID)
	if err != nil {
		return CanaryResult{}, err
	}
	current, err := s.blueprints.Current(ctx, overlay.ProjectID)
	if err != nil {
		return CanaryResult{}, err
	}
	if current.Assignment.BlueprintHash != overlay.BaseBlueprintHash {
		return CanaryResult{}, errors.New("global Blueprint no longer matches overlay base; explicit Owner reconciliation is required")
	}
	result := CanaryResult{OverlayID: overlay.OverlayID, ProposalID: proposal.ProposalID, Status: "rolled_back_to_base", BaseBlueprintHash: overlay.BaseBlueprintHash, EffectiveBlueprintHash: overlay.EffectiveBlueprintHash, FallbackBlueprintHash: overlay.BaseBlueprintHash, GlobalBlueprintUnchanged: true, EvaluationObjectIDs: []string{}}
	snapshot, _ := json.Marshal(map[string]any{"overlay_id": overlay.OverlayID, "proposal_id": proposal.ProposalID, "base_blueprint_hash": overlay.BaseBlueprintHash, "effective_blueprint_hash": overlay.EffectiveBlueprintHash})
	run, err := s.harness.StartRun(ctx, harness.StartRunInput{ProjectID: overlay.ProjectID, CallerType: "owner", CallerID: strings.TrimSpace(actorID), Channel: "adaptation-rollback", PipelineID: PluginID + ".rollback", PipelineVersion: "1.0.0", PipelineHash: contracts.HashBytes([]byte(PluginID + ".rollback@1.0.0")), IdempotencyKey: "adaptation-rollback:" + strings.TrimSpace(idempotencyKey), Snapshot: snapshot})
	if err != nil {
		return CanaryResult{}, err
	}
	result.RunID = run.RunID
	if run.Status == "queued" {
		_, _ = s.harness.AppendEvent(ctx, run.RunID, "run.started", PluginID, map[string]any{"purpose": "fallback_base"})
	}
	raw, _ := json.Marshal(result)
	if _, err := s.harness.RecordStageOutput(ctx, run.RunID, "adaptation.rollback", raw); err != nil {
		return CanaryResult{}, err
	}
	_, _ = s.harness.AppendEvent(ctx, run.RunID, "adaptation.fallback.base", PluginID, map[string]any{"overlay_id": overlay.OverlayID, "fallback_blueprint_hash": overlay.BaseBlueprintHash, "reason": "explicit_rollback"})
	_, err = s.harness.AppendEvent(ctx, run.RunID, "run.completed", PluginID, map[string]any{"rollback": "base_preserved"})
	return result, err
}
