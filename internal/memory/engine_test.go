package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestMemoryGrowthLoopIsTraceableIdempotentAndGoverned(t *testing.T) {
	a, cfg := testutil.Open(t)
	items := [][]byte{
		testutil.Evidence(t, "growth-1", "codex", "session-growth", "user", "2026-08-20T04:00:00Z", "MemoryOS 团队决定采用 MemoryOS 作为个人记忆系统。"),
		testutil.Evidence(t, "growth-2", "codex", "session-growth", "user", "2026-08-20T04:01:00Z", "我是一名人工智能研究人员。"),
		testutil.Evidence(t, "growth-3", "codex", "session-growth", "user", "2026-08-20T04:02:00Z", "每次操作时必须先运行完整测试。"),
		testutil.Evidence(t, "growth-4", "codex", "session-growth", "assistant", "2026-08-20T04:03:00Z", "验证通过，核心测试已经完成。"),
	}
	for _, raw := range items {
		if _, err := a.Ledger.Append(t.Context(), raw); err != nil {
			t.Fatal(err)
		}
	}

	result, err := a.Memory.EnqueueAndProcess(t.Context(), "session-growth")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Evidence != 4 || result.KnowledgeUnits < 4 {
		t.Fatalf("unexpected run result %#v", result)
	}

	overview, err := a.Memory.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Layers) != 6 || overview.Layers[0].Count != 4 || overview.Layers[1].Count < 4 || overview.Layers[2].Count != 1 || overview.NeedsReview < 2 {
		t.Fatalf("unexpected overview %#v", overview)
	}

	memories, err := a.Memory.ListMemories(t.Context(), "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) < 4 {
		t.Fatalf("expected episodic, semantic, procedural and identity memories, got %#v", memories)
	}
	var decisionMemoryID string
	for _, item := range memories {
		if item.Tier == "semantic" && strings.Contains(item.Summary, "决定采用 MemoryOS") {
			decisionMemoryID = item.MemoryID
		}
	}
	if decisionMemoryID == "" {
		t.Fatal("missing semantic decision memory")
	}
	trace, err := a.Memory.Trace(t.Context(), decisionMemoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Operations) != 1 || len(trace.Units) != 1 || len(trace.Episodes) != 1 || len(trace.Evidence) != 1 || trace.Evidence[0].EvidenceID != "growth-1" {
		t.Fatalf("trace is incomplete %#v", trace)
	}

	assets, err := a.Memory.ListAssets(t.Context(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Status != "candidate" || assets[0].ValidationStatus != "not_run" {
		t.Fatalf("protected asset candidate missing %#v", assets)
	}
	views, err := a.Memory.RebuildLivingViewsForProject(t.Context(), portfolio.InboxProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 3 {
		t.Fatalf("living views=%#v", views)
	}
	for _, view := range views {
		if view.ProjectID != portfolio.InboxProjectID {
			t.Fatalf("living view escaped project scope: %#v", view)
		}
		if _, err := os.Stat(filepath.Join(cfg.MemoryDir(), filepath.Base(view.CanonicalPath))); err != nil {
			t.Fatalf("missing living Markdown %s: %v", view.CanonicalPath, err)
		}
	}

	// Reprocessing an unchanged session must not duplicate any derived object.
	beforeUnits, beforeMemories := overview.Layers[1].Count, overview.Layers[3].Count
	if _, err := a.Memory.EnqueueAndProcess(t.Context(), "session-growth"); err != nil {
		t.Fatal(err)
	}
	overview, err = a.Memory.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if overview.Layers[1].Count != beforeUnits || overview.Layers[3].Count != beforeMemories {
		t.Fatalf("idempotency failure before=(%d,%d) after=(%d,%d)", beforeUnits, beforeMemories, overview.Layers[1].Count, overview.Layers[3].Count)
	}

	// Exact knowledge repeated in an independent session enriches the same
	// review candidate instead of creating a duplicate. It does not become an
	// active fact until its unresolved subject has been confirmed.
	raw := testutil.Evidence(t, "growth-repeat", "chatgpt", "session-repeat", "user", "2026-08-21T04:00:00Z", "MemoryOS 团队决定采用 MemoryOS 作为个人记忆系统。")
	if _, err := a.Ledger.Append(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Memory.EnqueueAndProcess(t.Context(), "session-repeat"); err != nil {
		t.Fatal(err)
	}
	decision, err := a.Memory.Memory(t.Context(), decisionMemoryID)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "candidate" || decision.Strength != 2 || len(decision.EvidenceIDs) != 2 || len(decision.EpisodeIDs) != 2 {
		t.Fatalf("candidate provenance accumulation failed %#v", decision)
	}

	reviews, err := a.Memory.ListOperations(t.Context(), "review_required", 50)
	if err != nil {
		t.Fatal(err)
	}
	var procedureReview string
	for _, operation := range reviews {
		if operation.RiskTier == "C" && operation.Type == "CREATE" {
			procedureReview = operation.OperationID
			break
		}
	}
	if procedureReview == "" {
		t.Fatal("missing protected procedure review")
	}
	operation, err := a.Memory.ReviewOperation(t.Context(), procedureReview, "approve", "test-user")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "applied" || operation.ReviewedBy != "test-user" {
		t.Fatalf("review was not applied %#v", operation)
	}
	assets, err = a.Memory.ListAssets(t.Context(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if assets[0].Status != "approved" {
		t.Fatalf("asset did not follow approved protected memory %#v", assets[0])
	}
}

func TestRulesFallbackRejectsSpokenFillerAndKeepsDurableClaims(t *testing.T) {
	a, _ := testutil.Open(t)
	raw := testutil.Evidence(t, "quality-1", "recording", "session-quality", "user", "2026-08-21T12:00:00Z", "啊，我懂你的意思。对对对，就是这样。至少我自己觉得是这样。项目目标是完成可检查的长期记忆应用。我决定先过滤口语噪声再生成知识点。当前风险是录音中的重复和指代不明会污染记忆。")
	if _, err := a.Ledger.Append(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	result, err := a.Memory.EnqueueAndProcess(t.Context(), "session-quality")
	if err != nil {
		t.Fatal(err)
	}
	units, err := a.Memory.ListKnowledgeUnits(t.Context(), result.EpisodeID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 3 {
		t.Fatalf("expected only three durable claims, got %#v", units)
	}
	joined := ""
	for _, unit := range units {
		joined += unit.Statement + "\n"
	}
	for _, filler := range []string{"我懂你的意思", "对对对", "至少我自己"} {
		if strings.Contains(joined, filler) {
			t.Fatalf("spoken filler leaked into durable knowledge: %q", joined)
		}
	}
	for _, durable := range []string{"项目目标", "决定", "当前风险"} {
		if !strings.Contains(joined, durable) {
			t.Fatalf("durable claim %q missing from %q", durable, joined)
		}
	}
}
