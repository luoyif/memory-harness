package search_test

import (
	"context"
	"os"
	"testing"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/doctor"
	"github.com/luoyif/memory-harness/internal/search"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func seed(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()
	rows := [][]byte{
		testutil.Evidence(t, "ev-a1", "codex", "sess-a", "user", "2026-08-19T01:00:00Z", "The Knowledge Hub needs an automatic review gate."),
		testutil.Evidence(t, "ev-a2", "codex", "sess-a", "assistant", "2026-08-19T01:01:00Z", "I added the runtime write guard around canonical Wiki writes."),
		testutil.Evidence(t, "ev-a3", "codex", "sess-a", "assistant", "2026-08-19T01:02:00Z", "Tests pass and the review gate is enforced."),
		testutil.Evidence(t, "ev-b1", "chatgpt", "sess-b", "user", "2026-07-01T01:00:00Z", "We discussed lightweight screen recording software."),
		testutil.Evidence(t, "ev-c1", "dsh", "sess-c", "user", "2026-08-20T03:00:00Z", "知识沉淀应该自动进行，但程序资产需要审核。"),
	}
	for _, raw := range rows {
		if _, err := a.Ledger.Append(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(r search.Result) []string {
	out := []string{}
	for _, h := range r.Hits {
		out = append(out, h.Turn.EvidenceID)
	}
	return out
}

func TestSearchSessionTimeNeighborChineseAndRebuild(t *testing.T) {
	home := t.TempDir()
	cfg, err := config.Resolve(home, "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	seed(t, a)
	ctx := context.Background()
	q := search.Query{Text: "review gate", Limit: 3, NeighborTurns: 1, SessionFusion: true, DateFrom: "2026-08-01"}
	before, err := a.Search.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Hits) == 0 {
		t.Fatal("no review-gate hits")
	}
	if before.Hits[0].Turn.SessionID != "sess-a" {
		t.Fatalf("unexpected session %#v", before.Hits[0])
	}
	if len(before.Hits[0].Context) < 2 {
		t.Fatalf("neighbor expansion missing: %#v", before.Hits[0].Context)
	}
	old, err := a.Search.Search(ctx, search.Query{Text: "screen recording", DateFrom: "2026-08-01", SessionFusion: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Hits) != 0 {
		t.Fatalf("time filter leaked old hit %#v", old.Hits)
	}
	zh, err := a.Search.Search(ctx, search.Query{Text: "知识沉淀", SessionFusion: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(zh.Hits) == 0 || zh.Hits[0].Turn.EvidenceID != "ev-c1" {
		t.Fatalf("CJK trigram failed %#v", zh)
	}
	scoped, err := a.Search.Search(ctx, search.Query{Text: "review gate", Scope: "test", SessionFusion: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Hits) == 0 {
		t.Fatal("scope filter lost expected hit")
	}
	notScoped, err := a.Search.Search(ctx, search.Query{Text: "review gate", Scope: "other", SessionFusion: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(notScoped.Hits) != 0 {
		t.Fatalf("scope isolation failed %#v", notScoped.Hits)
	}
	excluded, err := a.Search.Search(ctx, search.Query{Text: "review gate", ExcludeSessionIDs: []string{"sess-a"}, SessionFusion: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(excluded.Hits) != 0 {
		t.Fatalf("exclude_session_ids failed %#v", excluded.Hits)
	}
	rep, err := doctor.Run(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "pass" {
		t.Fatalf("doctor before rebuild: %#v", rep)
	}
	beforeIDs := ids(before)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.SearchDB()); err != nil {
		t.Fatal(err)
	}
	a2, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()
	n, err := a2.Ledger.RebuildSearch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("rebuild count=%d", n)
	}
	after, err := a2.Search.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	afterIDs := ids(after)
	if len(beforeIDs) != len(afterIDs) {
		t.Fatalf("result count changed %v -> %v", beforeIDs, afterIDs)
	}
	for i := range beforeIDs {
		if beforeIDs[i] != afterIDs[i] {
			t.Fatalf("result changed %v -> %v", beforeIDs, afterIDs)
		}
	}
	rep, err = doctor.Run(ctx, a2)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "pass" {
		t.Fatalf("doctor after rebuild: %#v", rep)
	}
}
