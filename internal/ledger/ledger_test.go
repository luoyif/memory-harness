package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestAppendIdempotentAndConflict(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	raw := testutil.Evidence(t, "ev-1", "codex", "sess-1", "user", "2026-08-20T02:00:00Z", "remember the review gate")
	first, err := a.Ledger.Append(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate {
		t.Fatal("first append reported duplicate")
	}
	second, err := a.Ledger.Append(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatal("repeat append must be duplicate")
	}
	if first.LineHash != second.LineHash || first.Ordinal != second.Ordinal {
		t.Fatalf("idempotent locator changed: %#v %#v", first, second)
	}
	changed := testutil.Evidence(t, "ev-1", "codex", "sess-1", "user", "2026-08-20T02:00:00Z", "different content")
	_, err = a.Ledger.Append(ctx, changed)
	if !errors.Is(err, ledger.ErrEvidenceConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	got, err := a.Ledger.ReadEvidence(ctx, "ev-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("empty evidence")
	}
	n, err := a.Control.CountReceipts(ctx)
	if err != nil || n != 1 {
		t.Fatalf("receipts=%d err=%v", n, err)
	}
}
