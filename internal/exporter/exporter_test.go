package exporter_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/exporter"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestPortableExportContainsLedgerAndManifest(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	raw := testutil.Evidence(t, "ev-export", "codex", "sess-export", "user", "2026-08-20T06:00:00Z", "portable export evidence")
	if _, err := a.Ledger.Append(ctx, raw); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir() + "/memoryos-export.tar.gz"
	if err := exporter.Create(ctx, a, out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	manifest, ledger, control := false, false, false
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Name == "export-manifest.json" {
			manifest = true
		}
		if strings.HasPrefix(h.Name, "ledger/") && strings.HasSuffix(h.Name, "events.jsonl") {
			ledger = true
		}
		if h.Name == "state/control.sqlite" {
			control = true
		}
	}
	if !manifest || !ledger || !control {
		t.Fatalf("manifest=%v ledger=%v control=%v", manifest, ledger, control)
	}
}

func TestExportRestorePreservesProjectControlStateAndRefusesOverwrite(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "restore-project", Name: "恢复验证", DefaultCurrency: "CNY", BudgetMinor: 88800})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Portfolio.CreateFact(ctx, portfolio.FactInput{ProjectID: project.ProjectID, Subject: "备份", Predicate: "状态", Object: "可恢复", ValidFrom: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	archive := filepath.Join(root, "backup.tar.gz")
	if err := exporter.Create(ctx, a, archive); err != nil {
		t.Fatal(err)
	}
	restoredHome := filepath.Join(root, "restored")
	if err := exporter.Restore(archive, restoredHome); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Resolve(restoredHome, "")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.Portfolio.Project(ctx, project.ProjectID)
	if err != nil || got.BudgetMinor != 88800 {
		t.Fatalf("restored project=%#v err=%v", got, err)
	}
	facts, err := restored.Portfolio.ListFacts(ctx, project.ProjectID, "2026-08-21T00:00:00Z", false, 10)
	if err != nil || len(facts) != 1 || facts[0].Object != "可恢复" {
		t.Fatalf("restored facts=%#v err=%v", facts, err)
	}
	if err := exporter.Restore(archive, restoredHome); err == nil {
		t.Fatal("restore overwrote a non-empty target")
	}
}
