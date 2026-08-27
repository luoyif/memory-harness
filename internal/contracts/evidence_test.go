package contracts_test

import (
	"github.com/luoyif/memory-harness/internal/contracts"
	"testing"
)

func TestParseEvidenceCanonicalizesObjectOrder(t *testing.T) {
	a := []byte(`{"schema_version":"0.1","evidence_id":"ev","source_system":"codex","captured_at":"2026-08-20T00:00:00Z","content":[{"type":"text","text":"same"}],"provenance":{"capture_method":"test"}}`)
	b := []byte(`{"provenance":{"capture_method":"test"},"content":[{"text":"same","type":"text"}],"captured_at":"2026-08-20T00:00:00Z","source_system":"codex","evidence_id":"ev","schema_version":"0.1"}`)
	_, ca, err := contracts.ParseEvidence(a)
	if err != nil {
		t.Fatal(err)
	}
	_, cb, err := contracts.ParseEvidence(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(ca) != string(cb) {
		t.Fatalf("canonical mismatch\n%s\n%s", ca, cb)
	}
	if contracts.HashBytes(ca) != contracts.HashBytes(cb) {
		t.Fatal("canonical hash mismatch")
	}
}
func TestParseEvidenceRejectsMissingBlockType(t *testing.T) {
	raw := []byte(`{"schema_version":"0.1","evidence_id":"ev","source_system":"codex","captured_at":"2026-08-20T00:00:00Z","content":[{"text":"x"}],"provenance":{"capture_method":"test"}}`)
	if _, _, err := contracts.ParseEvidence(raw); err == nil {
		t.Fatal("expected validation error")
	}
}
