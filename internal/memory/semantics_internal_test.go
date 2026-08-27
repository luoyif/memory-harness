package memory

import "testing"

func TestNamedSemanticSubjectGetsProjectLocalReferenceWithoutOwnerAssumption(t *testing.T) {
	item := turn{EvidenceID: "ev-1", Role: "speaker", Body: "长期记忆系统必须配套遗忘机制。", ObservedAt: "2026-08-22T00:00:00Z"}
	structure := normalizeKnowledgeStructure(KnowledgeStructure{
		Attribution: Attribution{SubjectSurface: "长期记忆系统", Resolution: "unresolved"},
		Frame: SemanticFrame{
			Subject:   EntityRef{EntityType: "system", Surface: "长期记忆系统", Resolution: "unresolved"},
			Predicate: "requires_forgetting_mechanism",
			Object:    SemanticObject{Kind: "literal", Value: "遗忘机制"},
		},
		Provenance: UnitProvenance{EvidenceSpan: EvidenceSpan{Quote: "长期记忆系统必须配套遗忘机制"}},
	}, item, "长期记忆系统必须配套遗忘机制。", "procedure", "agent/test", .9, nil)

	if structure.Attribution.Resolution != "resolved" || structure.Attribution.SubjectRef == "" {
		t.Fatalf("explicit named entity stayed unresolved: %#v", structure.Attribution)
	}
	if structure.Attribution.OwnerMapping != "not_assumed" || structure.Frame.Subject.EntityID != structure.Attribution.SubjectRef {
		t.Fatalf("project-local resolution changed owner safety: %#v", structure)
	}
	if structure.Frame.InverseLabel != "is_requires_forgetting_mechanism_of" {
		t.Fatalf("missing deterministic reverse relation: %#v", structure.Frame)
	}
}

func TestGenericSpeakerSubjectStillRequiresParticipantIdentity(t *testing.T) {
	item := turn{EvidenceID: "ev-2", Role: "speaker", Body: "讲述者计划去英国交流。", ObservedAt: "2026-08-22T00:00:00Z"}
	structure := normalizeKnowledgeStructure(KnowledgeStructure{
		Attribution: Attribution{SubjectSurface: "讲述者", Resolution: "unresolved"},
		Frame:       SemanticFrame{Subject: EntityRef{EntityType: "person", Surface: "讲述者", Resolution: "unresolved"}, Predicate: "plans_to_travel", Object: SemanticObject{Kind: "literal", Value: "英国"}},
	}, item, "讲述者计划去英国交流。", "goal", "agent/test", .9, nil)

	if structure.Attribution.Resolution != "unresolved" || structure.Attribution.SubjectRef != "" {
		t.Fatalf("generic speaker was guessed: %#v", structure.Attribution)
	}
}

func TestChineseGenericSpeakerAliasesStillRequireParticipantIdentity(t *testing.T) {
	for _, surface := range []string{"讲者", "该用户", "作者", "受访者"} {
		item := turn{EvidenceID: "ev-generic-alias", Role: "speaker", Body: surface + "计划搭建产品。", ObservedAt: "2026-08-22T00:00:00Z"}
		structure := normalizeKnowledgeStructure(KnowledgeStructure{
			Attribution: Attribution{SubjectSurface: surface, Resolution: "unresolved"},
			Frame:       SemanticFrame{Subject: EntityRef{EntityType: "person", Surface: surface, Resolution: "unresolved"}, Predicate: "plans_to_build", Object: SemanticObject{Kind: "literal", Value: "产品"}},
		}, item, surface+"计划搭建产品。", "goal", "agent/test", .8, nil)
		if structure.Attribution.Resolution != "unresolved" || structure.Attribution.SubjectRef != "" {
			t.Fatalf("generic speaker alias %q was incorrectly resolved: %#v", surface, structure.Attribution)
		}
	}
}

func TestInvalidProviderTemporalFieldsAreQuarantinedWithoutDroppingKnowledge(t *testing.T) {
	item := turn{EvidenceID: "ev-invalid-time", Role: "assistant", Body: "Memory Harness 的正式发布需要完成验收。", ObservedAt: "2026-08-24T13:00:00Z"}
	structure := normalizeKnowledgeStructure(KnowledgeStructure{
		Attribution: Attribution{SubjectSurface: "Memory Harness", Resolution: "resolved"},
		Frame: SemanticFrame{
			Subject:   EntityRef{EntityType: "system", Surface: "Memory Harness", EntityID: "memory-harness", Resolution: "resolved"},
			Predicate: "requires_acceptance",
			Object:    SemanticObject{Kind: "literal", Value: "正式发布"},
		},
		Temporal: TemporalContext{ValidFrom: "2026年8月24日", OccurredFrom: "not-a-time", Resolution: "resolved"},
	}, item, item.Body, "procedure", "agent/test", .9, nil)

	if structure.Temporal.ValidFrom != "" || structure.Temporal.OccurredFrom != "" || structure.Temporal.Resolution != "unresolved" {
		t.Fatalf("invalid provider time was not quarantined: %#v", structure.Temporal)
	}
	if !containsString(structure.Epistemic.QualityFlags, "invalid_temporal_format") || !containsString(structure.Epistemic.ReviewReasons, "temporal_normalization_required") {
		t.Fatalf("invalid provider time was not made reviewable: %#v", structure.Epistemic)
	}
	unit := KnowledgeUnit{
		SchemaVersion: KnowledgeUnitSchemaV2,
		UnitID:        "unit-invalid-time", EpisodeID: "episode-invalid-time", EvidenceID: item.EvidenceID,
		Statement: item.Body, UnitType: "procedure", TierHint: "procedural", RiskTier: "B", Confidence: .9,
		ObservedAt: item.ObservedAt, Structure: structure,
	}
	unit.Structure.Provenance.EpisodeID = unit.EpisodeID
	if err := ValidateStructuredKnowledgeRevision(KnowledgeUnit{}, unit); err != nil {
		t.Fatalf("quarantined structure should remain valid: %v", err)
	}
}
