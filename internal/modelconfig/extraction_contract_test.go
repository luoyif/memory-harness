package modelconfig

import "testing"

func TestDecodeCompactExtractionContractExpandsSemanticSlots(t *testing.T) {
	content := `{"candidates":[{"evidence_id":"ev-1","statement":"李明计划于 2026 年 9 月在伦敦发布 Memory Harness。","unit_type":"goal","confidence":0.91,"semantic":{"speaker_ref":"speaker-1","asserted_by_ref":"participant-li-ming","subject":{"ref":"participant-li-ming","surface":"李明","type":"person","resolution":"resolved"},"predicate":"plans_to_launch","inverse_label":"is_planned_by","object":{"kind":"entity","entity":{"ref":"product-memory-harness","surface":"Memory Harness","type":"product","resolution":"resolved"}},"action":"发布","participants":[{"role":"actor","entity":{"ref":"participant-li-ming","surface":"李明","type":"person","resolution":"resolved"}}],"locations":[{"role":"destination","entity":{"ref":"place-london","surface":"伦敦","type":"place","resolution":"resolved"}}],"time":{"text":"2026 年 9 月","occurred_from":"2026-09-01T00:00:00Z","precision":"month","resolution":"resolved"},"epistemic":{"polarity":"positive","modality":"planned","importance":0.9,"novelty":0.8},"quote":"我计划明年九月在伦敦发布 Memory Harness。"}}]}`

	candidates, ok := decodeExtractionContent(content)
	if !ok || len(candidates) != 1 {
		t.Fatalf("decode ok=%v candidates=%#v", ok, candidates)
	}
	got := candidates[0]
	if got.Structure.Attribution.SubjectRef != "participant-li-ming" || got.Structure.Frame.Subject.Surface != "李明" {
		t.Fatalf("subject attribution was not expanded: %#v", got.Structure)
	}
	if got.Structure.Frame.Predicate != "plans_to_launch" || got.Structure.Frame.InverseLabel != "is_planned_by" {
		t.Fatalf("semantic relation was not expanded: %#v", got.Structure.Frame)
	}
	if got.Structure.Temporal.Precision != "month" || got.Structure.Provenance.EvidenceSpan.Quote == "" {
		t.Fatalf("time or provenance was lost: %#v", got.Structure)
	}
}
