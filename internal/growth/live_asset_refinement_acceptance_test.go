package growth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/assettemplate"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/store"
	"github.com/luoyif/memory-harness/internal/testutil"
)

// TestLiveAssetRefinementAcceptance is an opt-in, isolated end-to-end check of
// the configured provider. Provider metadata is read from a SQLite backup and
// the API key remains in the operating-system credential store.
func TestLiveAssetRefinementAcceptance(t *testing.T) {
	sourceDatabase := os.Getenv("MEMORYOS_REAL_MODEL_ACCEPTANCE_DB")
	secretHome := os.Getenv("MEMORYOS_REAL_MODEL_SECRET_HOME")
	if sourceDatabase == "" || secretHome == "" {
		t.Skip("set isolated acceptance DB and secret-store home to run the real asset refinement check")
	}
	source, err := store.OpenControl(sourceDatabase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	var providerID, name, kind, protocol, baseURL, model, status, createdAt, updatedAt string
	var enabled, hasSecret int
	if err := source.DB.QueryRowContext(t.Context(), `SELECT p.provider_id,p.name,p.kind,p.protocol,p.base_url,p.model,p.status,p.enabled,p.has_secret,p.created_at,p.updated_at FROM model_providers p JOIN model_runtime r ON r.active_provider_id=p.provider_id WHERE r.singleton=1`).Scan(&providerID, &name, &kind, &protocol, &baseURL, &model, &status, &enabled, &hasSecret, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}

	a, _ := testutil.Open(t)
	ctx := t.Context()
	if _, err := a.Control.DB.ExecContext(ctx, `INSERT INTO model_providers(provider_id,name,kind,protocol,base_url,model,status,enabled,has_secret,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, providerID, name, kind, protocol, baseURL, model, status, enabled, hasSecret, createdAt, updatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Control.DB.ExecContext(ctx, `UPDATE model_runtime SET mode='agent',active_provider_id=?,fallback_to_rules=1 WHERE singleton=1`, providerID); err != nil {
		t.Fatal(err)
	}
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "live-refinement-acceptance", Name: "Live Refinement Acceptance", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	const assetID = "asset-live-refinement-acceptance"
	const sourceTime = "2026-08-25T01:00:00Z"
	const sourceText = "每周发布前，先确认备份完成，然后运行完整测试并检查结果；若失败则停止发布并恢复上一版本。完成条件是全部测试通过且回滚方案可用。"
	sourceIDs, _ := json.Marshal([]string{"memory-live-refinement-acceptance"})
	if _, err := a.Control.DB.ExecContext(ctx, `INSERT INTO agent_assets(asset_id,asset_type,title,summary,status,version,risk_level,source_memory_ids_json,validation_status,created_at,updated_at) VALUES(?,?,?,?,?,'0.3','high',?,'not_run',?,?)`, assetID, memory.AssetTypeProcedure, "每周发布检查流程", sourceText, "candidate", string(sourceIDs), sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Control.DB.ExecContext(ctx, `INSERT INTO agent_asset_classifications(asset_id,classifier_version,classification_status,scores_json,reasons_json,updated_at) VALUES(?,'acceptance/v1','classified','{"procedure":15}','["ordered-steps","trigger"]',?)`, assetID, sourceTime); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Control.DB.ExecContext(ctx, `INSERT INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES('asset',?,?,'derived',1,?)`, assetID, project.ProjectID, sourceTime); err != nil {
		t.Fatal(err)
	}
	models := modelconfig.New(a.Control, modelconfig.NewDefaultSecretStore(secretHome), nil)
	refiner := growth.New(a.Control, a.Memory, a.Portfolio, a.Harness, a.Pipelines, a.Blueprints, models)
	result, err := refiner.RefineAssets(ctx, growth.AssetRefinementInput{ProjectID: project.ProjectID, AssetIDs: []string{assetID}, Mode: "incremental", IdempotencyKey: "live-refinement-acceptance", RequestedBy: "owner-acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelGroups != 1 || result.FallbackGroups != 0 || result.Refined != 1 || result.Failed != 0 {
		t.Fatalf("live refinement did not complete with a verified model result: %#v", result)
	}
	objects, err := a.Harness.ListObjects(ctx, project.ProjectID, harness.GovernedAgentAssetTypeV4, "", 10)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects=%#v err=%v", objects, err)
	}
	var payload assettemplate.Payload
	if err := json.Unmarshal(objects[0].Revision.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ValidationStatus != "passed" || payload.Refinement.Mode != "model" || payload.AssetType != memory.AssetTypeProcedure {
		t.Fatalf("live payload was not a verified Procedure: %#v", payload)
	}
}

// TestLiveProductionAssetRefinementRecovery is an explicit Owner-run recovery
// path for a named project and an exact list of derived Agent Asset candidates.
// It never scans or recompiles Evidence and is skipped unless every scope
// variable is supplied. Callers must make an online SQLite backup first.
func TestLiveProductionAssetRefinementRecovery(t *testing.T) {
	home := strings.TrimSpace(os.Getenv("MEMORYOS_LIVE_REFINEMENT_HOME"))
	projectID := strings.TrimSpace(os.Getenv("MEMORYOS_LIVE_REFINEMENT_PROJECT"))
	idempotencyKey := strings.TrimSpace(os.Getenv("MEMORYOS_LIVE_REFINEMENT_KEY"))
	assetCSV := strings.TrimSpace(os.Getenv("MEMORYOS_LIVE_REFINEMENT_ASSETS"))
	if home == "" || projectID == "" || idempotencyKey == "" || assetCSV == "" {
		t.Skip("set the live refinement home, project, key and exact asset list")
	}
	assetIDs := []string{}
	for _, id := range strings.Split(assetCSV, ",") {
		if id = strings.TrimSpace(id); id != "" {
			assetIDs = append(assetIDs, id)
		}
	}
	if len(assetIDs) == 0 || len(assetIDs) > 20 {
		t.Fatalf("live recovery requires 1-20 exact asset IDs, got %d", len(assetIDs))
	}
	control, err := store.OpenControl(filepath.Join(home, "state", "control.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	search, err := store.OpenSearch(filepath.Join(home, "cache", "search.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = search.Close() })
	memoryEngine := memory.New(control, search, filepath.Join(home, "memory"))
	portfolioService := portfolio.New(control, search)
	harnessService := harness.New(control)
	modelService := modelconfig.New(control, modelconfig.NewDefaultSecretStore(home), nil)
	blueprintService := blueprint.New(control)
	pipelineService := pipeline.New(control, harnessService, modelService)
	refiner := growth.New(control, memoryEngine, portfolioService, harnessService, pipelineService, blueprintService, modelService)
	result, err := refiner.RefineAssets(t.Context(), growth.AssetRefinementInput{
		ProjectID: projectID, AssetIDs: assetIDs, Mode: "incremental", IdempotencyKey: idempotencyKey, RequestedBy: "owner-codex-recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.FallbackGroups != 0 || result.ModelGroups == 0 {
		t.Fatalf("live recovery did not complete with verified model envelopes: %#v", result)
	}
	for _, item := range result.Items {
		if item.Status != "skipped" && !item.UsedModel {
			t.Fatalf("asset %s did not receive a verified model result: %#v", item.AssetID, item)
		}
	}
	var status string
	if err := control.DB.QueryRowContext(t.Context(), `SELECT status FROM harness_runs WHERE run_id=?`, result.RunID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("run %s status=%s result=%#v", result.RunID, status, result)
	}
	t.Logf("run_id=%s total=%d refined=%d proposed=%d skipped=%d model_groups=%d", result.RunID, result.Total, result.Refined, result.Proposed, result.Skipped, result.ModelGroups)
}
