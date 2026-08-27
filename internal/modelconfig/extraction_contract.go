package modelconfig

import (
	"encoding/json"
	"strings"

	"github.com/luoyif/memory-harness/internal/memory"
)

// compactExtractionCandidate is the provider-facing transport contract. The
// model supplies semantic slots; Memory Harness deterministically expands them
// into the stable Knowledge Unit v2 envelope. This avoids asking a reasoning
// model to repeat dozens of empty fields for every candidate in a long audio
// transcript.
type compactExtractionCandidate struct {
	EvidenceID string          `json:"evidence_id"`
	Statement  string          `json:"statement"`
	UnitType   string          `json:"unit_type"`
	Confidence float64         `json:"confidence"`
	Semantic   compactSemantic `json:"semantic"`
}

type compactEntity struct {
	Ref        string `json:"ref,omitempty"`
	Surface    string `json:"surface,omitempty"`
	Type       string `json:"type,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type compactRole struct {
	Role   string        `json:"role"`
	Entity compactEntity `json:"entity"`
}

type compactObject struct {
	Kind   string        `json:"kind,omitempty"`
	Entity compactEntity `json:"entity,omitempty"`
	Value  string        `json:"value,omitempty"`
}

type compactTime struct {
	Text          string `json:"text,omitempty"`
	ValidFrom     string `json:"valid_from,omitempty"`
	ValidUntil    string `json:"valid_until,omitempty"`
	OccurredFrom  string `json:"occurred_from,omitempty"`
	OccurredUntil string `json:"occurred_until,omitempty"`
	Precision     string `json:"precision,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
}

type compactEpistemic struct {
	Polarity      string   `json:"polarity,omitempty"`
	Modality      string   `json:"modality,omitempty"`
	Importance    float64  `json:"importance,omitempty"`
	Novelty       float64  `json:"novelty,omitempty"`
	QualityFlags  []string `json:"quality_flags,omitempty"`
	ReviewReasons []string `json:"review_reasons,omitempty"`
}

type compactSemantic struct {
	SpeakerRef    string           `json:"speaker_ref,omitempty"`
	AssertedByRef string           `json:"asserted_by_ref,omitempty"`
	Subject       compactEntity    `json:"subject"`
	CandidateRefs []string         `json:"candidate_refs,omitempty"`
	ReasonCodes   []string         `json:"reason_codes,omitempty"`
	Predicate     string           `json:"predicate,omitempty"`
	InverseLabel  string           `json:"inverse_label,omitempty"`
	Object        compactObject    `json:"object"`
	Action        string           `json:"action,omitempty"`
	Participants  []compactRole    `json:"participants,omitempty"`
	Locations     []compactRole    `json:"locations,omitempty"`
	Context       string           `json:"context,omitempty"`
	Time          compactTime      `json:"time,omitempty"`
	Epistemic     compactEpistemic `json:"epistemic,omitempty"`
	Quote         string           `json:"quote"`
}

func expandEntity(item compactEntity) memory.EntityRef {
	return memory.EntityRef{EntityID: item.Ref, EntityType: item.Type, Surface: item.Surface, CanonicalName: item.Surface, Resolution: item.Resolution}
}

func expandCompactCandidate(item compactExtractionCandidate) memory.ExtractionCandidate {
	subject := expandEntity(item.Semantic.Subject)
	participants := make([]memory.ParticipantRole, 0, len(item.Semantic.Participants))
	for _, role := range item.Semantic.Participants {
		participants = append(participants, memory.ParticipantRole{Role: role.Role, Entity: expandEntity(role.Entity)})
	}
	locations := make([]memory.LocationRole, 0, len(item.Semantic.Locations))
	for _, role := range item.Semantic.Locations {
		locations = append(locations, memory.LocationRole{Role: role.Role, Entity: expandEntity(role.Entity)})
	}
	return memory.ExtractionCandidate{
		EvidenceID: item.EvidenceID, Statement: item.Statement, UnitType: item.UnitType, Confidence: item.Confidence,
		Structure: memory.KnowledgeStructure{
			Attribution: memory.Attribution{SourceSpeakerRef: item.Semantic.SpeakerRef, AssertedByRef: item.Semantic.AssertedByRef, SubjectRef: item.Semantic.Subject.Ref, SubjectSurface: item.Semantic.Subject.Surface, Resolution: item.Semantic.Subject.Resolution, CandidateRefs: item.Semantic.CandidateRefs, ReasonCodes: item.Semantic.ReasonCodes, OwnerMapping: "not_assumed"},
			Frame:       memory.SemanticFrame{Subject: subject, Predicate: item.Semantic.Predicate, InverseLabel: item.Semantic.InverseLabel, Object: memory.SemanticObject{Kind: item.Semantic.Object.Kind, Entity: expandEntity(item.Semantic.Object.Entity), Value: item.Semantic.Object.Value}, Action: item.Semantic.Action, Participants: participants, Locations: locations, Context: item.Semantic.Context},
			Temporal:    memory.TemporalContext{EventTimeText: item.Semantic.Time.Text, ValidFrom: item.Semantic.Time.ValidFrom, ValidUntil: item.Semantic.Time.ValidUntil, OccurredFrom: item.Semantic.Time.OccurredFrom, OccurredUntil: item.Semantic.Time.OccurredUntil, Precision: item.Semantic.Time.Precision, Resolution: item.Semantic.Time.Resolution},
			Epistemic:   memory.EpistemicContext{Polarity: item.Semantic.Epistemic.Polarity, Modality: item.Semantic.Epistemic.Modality, Confidence: item.Confidence, Importance: item.Semantic.Epistemic.Importance, Novelty: item.Semantic.Epistemic.Novelty, QualityFlags: item.Semantic.Epistemic.QualityFlags, ReviewReasons: item.Semantic.Epistemic.ReviewReasons},
			Provenance:  memory.UnitProvenance{EvidenceID: item.EvidenceID, EvidenceSpan: memory.EvidenceSpan{Quote: item.Semantic.Quote}},
		},
	}
}

func decodeExtractionContent(content string) ([]memory.ExtractionCandidate, bool) {
	for offset := 0; offset < len(content); {
		index := strings.Index(content[offset:], "{")
		if index < 0 {
			break
		}
		offset += index
		var envelope struct {
			Candidates []json.RawMessage `json:"candidates"`
		}
		decoder := json.NewDecoder(strings.NewReader(content[offset:]))
		if err := decoder.Decode(&envelope); err == nil && envelope.Candidates != nil {
			candidates := make([]memory.ExtractionCandidate, 0, len(envelope.Candidates))
			valid := true
			for _, raw := range envelope.Candidates {
				var shape map[string]json.RawMessage
				if err := json.Unmarshal(raw, &shape); err != nil {
					valid = false
					break
				}
				if _, compact := shape["semantic"]; compact {
					var item compactExtractionCandidate
					if err := json.Unmarshal(raw, &item); err != nil {
						valid = false
						break
					}
					candidates = append(candidates, expandCompactCandidate(item))
				} else {
					var item memory.ExtractionCandidate
					if err := json.Unmarshal(raw, &item); err != nil {
						valid = false
						break
					}
					candidates = append(candidates, item)
				}
			}
			if valid {
				return candidates, true
			}
		}
		offset++
	}
	return nil, false
}
