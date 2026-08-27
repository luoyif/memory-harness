package memory_test

import (
	"context"
	"testing"

	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/testutil"
)

type participantAwareExtractor struct {
	seen chan []memory.ParticipantIdentity
}

func (p participantAwareExtractor) Compiler(context.Context) string { return "agent/participant-test" }
func (p participantAwareExtractor) Extract(_ context.Context, request memory.ExtractionRequest) (memory.ExtractionResult, error) {
	p.seen <- request.Participants
	participant := request.Participants[0]
	return memory.ExtractionResult{Compiler: "agent/participant-test", TotalChunks: 1, SucceededChunks: 1, Candidates: []memory.ExtractionCandidate{{
		EvidenceID: request.Turns[0].EvidenceID, Statement: participant.DisplayName + "计划整理旅行记录。", UnitType: "goal", TierHint: "semantic", RiskTier: "B", Confidence: 0.93,
		Structure: memory.KnowledgeStructure{Attribution: memory.Attribution{SubjectRef: participant.ParticipantID, SubjectSurface: participant.DisplayName, Resolution: "resolved", OwnerMapping: "not_assumed"}, Frame: memory.SemanticFrame{Subject: memory.EntityRef{EntityID: participant.ParticipantID, EntityType: "person", Surface: participant.DisplayName, CanonicalName: participant.DisplayName, Resolution: "resolved"}, Predicate: "plans_to_organize", InverseLabel: "由其整理", Object: memory.SemanticObject{Kind: "literal", Value: "旅行记录"}}, Provenance: memory.UnitProvenance{EvidenceSpan: memory.EvidenceSpan{Quote: "计划整理旅行记录"}}},
	}}}, nil
}

func TestSessionParticipantsAreExplicitAndReachExtractor(t *testing.T) {
	a, _ := testutil.Open(t)
	appended, err := a.Ledger.Append(t.Context(), testutil.Evidence(t, "ev-participant-context", "recording", "session-participant-context", "speaker", "2026-08-21T13:00:00Z", "我计划整理旅行记录。"))
	if err != nil {
		t.Fatal(err)
	}
	participants, err := a.Memory.ReplaceSessionParticipants(t.Context(), appended.SessionID, []memory.ParticipantIdentity{{DisplayName: "李明", Role: "first_person_speaker", Aliases: []string{"我"}}, {DisplayName: "王芳", Role: "participant"}})
	if err != nil || len(participants) != 2 || participants[0].ParticipantID == "" {
		t.Fatalf("participants=%#v err=%v", participants, err)
	}
	seen := make(chan []memory.ParticipantIdentity, 1)
	a.Memory.SetCandidateExtractor(participantAwareExtractor{seen: seen})
	run, err := a.Memory.EnqueueAndProcess(t.Context(), appended.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if received := <-seen; len(received) != 2 || received[0].Role != "first_person_speaker" {
		t.Fatalf("extractor did not receive explicit participant registry: %#v", received)
	}
	units, err := a.Memory.ListKnowledgeUnits(t.Context(), run.EpisodeID, "", 10)
	if err != nil || len(units) != 1 || units[0].Structure.Attribution.Resolution != "resolved" {
		t.Fatalf("resolved participant semantics were not persisted: units=%#v err=%v", units, err)
	}
}
