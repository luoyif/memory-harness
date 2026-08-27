package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

type failingExtractor struct{}

func (failingExtractor) Compiler(context.Context) string { return "agent/failing+rules-fallback" }
func (failingExtractor) Extract(context.Context, memory.ExtractionRequest) (memory.ExtractionResult, error) {
	return memory.ExtractionResult{}, errors.New("simulated provider outage")
}

type mixedExtractor struct{}

func (mixedExtractor) Compiler(context.Context) string { return "agent/test-model+rules-fallback" }
func (mixedExtractor) Extract(_ context.Context, request memory.ExtractionRequest) (memory.ExtractionResult, error) {
	evidenceID := request.Turns[0].EvidenceID
	return memory.ExtractionResult{Compiler: "agent/test-model", Candidates: []memory.ExtractionCandidate{
		{EvidenceID: evidenceID, Statement: "项目已经决定采用分段模型提炼长录音。", UnitType: "decision", TierHint: "semantic", RiskTier: "B", Confidence: 0.93},
		{EvidenceID: "model-copied-the-id-incorrectly", Statement: "用户是一名人工智能研究人员。", UnitType: "identity", TierHint: "semantic", RiskTier: "A", Confidence: 0.91},
		{EvidenceID: evidenceID, Statement: "无法识别的候选类型不应进入记忆。", UnitType: "unsupported", TierHint: "semantic", RiskTier: "A", Confidence: 0.91},
	}}, nil
}

type structuredExtractor struct {
	unsafeOwnerMapping bool
}

func (structuredExtractor) Compiler(context.Context) string { return "agent/structured-test" }
func (s structuredExtractor) Extract(_ context.Context, request memory.ExtractionRequest) (memory.ExtractionResult, error) {
	evidenceID := request.Turns[0].EvidenceID
	if s.unsafeOwnerMapping {
		return memory.ExtractionResult{Compiler: "agent/structured-test", Candidates: []memory.ExtractionCandidate{{
			EvidenceID: evidenceID,
			Statement:  "我计划为项目配置生产环境访问权限。",
			UnitType:   "goal",
			TierHint:   "semantic",
			RiskTier:   "B",
			Confidence: 0.89,
			Structure: memory.KnowledgeStructure{
				Attribution: memory.Attribution{SourceSpeakerRef: "source-role:user", SubjectRef: "owner", SubjectSurface: "我", Resolution: "resolved", OwnerMapping: "assumed"},
				Frame: memory.SemanticFrame{
					Subject:      memory.EntityRef{EntityID: "owner", EntityType: "person", Surface: "我", CanonicalName: "Owner", Resolution: "resolved"},
					Predicate:    "plans_to_configure",
					InverseLabel: "将由其配置",
					Object:       memory.SemanticObject{Kind: "entity", Entity: memory.EntityRef{EntityType: "system", Surface: "生产环境访问权限", CanonicalName: "生产环境访问权限", Resolution: "resolved"}},
				},
				Epistemic:  memory.EpistemicContext{Modality: "planned", Confidence: 0.89},
				Provenance: memory.UnitProvenance{EvidenceSpan: memory.EvidenceSpan{Quote: "我计划为项目配置生产环境访问权限"}},
			},
		}}}, nil
	}
	return memory.ExtractionResult{Compiler: "agent/structured-test", Candidates: []memory.ExtractionCandidate{{
		EvidenceID: evidenceID,
		Statement:  "王芳计划下周在伦敦参加项目评审会。",
		UnitType:   "goal",
		TierHint:   "semantic",
		RiskTier:   "B",
		Confidence: 0.96,
		Structure: memory.KnowledgeStructure{
			Attribution: memory.Attribution{SourceSpeakerRef: "participant:li-ming", AssertedByRef: "participant:li-ming", SubjectRef: "person:wang-fang", SubjectSurface: "王芳", Resolution: "resolved", OwnerMapping: "not_assumed"},
			Frame: memory.SemanticFrame{
				Subject:      memory.EntityRef{EntityID: "person:wang-fang", EntityType: "person", Surface: "王芳", CanonicalName: "王芳", Resolution: "resolved"},
				Predicate:    "plans_to_attend",
				InverseLabel: "将由其参加",
				Object:       memory.SemanticObject{Kind: "entity", Entity: memory.EntityRef{EntityType: "event", Surface: "项目评审会", CanonicalName: "项目评审会", Resolution: "resolved"}},
				Action:       "参加",
				Locations:    []memory.LocationRole{{Role: "at", Entity: memory.EntityRef{EntityType: "place", Surface: "伦敦", CanonicalName: "伦敦", Resolution: "resolved"}}},
			},
			Temporal:   memory.TemporalContext{EventTimeText: "下周", Precision: "week", Resolution: "relative_pending"},
			Epistemic:  memory.EpistemicContext{Modality: "planned", Confidence: 0.96},
			Provenance: memory.UnitProvenance{EvidenceSpan: memory.EvidenceSpan{Quote: "王芳下周去伦敦参加项目评审会"}},
		},
	}}}, nil
}

