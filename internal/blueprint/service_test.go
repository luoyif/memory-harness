package blueprint_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func cloneDefinition(t *testing.T, definition blueprint.Definition) blueprint.Definition {
	t.Helper()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var result blueprint.Definition
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDefaultBlueprintCanBeClonedActivatedAndPinnedToRun(t *testing.T) {
	a, _ := testutil.Open(t)
	current, err := a.Blueprints.Current(t.Context(), "project-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if !current.Inherited || !current.Validation.Valid || current.Blueprint.BlueprintID != blueprint.DefaultBlueprintID || current.Validation.EnabledComponentCount != 14 {
		t.Fatalf("unexpected default blueprint: %#v", current)
	}

	custom := cloneDefinition(t, current.Blueprint.Definition)
	custom.BlueprintID = "builtin.user-workflows.inbox-memory"
	custom.Version = "1.0.0"
	custom.Name = "Inbox 精确召回"
	for trackIndex := range custom.Tracks {
		for nodeIndex := range custom.Tracks[trackIndex].Nodes {
			node := &custom.Tracks[trackIndex].Nodes[nodeIndex]
			if node.Role == "recall.deep" {
				node.PluginID = "builtin.progressive-recall"
				node.PluginVersion = "1.0.0"
				node.ComponentID = "builtin.progressive-recall.exact-deep"
				node.ComponentVersion = "1.0.0"
				node.DisplayName = "L3 精确深搜"
				node.Config = json.RawMessage(`{"load":"on_demand","budget_chars":8000}`)
			}
		}
	}
	published, err := a.Blueprints.Publish(t.Context(), "builtin.user-workflows", custom)
	if err != nil {
		t.Fatal(err)
	}
	activated, err := a.Blueprints.Activate(t.Context(), "project-inbox", published.BlueprintID, published.Version, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	if activated.Inherited || activated.Assignment.BlueprintHash != published.ContentHash || !activated.Validation.Valid {
		t.Fatalf("activation=%#v", activated)
	}
	custom.Description = "mutation after publish"
	if _, err := a.Blueprints.Publish(t.Context(), "builtin.user-workflows", custom); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("published blueprint must be immutable, err=%v", err)
	}

	result, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: "project-inbox", CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: "builtin.core-memory-growth.inspect-candidate", PipelineVersion: "1.0.0",
		IdempotencyKey: "blueprint-run", Input: json.RawMessage(`{"statement":"trace blueprint"}`), EffectiveCapabilities: pipeline.OwnerCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := a.Harness.RunDetail(t.Context(), result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Blueprint struct {
			Assignment blueprint.Assignment `json:"assignment"`
		} `json:"blueprint"`
	}
	if err := json.Unmarshal(detail.Run.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Blueprint.Assignment.BlueprintHash != published.ContentHash {
		t.Fatalf("run did not pin blueprint: %s != %s", snapshot.Blueprint.Assignment.BlueprintHash, published.ContentHash)
	}
}

func TestExperimentalPluginRequiresExplicitProjectGrant(t *testing.T) {
	a, _ := testutil.Open(t)
	current, err := a.Blueprints.Current(t.Context(), "project-inbox")
	if err != nil {
		t.Fatal(err)
	}
	custom := cloneDefinition(t, current.Blueprint.Definition)
	custom.BlueprintID = "builtin.user-workflows.experimental-memory"
	custom.Version = "1.0.0"
	custom.Tracks[2].Nodes[3].PluginID = "builtin.dsh-bridge"
	custom.Tracks[2].Nodes[3].PluginVersion = "0.1.0"
	custom.Tracks[2].Nodes[3].ComponentID = "builtin.dsh-bridge.deep-recall"
	custom.Tracks[2].Nodes[3].ComponentVersion = "1.0.0"
	custom.Tracks[2].Nodes[3].RequiredCapabilities = []string{"memory.read"}
	published, err := a.Blueprints.Publish(t.Context(), "builtin.user-workflows", custom)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(t.Context(), "project-inbox", published.BlueprintID, published.Version, "owner-test"); err == nil || !strings.Contains(err.Error(), "experimental") {
		t.Fatalf("experimental plugin should require explicit grant, err=%v", err)
	}
	if _, err := a.Plugins.SetProjectState(t.Context(), "builtin.dsh-bridge", "0.1.0", "project-inbox", "enabled", []string{"memory.read"}, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Blueprints.Activate(t.Context(), "project-inbox", published.BlueprintID, published.Version, "owner-test"); err != nil {
		t.Fatalf("explicit experimental grant should allow activation: %v", err)
	}
}

func TestBlueprintRejectsEmbeddedSecrets(t *testing.T) {
	a, _ := testutil.Open(t)
	current, err := a.Blueprints.Current(t.Context(), "project-inbox")
	if err != nil {
		t.Fatal(err)
	}
	custom := cloneDefinition(t, current.Blueprint.Definition)
	custom.BlueprintID = "builtin.user-workflows.secret-memory"
	custom.Version = "1.0.0"
	custom.Tracks[2].Nodes[0].Config = json.RawMessage(`{"api_key":"must-not-be-here"}`)
	if _, err := a.Blueprints.Publish(t.Context(), "builtin.user-workflows", custom); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("embedded secret was not rejected: %v", err)
	}
}

func TestListHidesLegacyDefaultAliasWhenCurrentDefaultExists(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := context.Background()
	current, err := a.Blueprints.Version(ctx, blueprint.DefaultBlueprintID, blueprint.DefaultBlueprintVersion)
	if err != nil {
		t.Fatal(err)
	}
	legacy := current.Definition
	legacy.BlueprintID = blueprint.LegacyDefaultBlueprintID
	raw, _ := json.Marshal(legacy)
	if _, err := a.Control.DB.ExecContext(ctx, `INSERT INTO harness_blueprint_versions(blueprint_id,version,plugin_id,name,description,definition_json,content_hash,status,created_at) VALUES(?,?,?,?,?,?,?,'published',?)`, legacy.BlueprintID, legacy.Version, current.PluginID, legacy.Name, legacy.Description, string(raw), "sha256:legacy", current.CreatedAt); err != nil {
		t.Fatal(err)
	}
	items, err := a.Blueprints.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.BlueprintID == blueprint.LegacyDefaultBlueprintID {
			t.Fatalf("legacy alias leaked into catalog: %#v", items)
		}
	}
}

func TestContextTrackIsOptionalAndDoesNotRewriteLegacyDefault(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	legacy, err := a.Blueprints.Version(ctx, blueprint.DefaultBlueprintID, blueprint.DefaultBlueprintVersion)
	if err != nil {
		t.Fatal(err)
	}
	contextual, err := a.Blueprints.Version(ctx, blueprint.DefaultBlueprintID, blueprint.ContextBlueprintVersion)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.ContentHash == contextual.ContentHash {
		t.Fatal("context Blueprint must be a new immutable version")
	}
	if !blueprint.Validate(legacy.Definition).Valid {
		t.Fatal("legacy three-track Blueprint became invalid")
	}
	validation := blueprint.Validate(contextual.Definition)
	if !validation.Valid || validation.TrackCount != 4 {
		t.Fatalf("context Blueprint validation=%#v", validation)
	}
	current, err := a.Blueprints.Current(ctx, "project-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if current.Blueprint.Version != blueprint.DefaultBlueprintVersion {
		t.Fatalf("existing inherited default changed implicitly to %s", current.Blueprint.Version)
	}
}
