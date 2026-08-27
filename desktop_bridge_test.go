package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

func TestDesktopBridgeOwnsLocalCoreLifecycle(t *testing.T) {
	bridge := newDesktopBridgeForTest(t.TempDir())
	bridge.Startup(context.Background())
	t.Cleanup(func() { bridge.Shutdown(context.Background()) })

	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Endpoint == "" || bootstrap.Token == "" || bootstrap.CSRFToken == "" {
		t.Fatalf("incomplete bootstrap: %#v", bootstrap)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	publicResponse, err := client.Get(bootstrap.Endpoint + "/health")
	if err != nil {
		t.Fatal(err)
	}
	publicBody, _ := io.ReadAll(publicResponse.Body)
	publicResponse.Body.Close()
	if publicResponse.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d body=%s", publicResponse.StatusCode, publicBody)
	}

	lockedResponse, err := client.Get(bootstrap.Endpoint + "/v1/harness/types")
	if err != nil {
		t.Fatal(err)
	}
	lockedResponse.Body.Close()
	if lockedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ordinary browser status=%d, want 401", lockedResponse.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, bootstrap.Endpoint+"/v1/harness/types", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Memory-Harness-Owner", bootstrap.Token)
	request.Header.Set("Origin", "wails://wails")
	ownerResponse, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerResponse.Body.Close()
	if ownerResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ownerResponse.Body)
		t.Fatalf("owner status=%d body=%s", ownerResponse.StatusCode, body)
	}
	var payload struct {
		Types []json.RawMessage `json:"types"`
	}
	if err := json.NewDecoder(ownerResponse.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Types) < 9 {
		t.Fatalf("built-in types=%d, want at least 9", len(payload.Types))
	}
}

func TestDesktopBridgeShutdownStopsCore(t *testing.T) {
	bridge := newDesktopBridgeForTest(t.TempDir())
	bridge.Startup(context.Background())
	bootstrap, err := bridge.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	bridge.Shutdown(context.Background())

	client := &http.Client{Timeout: 300 * time.Millisecond}
	if response, err := client.Get(bootstrap.Endpoint + "/health"); err == nil {
		response.Body.Close()
		t.Fatalf("desktop core still accepted requests after shutdown: %s", response.Status)
	}
}

func TestDesktopPortableImportRequiresExactPreflightIdentity(t *testing.T) {
	bridge := newDesktopBridgeForTest(t.TempDir())
	bridge.Startup(context.Background())
	t.Cleanup(func() { bridge.Shutdown(context.Background()) })
	if _, err := bridge.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	source, err := bridge.core.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "portable-source", Name: "Portable Source", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := bridge.core.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "portable-target", Name: "Portable Target", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	object, err := bridge.core.Harness.Materialize(ctx, harness.MaterializeInput{
		ObjectID: "desktop-portable-object", TypeID: "builtin.living-asset-vault.document", ProjectID: source.ProjectID,
		Status: "candidate", Payload: json.RawMessage(`{"title":"Portable","summary":"desktop identity gate","format":"markdown"}`),
		PluginID: "builtin.living-asset-vault", PluginVersion: "2.0.0", IdempotencyKey: "desktop-portable-object-r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "desktop.mhbundle.tar.gz")
	manifest, err := bridge.core.Portable.Export(ctx, portablebundle.ExportOptions{ProjectID: source.ProjectID, ObjectIDs: []string{object.ObjectID}}, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.ImportPortableBundle(target.ProjectID, archive, manifest.BundleID, "sha256:wrong", "desktop-wrong"); err == nil || !strings.Contains(err.Error(), "changed or became blocked") {
		t.Fatalf("mismatched preflight identity was accepted: %v", err)
	}
	items, err := bridge.core.Harness.ListObjects(ctx, target.ProjectID, portablebundle.ImportCandidateTypeV1, "", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("failed TOCTOU check wrote candidates: %#v err=%v", items, err)
	}
	result, err := bridge.ImportPortableBundle(target.ProjectID, archive, manifest.BundleID, manifest.BundleHash, "desktop-correct")
	if err != nil {
		t.Fatal(err)
	}
	if !result.NoDirectActivation || len(result.CandidateObjectIDs) != 1 {
		t.Fatalf("portable import=%#v", result)
	}
	candidate, err := bridge.core.Harness.Object(ctx, result.CandidateObjectIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != "candidate" || candidate.TypeID != portablebundle.ImportCandidateTypeV1 {
		t.Fatalf("unsafe imported object=%#v", candidate)
	}
}