func TestExtractorFailureFallsBackWithoutLosingEvidence(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(failingExtractor{})
	result, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-agent-fallback", "codex", "session-agent-fallback", "user", "2026-08-21T13:00:00Z", "We decided to keep the local rules fallback."))
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Memory.EnqueueAndProcess(t.Context(), result.SessionID)
	if err != nil {
		t.Fatalf("canonical capture must survive provider failure: %v", err)
	}
	episode, err := a.Memory.Episode(t.Context(), run.EpisodeID)
	if err != nil || episode.Compiler != memory.CompilerVersion+"/fallback" || episode.Units == 0 {
		t.Fatalf("episode=%#v err=%v", episode, err)
	}
	if _, err := a.Ledger.ReadEvidence(t.Context(), "ev-agent-fallback"); err != nil {
		t.Fatalf("canonical Evidence missing after fallback: %v", err)
	}
}

func TestExplicitReprocessPreservesExistingProjectionWhenModelFails(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(mixedExtractor{})
	result, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-preserve-on-outage", "recording", "session-preserve-on-outage", "speaker", "2026-08-21T13:00:00Z", "项目已经决定采用分段模型提炼长录音。"))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := a.Memory.EnqueueAndProcess(t.Context(), result.SessionID)
	if err != nil || initial.KnowledgeUnits != 2 {
		t.Fatalf("initial=%#v err=%v", initial, err)
	}

	a.Memory.SetCandidateExtractor(failingExtractor{})
	if _, err := a.Memory.ReprocessSession(t.Context(), result.SessionID); err == nil {
		t.Fatal("expected explicit reprocess to stop before replacing memory with fallback")
	}
	episode, err := a.Memory.Episode(t.Context(), initial.EpisodeID)
	if err != nil || episode.Compiler != "agent/test-model+filtered" || episode.Units != 2 {
		t.Fatalf("existing model projection was not preserved: episode=%#v err=%v", episode, err)
	}
}

func TestExtractorFiltersOneInvalidCandidateWithoutDiscardingValidModelKnowledge(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(mixedExtractor{})
	result, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-agent-mixed", "recording", "session-agent-mixed", "user", "2026-08-21T13:00:00Z", "项目决定采用分段模型提炼长录音。"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Memory.EnqueueAndProcess(t.Context(), result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := a.Memory.Episode(t.Context(), run.EpisodeID)
	if err != nil || episode.Compiler != "agent/test-model+filtered" || episode.Units != 2 {
		t.Fatalf("one invalid candidate should not force rules fallback: episode=%#v err=%v", episode, err)
	}
}

func TestStructuredExtractionSeparatesSpeakerSubjectTimeAndEvidence(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(structuredExtractor{})
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "atlas", Name: "Atlas", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-semantic-frame", "recording", "session-semantic-frame", "speaker", "2026-08-21T13:00:00Z", "李明说，王芳下周去伦敦参加项目评审会。"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Memory.EnqueueAndProcess(t.Context(), result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	units, err := a.Memory.ListKnowledgeUnits(t.Context(), run.EpisodeID, "", 10)
	if err != nil || len(units) != 1 {
		t.Fatalf("units=%#v err=%v", units, err)
	}
	unit := units[0]
	if unit.SchemaVersion != memory.KnowledgeUnitSchemaV2 || unit.Structure.Attribution.SourceSpeakerRef == unit.Structure.Attribution.SubjectRef {
		t.Fatalf("speaker and subject were not separated: %#v", unit)
	}
	if unit.Structure.Frame.Subject.CanonicalName != "王芳" || unit.Structure.Temporal.EventTimeText != "下周" || unit.Structure.Temporal.Resolution != "relative_pending" {
		t.Fatalf("structured roles or time were lost: %#v", unit.Structure)
	}
	if unit.Structure.Provenance.EvidenceSpan.Start < 0 || unit.Structure.Provenance.EvidenceSpan.End <= unit.Structure.Provenance.EvidenceSpan.Start || unit.Structure.Provenance.EvidenceSpan.QuoteHash == "" {
		t.Fatalf("exact evidence span was not retained: %#v", unit.Structure.Provenance.EvidenceSpan)
	}
	materialized, err := a.Memory.MaterializeSemantics(t.Context(), project.ProjectID, run.EpisodeID, "run-test", "stage-test")
	if err != nil || materialized.Entities != 2 || materialized.Assertions != 1 || materialized.AmbiguousUnits != 0 {
		t.Fatalf("materialized=%#v err=%v", materialized, err)
	}
	graph, err := a.Memory.SemanticGraph(t.Context(), project.ProjectID, 10)
	if err != nil || len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("graph=%#v err=%v", graph, err)
	}
	if graph.Edges[0].Label != "plans_to_attend" || graph.Edges[0].InverseLabel != "将由其参加" || graph.Edges[0].EvidenceID != "ev-semantic-frame" {
		t.Fatalf("semantic relationship lost provenance or reverse label: %#v", graph.Edges[0])
	}
}

func TestUnsafeFirstPersonOwnerMappingIsForcedToReview(t *testing.T) {
	a, _ := testutil.Open(t)
	a.Memory.SetCandidateExtractor(structuredExtractor{unsafeOwnerMapping: true})
	result, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-owner-guard", "recording", "session-owner-guard", "user", "2026-08-21T13:00:00Z", "我计划为项目配置生产环境访问权限。"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Memory.EnqueueAndProcess(t.Context(), result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	units, err := a.Memory.ListKnowledgeUnits(t.Context(), run.EpisodeID, "", 10)
	if err != nil || len(units) != 1 {
		t.Fatalf("units=%#v err=%v", units, err)
	}
	attribution := units[0].Structure.Attribution
	if attribution.Resolution != "unresolved" || attribution.SubjectRef != "" || attribution.OwnerMapping != "not_assumed" {
		t.Fatalf("unsafe Owner attribution survived normalization: %#v", attribution)
	}
	found := false
	for _, reason := range attribution.ReasonCodes {
		if reason == "owner_mapping_not_explicit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing explainable review reason: %#v", attribution)
	}
	memories, err := a.Memory.ListMemories(t.Context(), "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	var guardedMemoryID string
	for _, item := range memories {
		if item.Tier == "semantic" && item.Summary == "我计划为项目配置生产环境访问权限。" {
			guardedMemoryID = item.MemoryID
			if item.Status != "candidate" {
				t.Fatalf("unresolved subject became an active memory: %#v", item)
			}
		}
	}
	if guardedMemoryID == "" {
		t.Fatal("missing guarded semantic memory candidate")
	}
	reviews, err := a.Memory.ListOperations(t.Context(), "review_required", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range reviews {
		if operation.TargetMemoryID == guardedMemoryID {
			for _, reason := range operation.ReasonCodes {
				if reason == "subject_unresolved" {
					return
				}
			}
		}
	}
	t.Fatal("unresolved subject did not create an explainable review operation")
}
